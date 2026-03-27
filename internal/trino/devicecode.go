package trino

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/tuannvm/mcp-trino/internal/config"
	"golang.org/x/oauth2"
)

// deviceCodeResponse represents the Azure AD device code response
type deviceCodeResponse struct {
	DeviceCode      string `json:"device_code"`
	UserCode        string `json:"user_code"`
	VerificationURI string `json:"verification_uri"`
	ExpiresIn       int    `json:"expires_in"`
	Interval        int    `json:"interval"`
	Message         string `json:"message"`
}

// tokenCache represents cached OAuth tokens on disk
type tokenCache struct {
	AccessToken  string    `json:"access_token"`
	RefreshToken string    `json:"refresh_token"`
	TokenType    string    `json:"token_type"`
	ExpiresAt    time.Time `json:"expires_at"`
}

// oauthError is a structured error from the OAuth token endpoint
type oauthError struct {
	Code        string
	Description string
}

func (e *oauthError) Error() string {
	return e.Code + ": " + e.Description
}

// tokenResponse is the shared JSON structure for token endpoint responses
type tokenResponse struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
	TokenType    string `json:"token_type"`
	ExpiresIn    int    `json:"expires_in"`
	Error        string `json:"error"`
	ErrorDesc    string `json:"error_description"`
}

// minTokenExpiry is the minimum token lifetime to prevent refresh storms
const minTokenExpiry = 60 // seconds

// deviceCodeTokenSource implements oauth2TokenSource using the device code flow
type deviceCodeTokenSource struct {
	oauthCacheHelper

	mu    sync.Mutex
	token *oauth2.Token

	// For testing: override the device code URL
	deviceCodeURL string
}

// createDeviceCodeTokenSource creates a device code token source from config
func createDeviceCodeTokenSource(cfg *config.TrinoConfig) oauth2TokenSource {
	if cfg.TrinoAuthMode != "device-code" {
		return nil
	}

	scopes := parseScopes(cfg.TrinoOAuthScopes)

	// Derive device code URL from token URL (replace /token with /devicecode)
	deviceCodeURL := deriveDeviceCodeURL(cfg.TrinoOAuthTokenURL)

	// Cache path scoped to this specific profile (tokenURL + clientID hash)
	cachePath := scopedCachePath(cfg.TrinoOAuthTokenURL, cfg.TrinoOAuthClientID)

	// Migrate legacy unscoped cache if scoped cache doesn't exist yet
	migrateLegacyCache(cachePath)

	ts := &deviceCodeTokenSource{
		oauthCacheHelper: oauthCacheHelper{
			clientID:     cfg.TrinoOAuthClientID,
			clientSecret: cfg.TrinoOAuthClientSecret,
			tokenURL:     cfg.TrinoOAuthTokenURL,
			scopes:       scopes,
			cachePath:    cachePath,
			httpClient:   &http.Client{Timeout: 30 * time.Second},
		},
		deviceCodeURL: deviceCodeURL,
	}
	return ts
}

// parseScopes splits a comma-separated scope string into trimmed tokens
func parseScopes(csv string) []string {
	if csv == "" {
		return nil
	}
	parts := strings.Split(csv, ",")
	scopes := make([]string, 0, len(parts))
	for _, s := range parts {
		if trimmed := strings.TrimSpace(s); trimmed != "" {
			scopes = append(scopes, trimmed)
		}
	}
	return scopes
}

// deriveDeviceCodeURL derives the device code endpoint from the token URL
func deriveDeviceCodeURL(tokenURL string) string {
	// Azure AD: replace /oauth2/v2.0/token with /oauth2/v2.0/devicecode
	if strings.Contains(tokenURL, "/oauth2/v2.0/token") {
		return strings.Replace(tokenURL, "/oauth2/v2.0/token", "/oauth2/v2.0/devicecode", 1)
	}
	// Generic: replace last /token with /devicecode
	if strings.HasSuffix(tokenURL, "/token") {
		return strings.TrimSuffix(tokenURL, "/token") + "/devicecode"
	}
	return tokenURL + "/devicecode"
}

// scopedCachePath returns a token cache path scoped to a specific tokenURL+clientID
func scopedCachePath(tokenURL, clientID string) string {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		log.Printf("WARNING: Could not determine home directory; token caching disabled")
		return ""
	}
	hash := sha256.Sum256([]byte(tokenURL + "|" + clientID))
	filename := fmt.Sprintf("token-cache-%s.json", hex.EncodeToString(hash[:8]))
	return filepath.Join(homeDir, ".config", "trino", filename)
}

// defaultCachePath returns the default (unscoped) token cache path — used only in tests and migration
func defaultCachePath() string {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return filepath.Join(homeDir, ".config", "trino", "token-cache.json")
}

