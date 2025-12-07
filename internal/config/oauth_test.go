package config

import (
	"os"
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