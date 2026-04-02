package secret

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"os"
	"os/exec"
	"strings"
	"time"
)

const commandEnv = "TRINO_SECRET_COMMAND"

// CommandProvider executes a custom command that returns a JSON object.
type CommandProvider struct {
	command string
	runner  func(ctx context.Context, cmd string) ([]byte, error)
}

func NewCommandProvider(u *url.URL) (*CommandProvider, error) {
	command := strings.TrimSpace(os.Getenv(commandEnv))
	if command == "" {
		command = strings.TrimSpace(u.Query().Get("command"))
	}
	if command == "" {
		return nil, fmt.Errorf("command source requires TRINO_SECRET_COMMAND or command query parameter")
	}

	return &CommandProvider{
		command: command,
		runner:  defaultShellRunner,
	}, nil
}

func (p *CommandProvider) Name() string {
	return "command"
}

func (p *CommandProvider) Load(ctx context.Context) (map[string][]byte, error) {
	output, err := p.runner(ctx, p.command)
	if err != nil {
		return nil, fmt.Errorf("secret command failed")
	}
	defer zeroBytes(output)

	parsed := map[string]any{}
	if err := json.Unmarshal(output, &parsed); err != nil {
		return nil, fmt.Errorf("secret command returned invalid JSON object: %w", err)
	}

	out := make(map[string][]byte, len(parsed))
	for key, raw := range parsed {
		if value, ok := stringifySecret(raw); ok {
			out[key] = value
		}
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("secret command JSON did not contain string values")
	}

	return out, nil
}

func (p *CommandProvider) Close() error {
	return nil
}

func defaultShellRunner(ctx context.Context, cmdStr string) ([]byte, error) {
	// Ensure a reasonable timeout if none is set
	if _, ok := ctx.Deadline(); !ok {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, 30*time.Second)
		defer cancel()
	}
	cmd := exec.CommandContext(ctx, "sh", "-c", cmdStr)
	return cmd.Output()
}
