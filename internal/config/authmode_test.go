package config

import (
	"os"
	"testing"
)

func TestTrinoAuthModeConfiguration(t *testing.T) {
	origAuthMode := os.Getenv("TRINO_AUTH_MODE")
	origScheme := os.Getenv("TRINO_SCHEME")
	defer func() {
		_ = os.Setenv("TRINO_AUTH_MODE", origAuthMode)
		_ = os.Setenv("TRINO_SCHEME", origScheme)
	}()

	tests := []struct {
		name      string
		authMode  string
		expectErr bool
	}{
		{name: "default basic", authMode: "", expectErr: false},
		{name: "explicit basic", authMode: "basic", expectErr: false},
		{name: "external mode", authMode: "external", expectErr: false},
		{name: "invalid mode", authMode: "bogus", expectErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.authMode == "" {
				_ = os.Unsetenv("TRINO_AUTH_MODE")
			} else {
				_ = os.Setenv("TRINO_AUTH_MODE", tt.authMode)
			}

			cfg, err := NewTrinoConfig()
			if tt.expectErr {
				if err == nil {
					t.Fatal("expected error")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			expected := AuthMode(tt.authMode)
			if expected == "" {
				expected = AuthModeBasic
			}
			if cfg.AuthMode != expected {
				t.Fatalf("AuthMode = %q, want %q", cfg.AuthMode, expected)
			}
		})
	}
}

func TestTrinoAuthModeExternalRequiresHTTPS(t *testing.T) {
	origAuthMode := os.Getenv("TRINO_AUTH_MODE")
	origScheme := os.Getenv("TRINO_SCHEME")
	defer func() {
		_ = os.Setenv("TRINO_AUTH_MODE", origAuthMode)
		_ = os.Setenv("TRINO_SCHEME", origScheme)
	}()

	_ = os.Setenv("TRINO_AUTH_MODE", AuthModeExternalAuth.String())
	_ = os.Setenv("TRINO_SCHEME", "http")

	if _, err := NewTrinoConfig(); err == nil {
		t.Fatal("expected error when external auth is configured over http")
	}
}
