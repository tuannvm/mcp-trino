# Secrets — Piping Patterns

`mcp-trino` reads **all** configuration from environment variables. It has no built-in secret-manager client: that responsibility belongs to purpose-built tools like the [1Password CLI](https://developer.1password.com/docs/cli/) (`op`), HashiCorp Vault, or your platform's secret driver. This keeps the binary small, reduces the attack surface, and lets you pick any backend without a code change.

The recipes below cover the three environments that matter: local development, CI, and Kubernetes.

---

## TL;DR

```bash
# Local dev with 1Password (recommended)
op run --env-file=.env -- mcp-trino

# Inline one-shot
TRINO_PASSWORD=$(op read 'op://Engineering/Trino/password') mcp-trino --help

# Vault
TRINO_PASSWORD=$(vault kv get -field=password secret/mcp-trino) mcp-trino

# Kubernetes: use Secret + envFrom (no app-side vault client)
```

---

## 1Password via `op run`

`op run` spawns a child process, substitutes `op://` references in the environment, and wipes them on exit. Secrets never hit disk and never appear in shell history. This is the preferred pattern.

### Step 1 — Store secrets in 1Password

Create an item (e.g., `Trino` in vault `Engineering`) with fields `host`, `port`, `username`, `password`.

### Step 2 — Write a `.env` file with references

```bash
# .env  (safe to commit — these are references, not secrets)
TRINO_HOST=op://Engineering/Trino/host
TRINO_PORT=op://Engineering/Trino/port
TRINO_USER=op://Engineering/Trino/username
TRINO_PASSWORD=op://Engineering/Trino/password
TRINO_SCHEME=https
```

### Step 3 — Launch through `op run`

```bash
op run --env-file=.env -- mcp-trino
```

`op` prompts for Touch ID / 1Password unlock, resolves each `op://` reference, and execs `mcp-trino` with the plain values in its env. When the process exits, the values are gone.

### Verify without leaking

`op run` masks secret values in the child process's stdout/stderr by default (`--no-masking` disables). So a quick smoke test is safe:

```bash
op run --env-file=.env -- mcp-trino --version
op run --env-file=.env -- mcp-trino query "SELECT 1"
```

### Inline `op read` (no env file)

For one-off commands:

```bash
TRINO_PASSWORD=$(op read 'op://Engineering/Trino/password') \
  mcp-trino query "SELECT current_user"
```

---

## HashiCorp Vault

Use `vault kv get` in a subshell, or run `vault agent` with a template that renders env-file style output and have your supervisor load it.

```bash
# Quick: single value
TRINO_PASSWORD=$(vault kv get -field=password secret/mcp-trino) mcp-trino

# Many values: bulk-export then exec
eval "$(vault kv get -format=json secret/mcp-trino |
  jq -r '.data.data | to_entries[] | "export \(.key)=\(.value | @sh)"')"
mcp-trino
unset TRINO_PASSWORD TRINO_USER   # clean up in shared shells
```

For long-running services, prefer **Vault Agent** with [auto-auth + template](https://developer.hashicorp.com/vault/docs/agent-and-proxy/agent) rendering an env file, then launch `mcp-trino` via `env $(cat /run/secrets/mcp-trino.env | xargs) mcp-trino` or a systemd `EnvironmentFile=`.

---

## Kubernetes

`mcp-trino`'s Helm chart takes plain env vars. Combine with **any** secret source that can produce a `Secret`:

- [External Secrets Operator](https://external-secrets.io/) (syncs from 1Password, Vault, AWS SM, GCP SM, Azure Key Vault)
- [Vault CSI driver](https://developer.hashicorp.com/vault/docs/platform/k8s/csi)
- Vault Agent Injector sidecar

The chart already wires `envFrom.secretRef`; point it at whichever `Secret` your chosen operator produces. No app-side vault client means no pod-identity complexity in `mcp-trino` itself.

---

## Security Nuances

### 1. Avoid shell history

Commands starting with a secret assignment (`TRINO_PASSWORD=hunter2 mcp-trino`) land in `~/.zsh_history` / `~/.bash_history`. Prefer:

- `op run --env-file=...` — nothing in history
- Command substitution (`$(...)`) — only the command is recorded, not the value
- Leading space (zsh/bash with `HISTCONTROL=ignorespace`) — ` TRINO_PASSWORD=... mcp-trino`

### 2. Avoid process-list leakage

Never pass secrets as CLI flags. Anything in `argv` is visible via `ps -ef` to any user on the box. `mcp-trino` deliberately accepts secrets via env vars only.

### 3. Env is inherited — scope it

Env vars exported in your shell leak into **every** child process, including editors, web browsers, and shells you `exec` into. Two mitigations:

- Use `op run` / inline `VAR=...` prefixes — the variable lives only for the one child
- If you must `export`, `unset` when done

### 4. Don't log secrets

`mcp-trino` never logs `TRINO_PASSWORD` or OAuth client secrets. If you add tooling around it, grep your log lines for `PASSWORD` / `SECRET` / `TOKEN` before shipping.

### 5. `.env` files are references, not values

The `.env` used with `op run` contains only `op://` references — safe to commit. An `.env` with resolved values is a credential file: `.gitignore` it, `chmod 600`, and consider disk encryption.

---

## Testing Your Setup

A non-destructive verification that secrets reach the app without exposing them:

```bash
# 1. Prove op can resolve the refs (masked by default)
op run --env-file=.env -- sh -c 'echo host=$TRINO_HOST user=$TRINO_USER'

# 2. Confirm the CLI can round-trip a trivial query
op run --env-file=.env -- mcp-trino query "SELECT 1 AS ok"

# 3. Leak-test against a throwaway credential (NOT via `op run`).
#    `op run` masks any secret it injected in the child's stdout/stderr, so
#    grepping its output for the real password would always report "no leak"
#    even if the app did leak it. Use a disposable value outside op instead:
TRINO_PASSWORD='leak-canary-4e7a' mcp-trino query "SELECT 1" 2>&1 |
  tee /tmp/mcp-trino.log
grep -F 'leak-canary-4e7a' /tmp/mcp-trino.log && {
  echo "LEAK DETECTED"; exit 1;
} || echo "no leak"
```

For reproducible integration tests, run Trino under Docker Compose and inject a throwaway password — no 1Password needed for the test itself.

---

## Migrating from `TRINO_SECRET_SOURCE`

Earlier versions shipped an in-process secret resolver (`TRINO_SECRET_SOURCE=vault://...` / `op://...` / `command://...`). It has been removed. Replace:

| Old                                           | New                                                         |
| --------------------------------------------- | ----------------------------------------------------------- |
| `TRINO_SECRET_SOURCE=op://Engineering/Trino`  | `op run --env-file=.env -- mcp-trino` (refs in `.env`)      |
| `TRINO_SECRET_SOURCE=vault://secret/mcp-trino`| `vault agent` → env-file, or `$(vault kv get -field=...)`   |
| `TRINO_SECRET_SOURCE=command://local`         | Plain shell: `eval "$(your-cmd)" && mcp-trino`              |

No code changes to `mcp-trino` itself are required — the change is entirely in how you launch it.
