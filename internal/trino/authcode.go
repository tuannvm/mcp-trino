package trino

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"fmt"
	"html"
	"log"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"sync"
	"time"

	"github.com/tuannvm/mcp-trino/internal/config"
	"golang.org/x/oauth2"
)

// authCodeTokenSource implements oauth2TokenSource using Authorization Code + PKCE flow
type authCodeTokenSource struct {
	oauthCacheHelper
	authorizeURL string

	mu    sync.Mutex
	token *oauth2.Token

	// For testing: override browser open and callback server
	openBrowser   func(url string) error
	callbackPort  int // 0 = auto-assign
	serverTimeout time.Duration
}

// createAuthCodeTokenSource creates an auth code + PKCE token source from config
func createAuthCodeTokenSource(cfg *config.TrinoConfig) oauth2TokenSource {
	if cfg.TrinoAuthMode != "auth-code" {
		return nil
	}

	scopes := parseScopes(cfg.TrinoOAuthScopes)
	authorizeURL := deriveAuthorizeURL(cfg.TrinoOAuthTokenURL)
	cachePath := scopedCachePath(cfg.TrinoOAuthTokenURL, cfg.TrinoOAuthClientID)

	migrateLegacyCache(cachePath)

	ts := &authCodeTokenSource{
		oauthCacheHelper: oauthCacheHelper{
			clientID:     cfg.TrinoOAuthClientID,
			clientSecret: cfg.TrinoOAuthClientSecret,
			tokenURL:     cfg.TrinoOAuthTokenURL,
			scopes:       scopes,
			cachePath:    cachePath,
			httpClient:   &http.Client{Timeout: 30 * time.Second},
		},
		authorizeURL:  authorizeURL,
		openBrowser:   openBrowserDefault,
		serverTimeout: 120 * time.Second,
	}
	return ts
}

// deriveAuthorizeURL derives the authorization endpoint from the token URL
func deriveAuthorizeURL(tokenURL string) string {
	// Azure AD: replace /oauth2/v2.0/token with /oauth2/v2.0/authorize
	if strings.Contains(tokenURL, "/oauth2/v2.0/token") {
		return strings.Replace(tokenURL, "/oauth2/v2.0/token", "/oauth2/v2.0/authorize", 1)
	}
	// Generic: replace /token with /authorize
	if strings.HasSuffix(tokenURL, "/token") {
		return strings.TrimSuffix(tokenURL, "/token") + "/authorize"
	}
	return tokenURL + "/authorize"
}

// Token implements oauth2TokenSource
func (a *authCodeTokenSource) Token() (*oauth2Token, error) {
	a.mu.Lock()
	defer a.mu.Unlock()

	// Return valid in-memory token
	if a.token != nil && a.token.Valid() {
		return &oauth2Token{AccessToken: a.token.AccessToken}, nil
	}

	// Try disk cache
	if cached := a.loadCacheShared(); cached != nil {
		if cached.Valid() {
			a.token = cached
			return &oauth2Token{AccessToken: cached.AccessToken}, nil
		}
		// Refresh if possible
		if cached.RefreshToken != "" {
			refreshed, err := a.refreshTokenShared(cached.RefreshToken)
			if err == nil {
				a.token = refreshed
				a.saveCacheShared(refreshed)
				return &oauth2Token{AccessToken: refreshed.AccessToken}, nil
			}
			log.Printf("Token refresh failed, re-authenticating: %v", err)
		}
	}

	// Do auth code + PKCE flow
	token, err := a.doAuthCodeFlow()
	if err != nil {
		return nil, fmt.Errorf("authorization code authentication failed: %w", err)
	}
	a.token = token
	a.saveCacheShared(token)
	return &oauth2Token{AccessToken: token.AccessToken}, nil
}

// generatePKCE creates a code verifier and its S256 challenge
func generatePKCE() (verifier string, challenge string, err error) {
	// 32 bytes = 43 base64url chars (RFC 7636 recommends 43-128)
	buf := make([]byte, 32)
	if _, err := rand.Read(buf); err != nil {
		return "", "", fmt.Errorf("failed to generate PKCE verifier: %w", err)
	}
	verifier = base64.RawURLEncoding.EncodeToString(buf)

	hash := sha256.Sum256([]byte(verifier))
	challenge = base64.RawURLEncoding.EncodeToString(hash[:])

	return verifier, challenge, nil
}

// generateState creates a random state parameter for CSRF protection
func generateState() (string, error) {
	buf := make([]byte, 16)
	if _, err := rand.Read(buf); err != nil {
		return "", fmt.Errorf("failed to generate state: %w", err)
	}
	return base64.RawURLEncoding.EncodeToString(buf), nil
}

