package secret

import (
	"context"
	"fmt"
	"net/url"
	"os"
	"strings"
	"sync"
)

const (
	secretSourceEnv   = "TRINO_SECRET_SOURCE"
	secretRequiredEnv = "TRINO_SECRET_REQUIRED"
)

// Provider loads secrets from an external source.
type Provider interface {
	Name() string
	Load(ctx context.Context) (map[string][]byte, error)
	Close() error
}

// Resolver loads secrets once and serves per-key lookups.
type Resolver struct {
	source   string
	required bool
	provider Provider

	loadOnce sync.Once
	loadErr  error
	secrets  map[string][]byte
}

// NewResolverFromEnv creates a resolver from TRINO_SECRET_SOURCE.
// If TRINO_SECRET_SOURCE is not set, it returns nil, nil.
func NewResolverFromEnv() (*Resolver, error) {
	source := strings.TrimSpace(os.Getenv(secretSourceEnv))
	if source == "" {
		return nil, nil
	}

	required := strings.EqualFold(strings.TrimSpace(os.Getenv(secretRequiredEnv)), "true")

	provider, err := providerFromSource(source)
	if err != nil {
		return nil, err
	}

	return &Resolver{
		source:   source,
		required: required,
		provider: provider,
	}, nil
}

func providerFromSource(source string) (Provider, error) {
	u, err := url.Parse(source)
	if err != nil {
		return nil, fmt.Errorf("invalid secret source: %w", err)
	}

	switch strings.ToLower(u.Scheme) {
	case "vault":
		return NewVaultProvider(u)
	case "op", "1password":
		return NewOnePasswordProvider(source)
	case "command", "cmd":
		return NewCommandProvider(u)
	default:
		return nil, fmt.Errorf("unsupported secret source scheme %q (supported: vault://, op://, command://)", u.Scheme)
	}
}

func (r *Resolver) ensureLoaded(ctx context.Context) error {
	r.loadOnce.Do(func() {
		r.secrets, r.loadErr = r.provider.Load(ctx)
	})
	return r.loadErr
}

// Preload forces one-time secret retrieval.
func (r *Resolver) Preload(ctx context.Context) error {
	if r == nil {
		return nil
	}
	return r.ensureLoaded(ctx)
}

// Lookup returns a secret value for key, if found.
func (r *Resolver) Lookup(ctx context.Context, key string) (string, bool, error) {
	if r == nil {
		return "", false, nil
	}
	if err := r.ensureLoaded(ctx); err != nil {
		return "", false, err
	}
	value, ok := r.secrets[key]
	if !ok {
		return "", false, nil
	}
	return string(value), true, nil
}

func (r *Resolver) Source() string {
	if r == nil {
		return ""
	}
	return r.source
}

func (r *Resolver) ProviderName() string {
	if r == nil || r.provider == nil {
		return ""
	}
	return r.provider.Name()
}

func (r *Resolver) Required() bool {
	if r == nil {
		return false
	}
	return r.required
}

// Close wipes cached secrets and closes the provider.
func (r *Resolver) Close() error {
	if r == nil {
		return nil
	}
	for k, v := range r.secrets {
		zeroBytes(v)
		delete(r.secrets, k)
	}
	if r.provider != nil {
		return r.provider.Close()
	}
	return nil
}

func cloneBytes(b []byte) []byte {
	if len(b) == 0 {
		return nil
	}
	out := make([]byte, len(b))
	copy(out, b)
	return out
}

func zeroBytes(b []byte) {
	for i := range b {
		b[i] = 0
	}
}
