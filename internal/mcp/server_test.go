package mcp

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
	mcpserver "github.com/mark3labs/mcp-go/server"
	oauth "github.com/tuannvm/oauth-mcp-proxy"

	"github.com/tuannvm/mcp-trino/internal/config"
)

// Test fixtures for the HMAC provider, mirroring oauth-mcp-proxy's own test
// pattern (api_test.go TestServerWrapHandler) so no live IdP is needed.
const (
	testJWTSecret = "test-secret-key-must-be-32-bytes-long!"
	testAudience  = "api://test"
	testIssuer    = "https://test.example.com"
)

// newOAuthTestServer builds a *Server with OAuth enabled against the HMAC
// provider (auto-detects "native" mode since no ClientID is set - mode is
// irrelevant to the transport-gate behavior under test here).
func newOAuthTestServer(t *testing.T) *Server {
	t.Helper()

	oauthServer, err := oauth.NewServer(&oauth.Config{
		Provider:  "hmac",
		Issuer:    testIssuer,
		Audience:  testAudience,
		ServerURL: "https://test-server.example.com",
		JWTSecret: []byte(testJWTSecret),
	})
	if err != nil {
		t.Fatalf("oauth.NewServer failed: %v", err)
	}

	return &Server{
		mcpServer:   mcpserver.NewMCPServer("test", "0.0.0"),
		config:      &config.TrinoConfig{OAuthEnabled: true},
		version:     "0.0.0",
		oauthServer: oauthServer,
	}
}

func newStreamableServer(s *Server) *mcpserver.StreamableHTTPServer {
	return mcpserver.NewStreamableHTTPServer(s.mcpServer, mcpserver.WithEndpointPath("/mcp"))
}

// signHMACToken mints an HS256 JWT satisfying oauth-mcp-proxy's HMACValidator
// (requires sub, aud matching testAudience, and exp).
func signHMACToken(t *testing.T, exp time.Time) string {
	t.Helper()

	claims := jwt.MapClaims{
		"sub": "test-user",
		"aud": testAudience,
		"exp": exp.Unix(),
	}
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	signed, err := token.SignedString([]byte(testJWTSecret))
	if err != nil {
		t.Fatalf("failed to sign test token: %v", err)
	}
	return signed
}

func postMCP(handler http.HandlerFunc, authHeader string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodPost, "/mcp", strings.NewReader(`{}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json, text/event-stream")
	if authHeader != "" {
		req.Header.Set("Authorization", authHeader)
	}
	w := httptest.NewRecorder()
	handler(w, req)
	return w
}

func TestMCPEndpointRejectsMissingToken(t *testing.T) {
	s := newOAuthTestServer(t)
	handler := s.createMCPHandler(newStreamableServer(s))

	w := postMCP(handler, "")

	if w.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401 for missing token, got %d: %s", w.Code, w.Body.String())
	}
}

func TestMCPEndpointRejectsGarbageToken(t *testing.T) {
	// Regression test for the reported bug: a non-empty "Bearer <garbage>"
	// header previously satisfied the presence-only check and reached the
	// JSON-RPC layer with an HTTP 200 for initialize/tools/list.
	s := newOAuthTestServer(t)
	handler := s.createMCPHandler(newStreamableServer(s))

	w := postMCP(handler, "Bearer aaa.bbb.ccc")

	if w.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401 for garbage token, got %d: %s", w.Code, w.Body.String())
	}
}

func TestMCPEndpointRejectsExpiredToken(t *testing.T) {
	s := newOAuthTestServer(t)
	handler := s.createMCPHandler(newStreamableServer(s))

	expired := signHMACToken(t, time.Now().Add(-1*time.Hour))
	w := postMCP(handler, "Bearer "+expired)

	if w.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401 for expired token, got %d: %s", w.Code, w.Body.String())
	}
}

func TestMCPEndpointAcceptsValidToken(t *testing.T) {
	// Sanity check that the gate isn't rejecting everything: a well-formed,
	// unexpired token must not be turned away by the auth layer itself.
	s := newOAuthTestServer(t)
	handler := s.createMCPHandler(newStreamableServer(s))

	valid := signHMACToken(t, time.Now().Add(1*time.Hour))
	w := postMCP(handler, "Bearer "+valid)

	if w.Code == http.StatusUnauthorized {
		t.Fatalf("expected valid token to pass the auth gate, got 401: %s", w.Body.String())
	}
}

func TestMCPEndpoint401Shape(t *testing.T) {
	s := newOAuthTestServer(t)
	handler := s.createMCPHandler(newStreamableServer(s))

	w := postMCP(handler, "")

	if w.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", w.Code)
	}

	// WrapHandler sends WWW-Authenticate as separate header lines (one for
	// the error, one for resource_metadata) rather than one combined value -
	// both are valid per RFC 7235 §4.1, so check across all of them.
	authHeaders := w.Header().Values("WWW-Authenticate")
	joined := strings.Join(authHeaders, " | ")
	if len(authHeaders) == 0 || !strings.Contains(joined, "Bearer") {
		t.Errorf("expected a Bearer challenge in WWW-Authenticate, got: %q", joined)
	}
	if !strings.Contains(joined, "invalid_token") && !strings.Contains(joined, "invalid_request") {
		t.Errorf("expected WWW-Authenticate to carry an OAuth error code, got: %q", joined)
	}
	if !strings.Contains(joined, "resource_metadata=") {
		t.Errorf("expected WWW-Authenticate to carry resource_metadata for client discovery, got: %q", joined)
	}

	var body map[string]string
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatalf("expected a JSON error body, got %q: %v", w.Body.String(), err)
	}
	if body["error"] == "" {
		t.Errorf("expected a non-empty \"error\" field, got body: %v", body)
	}
}

func TestMCPEndpointAllowsOptionsWithoutAuth(t *testing.T) {
	// CORS preflight must be answered before the auth gate runs.
	s := newOAuthTestServer(t)
	handler := s.createMCPHandler(newStreamableServer(s))

	req := httptest.NewRequest(http.MethodOptions, "/mcp", nil)
	w := httptest.NewRecorder()
	handler(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected OPTIONS preflight to succeed without auth, got %d", w.Code)
	}
}

func TestMCPEndpointOAuthDisabledSkipsGate(t *testing.T) {
	// With OAuth off entirely, the MCP endpoint must not require a bearer
	// token at all.
	s := &Server{
		mcpServer: mcpserver.NewMCPServer("test", "0.0.0"),
		config:    &config.TrinoConfig{OAuthEnabled: false},
		version:   "0.0.0",
	}
	handler := s.createMCPHandler(newStreamableServer(s))

	w := postMCP(handler, "")

	if w.Code == http.StatusUnauthorized {
		t.Fatalf("expected no auth requirement with OAuth disabled, got 401: %s", w.Body.String())
	}
}
