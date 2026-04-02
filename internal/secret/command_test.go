package secret

import (
	"context"
	"errors"
	"net/url"
	"testing"
)

func TestCommandProviderLoad(t *testing.T) {
	u, err := url.Parse("command://local")
	if err != nil {
		t.Fatalf("url.Parse error: %v", err)
	}
	t.Setenv(commandEnv, "echo test")

	provider, err := NewCommandProvider(u)
	if err != nil {
		t.Fatalf("NewCommandProvider error: %v", err)
	}
	provider.runner = func(context.Context, string) ([]byte, error) {
		return []byte(`{"TRINO_USER":"alice","TRINO_PASSWORD":"secret"}`), nil
	}

	secrets, err := provider.Load(context.Background())
	if err != nil {
		t.Fatalf("Load error: %v", err)
	}

	if got := string(secrets["TRINO_USER"]); got != "alice" {
		t.Fatalf("TRINO_USER = %q, want alice", got)
	}
	if got := string(secrets["TRINO_PASSWORD"]); got != "secret" {
		t.Fatalf("TRINO_PASSWORD = %q, want secret", got)
	}
}

func TestCommandProviderLoadFailure(t *testing.T) {
	u, err := url.Parse("command://local")
	if err != nil {
		t.Fatalf("url.Parse error: %v", err)
	}
	t.Setenv(commandEnv, "echo test")

	provider, err := NewCommandProvider(u)
	if err != nil {
		t.Fatalf("NewCommandProvider error: %v", err)
	}
	provider.runner = func(context.Context, string) ([]byte, error) {
		return nil, errors.New("boom")
	}

	if _, err := provider.Load(context.Background()); err == nil {
		t.Fatalf("expected load error")
	}
}
