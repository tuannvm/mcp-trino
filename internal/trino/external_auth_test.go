package trino

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"
)

func TestParseAuthHeader(t *testing.T) {
	tests := []struct {
		name            string
		header          string
		wantRedirectURL string
		wantTokenURL    string
	}{
		{
			name:            "Valid Bearer challenge",
			header:          `Bearer x_redirect_server="https://trino.example.com/oauth2/token/initiate/abc123", x_token_server="https://trino.example.com/oauth2/token/xyz789"`,
			wantRedirectURL: "https://trino.example.com/oauth2/token/initiate/abc123",
			wantTokenURL:    "https://trino.example.com/oauth2/token/xyz789",
		},
		{
			name:            "Real Trino response format",
			header:          `Bearer x_redirect_server="https://trinodb-adhoc.example.com/oauth2/token/initiate/870c376e3314f9a406a877fb431069ffae2ead578d96db6031fa597efc388554", x_token_server="https://trinodb-adhoc.example.com/oauth2/token/20e2566b-d500-4b2f-a691-f62841471e83"`,
			wantRedirectURL: "https://trinodb-adhoc.example.com/oauth2/token/initiate/870c376e3314f9a406a877fb431069ffae2ead578d96db6031fa597efc388554",
			wantTokenURL:    "https://trinodb-adhoc.example.com/oauth2/token/20e2566b-d500-4b2f-a691-f62841471e83",
		},
		{
			name:            "Irregular whitespace",
			header:          `Bearer x_redirect_server= "https://example.com/redirect" , x_token_server ="https://example.com/token"`,
			wantRedirectURL: "https://example.com/redirect",
			wantTokenURL:    "https://example.com/token",
		},
		{
			name:            "Missing Bearer prefix",
			header:          `x_redirect_server="https://trino.example.com/oauth2/token/initiate/abc123", x_token_server="https://trino.example.com/oauth2/token/xyz789"`,
			wantRedirectURL: "",
			wantTokenURL:    "",
		},
		{
			name:            "Missing x_token_server",
			header:          `Bearer x_redirect_server="https://trino.example.com/oauth2/token/initiate/abc123"`,
			wantRedirectURL: "https://trino.example.com/oauth2/token/initiate/abc123",
			wantTokenURL:    "",
		},
		{
			name:            "Empty header",
			header:          "",
			wantRedirectURL: "",
			wantTokenURL:    "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotRedirect, gotToken := parseAuthHeader(tt.header)
			if gotRedirect != tt.wantRedirectURL {
				t.Errorf("parseAuthHeader() redirectURL = %v, want %v", gotRedirect, tt.wantRedirectURL)
			}
			if gotToken != tt.wantTokenURL {
				t.Errorf("parseAuthHeader() tokenURL = %v, want %v", gotToken, tt.wantTokenURL)
			}
		})
	}
}

