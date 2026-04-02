package secret

import (
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
)

// OnePasswordProvider loads secrets from an op:// item reference.
type OnePasswordProvider struct {
	reference string
	runner    func(ctx context.Context, name string, args ...string) ([]byte, error)
}

func NewOnePasswordProvider(reference string) (*OnePasswordProvider, error) {
	if reference == "" {
		return nil, fmt.Errorf("1Password source cannot be empty")
	}
	return &OnePasswordProvider{
		reference: reference,
		runner:    defaultCommandRunner,
	}, nil
}

func (p *OnePasswordProvider) Name() string {
	return "1password"
}

func (p *OnePasswordProvider) Load(ctx context.Context) (map[string][]byte, error) {
	output, err := p.runner(ctx, "op", "item", "get", p.reference, "--format", "json")
	if err != nil {
		return nil, fmt.Errorf("failed to fetch secret from 1Password CLI")
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
		if field.ID != "" {
			out[field.ID] = cloneBytes(value)
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
