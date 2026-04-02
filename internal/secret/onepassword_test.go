package secret

import (
	"context"
	"errors"
	"testing"
)

func TestOnePasswordProviderLoad(t *testing.T) {
	provider, err := NewOnePasswordProvider("op://Engineering/Trino")
	if err != nil {
		t.Fatalf("NewOnePasswordProvider error: %v", err)
	}
	provider.runner = func(context.Context, string, ...string) ([]byte, error) {
		return []byte(`{"fields":[{"id":"password","label":"TRINO_PASSWORD","value":"p@ss"},{"id":"username","label":"TRINO_USER","value":"alice"}]}`), nil
	}

	secrets, err := provider.Load(context.Background())
	if err != nil {
		t.Fatalf("Load error: %v", err)
	}
	if got := string(secrets["TRINO_PASSWORD"]); got != "p@ss" {
		t.Fatalf("TRINO_PASSWORD = %q, want p@ss", got)
	}
	if got := string(secrets["username"]); got != "alice" {
		t.Fatalf("username = %q, want alice", got)
	}
}

func TestOnePasswordProviderLoadFailure(t *testing.T) {
	provider, err := NewOnePasswordProvider("op://Engineering/Trino")
	if err != nil {
		t.Fatalf("NewOnePasswordProvider error: %v", err)
	}
	provider.runner = func(context.Context, string, ...string) ([]byte, error) {
		return nil, errors.New("cli failed")
	}

	if _, err := provider.Load(context.Background()); err == nil {
		t.Fatalf("expected load error")
	}
}
