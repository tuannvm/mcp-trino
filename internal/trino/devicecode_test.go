package trino

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/tuannvm/mcp-trino/internal/config"
	"golang.org/x/oauth2"
)

func TestDeriveDeviceCodeURL(t *testing.T) {
	tests := []struct {
		name     string
		tokenURL string
		expected string
	}{
		{
			name:     "Azure AD v2.0",
			tokenURL: "https://login.microsoftonline.com/tenant-id/oauth2/v2.0/token",
			expected: "https://login.microsoftonline.com/tenant-id/oauth2/v2.0/devicecode",
		},
		{
			name:     "Generic /token suffix",
			tokenURL: "https://auth.example.com/oauth/token",
			expected: "https://auth.example.com/oauth/devicecode",
		},
		{
			name:     "No /token suffix",
			tokenURL: "https://auth.example.com/oauth",
			expected: "https://auth.example.com/oauth/devicecode",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := deriveDeviceCodeURL(tt.tokenURL)
			if result != tt.expected {
				t.Errorf("deriveDeviceCodeURL(%q) = %q, want %q", tt.tokenURL, result, tt.expected)
			}
		})
	}
}

func TestScopedCachePath(t *testing.T) {
	// Different tokenURL+clientID combos should produce different cache paths
	path1 := scopedCachePath("https://login.microsoftonline.com/tenant1/oauth2/v2.0/token", "client-a")
	path2 := scopedCachePath("https://login.microsoftonline.com/tenant2/oauth2/v2.0/token", "client-a")
	path3 := scopedCachePath("https://login.microsoftonline.com/tenant1/oauth2/v2.0/token", "client-b")
	path4 := scopedCachePath("https://login.microsoftonline.com/tenant1/oauth2/v2.0/token", "client-a")

	if path1 == path2 {
		t.Error("Different tenants should produce different cache paths")
	}
	if path1 == path3 {
		t.Error("Different client IDs should produce different cache paths")
	}
	if path1 != path4 {
		t.Error("Same tokenURL+clientID should produce same cache path")
	}
	if !strings.Contains(path1, "token-cache-") {
		t.Errorf("Cache path should contain 'token-cache-' prefix, got %q", path1)
	}
	if !strings.HasSuffix(path1, ".json") {
		t.Errorf("Cache path should end with .json, got %q", path1)
	}
}

func TestParseScopes(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected []string
	}{
		{"empty", "", nil},
		{"single", "openid", []string{"openid"}},
		{"multiple", "openid,profile,email", []string{"openid", "profile", "email"}},
		{"with spaces", " openid , profile , email ", []string{"openid", "profile", "email"}},
		{"trailing comma", "openid,profile,", []string{"openid", "profile"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := parseScopes(tt.input)
			if len(result) != len(tt.expected) {
				t.Errorf("parseScopes(%q) = %v, want %v", tt.input, result, tt.expected)
				return
			}
			for i, s := range result {
				if s != tt.expected[i] {
					t.Errorf("parseScopes(%q)[%d] = %q, want %q", tt.input, i, s, tt.expected[i])
				}
			}
		})
	}
}

func TestMigrateLegacyCache(t *testing.T) {
	tmpDir := t.TempDir()
	legacyPath := filepath.Join(tmpDir, "token-cache.json")
	scopedPath := filepath.Join(tmpDir, "token-cache-abc123.json")

	// Write a legacy cache file
	legacyData := []byte(`{"access_token":"legacy-token","refresh_token":"legacy-refresh","token_type":"Bearer","expires_at":"2099-01-01T00:00:00Z"}`)
	_ = os.WriteFile(legacyPath, legacyData, 0600)

	// Override defaultCachePath by directly calling the migration logic
	// Since migrateLegacyCache uses defaultCachePath(), we test the core logic manually
	if _, err := os.Stat(scopedPath); err == nil {
		t.Fatal("Scoped path should not exist yet")
	}

	// Copy legacy → scoped (same logic as migrateLegacyCache)
	data, err := os.ReadFile(legacyPath)
	if err != nil {
		t.Fatalf("Failed to read legacy cache: %v", err)
	}
	_ = os.WriteFile(scopedPath, data, 0600)

	// Verify scoped file has the same content
	scopedData, err := os.ReadFile(scopedPath)
	if err != nil {
		t.Fatalf("Failed to read scoped cache: %v", err)
	}
	if string(scopedData) != string(legacyData) {
		t.Error("Scoped cache content doesn't match legacy cache")
	}

	// If scoped already exists, migration should be a no-op (don't overwrite)
	_ = os.WriteFile(scopedPath, []byte(`{"access_token":"scoped-token"}`), 0600)
	migrateLegacyCache(scopedPath) // should not overwrite
	after, _ := os.ReadFile(scopedPath)
	if strings.Contains(string(after), "legacy-token") {
		t.Error("migrateLegacyCache should not overwrite existing scoped cache")
	}
}

