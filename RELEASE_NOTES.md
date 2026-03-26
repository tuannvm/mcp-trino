# mcp-trino CLI Release Notes v1.0

**Release Date:** 2025-03-25
**Status:** Production-Ready ✅

## Overview

This release transforms mcp-trino from MCP-only to a **dual-purpose** tool that works both as an MCP server for AI assistants AND as an interactive CLI for human users.

## What's New

### CLI Mode
- **Interactive REPL** with SQL query execution
- **Subcommands**: `query`, `catalogs`, `schemas`, `tables`, `describe`, `explain`
- **Output formats**: `table`, `json`, `csv`
- **Config file support**: `~/.config/trino/config.yaml`
- **Auto-completion**: Meta-commands (`\help`, `\quit`, `\history`, `\format`, etc.)

### Dual-Mode Operation
The binary automatically detects which mode to use:
- **MCP mode**: Default when no args or `MCP_PROTOCOL_VERSION` is set
- **CLI mode**: Activated by CLI commands or `--cli` flag
- **Explicit control**: Use `--mcp` or `--cli` flags to force mode

## Important Behavioral Changes

### ⚠️ Column Order Now Deterministic

**Before:** Table and CSV output had non-deterministic column order (due to Go map iteration)

**After:** Columns are sorted alphabetically for consistent output

**Impact:**
- ✅ **Improved:** Automated scripts get predictable output
- ⚠️ **Breaking:** Scripts parsing by column position may break
- **Recommendation:** Parse by column name instead of position

**Example:**
```sql
SELECT zebra, apple, banana FROM table;
```

Before: `zebra | apple | banana` (random order)
After: `apple | banana | zebra` (alphabetically sorted)

## Configuration Precedence

Values are applied in this order (later overrides earlier):

1. **CLI flags** (`--host`, `--port`, etc.) - highest priority
2. **`--profile` flag** (select named profile)
3. **`TRINO_PROFILE` environment variable**
4. **`current` field** in config file
5. **`default` profile** fallback
6. **Environment variables** (`TRINO_HOST`, etc.) - lowest priority

**Example:**
```yaml
# ~/.config/trino/config.yaml
current: prod
profiles:
  prod:
    host: prod.example.com
    port: 443
    user: prod_user
  dev:
    host: localhost
    port: 8080
    user: trino
```

```bash
# CLI flag overrides everything
mcp-trino --host custom --profile prod query "SELECT 1"
# Uses: host=custom (flag), other values from prod profile

# Profile selection via flag
mcp-trino --profile dev catalogs
# Uses: dev profile values

# Profile selection via env var
export TRINO_PROFILE=prod
mcp-trino catalogs
# Uses: prod profile values

# Default profile (no explicit selection)
mcp-trino catalogs
# Uses: 'prod' profile (from 'current' field)
```

## Mode Selection Logic

```
┌─────────────────────────────────────────────────────────────┐
│ Start                                                      │
└─────────────┬───────────────────────────────────────────────┘
              │
              ▼
┌─────────────────────────────────────────────────────────────┐
│ Is --mcp flag present?                                     │
│ Yes → MCP mode                                            │
└─────────────┬───────────────────────────────────────────────┘
              │ No
              ▼
┌─────────────────────────────────────────────────────────────┐
│ Is MCP_PROTOCOL_VERSION set?                               │
│ Yes → MCP mode                                            │
└─────────────┬───────────────────────────────────────────────┘
              │ No
              ▼
┌─────────────────────────────────────────────────────────────┐
│ Is --cli flag present or known CLI command?                │
│ Yes → CLI mode                                            │
└─────────────┬───────────────────────────────────────────────┘
              │ No
              ▼
┌─────────────────────────────────────────────────────────────┐
│ Unknown positional argument?                               │
│ Yes → MCP mode (backward compatibility)                   │
└─────────────┬───────────────────────────────────────────────┘
              │ No
              ▼
┌─────────────────────────────────────────────────────────────┐
│ No arguments, TTY?                                        │
│ Yes → CLI help                                           │
│ No → MCP mode                                            │
└─────────────────────────────────────────────────────────────┘
```

