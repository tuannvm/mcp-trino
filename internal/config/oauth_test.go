package config

import (
	"os"
	"strings"
	"testing"
)

func TestOAuthModeConfiguration(t *testing.T) {
	// Save original environment
	origOAuthMode := os.Getenv("OAUTH_MODE")
	origOAuthEnabled := os.Getenv("OAUTH_ENABLED")
	origOAuthProvider := os.Getenv("OAUTH_PROVIDER")

	// Clean up after test
	defer func() {
		_ = os.Setenv("OAUTH_MODE", origOAuthMode)
		_ = os.Setenv("OAUTH_ENABLED", origOAuthEnabled)
		_ = os.Setenv("OAUTH_PROVIDER", origOAuthProvider)
	}()

	tests := []struct {
		name           string
		oauthMode      string
		oauthEnabled   string
		oauthProvider  string
		expectedMode   string
		expectedEnable bool
	}{
		{
			name:           "Default native mode disabled",
			oauthMode:      "native",
			oauthEnabled:   "false",
			oauthProvider:  "hmac",
			expectedMode:   "native",
			expectedEnable: false,
		},
		{
			name:           "Explicit native mode with HMAC",
			oauthMode:      "native",
			oauthEnabled:   "true",
			oauthProvider:  "hmac",
			expectedMode:   "native",
			expectedEnable: true,
		},
		{
			name:           "Proxy mode with HMAC",
			oauthMode:      "proxy",
			oauthEnabled:   "true",
			oauthProvider:  "hmac",
			expectedMode:   "proxy",
			expectedEnable: true,
		},
		{
			name:           "Invalid mode accepted (validation delegated)",
			oauthMode:      "invalid",
			oauthEnabled:   "false",
			oauthProvider:  "hmac",
			expectedMode:   "invalid",
			expectedEnable: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_ = os.Setenv("OAUTH_MODE", tt.oauthMode)
			_ = os.Setenv("OAUTH_ENABLED", tt.oauthEnabled)
			_ = os.Setenv("OAUTH_PROVIDER", tt.oauthProvider)

			config, err := NewTrinoConfig()
			if err != nil {
				t.Errorf("Unexpected error: %v", err)
				return
			}

			if config.OAuthMode != tt.expectedMode {
				t.Errorf("OAuthMode = %s, expected %s", config.OAuthMode, tt.expectedMode)
			}

			if config.OAuthEnabled != tt.expectedEnable {
				t.Errorf("OAuthEnabled = %t, expected %t", config.OAuthEnabled, tt.expectedEnable)
			}
		})
	}
}

func TestOAuthAllowedRedirectsConfiguration(t *testing.T) {
	// Save original environment
	origRedirects := os.Getenv("OAUTH_ALLOWED_REDIRECT_URIS")
	origOAuthEnabled := os.Getenv("OAUTH_ENABLED")

	// Clean up after test
	defer func() {
		_ = os.Setenv("OAUTH_ALLOWED_REDIRECT_URIS", origRedirects)
		_ = os.Setenv("OAUTH_ENABLED", origOAuthEnabled)
	}()

	tests := []struct {
		name              string
		allowedRedirects  string
		expectedRedirects string
	}{
		{
			name:              "No redirects configured",
			allowedRedirects:  "",
			expectedRedirects: "",
		},
		{
			name:              "Single redirect URI",
			allowedRedirects:  "https://client.example.com/callback",
			expectedRedirects: "https://client.example.com/callback",
		},
		{
			name:              "Multiple redirect URIs",
			allowedRedirects:  "https://client1.example.com/callback,https://client2.example.com/callback",
			expectedRedirects: "https://client1.example.com/callback,https://client2.example.com/callback",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_ = os.Setenv("OAUTH_ALLOWED_REDIRECT_URIS", tt.allowedRedirects)
			_ = os.Setenv("OAUTH_ENABLED", "false") // Disable OAuth to avoid validation errors
			_ = os.Setenv("OAUTH_MODE", "native") // Set explicit mode to avoid validation errors

			config, err := NewTrinoConfig()
			if err != nil {
				t.Errorf("Unexpected error: %v", err)
				return
			}

			if config.OAuthRedirectURIs != tt.expectedRedirects {
				t.Errorf("OAuthRedirectURIs = %s, expected %s", config.OAuthRedirectURIs, tt.expectedRedirects)
			}
		})
	}
}

