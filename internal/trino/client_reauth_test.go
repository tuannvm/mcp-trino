package trino

import (
	"sync"
	"testing"

	"github.com/tuannvm/mcp-trino/internal/config"
)

func TestClearConnectionForReauth(t *testing.T) {
	cfg := &config.TrinoConfig{
		Host:                "localhost",
		Port:                8080,
		Scheme:              "http",
		ExternalAuth:        true,
		ExternalAuthTimeout: 300,
	}

	// Create client with external auth (lazy init, no actual connection)
	client := &Client{
		config:      cfg,
		initialized: true,
		authenticator: NewExternalAuthenticator(
			"http://localhost:8080",
			"testuser",
			300,
			false,
		),
	}

	// Set a token in cache
	client.authenticator.tokenCache = &tokenCache{
		token: "test-token",
	}

	// Clear connection for re-auth
	client.clearConnectionForReauth()

	// Verify state is cleared
	if client.initialized {
		t.Error("Expected initialized to be false after clearConnectionForReauth")
	}
	if client.db != nil {
		t.Error("Expected db to be nil after clearConnectionForReauth")
	}
	if client.authenticator.tokenCache != nil {
		t.Error("Expected token cache to be cleared after clearConnectionForReauth")
	}
}

func TestClientCloseWithNilDB(t *testing.T) {
	cfg := &config.TrinoConfig{
		Host:         "localhost",
		Port:         8080,
		Scheme:       "http",
		ExternalAuth: true,
	}

	// Create client with nil db (lazy auth, not yet connected)
	client := &Client{
		config:      cfg,
		db:          nil,
		initialized: false,
	}

	// Close should not panic with nil db
	err := client.Close()
	if err != nil {
		t.Errorf("Expected no error closing client with nil db, got: %v", err)
	}
}

func TestLazyAuthClientCreation(t *testing.T) {
	cfg := &config.TrinoConfig{
		Host:                "localhost",
		Port:                8080,
		Scheme:              "http",
		ExternalAuth:        true,
		ExternalAuthTimeout: 300,
	}

	// NewClient with external auth should NOT connect immediately
	client, err := NewClient(cfg)
	if err != nil {
		t.Fatalf("NewClient() failed: %v", err)
	}
	defer client.Close()

	// Should have authenticator
	if client.authenticator == nil {
		t.Error("Expected authenticator to be set for external auth")
	}

	// Should NOT be initialized yet (lazy)
	if client.initialized {
		t.Error("Expected client to NOT be initialized with external auth (lazy)")
	}

	// db should be nil until first query
	if client.db != nil {
		t.Error("Expected db to be nil until first query")
	}
}

// TestConcurrentCloseAndClear verifies no race between Close() and clearConnectionForReauth().
// Run with -race to detect data races.
func TestConcurrentCloseAndClear(t *testing.T) {
	cfg := &config.TrinoConfig{
		Host:                "localhost",
		Port:                8080,
		Scheme:              "http",
		ExternalAuth:        true,
		ExternalAuthTimeout: 300,
	}

	for i := 0; i < 100; i++ {
		client := &Client{
			config:      cfg,
			initialized: true,
			authenticator: NewExternalAuthenticator(
				"http://localhost:8080",
				"testuser",
				300,
				false,
			),
		}
		client.authenticator.tokenCache = &tokenCache{token: "test"}

		var wg sync.WaitGroup

		// Goroutine 1: Close
		wg.Add(1)
		go func() {
			defer wg.Done()
			_ = client.Close()
		}()

		// Goroutine 2: clearConnectionForReauth
		wg.Add(1)
		go func() {
			defer wg.Done()
			client.clearConnectionForReauth()
		}()

		wg.Wait()
	}
	// If we get here without -race detecting issues, the test passes
}

// TestConcurrentMultipleCloses verifies Close() is safe to call concurrently.
// Run with -race to detect data races.
func TestConcurrentMultipleCloses(t *testing.T) {
	cfg := &config.TrinoConfig{
		Host:                "localhost",
		Port:                8080,
		Scheme:              "http",
		ExternalAuth:        true,
		ExternalAuthTimeout: 300,
	}

	client := &Client{
		config:      cfg,
		initialized: true,
		authenticator: NewExternalAuthenticator(
			"http://localhost:8080",
			"testuser",
			300,
			false,
		),
	}

	var wg sync.WaitGroup
	numGoroutines := 10

	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_ = client.Close()
		}()
	}

	wg.Wait()
	// Should not panic or race
}
