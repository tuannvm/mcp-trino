package trino

import (
	"bytes"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/tuannvm/mcp-trino/internal/config"
)

type roundTripFunc func(req *http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req)
}

func TestParseBearerAuthChallenge(t *testing.T) {
	headers := http.Header{}
	headers.Add("WWW-Authenticate", `Bearer x_redirect_server="https://login.example.com/auth", x_token_server="https://trino.example.com/token/123"`)

	challenge, err := parseBearerAuthChallenge(headers)
	if err != nil {
		t.Fatalf("parseBearerAuthChallenge() error = %v", err)
	}
	if challenge.RedirectURL != "https://login.example.com/auth" {
		t.Errorf("RedirectURL = %q", challenge.RedirectURL)
	}
	if challenge.TokenURL != "https://trino.example.com/token/123" {
		t.Errorf("TokenURL = %q", challenge.TokenURL)
	}
}

func TestExternalAuthTokenManagerWaitForToken(t *testing.T) {
	var deleteCalled bool
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"token":"fresh-token"}`))
		case http.MethodDelete:
			deleteCalled = true
			w.WriteHeader(http.StatusNoContent)
		default:
			w.WriteHeader(http.StatusMethodNotAllowed)
		}
	}))
	defer server.Close()

	manager := &externalAuthTokenManager{
		httpClient:   server.Client(),
		pollInterval: 10 * time.Millisecond,
		pollTimeout:  time.Second,
	}

	token, err := manager.waitForToken(server.URL)
	if err != nil {
		t.Fatalf("waitForToken() error = %v", err)
	}
	if token != "fresh-token" {
		t.Errorf("token = %q", token)
	}
	if !deleteCalled {
		t.Error("expected token cleanup DELETE after successful poll")
	}
}

func TestExternalAuthTokenManagerCache(t *testing.T) {
	cachePath := t.TempDir() + `\external-cache.json`
	manager := &externalAuthTokenManager{cachePath: cachePath}
	manager.saveCache(&externalTokenCache{AccessToken: "cached-token"})

	cache := manager.loadCache()
	if cache == nil || cache.AccessToken != "cached-token" {
		t.Fatalf("loadCache() = %#v", cache)
	}

	manager.token = "cached-token"
	manager.InvalidateToken()
	if manager.CurrentToken() != "" {
		t.Error("expected invalidated token to be cleared")
	}
	if _, err := os.Stat(cachePath); !os.IsNotExist(err) {
		t.Error("expected cache file to be removed")
	}
}

func TestHeaderRoundTripperExternalAuthChallenge(t *testing.T) {
	manager := &externalAuthTokenManager{
		pollInterval: 10 * time.Millisecond,
		pollTimeout:  time.Second,
	}
	manager.openBrowser = func(url string) error { return nil }

	pollServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"token":"fresh-bearer-token"}`))
	}))
	defer pollServer.Close()
	manager.httpClient = pollServer.Client()

	requestCount := 0
	transport := &headerRoundTripper{
		config: &config.TrinoConfig{
			AuthMode:    config.AuthModeExternal,
			TrinoSource: "mcp-trino/test",
		},
		tokenManager: manager,
		base: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			requestCount++
			body, _ := io.ReadAll(req.Body)
			_ = req.Body.Close()

			if requestCount == 1 {
				if got := req.Header.Get("Authorization"); got != "" {
					t.Fatalf("unexpected Authorization on first request: %q", got)
				}
				headers := http.Header{}
				headers.Add("WWW-Authenticate", `Bearer x_redirect_server="https://login.example.com/auth", x_token_server="`+pollServer.URL+`"`)
				return &http.Response{
					StatusCode: http.StatusUnauthorized,
					Header:     headers,
					Body: io.NopCloser(strings.NewReader("unauthorized")),
				}, nil
			}

			if got := req.Header.Get("Authorization"); got != "Bearer fresh-bearer-token" {
				t.Fatalf("expected bearer token on retry, got %q", got)
			}
			if got := string(body); got != "SELECT 1" {
				t.Fatalf("expected retried request body, got %q", got)
			}
			return &http.Response{
				StatusCode: http.StatusOK,
				Header:     http.Header{},
				Body:       io.NopCloser(strings.NewReader("ok")),
			}, nil
		}),
	}

	req, err := http.NewRequest(http.MethodPost, "https://trino.example.com/v1/statement", io.NopCloser(bytes.NewReader([]byte("SELECT 1"))))
	if err != nil {
		t.Fatalf("http.NewRequest() error = %v", err)
	}
	resp, err := transport.RoundTrip(req)
	if err != nil {
		t.Fatalf("RoundTrip() error = %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}
	if requestCount != 2 {
		t.Fatalf("expected 2 requests, got %d", requestCount)
	}
}

func TestBuildDSNExternalModeOmitsPassword(t *testing.T) {
	dsn := buildDSN(&config.TrinoConfig{
		Scheme:      "https",
		Host:        "trino.example.com",
		Port:        443,
		User:        "alice",
		Password:    "secret",
		Catalog:     "memory",
		Schema:      "default",
		SSL:         true,
		SSLInsecure: true,
		AuthMode:    config.AuthModeExternal,
	})

	if strings.Contains(dsn, "secret") {
		t.Fatalf("expected external auth DSN to omit password, got %q", dsn)
	}
	if !strings.Contains(dsn, "alice@trino.example.com:443") {
		t.Fatalf("expected DSN to retain user attribution, got %q", dsn)
	}
}