// migrateLegacyCache moves the old unscoped token-cache.json to the new scoped path
func migrateLegacyCache(scopedPath string) {
	if scopedPath == "" {
		return
	}
	// Only migrate if scoped cache doesn't exist yet
	if _, err := os.Stat(scopedPath); err == nil {
		return
	}
	legacyPath := defaultCachePath()
	if legacyPath == "" {
		return
	}
	if _, err := os.Stat(legacyPath); err != nil {
		return
	}
	// Copy legacy to scoped path (don't delete legacy in case other profiles use it)
	data, err := os.ReadFile(legacyPath)
	if err != nil {
		return
	}
	dir := filepath.Dir(scopedPath)
	_ = os.MkdirAll(dir, 0700)
	if err := os.WriteFile(scopedPath, data, 0600); err == nil {
		log.Printf("Migrated legacy token cache to %s", filepath.Base(scopedPath))
	}
}

// Token implements oauth2TokenSource
func (d *deviceCodeTokenSource) Token() (*oauth2Token, error) {
	d.mu.Lock()
	defer d.mu.Unlock()

	// If we have a valid in-memory token, return it
	if d.token != nil && d.token.Valid() {
		return &oauth2Token{AccessToken: d.token.AccessToken}, nil
	}

	// Try to load from disk cache and refresh
	if cached := d.loadCacheShared(); cached != nil {
		if cached.Valid() {
			d.token = cached
			return &oauth2Token{AccessToken: cached.AccessToken}, nil
		}
		// Try to refresh using the refresh token
		if cached.RefreshToken != "" {
			refreshed, err := d.refreshTokenShared(cached.RefreshToken)
			if err == nil {
				d.token = refreshed
				d.saveCacheShared(refreshed)
				return &oauth2Token{AccessToken: refreshed.AccessToken}, nil
			}
			log.Printf("Token refresh failed, re-authenticating: %v", err)
		}
	}

	// Initiate device code flow
	token, err := d.doDeviceCodeFlow()
	if err != nil {
		return nil, fmt.Errorf("device code authentication failed: %w", err)
	}
	d.token = token
	d.saveCacheShared(token)
	return &oauth2Token{AccessToken: token.AccessToken}, nil
}

// doDeviceCodeFlow initiates the OAuth 2.0 Device Authorization Grant
func (d *deviceCodeTokenSource) doDeviceCodeFlow() (*oauth2.Token, error) {
	// Step 1: Request device code
	data := url.Values{
		"client_id": {d.clientID},
		"scope":     {strings.Join(d.scopes, " ")},
	}

	resp, err := d.httpClient.PostForm(d.deviceCodeURL, data)
	if err != nil {
		return nil, fmt.Errorf("failed to request device code: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read device code response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("device code request failed (%d): %s", resp.StatusCode, string(body))
	}

	var dcResp deviceCodeResponse
	if err := json.Unmarshal(body, &dcResp); err != nil {
		return nil, fmt.Errorf("failed to parse device code response: %w", err)
	}

	if dcResp.ExpiresIn <= 0 {
		return nil, fmt.Errorf("device code response has invalid expires_in: %d", dcResp.ExpiresIn)
	}

	// Step 2: Display instructions to user
	if dcResp.Message != "" {
		fmt.Fprintf(os.Stderr, "\n%s\n\n", dcResp.Message)
	} else {
		fmt.Fprintf(os.Stderr, "\nTo sign in, open %s in a browser and enter code: %s\n\n",
			dcResp.VerificationURI, dcResp.UserCode)
	}

	// Step 3: Poll for token
	interval := time.Duration(dcResp.Interval) * time.Second
	if interval < 1*time.Second {
		interval = 5 * time.Second
	}
	deadline := time.Now().Add(time.Duration(dcResp.ExpiresIn) * time.Second)

	for time.Now().Before(deadline) {
		time.Sleep(interval)

		token, err := d.pollForToken(dcResp.DeviceCode)
		if err != nil {
			var oauthErr *oauthError
			if errors.As(err, &oauthErr) {
				if oauthErr.Code == "authorization_pending" {
					continue
				}
				if oauthErr.Code == "slow_down" {
					interval += 5 * time.Second
					continue
				}
			}
			return nil, err
		}
		fmt.Fprintf(os.Stderr, "Authentication successful!\n\n")
		return token, nil
	}

	return nil, fmt.Errorf("device code flow timed out after %d seconds", dcResp.ExpiresIn)
}

// pollForToken polls the token endpoint during device code flow
func (d *deviceCodeTokenSource) pollForToken(deviceCode string) (*oauth2.Token, error) {
	data := url.Values{
		"client_id":   {d.clientID},
		"device_code": {deviceCode},
		"grant_type":  {"urn:ietf:params:oauth:grant-type:device_code"},
	}
	if d.clientSecret != "" {
		data.Set("client_secret", d.clientSecret)
	}

	tokenResp, err := d.doTokenRequestShared(data)
	if err != nil {
		return nil, err
	}

	if tokenResp.AccessToken == "" {
		return nil, fmt.Errorf("empty access token in response")
	}

	expiresIn := tokenResp.ExpiresIn
	if expiresIn < minTokenExpiry {
		expiresIn = minTokenExpiry
	}

	return &oauth2.Token{
		AccessToken:  tokenResp.AccessToken,
		RefreshToken: tokenResp.RefreshToken,
		TokenType:    tokenResp.TokenType,
		Expiry:       time.Now().Add(time.Duration(expiresIn) * time.Second),
	}, nil
}