func TestTrinoAuthModeConfiguration(t *testing.T) {
	// Save original environment
	origAuthMode := os.Getenv("TRINO_AUTH_MODE")
	origOAuthEnabled := os.Getenv("OAUTH_ENABLED")
	origTokenURL := os.Getenv("TRINO_OAUTH_TOKEN_URL")
	origClientID := os.Getenv("TRINO_OAUTH_CLIENT_ID")
	origClientSecret := os.Getenv("TRINO_OAUTH_CLIENT_SECRET")
	origScopes := os.Getenv("TRINO_OAUTH_SCOPES")

	defer func() {
		_ = os.Setenv("TRINO_AUTH_MODE", origAuthMode)
		_ = os.Setenv("OAUTH_ENABLED", origOAuthEnabled)
		_ = os.Setenv("TRINO_OAUTH_TOKEN_URL", origTokenURL)
		_ = os.Setenv("TRINO_OAUTH_CLIENT_ID", origClientID)
		_ = os.Setenv("TRINO_OAUTH_CLIENT_SECRET", origClientSecret)
		_ = os.Setenv("TRINO_OAUTH_SCOPES", origScopes)
	}()

	tests := []struct {
		name             string
		authMode         string
		expectedAuthMode string
	}{
		{
			name:             "Default auth mode is basic",
			authMode:         "",
			expectedAuthMode: "basic",
		},
		{
			name:             "Explicit basic mode",
			authMode:         "basic",
			expectedAuthMode: "basic",
		},
		{
			name:             "OAuth mode",
			authMode:         "oauth",
			expectedAuthMode: "oauth",
		},
		{
			name:             "Case insensitive",
			authMode:         "OAuth",
			expectedAuthMode: "oauth",
		},
		{
			name:             "Device code mode",
			authMode:         "device-code",
			expectedAuthMode: "device-code",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_ = os.Setenv("OAUTH_ENABLED", "false")
			if tt.authMode == "" {
				_ = os.Unsetenv("TRINO_AUTH_MODE")
			} else {
				_ = os.Setenv("TRINO_AUTH_MODE", tt.authMode)
			}
			// When oauth mode, provide required fields to avoid validation error
			if tt.expectedAuthMode == "oauth" {
				_ = os.Setenv("TRINO_OAUTH_TOKEN_URL", "https://login.microsoftonline.com/tenant/oauth2/v2.0/token")
				_ = os.Setenv("TRINO_OAUTH_CLIENT_ID", "test-client-id")
				_ = os.Setenv("TRINO_OAUTH_CLIENT_SECRET", "test-secret")
			} else if tt.expectedAuthMode == "device-code" {
				_ = os.Setenv("TRINO_OAUTH_TOKEN_URL", "https://login.microsoftonline.com/tenant/oauth2/v2.0/token")
				_ = os.Setenv("TRINO_OAUTH_CLIENT_ID", "test-client-id")
				_ = os.Unsetenv("TRINO_OAUTH_CLIENT_SECRET")
			} else {
				_ = os.Unsetenv("TRINO_OAUTH_TOKEN_URL")
				_ = os.Unsetenv("TRINO_OAUTH_CLIENT_ID")
				_ = os.Unsetenv("TRINO_OAUTH_CLIENT_SECRET")
			}

			config, err := NewTrinoConfig()
			if err != nil {
				t.Fatalf("Unexpected error: %v", err)
			}

			if config.TrinoAuthMode != tt.expectedAuthMode {
				t.Errorf("TrinoAuthMode = %q, expected %q", config.TrinoAuthMode, tt.expectedAuthMode)
			}
		})
	}
}