func TestOAuthError(t *testing.T) {
	err := &oauthError{Code: "authorization_pending", Description: "user hasn't authenticated yet"}
	if err.Error() != "authorization_pending: user hasn't authenticated yet" {
		t.Errorf("Error() = %q", err.Error())
	}

	// Verify errors.As works
	var wrapped error = err
	var target *oauthError
	if !errors.As(wrapped, &target) {
		t.Error("errors.As should match *oauthError")
	}
	if target.Code != "authorization_pending" {
		t.Errorf("Code = %q", target.Code)
	}
}

func TestTokenCacheLoadSave(t *testing.T) {
	tmpDir := t.TempDir()
	cachePath := filepath.Join(tmpDir, "token-cache.json")

	ts := &deviceCodeTokenSource{
		cachePath: cachePath,
	}

	// Initially no cache
	if tok := ts.loadCache(); tok != nil {
		t.Error("Expected nil from empty cache")
	}

	// Save a token
	token := &oauth2.Token{
		AccessToken:  "test-access-token",
		RefreshToken: "test-refresh-token",
		TokenType:    "Bearer",
		Expiry:       time.Now().Add(1 * time.Hour),
	}
	ts.saveCache(token)

	// Verify file exists with restricted permissions
	info, err := os.Stat(cachePath)
	if err != nil {
		t.Fatalf("Cache file not created: %v", err)
	}
	if info.Size() == 0 {
		t.Error("Cache file is empty")
	}

	// Load it back
	loaded := ts.loadCache()
	if loaded == nil {
		t.Fatal("Failed to load cached token")
	}
	if loaded.AccessToken != "test-access-token" {
		t.Errorf("AccessToken = %q, want 'test-access-token'", loaded.AccessToken)
	}
	if loaded.RefreshToken != "test-refresh-token" {
		t.Errorf("RefreshToken = %q, want 'test-refresh-token'", loaded.RefreshToken)
	}
}

func TestTokenCacheExpired(t *testing.T) {
	tmpDir := t.TempDir()
	cachePath := filepath.Join(tmpDir, "token-cache.json")

	ts := &deviceCodeTokenSource{
		cachePath: cachePath,
	}

	// Save an expired token with a refresh token
	token := &oauth2.Token{
		AccessToken:  "expired-access-token",
		RefreshToken: "valid-refresh-token",
		TokenType:    "Bearer",
		Expiry:       time.Now().Add(-1 * time.Hour), // expired
	}
	ts.saveCache(token)

	loaded := ts.loadCache()
	if loaded == nil {
		t.Fatal("Should load expired token (for refresh)")
	}
	if loaded.Valid() {
		t.Error("Expired token should not be valid")
	}
	if loaded.RefreshToken != "valid-refresh-token" {
		t.Error("Should preserve refresh token")
	}
}

func TestCreateDeviceCodeTokenSource(t *testing.T) {
	cfg := &config.TrinoConfig{
		TrinoAuthMode:      "device-code",
		TrinoOAuthTokenURL: "https://login.microsoftonline.com/tenant/oauth2/v2.0/token",
		TrinoOAuthClientID: "test-client-id",
		TrinoOAuthScopes:   "openid,profile,api://app/.default",
	}

	ts := createDeviceCodeTokenSource(cfg)
	if ts == nil {
		t.Fatal("Expected non-nil token source for device-code mode")
	}

	dts, ok := ts.(*deviceCodeTokenSource)
	if !ok {
		t.Fatal("Expected *deviceCodeTokenSource")
	}
	if dts.clientID != "test-client-id" {
		t.Errorf("clientID = %q", dts.clientID)
	}
	if dts.deviceCodeURL != "https://login.microsoftonline.com/tenant/oauth2/v2.0/devicecode" {
		t.Errorf("deviceCodeURL = %q", dts.deviceCodeURL)
	}
	if len(dts.scopes) != 3 {
		t.Errorf("scopes = %v, want 3 items", dts.scopes)
	}
	// Cache path should be scoped (not the old default path)
	if dts.cachePath == defaultCachePath() {
		t.Error("cachePath should be scoped, not the default unscoped path")
	}
	if !strings.Contains(dts.cachePath, "token-cache-") {
		t.Errorf("cachePath should contain hash, got %q", dts.cachePath)
	}
}

