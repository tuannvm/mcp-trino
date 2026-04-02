package secret

import (
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"strings"
)

// OnePasswordProvider loads secrets from an op:// item reference.
type OnePasswordProvider struct {
	vault  string
	item   string
	runner func(ctx context.Context, name string, args ...string) ([]byte, error)
}

func NewOnePasswordProvider(reference string) (*OnePasswordProvider, error) {
	if reference == "" {
		return nil, fmt.Errorf("1Password source cannot be empty")
	}

	// Parse op://vault/item or op://item format
	// Remove the op:// prefix if present
	ref := strings.TrimPrefix(reference, "op://")
	if ref == reference {
		// No op:// prefix, use as-is
		return &OnePasswordProvider{
			item:   ref,
			runner: defaultCommandRunner,
		}, nil
	}

	// Split by / to separate vault and item
	parts := strings.SplitN(ref, "/", 2)
	var vault, item string
	if len(parts) == 2 {
		vault = parts[0]
		item = parts[1]
	} else {
		item = parts[0]
	}

	if item == "" {
		return nil, fmt.Errorf("1Password item name cannot be empty")
	}

	return &OnePasswordProvider{
		vault:  vault,
		item:   item,
		runner: defaultCommandRunner,
	}, nil
}

func (p *OnePasswordProvider) Name() string {
	return "1password"
}

func (p *OnePasswordProvider) Load(ctx context.Context) (map[string][]byte, error) {
	args := []string{"item", "get"}
	if p.vault != "" {
		args = append(args, "--vault", p.vault)
	}
	args = append(args, p.item, "--format", "json")

	output, err := p.runner(ctx, "op", args...)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch secret from 1Password CLI: %w", err)
	}
	defer zeroBytes(output)

	var parsed struct {
		Fields []struct {
			ID    string `json:"id"`
			Label string `json:"label"`
			Value string `json:"value"`
		} `json:"fields"`
	}
	if err := json.Unmarshal(output, &parsed); err != nil {
		return nil, fmt.Errorf("invalid 1Password CLI response: %w", err)
	}

	out := make(map[string][]byte, len(parsed.Fields))
	for _, field := range parsed.Fields {
		if field.Value == "" {
			continue
		}
		value := cloneBytes([]byte(field.Value))
		if field.Label != "" {
			out[field.Label] = value
		}
		if field.ID != "" && field.ID != field.Label {
			// Only clone if ID is different from Label to avoid duplicate entries
			out[field.ID] = value
		}
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("1Password item did not contain accessible fields")
	}

	return out, nil
}

func (p *OnePasswordProvider) Close() error {
	return nil
}

func defaultCommandRunner(ctx context.Context, name string, args ...string) ([]byte, error) {
	cmd := exec.CommandContext(ctx, name, args...)
	return cmd.Output()
}