func TestTrinoOAuthFieldsParsing(t *testing.T) {
	origAuthMode := os.Getenv("TRINO_AUTH_MODE")
	origOAuthEnabled := os.Getenv("OAUTH_ENABLED")
	origTokenURL := os.Getenv("TRINO_OAUTH_TOKEN_URL")
	origClientID := os.Getenv("TRINO_OAUTH_CLIENT_ID")
	origClientSecret := os.Getenv("TRINO_OAUTH_CLIENT_SECRET")
	origScopes := os.Getenv("TRINO_OAUTH_SCOPES")

	defer func() {
		_ = os.Setenv("TRINO_AUTH_MODE", origAuthMode)
		_ = os.Setenv("OAUTH_ENABLED", origOAuthEnabled)
		_ = os.Setenv("TRINO_OAUTH_TOKEN_URL", origTokenURL)
		_ = os.Setenv("TRINO_OAUTH_CLIENT_ID", origClientID)
		_ = os.Setenv("TRINO_OAUTH_CLIENT_SECRET", origClientSecret)
		_ = os.Setenv("TRINO_OAUTH_SCOPES", origScopes)
	}()

	_ = os.Setenv("OAUTH_ENABLED", "false")
	_ = os.Setenv("TRINO_AUTH_MODE", "oauth")
	_ = os.Setenv("TRINO_OAUTH_TOKEN_URL", "https://login.microsoftonline.com/my-tenant/oauth2/v2.0/token")
	_ = os.Setenv("TRINO_OAUTH_CLIENT_ID", "my-client-id")
	_ = os.Setenv("TRINO_OAUTH_CLIENT_SECRET", "my-client-secret")
	_ = os.Setenv("TRINO_OAUTH_SCOPES", "api://trino/.default,openid")

	config, err := NewTrinoConfig()
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}

	if config.TrinoOAuthTokenURL != "https://login.microsoftonline.com/my-tenant/oauth2/v2.0/token" {
		t.Errorf("TrinoOAuthTokenURL = %q", config.TrinoOAuthTokenURL)
	}
	if config.TrinoOAuthClientID != "my-client-id" {
		t.Errorf("TrinoOAuthClientID = %q", config.TrinoOAuthClientID)
	}
	if config.TrinoOAuthClientSecret != "my-client-secret" {
		t.Errorf("TrinoOAuthClientSecret = %q", config.TrinoOAuthClientSecret)
	}
	if config.TrinoOAuthScopes != "api://trino/.default,openid" {
		t.Errorf("TrinoOAuthScopes = %q", config.TrinoOAuthScopes)
	}
}

func TestTrinoOAuthValidation_MissingFields(t *testing.T) {
	origAuthMode := os.Getenv("TRINO_AUTH_MODE")
	origOAuthEnabled := os.Getenv("OAUTH_ENABLED")
	origTokenURL := os.Getenv("TRINO_OAUTH_TOKEN_URL")
	origClientID := os.Getenv("TRINO_OAUTH_CLIENT_ID")
	origClientSecret := os.Getenv("TRINO_OAUTH_CLIENT_SECRET")
	origScopes := os.Getenv("TRINO_OAUTH_SCOPES")

	defer func() {
		_ = os.Setenv("TRINO_AUTH_MODE", origAuthMode)
		_ = os.Setenv("OAUTH_ENABLED", origOAuthEnabled)
		_ = os.Setenv("TRINO_OAUTH_TOKEN_URL", origTokenURL)
		_ = os.Setenv("TRINO_OAUTH_CLIENT_ID", origClientID)
		_ = os.Setenv("TRINO_OAUTH_CLIENT_SECRET", origClientSecret)
		_ = os.Setenv("TRINO_OAUTH_SCOPES", origScopes)
	}()

	_ = os.Setenv("OAUTH_ENABLED", "false")

	tests := []struct {
		name         string
		tokenURL     string
		clientID     string
		clientSecret string
		expectErr    string
	}{
		{
			name:         "Missing token URL",
			tokenURL:     "",
			clientID:     "my-id",
			clientSecret: "my-secret",
			expectErr:    "TRINO_OAUTH_TOKEN_URL is required",
		},
		{
			name:         "Missing client ID",
			tokenURL:     "https://login.microsoftonline.com/tenant/oauth2/v2.0/token",
			clientID:     "",
			clientSecret: "my-secret",
			expectErr:    "TRINO_OAUTH_CLIENT_ID is required",
		},
		{
			name:         "Missing client secret",
			tokenURL:     "https://login.microsoftonline.com/tenant/oauth2/v2.0/token",
			clientID:     "my-id",
			clientSecret: "",
			expectErr:    "TRINO_OAUTH_CLIENT_SECRET is required",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_ = os.Setenv("TRINO_AUTH_MODE", "oauth")
			setOrUnset("TRINO_OAUTH_TOKEN_URL", tt.tokenURL)
			setOrUnset("TRINO_OAUTH_CLIENT_ID", tt.clientID)
			setOrUnset("TRINO_OAUTH_CLIENT_SECRET", tt.clientSecret)
			_ = os.Unsetenv("TRINO_OAUTH_SCOPES")

			_, err := NewTrinoConfig()
			if err == nil {
				t.Fatal("Expected error but got nil")
			}
			if !strings.Contains(err.Error(), tt.expectErr) {
				t.Errorf("Error = %q, expected to contain %q", err.Error(), tt.expectErr)
			}
		})
	}
}