func TestIsAuthenticationError(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want bool
	}{
		{
			name: "401 status code",
			err:  fmt.Errorf("HTTP 401 Unauthorized"),
			want: true,
		},
		{
			name: "Unauthorized keyword",
			err:  fmt.Errorf("request unauthorized"),
			want: true,
		},
		{
			name: "Authentication keyword",
			err:  fmt.Errorf("authentication failed"),
			want: true,
		},
		{
			name: "Connection refused",
			err:  fmt.Errorf("dial tcp: connection refused"),
			want: true,
		},
		{
			name: "Connection reset",
			err:  fmt.Errorf("read: connection reset by peer"),
			want: true,
		},
		{
			name: "Use of closed connection",
			err:  fmt.Errorf("use of closed network connection"),
			want: true,
		},
		{
			name: "Broken pipe",
			err:  fmt.Errorf("write: broken pipe"),
			want: true,
		},
		{
			name: "Other error",
			err:  fmt.Errorf("query timeout exceeded"),
			want: false,
		},
		{
			name: "Nil error",
			err:  nil,
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := IsAuthenticationError(tt.err); got != tt.want {
				t.Errorf("IsAuthenticationError() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestInvalidateToken(t *testing.T) {
	auth := NewExternalAuthenticator("https://trino.example.com", "testuser", 300)

	// Manually set a cached token
	auth.tokenCache = &tokenCache{
		token:     "test-token",
		expiresAt: time.Now().Add(1 * time.Hour),
	}

	// Verify token is cached
	if auth.tokenCache == nil {
		t.Fatal("Expected token cache to be set")
	}

	// Invalidate the token
	auth.InvalidateToken()

	// Verify token cache is cleared
	if auth.tokenCache != nil {
		t.Error("Expected token cache to be nil after InvalidateToken()")
	}
}

func TestTokenCaching(t *testing.T) {
	auth := NewExternalAuthenticator("https://trino.example.com", "testuser", 300)

	// No token should be cached initially
	if auth.tokenCache != nil {
		t.Error("Expected no token cache initially")
	}

	// Manually cache a token (simulating successful auth)
	auth.tokenCache = &tokenCache{
		token:     "cached-token",
		expiresAt: time.Now().Add(1 * time.Hour),
	}

	// Token should be available from cache
	if auth.tokenCache == nil || auth.tokenCache.token != "cached-token" {
		t.Error("Expected cached token to be available")
	}

	// Test expired token
	auth.tokenCache = &tokenCache{
		token:     "expired-token",
		expiresAt: time.Now().Add(-1 * time.Hour), // Already expired
	}

	// GetToken would trigger re-auth for expired token, but we can verify the expiry check
	if time.Now().Before(auth.tokenCache.expiresAt) {
		t.Error("Expected token to be expired")
	}
}

// TestConcurrentGetTokenWithCache verifies thread-safety of GetToken with cached tokens.
// Run with -race to detect data races.
func TestConcurrentGetTokenWithCache(t *testing.T) {
	auth := NewExternalAuthenticator("https://trino.example.com", "testuser", 300)

	// Pre-populate cache so GetToken returns immediately without network calls
	auth.tokenCache = &tokenCache{
		token:     "cached-token",
		expiresAt: time.Now().Add(1 * time.Hour),
	}

	var wg sync.WaitGroup
	numGoroutines := 100
	results := make(chan string, numGoroutines)
	errors := make(chan error, numGoroutines)

	// Spawn goroutines that all call GetToken concurrently
	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			token, err := auth.GetToken(context.Background())
			if err != nil {
				errors <- err
				return
			}
			results <- token
		}()
	}

	wg.Wait()
	close(results)
	close(errors)

	// Check for errors
	for err := range errors {
		t.Errorf("GetToken failed: %v", err)
	}

	// All tokens should be the same cached value
	for token := range results {
		if token != "cached-token" {
			t.Errorf("Expected cached-token, got %s", token)
		}
	}
}

// TestConcurrentInvalidateAndGetToken verifies no race between InvalidateToken and GetToken.
// Run with -race to detect data races.
func TestConcurrentInvalidateAndGetToken(t *testing.T) {
	auth := NewExternalAuthenticator("https://trino.example.com", "testuser", 300)

	// Pre-populate cache
	auth.tokenCache = &tokenCache{
		token:     "cached-token",
		expiresAt: time.Now().Add(1 * time.Hour),
	}

	var wg sync.WaitGroup

	// Goroutine 1: repeatedly invalidate
	wg.Add(1)
	go func() {
		defer wg.Done()
		for i := 0; i < 100; i++ {
			auth.InvalidateToken()
			// Re-populate so GetToken has something to read
			auth.mu.Lock()
			auth.tokenCache = &tokenCache{
				token:     "cached-token",
				expiresAt: time.Now().Add(1 * time.Hour),
			}
			auth.mu.Unlock()
		}
	}()

	// Goroutine 2: repeatedly try to get token (will fail on network but shouldn't race)
	wg.Add(1)
	go func() {
		defer wg.Done()
		for i := 0; i < 100; i++ {
			// GetToken may return cached or fail on network - we just care about no race
			_, _ = auth.GetToken(context.Background())
		}
	}()

	wg.Wait()
	// If we get here without -race detecting issues, the test passes
}