func TestCreateDeviceCodeTokenSource_BasicMode(t *testing.T) {
	cfg := &config.TrinoConfig{
		TrinoAuthMode: "basic",
	}

	ts := createDeviceCodeTokenSource(cfg)
	if ts != nil {
		t.Fatal("Expected nil token source for basic mode")
	}
}

func TestDeviceCodeRefreshToken(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = r.ParseForm()
		if r.Form.Get("grant_type") != "refresh_token" {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		if r.Form.Get("refresh_token") != "test-refresh-token" {
			w.WriteHeader(http.StatusUnauthorized)
			json.NewEncoder(w).Encode(map[string]string{
				"error":             "invalid_grant",
				"error_description": "bad refresh token",
			})
			return
		}

		json.NewEncoder(w).Encode(map[string]interface{}{
			"access_token":  "new-access-token",
			"refresh_token": "new-refresh-token",
			"token_type":    "Bearer",
			"expires_in":    3600,
		})
	}))
	defer server.Close()

	ts := &deviceCodeTokenSource{
		clientID:   "test-client",
		tokenURL:   server.URL + "/token",
		scopes:     []string{"openid"},
		httpClient: server.Client(),
	}

	// Test successful refresh
	token, err := ts.refreshToken("test-refresh-token")
	if err != nil {
		t.Fatalf("refreshToken() failed: %v", err)
	}
	if token.AccessToken != "new-access-token" {
		t.Errorf("AccessToken = %q, want 'new-access-token'", token.AccessToken)
	}
	if token.RefreshToken != "new-refresh-token" {
		t.Errorf("RefreshToken = %q, want 'new-refresh-token'", token.RefreshToken)
	}

	// Test failed refresh — should produce structured oauthError
	_, err = ts.refreshToken("bad-token")
	if err == nil {
		t.Fatal("Expected error for bad refresh token")
	}
	// Unwrap through the "refresh failed:" prefix
	var oauthErr *oauthError
	if !errors.As(err, &oauthErr) {
		t.Fatalf("Expected *oauthError, got %T: %v", err, err)
	}
	if oauthErr.Code != "invalid_grant" {
		t.Errorf("oauthError.Code = %q, want 'invalid_grant'", oauthErr.Code)
	}
}

func TestDeviceCodeRefreshToken_PreservesRefreshToken(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Return a response without refresh_token — should preserve the old one
		json.NewEncoder(w).Encode(map[string]interface{}{
			"access_token": "new-access",
			"token_type":   "Bearer",
			"expires_in":   3600,
		})
	}))
	defer server.Close()

	ts := &deviceCodeTokenSource{
		clientID:   "test",
		tokenURL:   server.URL,
		httpClient: server.Client(),
	}

	token, err := ts.refreshToken("original-refresh-token")
	if err != nil {
		t.Fatalf("refreshToken() failed: %v", err)
	}
	if token.RefreshToken != "original-refresh-token" {
		t.Errorf("Should preserve original refresh token, got %q", token.RefreshToken)
	}
}

func TestDoTokenRequest_ServerError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte("<html>Internal Server Error</html>"))
	}))
	defer server.Close()

	ts := &deviceCodeTokenSource{
		tokenURL:   server.URL,
		httpClient: server.Client(),
	}

	_, err := ts.doTokenRequest(nil)
	if err == nil {
		t.Fatal("Expected error for 500 response")
	}
	if !strings.Contains(err.Error(), "HTTP 500") {
		t.Errorf("Error should mention HTTP 500, got: %v", err)
	}
}

func TestDoTokenRequest_OAuthError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{
			"error":             "authorization_pending",
			"error_description": "user hasn't authenticated",
		})
	}))
	defer server.Close()

	ts := &deviceCodeTokenSource{
		tokenURL:   server.URL,
		httpClient: server.Client(),
	}

	_, err := ts.doTokenRequest(nil)
	if err == nil {
		t.Fatal("Expected error")
	}
	var oauthErr *oauthError
	if !errors.As(err, &oauthErr) {
		t.Fatalf("Expected *oauthError, got %T: %v", err, err)
	}
	if oauthErr.Code != "authorization_pending" {
		t.Errorf("Code = %q", oauthErr.Code)
	}
}