func TestTrinoAuthModeInvalid(t *testing.T) {
	origAuthMode := os.Getenv("TRINO_AUTH_MODE")
	origOAuthEnabled := os.Getenv("OAUTH_ENABLED")

	defer func() {
		_ = os.Setenv("TRINO_AUTH_MODE", origAuthMode)
		_ = os.Setenv("OAUTH_ENABLED", origOAuthEnabled)
	}()

	_ = os.Setenv("OAUTH_ENABLED", "false")
	_ = os.Setenv("TRINO_AUTH_MODE", "kerberos")

	_, err := NewTrinoConfig()
	if err == nil {
		t.Fatal("Expected error for invalid auth mode but got nil")
	}
	if !strings.Contains(err.Error(), "invalid TRINO_AUTH_MODE") {
		t.Errorf("Error = %q, expected to contain 'invalid TRINO_AUTH_MODE'", err.Error())
	}
}

func TestTrinoOAuthBasicModeNoValidation(t *testing.T) {
	// When auth mode is basic, OAuth fields should NOT be validated
	origAuthMode := os.Getenv("TRINO_AUTH_MODE")
	origOAuthEnabled := os.Getenv("OAUTH_ENABLED")
	origTokenURL := os.Getenv("TRINO_OAUTH_TOKEN_URL")
	origClientID := os.Getenv("TRINO_OAUTH_CLIENT_ID")
	origClientSecret := os.Getenv("TRINO_OAUTH_CLIENT_SECRET")

	defer func() {
		_ = os.Setenv("TRINO_AUTH_MODE", origAuthMode)
		_ = os.Setenv("OAUTH_ENABLED", origOAuthEnabled)
		_ = os.Setenv("TRINO_OAUTH_TOKEN_URL", origTokenURL)
		_ = os.Setenv("TRINO_OAUTH_CLIENT_ID", origClientID)
		_ = os.Setenv("TRINO_OAUTH_CLIENT_SECRET", origClientSecret)
	}()

	_ = os.Setenv("OAUTH_ENABLED", "false")
	_ = os.Setenv("TRINO_AUTH_MODE", "basic")
	_ = os.Unsetenv("TRINO_OAUTH_TOKEN_URL")
	_ = os.Unsetenv("TRINO_OAUTH_CLIENT_ID")
	_ = os.Unsetenv("TRINO_OAUTH_CLIENT_SECRET")

	config, err := NewTrinoConfig()
	if err != nil {
		t.Fatalf("Basic mode should not require OAuth fields, got error: %v", err)
	}
	if config.TrinoAuthMode != "basic" {
		t.Errorf("TrinoAuthMode = %q, expected 'basic'", config.TrinoAuthMode)
	}
}

// setOrUnset sets an env var if value is non-empty, otherwise unsets it
func setOrUnset(key, value string) {
	if value == "" {
		_ = os.Unsetenv(key)
	} else {
		_ = os.Setenv(key, value)
	}
}

func TestOAuthProxyModeValidation(t *testing.T) {
	// Save original environment
	origMode := os.Getenv("OAUTH_MODE")
	origEnabled := os.Getenv("OAUTH_ENABLED")
	origProvider := os.Getenv("OAUTH_PROVIDER")
	origClientSecret := os.Getenv("OIDC_CLIENT_SECRET")
	origAllowedRedirects := os.Getenv("OAUTH_ALLOWED_REDIRECT_URIS")
	origIssuer := os.Getenv("OIDC_ISSUER")
	origAudience := os.Getenv("OIDC_AUDIENCE")

	// Clean up after test
	defer func() {
		_ = os.Setenv("OAUTH_MODE", origMode)
		_ = os.Setenv("OAUTH_ENABLED", origEnabled)
		_ = os.Setenv("OAUTH_PROVIDER", origProvider)
		_ = os.Setenv("OIDC_CLIENT_SECRET", origClientSecret)
		_ = os.Setenv("OAUTH_ALLOWED_REDIRECT_URIS", origAllowedRedirects)
		_ = os.Setenv("OIDC_ISSUER", origIssuer)
		_ = os.Setenv("OIDC_AUDIENCE", origAudience)
	}()

	// Set up proxy mode with OIDC provider
	_ = os.Setenv("OAUTH_MODE", "proxy")
	_ = os.Setenv("OAUTH_ENABLED", "true")
	_ = os.Setenv("OAUTH_PROVIDER", "okta")
	_ = os.Setenv("OIDC_ISSUER", "https://dev.okta.com")
	_ = os.Setenv("OIDC_AUDIENCE", "https://example.com")
	_ = os.Setenv("OIDC_CLIENT_SECRET", "")  // Missing client secret
	_ = os.Setenv("OAUTH_ALLOWED_REDIRECT_URIS", "") // Missing allowed redirects

	config, err := NewTrinoConfig()
	if err != nil {
		t.Errorf("Unexpected error: %v", err)
		return
	}

	if config.OAuthMode != "proxy" {
		t.Errorf("Expected proxy mode, got %s", config.OAuthMode)
	}

	if config.OAuthEnabled != true {
		t.Errorf("Expected OAuth enabled")
	}
}