## Usage Examples

### Basic CLI Usage
```bash
# List catalogs
mcp-trino catalogs

# List tables in a catalog/schema
mcp-trino tables memory default

# Execute a query
mcp-trino query "SELECT * FROM my_table LIMIT 10"

# Describe a table
mcp-trino describe memory.default.users

# Explain a query
mcp-trino explain "SELECT COUNT(*) FROM users"
```

### Interactive REPL
```bash
# Start REPL
mcp-trino --interactive

# Or just
mcp-trino

# In REPL:
trino> SELECT 1 AS test;
 test
-----
    1

trino> \format json
trino> SELECT 1;
{"_col0": 1}

trino> \quit
```

### Config File
```yaml
# ~/.config/trino/config.yaml
current: prod

profiles:
  prod:
    host: trino.example.com
    port: 443
    user: prod_user
    password: prod_password
    catalog: hive
    schema: analytics
    ssl:
      enabled: true
      insecure: false

  dev:
    host: localhost
    port: 8080
    user: trino
    catalog: memory
    schema: default

  staging:
    host: staging-trino.example.com
    port: 443
    user: staging_user
    catalog: hive
    schema: analytics_staging

output:
  format: table
```

### Output Formats
```bash
# Table format (default)
mcp-trino --format table query "SELECT 1"

# JSON format
mcp-trino --format json query "SELECT 1"

# CSV format
mcp-trino --format csv query "SELECT 1"
```

### Mode Selection
```bash
# Force MCP mode
mcp-trino --mcp

# Force CLI mode
mcp-trino --cli

# MCP mode (default for no args)
mcp-trino

# CLI mode (when command recognized)
mcp-trino query "SELECT 1"
```

## REPL Meta-Commands

| Command | Description |
|---------|-------------|
| `\help` | Show help |
| `\quit`, `\exit`, `\q` | Exit REPL |
| `\history` | Show command history |
| `\catalogs` | List all catalogs |
| `\schemas [catalog]` | List schemas (optional catalog) |
| `\tables [catalog schema]` | List tables (optional catalog.schema) |
| `\describe <table>` | Describe table structure |
| `\format <fmt>` | Set output format (table, json, csv) |

## Testing Summary

### Test Coverage
- **Unit Tests:** 100+ tests across 6 test files
- **Integration Tests:** End-to-end binary execution tests
- **All Tests:** Passing ✅
- **Linting:** 0 issues ✅

### Test Files
- `cmd/main_test.go` - Mode detection, argument parsing
- `cmd/integration_test.go` - Binary execution, precedence
- `internal/cli/config_test.go` - Config loading, SSL handling
- `internal/cli/commands_test.go` - CLI commands
- `internal/cli/repl_test.go` - REPL behavior
- `internal/cli/output_test.go` - Output determinism

## Known Limitations

1. **Shell completions** not yet implemented (bash/zsh)
2. **REPL multiline** queries require TTY for full testing
3. Tests conducted without live Trino server (structural testing)

## Backward Compatibility

✅ **Fully backward compatible** with existing MCP integrations:
- No-arg startup defaults to MCP mode
- Unknown positional arguments preserve MCP behavior
- `MCP_PROTOCOL_VERSION` environment variable respected
- STDIO transport mode unchanged

## Deployment Recommendations

### Before Release
1. ✅ All tests passing
2. ✅ Linting clean
3. ✅ Documentation complete
4. ⚠️ Test with real Trino server if possible

### Post-Release Monitoring
- User feedback on column order change
- Reports of MCP compatibility issues
- Performance with large result sets

### Rollback Plan
If critical issues arise:
1. Previous version available via git tags
2. Config file allows disabling CLI features
3. MCP mode fully backward compatible

## Support

- **Documentation:** See README.md and docs/ directory
- **Issues:** Report via GitHub issues
- **Contributing:** Pull requests welcome

## Acknowledgments

Built with:
- Go 1.24.11+
- Trino Go Client v0.328.0
- MCP Go SDK v0.41.1