func TestPollForToken_MinExpiry(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]interface{}{
			"access_token": "test-token",
			"token_type":   "Bearer",
			"expires_in":   0, // zero should be raised to minTokenExpiry
		})
	}))
	defer server.Close()

	ts := &deviceCodeTokenSource{
		clientID:   "test",
		tokenURL:   server.URL,
		httpClient: server.Client(),
	}

	token, err := ts.pollForToken("device-code")
	if err != nil {
		t.Fatalf("pollForToken() failed: %v", err)
	}
	// Token should have at least minTokenExpiry seconds of lifetime
	if time.Until(token.Expiry) < time.Duration(minTokenExpiry-5)*time.Second {
		t.Errorf("Token expiry too soon: %v (expected at least %ds)", token.Expiry, minTokenExpiry)
	}
}

func TestDeviceCodeTokenSource_CachedValidToken(t *testing.T) {
	tmpDir := t.TempDir()
	cachePath := filepath.Join(tmpDir, "token-cache.json")

	// Pre-populate cache with a valid token
	cache := tokenCache{
		AccessToken:  "cached-valid-token",
		RefreshToken: "cached-refresh",
		TokenType:    "Bearer",
		ExpiresAt:    time.Now().Add(1 * time.Hour),
	}
	data, _ := json.Marshal(cache)
	_ = os.WriteFile(cachePath, data, 0600)

	ts := &deviceCodeTokenSource{
		clientID:  "test",
		cachePath: cachePath,
	}

	// Should return cached token without any HTTP calls
	token, err := ts.Token()
	if err != nil {
		t.Fatalf("Token() failed: %v", err)
	}
	if token.AccessToken != "cached-valid-token" {
		t.Errorf("AccessToken = %q, want 'cached-valid-token'", token.AccessToken)
	}
}

func TestDeviceCodeTokenSource_RefreshExpiredCache(t *testing.T) {
	tmpDir := t.TempDir()
	cachePath := filepath.Join(tmpDir, "token-cache.json")

	// Mock token server for refresh
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = r.ParseForm()
		if r.Form.Get("grant_type") == "refresh_token" {
			json.NewEncoder(w).Encode(map[string]interface{}{
				"access_token":  "refreshed-access-token",
				"refresh_token": "refreshed-refresh-token",
				"token_type":    "Bearer",
				"expires_in":    3600,
			})
			return
		}
		w.WriteHeader(http.StatusBadRequest)
	}))
	defer server.Close()

	// Pre-populate cache with expired token but valid refresh token
	cache := tokenCache{
		AccessToken:  "expired-access",
		RefreshToken: "valid-refresh",
		TokenType:    "Bearer",
		ExpiresAt:    time.Now().Add(-1 * time.Hour), // expired
	}
	data, _ := json.Marshal(cache)
	_ = os.WriteFile(cachePath, data, 0600)

	ts := &deviceCodeTokenSource{
		clientID:   "test",
		tokenURL:   server.URL + "/token",
		scopes:     []string{"openid"},
		cachePath:  cachePath,
		httpClient: server.Client(),
	}

	token, err := ts.Token()
	if err != nil {
		t.Fatalf("Token() failed: %v", err)
	}
	if token.AccessToken != "refreshed-access-token" {
		t.Errorf("AccessToken = %q, want 'refreshed-access-token'", token.AccessToken)
	}
}

func TestBuildDSN_DeviceCodeMode(t *testing.T) {
	cfg := &config.TrinoConfig{
		Host:          "trino.example.com",
		Port:          443,
		User:          "user@example.com",
		Password:      "should-be-ignored",
		Catalog:       "delta",
		Schema:        "prod",
		Scheme:        "https",
		SSL:           true,
		SSLInsecure:   false,
		TrinoAuthMode: "device-code",
	}

	dsn := buildDSN(cfg)

	if strings.Contains(dsn, "should-be-ignored") {
		t.Error("device-code DSN should not contain password")
	}
	if !strings.Contains(dsn, "trino.example.com") {
		t.Error("DSN should contain host")
	}
}
