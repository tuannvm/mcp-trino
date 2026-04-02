package secret

import (
	"context"
	"io"
	"net/http"
	"net/url"
	"strings"
	"testing"
)

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req)
}

func TestVaultProviderLoad(t *testing.T) {
	t.Setenv("VAULT_ADDR", "https://vault.example")
	t.Setenv("VAULT_TOKEN", "test-token")

	u, err := url.Parse("vault://secret/data/trino")
	if err != nil {
		t.Fatalf("url.Parse error: %v", err)
	}
	provider, err := NewVaultProvider(u)
	if err != nil {
		t.Fatalf("NewVaultProvider error: %v", err)
	}
	provider.client = &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
		if req.URL.String() != "https://vault.example/v1/secret/data/trino" {
			t.Fatalf("unexpected URL: %s", req.URL.String())
		}
		if req.Header.Get("X-Vault-Token") != "test-token" {
			t.Fatalf("missing Vault token")
		}
		return &http.Response{
			StatusCode: http.StatusOK,
			Body:       io.NopCloser(strings.NewReader(`{"data":{"data":{"TRINO_USER":"alice","TRINO_PASSWORD":"secret"}}}`)),
			Header:     make(http.Header),
		}, nil
	})}

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

func TestVaultProviderRequiresPath(t *testing.T) {
	u, err := url.Parse("vault://")
	if err != nil {
		t.Fatalf("url.Parse error: %v", err)
	}
	if _, err := NewVaultProvider(u); err == nil {
		t.Fatal("expected error for empty path")
	}
}

func TestVaultProviderNotFound(t *testing.T) {
	t.Setenv("VAULT_ADDR", "https://vault.example")
	t.Setenv("VAULT_TOKEN", "test-token")

	u, err := url.Parse("vault://secret/data/missing")
	if err != nil {
		t.Fatalf("url.Parse error: %v", err)
	}
	provider, err := NewVaultProvider(u)
	if err != nil {
		t.Fatalf("NewVaultProvider error: %v", err)
	}
	provider.client = &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
		return &http.Response{
			StatusCode: http.StatusNotFound,
			Body:       io.NopCloser(strings.NewReader(`{"errors":["not found"]}`)),
			Header:     make(http.Header),
		}, nil
	})}

	if _, err := provider.Load(context.Background()); err == nil {
		t.Fatal("expected load error")
	}
}