// doAuthCodeFlow performs the full Authorization Code + PKCE flow
func (a *authCodeTokenSource) doAuthCodeFlow() (*oauth2.Token, error) {
	verifier, challenge, err := generatePKCE()
	if err != nil {
		return nil, err
	}

	state, err := generateState()
	if err != nil {
		return nil, err
	}

	// Start local callback server
	codeChan := make(chan string, 1)
	errChan := make(chan error, 1)

	listener, err := net.Listen("tcp", fmt.Sprintf("127.0.0.1:%d", a.callbackPort))
	if err != nil {
		return nil, fmt.Errorf("failed to start callback server: %w", err)
	}
	port := listener.Addr().(*net.TCPAddr).Port
	redirectURI := fmt.Sprintf("http://localhost:%d/callback", port)

	mux := http.NewServeMux()
	mux.HandleFunc("/callback", func(w http.ResponseWriter, r *http.Request) {
		// Validate state
		if r.URL.Query().Get("state") != state {
			errChan <- fmt.Errorf("state mismatch: possible CSRF attack")
			http.Error(w, "State mismatch", http.StatusBadRequest)
			return
		}

		if errMsg := r.URL.Query().Get("error"); errMsg != "" {
			desc := r.URL.Query().Get("error_description")
			errChan <- fmt.Errorf("authorization error: %s: %s", errMsg, desc)
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			fmt.Fprintf(w, "<html><body><h2>Authentication failed</h2><p>%s: %s</p><p>You can close this tab.</p></body></html>",
				html.EscapeString(errMsg), html.EscapeString(desc))
			return
		}

		code := r.URL.Query().Get("code")
		if code == "" {
			errChan <- fmt.Errorf("no authorization code in callback")
			http.Error(w, "No code", http.StatusBadRequest)
			return
		}

		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		fmt.Fprintf(w, "<html><body><h2>Authentication successful!</h2><p>You can close this tab and return to the terminal.</p></body></html>")
		codeChan <- code
	})

	srv := &http.Server{Handler: mux}
	go func() {
		if err := srv.Serve(listener); err != nil && err != http.ErrServerClosed {
			errChan <- fmt.Errorf("callback server error: %w", err)
		}
	}()
	defer func() {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		srv.Shutdown(ctx)
	}()

	// Build authorization URL
	params := url.Values{
		"client_id":             {a.clientID},
		"response_type":         {"code"},
		"redirect_uri":          {redirectURI},
		"scope":                 {strings.Join(a.scopes, " ")},
		"state":                 {state},
		"code_challenge":        {challenge},
		"code_challenge_method": {"S256"},
	}
	authURL := a.authorizeURL + "?" + params.Encode()

	// Open browser
	fmt.Fprintf(os.Stderr, "\nOpening browser for authentication...\n")
	fmt.Fprintf(os.Stderr, "If the browser doesn't open, visit:\n%s\n\n", authURL)

	if err := a.openBrowser(authURL); err != nil {
		log.Printf("WARNING: Could not open browser: %v", err)
	}

	// Wait for callback
	select {
	case code := <-codeChan:
		fmt.Fprintf(os.Stderr, "Authorization code received, exchanging for token...\n")
		return a.exchangeCode(code, verifier, redirectURI)
	case err := <-errChan:
		return nil, err
	case <-time.After(a.serverTimeout):
		return nil, fmt.Errorf("authentication timed out after %v", a.serverTimeout)
	}
}

// exchangeCode exchanges an authorization code for tokens
func (a *authCodeTokenSource) exchangeCode(code, verifier, redirectURI string) (*oauth2.Token, error) {
	data := url.Values{
		"client_id":     {a.clientID},
		"grant_type":    {"authorization_code"},
		"code":          {code},
		"redirect_uri":  {redirectURI},
		"code_verifier": {verifier},
	}
	if a.clientSecret != "" {
		data.Set("client_secret", a.clientSecret)
	}

	tokenResp, err := a.doTokenRequestShared(data)
	if err != nil {
		return nil, fmt.Errorf("token exchange failed: %w", err)
	}

	if tokenResp.AccessToken == "" {
		return nil, fmt.Errorf("empty access token in exchange response")
	}

	expiresIn := tokenResp.ExpiresIn
	if expiresIn < minTokenExpiry {
		expiresIn = minTokenExpiry
	}

	fmt.Fprintf(os.Stderr, "Authentication successful!\n\n")

	return &oauth2.Token{
		AccessToken:  tokenResp.AccessToken,
		RefreshToken: tokenResp.RefreshToken,
		TokenType:    tokenResp.TokenType,
		Expiry:       time.Now().Add(time.Duration(expiresIn) * time.Second),
	}, nil
}

// openBrowserDefault opens the URL in the user's default browser
func openBrowserDefault(url string) error {
	switch runtime.GOOS {
	case "windows":
		return exec.Command("rundll32", "url.dll,FileProtocolHandler", url).Start()
	case "darwin":
		return exec.Command("open", url).Start()
	default:
		return exec.Command("xdg-open", url).Start()
	}
}
