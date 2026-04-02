package secret

import (
	"context"
	"errors"
	"testing"
)

func TestOnePasswordProviderParsing(t *testing.T) {
	tests := []struct {
		name      string
		reference string
		wantVault string
		wantItem  string
		wantErr   bool
	}{
		{
			name:      "vault and item",
			reference: "op://Engineering/Trino",
			wantVault: "Engineering",
			wantItem:  "Trino",
			wantErr:   false,
		},
		{
			name:      "item only",
			reference: "op://Trino",
			wantVault: "",
			wantItem:  "Trino",
			wantErr:   false,
		},
		{
			name:      "no prefix",
			reference: "Trino",
			wantVault: "",
			wantItem:  "Trino",
			wantErr:   false,
		},
		{
			name:      "empty item",
			reference: "op://",
			wantVault: "",
			wantItem:  "",
			wantErr:   true,
		},
		{
			name:      "empty reference",
			reference: "",
			wantVault: "",
			wantItem:  "",
			wantErr:   true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			provider, err := NewOnePasswordProvider(tt.reference)
			if (err != nil) != tt.wantErr {
				t.Errorf("NewOnePasswordProvider() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !tt.wantErr {
				if provider.vault != tt.wantVault {
					t.Errorf("vault = %q, want %q", provider.vault, tt.wantVault)
				}
				if provider.item != tt.wantItem {
					t.Errorf("item = %q, want %q", provider.item, tt.wantItem)
				}
			}
		})
	}
}

func TestOnePasswordProviderCommandArguments(t *testing.T) {
	t.Run("with vault", func(t *testing.T) {
		provider, err := NewOnePasswordProvider("op://Engineering/Trino")
		if err != nil {
			t.Fatalf("NewOnePasswordProvider error: %v", err)
		}
		var gotArgs []string
		provider.runner = func(ctx context.Context, name string, args ...string) ([]byte, error) {
			gotArgs = args
			return []byte(`{}`), nil
		}
		_, _ = provider.Load(context.Background())
		wantArgs := []string{"item", "get", "--vault", "Engineering", "Trino", "--format", "json"}
		if len(gotArgs) != len(wantArgs) {
			t.Fatalf("got %d args, want %d", len(gotArgs), len(wantArgs))
		}
		for i, arg := range wantArgs {
			if gotArgs[i] != arg {
				t.Errorf("arg[%d] = %q, want %q", i, gotArgs[i], arg)
			}
		}
	})

	t.Run("without vault", func(t *testing.T) {
		provider, err := NewOnePasswordProvider("op://Trino")
		if err != nil {
			t.Fatalf("NewOnePasswordProvider error: %v", err)
		}
		var gotArgs []string
		provider.runner = func(ctx context.Context, name string, args ...string) ([]byte, error) {
			gotArgs = args
			return []byte(`{}`), nil
		}
		_, _ = provider.Load(context.Background())
		wantArgs := []string{"item", "get", "Trino", "--format", "json"}
		if len(gotArgs) != len(wantArgs) {
			t.Fatalf("got %d args, want %d", len(gotArgs), len(wantArgs))
		}
		for i, arg := range wantArgs {
			if gotArgs[i] != arg {
				t.Errorf("arg[%d] = %q, want %q", i, gotArgs[i], arg)
			}
		}
	})
}

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