func TestTrinoDeviceCodeValidation_MissingFields(t *testing.T) {
	origAuthMode := os.Getenv("TRINO_AUTH_MODE")
	origOAuthEnabled := os.Getenv("OAUTH_ENABLED")
	origTokenURL := os.Getenv("TRINO_OAUTH_TOKEN_URL")
	origClientID := os.Getenv("TRINO_OAUTH_CLIENT_ID")
	origClientSecret := os.Getenv("TRINO_OAUTH_CLIENT_SECRET")

	defer func() {
		_ = os.Setenv("TRINO_AUTH_MODE", origAuthMode)
		_ = os.Setenv("OAUTH_ENABLED", origOAuthEnabled)
		_ = os.Setenv("TRINO_OAUTH_TOKEN_URL", origTokenURL)
		_ = os.Setenv("TRINO_OAUTH_CLIENT_ID", origClientID)
		_ = os.Setenv("TRINO_OAUTH_CLIENT_SECRET", origClientSecret)
	}()

	_ = os.Setenv("OAUTH_ENABLED", "false")
	_ = os.Unsetenv("TRINO_OAUTH_CLIENT_SECRET")

	tests := []struct {
		name      string
		tokenURL  string
		clientID  string
		expectErr string
	}{
		{
			name:      "Missing token URL for device-code",
			tokenURL:  "",
			clientID:  "my-id",
			expectErr: "TRINO_OAUTH_TOKEN_URL is required when TRINO_AUTH_MODE=device-code",
		},
		{
			name:      "Missing client ID for device-code",
			tokenURL:  "https://login.microsoftonline.com/tenant/oauth2/v2.0/token",
			clientID:  "",
			expectErr: "TRINO_OAUTH_CLIENT_ID is required when TRINO_AUTH_MODE=device-code",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_ = os.Setenv("TRINO_AUTH_MODE", "device-code")
			setOrUnset("TRINO_OAUTH_TOKEN_URL", tt.tokenURL)
			setOrUnset("TRINO_OAUTH_CLIENT_ID", tt.clientID)

			_, err := NewTrinoConfig()
			if err == nil {
				t.Fatal("Expected error but got nil")
			}
			if !strings.Contains(err.Error(), tt.expectErr) {
				t.Errorf("Error = %q, expected to contain %q", err.Error(), tt.expectErr)
			}
		})
	}
}

func TestTrinoDeviceCodeNoSecretRequired(t *testing.T) {
	origAuthMode := os.Getenv("TRINO_AUTH_MODE")
	origOAuthEnabled := os.Getenv("OAUTH_ENABLED")
	origTokenURL := os.Getenv("TRINO_OAUTH_TOKEN_URL")
	origClientID := os.Getenv("TRINO_OAUTH_CLIENT_ID")
	origClientSecret := os.Getenv("TRINO_OAUTH_CLIENT_SECRET")

	defer func() {
		_ = os.Setenv("TRINO_AUTH_MODE", origAuthMode)
		_ = os.Setenv("OAUTH_ENABLED", origOAuthEnabled)
		_ = os.Setenv("TRINO_OAUTH_TOKEN_URL", origTokenURL)
		_ = os.Setenv("TRINO_OAUTH_CLIENT_ID", origClientID)
		_ = os.Setenv("TRINO_OAUTH_CLIENT_SECRET", origClientSecret)
	}()

	// device-code should NOT require client_secret (public clients)
	_ = os.Setenv("OAUTH_ENABLED", "false")
	_ = os.Setenv("TRINO_AUTH_MODE", "device-code")
	_ = os.Setenv("TRINO_OAUTH_TOKEN_URL", "https://login.microsoftonline.com/tenant/oauth2/v2.0/token")
	_ = os.Setenv("TRINO_OAUTH_CLIENT_ID", "my-client-id")
	_ = os.Unsetenv("TRINO_OAUTH_CLIENT_SECRET")

	config, err := NewTrinoConfig()
	if err != nil {
		t.Fatalf("device-code should not require client_secret, got: %v", err)
	}
	if config.TrinoAuthMode != "device-code" {
		t.Errorf("TrinoAuthMode = %q, want 'device-code'", config.TrinoAuthMode)
	}
}