package secret

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"
)

// VaultProvider reads secrets from a Vault KV path.
type VaultProvider struct {
	addr   string
	path   string
	client *http.Client
}

func NewVaultProvider(u *url.URL) (*VaultProvider, error) {
	path := strings.TrimPrefix(strings.TrimSpace(u.Host+u.Path), "/")
	if path == "" {
		return nil, fmt.Errorf("vault source path cannot be empty")
	}

	addr := strings.TrimSpace(os.Getenv("VAULT_ADDR"))
	if addr == "" {
		return nil, fmt.Errorf("VAULT_ADDR is required for vault secret source")
	}
	token := strings.TrimSpace(os.Getenv("VAULT_TOKEN"))
	if token == "" {
		return nil, fmt.Errorf("VAULT_TOKEN is required for vault secret source")
	}
	_ = token // Used for validation only

	return &VaultProvider{
		addr:   strings.TrimRight(addr, "/"),
		path:   path,
		client: &http.Client{
			Timeout: 30 * time.Second,
		},
	}, nil
}

func (p *VaultProvider) Name() string {
	return "vault"
}

func (p *VaultProvider) Load(ctx context.Context) (map[string][]byte, error) {
	token := strings.TrimSpace(os.Getenv("VAULT_TOKEN"))
	if token == "" {
		return nil, fmt.Errorf("VAULT_TOKEN is required for vault secret source")
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, p.addr+"/v1/"+p.path, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create vault request: %w", err)
	}
	req.Header.Set("X-Vault-Token", token)

	resp, err := p.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("vault read failed: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read vault response: %w", err)
	}
	defer zeroBytes(body)

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("vault read failed with HTTP %d", resp.StatusCode)
	}

	var parsed struct {
		Data map[string]any `json:"data"`
	}
	if err := json.Unmarshal(body, &parsed); err != nil {
		return nil, fmt.Errorf("invalid vault response: %w", err)
	}
	if parsed.Data == nil {
		return nil, fmt.Errorf("vault response missing data object")
	}

	payload := parsed.Data
	if nested, ok := parsed.Data["data"].(map[string]any); ok {
		payload = nested
	}

	out := make(map[string][]byte, len(payload))
	for key, raw := range payload {
		if value, ok := stringifySecret(raw); ok {
			out[key] = value
		}
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("vault secret at path %q did not contain string values", p.path)
	}

	return out, nil
}

func (p *VaultProvider) Close() error {
	return nil
}
