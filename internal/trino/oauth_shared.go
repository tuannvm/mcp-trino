package trino

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	"golang.org/x/oauth2"
)

// oauthCacheHelper provides shared token cache and refresh logic
// for both device-code and auth-code flows
type oauthCacheHelper struct {
	clientID     string
	clientSecret string
	tokenURL     string
	scopes       []string
	cachePath    string
	httpClient   *http.Client
}

// doTokenRequestShared sends a POST to the token endpoint and parses the response
func (h *oauthCacheHelper) doTokenRequestShared(data url.Values) (*tokenResponse, error) {
	resp, err := h.httpClient.PostForm(h.tokenURL, data)
	if err != nil {
		return nil, fmt.Errorf("token request failed: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read token response: %w", err)
	}

	if resp.StatusCode >= 500 {
		return nil, fmt.Errorf("token endpoint returned HTTP %d: %s", resp.StatusCode, string(body))
	}

	var tokenResp tokenResponse
	if err := json.Unmarshal(body, &tokenResp); err != nil {
		return nil, fmt.Errorf("token endpoint returned non-JSON response (HTTP %d): %s", resp.StatusCode, string(body))
	}

	if tokenResp.Error != "" {
		return nil, &oauthError{Code: tokenResp.Error, Description: tokenResp.ErrorDesc}
	}

	return &tokenResp, nil
}

// refreshTokenShared uses a refresh token to get a new access token
func (h *oauthCacheHelper) refreshTokenShared(refreshTok string) (*oauth2.Token, error) {
	data := url.Values{
		"client_id":     {h.clientID},
		"grant_type":    {"refresh_token"},
		"refresh_token": {refreshTok},
		"scope":         {strings.Join(h.scopes, " ")},
	}
	if h.clientSecret != "" {
		data.Set("client_secret", h.clientSecret)
	}

	tokenResp, err := h.doTokenRequestShared(data)
	if err != nil {
		return nil, fmt.Errorf("refresh failed: %w", err)
	}

	if tokenResp.AccessToken == "" {
		return nil, fmt.Errorf("refresh failed: server returned empty access token")
	}

	rt := tokenResp.RefreshToken
	if rt == "" {
		rt = refreshTok
	}

	expiresIn := tokenResp.ExpiresIn
	if expiresIn < minTokenExpiry {
		expiresIn = minTokenExpiry
	}

	return &oauth2.Token{
		AccessToken:  tokenResp.AccessToken,
		RefreshToken: rt,
		TokenType:    tokenResp.TokenType,
		Expiry:       time.Now().Add(time.Duration(expiresIn) * time.Second),
	}, nil
}

// loadCacheShared loads the cached token from disk
func (h *oauthCacheHelper) loadCacheShared() *oauth2.Token {
	if h.cachePath == "" {
		return nil
	}

	data, err := os.ReadFile(h.cachePath)
	if err != nil {
		return nil
	}

	var cache tokenCache
	if err := json.Unmarshal(data, &cache); err != nil {
		return nil
	}

	return &oauth2.Token{
		AccessToken:  cache.AccessToken,
		RefreshToken: cache.RefreshToken,
		TokenType:    cache.TokenType,
		Expiry:       cache.ExpiresAt,
	}
}

// saveCacheShared saves the token to disk cache
func (h *oauthCacheHelper) saveCacheShared(token *oauth2.Token) {
	if h.cachePath == "" || token == nil {
		return
	}

	cache := tokenCache{
		AccessToken:  token.AccessToken,
		RefreshToken: token.RefreshToken,
		TokenType:    token.TokenType,
		ExpiresAt:    token.Expiry,
	}

	data, err := json.MarshalIndent(cache, "", "  ")
	if err != nil {
		log.Printf("WARNING: Failed to marshal token cache: %v", err)
		return
	}

	dir := filepath.Dir(h.cachePath)
	if err := os.MkdirAll(dir, 0700); err != nil {
		log.Printf("WARNING: Failed to create token cache directory: %v", err)
		return
	}

	if err := os.WriteFile(h.cachePath, data, 0600); err != nil {
		log.Printf("WARNING: Failed to write token cache: %v", err)
		return
	}

	if err := os.Chmod(h.cachePath, 0600); err != nil {
		log.Printf("WARNING: Failed to set token cache permissions: %v", err)
	}
}
