package secret

import (
	"context"
	"testing"
)

type mockProvider struct {
	loadFn  func(context.Context) (map[string][]byte, error)
	closed  bool
	loadCnt int
}

func (m *mockProvider) Name() string { return "mock" }
func (m *mockProvider) Load(ctx context.Context) (map[string][]byte, error) {
	m.loadCnt++
	return m.loadFn(ctx)
}
func (m *mockProvider) Close() error { m.closed = true; return nil }

func TestResolverLookupAndClose(t *testing.T) {
	provider := &mockProvider{
		loadFn: func(context.Context) (map[string][]byte, error) {
			return map[string][]byte{"TRINO_PASSWORD": []byte("s3cr3t")}, nil
		},
	}

	resolver := &Resolver{provider: provider}
	value, ok, err := resolver.Lookup(context.Background(), "TRINO_PASSWORD")
	if err != nil {
		t.Fatalf("Lookup returned error: %v", err)
	}
	if !ok || value != "s3cr3t" {
		t.Fatalf("Lookup = (%q, %v), want (s3cr3t, true)", value, ok)
	}

	if err := resolver.Close(); err != nil {
		t.Fatalf("Close returned error: %v", err)
	}
	if !provider.closed {
		t.Fatalf("provider was not closed")
	}
	if got := string(resolver.secrets["TRINO_PASSWORD"]); got != "" {
		t.Fatalf("secret bytes were not cleared")
	}
	if provider.loadCnt != 1 {
		t.Fatalf("loadCnt = %d, want 1 (lazy loading)", provider.loadCnt)
	}
}
