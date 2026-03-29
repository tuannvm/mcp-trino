package trino

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"sync"
	"time"

	"github.com/tuannvm/mcp-trino/internal/config"
)

const trinoAuthUserAgent = "mcp-trino-external-auth"

var (
	redirectServerPattern = regexp.MustCompile(`x_redirect_server="([^"]+)"`)
	tokenServerPattern    = regexp.MustCompile(`x_token_server="([^"]+)"`)
)

type bearerTokenManager interface {
	CurrentToken() string
	// AcquireToken returns a valid token. rejectedToken is the token that was just
	// rejected; if the cache already holds a different token (another goroutine
	// refreshed), it is returned immediately without opening a browser.
	AcquireToken(ctx context.Context, challenge bearerAuthChallenge, rejectedToken string) (string, error)
	InvalidateToken()
}

type bearerAuthChallenge struct {
	RedirectURL string
	TokenURL    string
}

type externalAuthTokenManager struct {
	cachePath    string
	httpClient   *http.Client
	openBrowser  func(string) error
	pollInterval time.Duration
	pollTimeout  time.Duration

	mu         sync.Mutex
	token      string
	generation uint64 // incremented on each InvalidateToken to fence stale writes

	inFlight *tokenAcquisition
}

type tokenAcquisition struct {
	done  chan struct{}
	token string
	err   error
}

type externalTokenCache struct {
	AccessToken string `json:"access_token"`
}

type externalTokenPollResponse struct {
	Token   string `json:"token"`
	Error   string `json:"error"`
	NextURI string `json:"nextUri"`
}

func createBearerTokenManager(cfg *config.TrinoConfig) bearerTokenManager {
	if cfg.AuthMode != config.AuthModeExternalAuth {
		return nil
	}

	return &externalAuthTokenManager{
		cachePath:    externalTokenCachePath(cfg),
		httpClient:   &http.Client{Timeout: 30 * time.Second},
		openBrowser:  openBrowserDefault,
		pollInterval: 2 * time.Second,
		pollTimeout:  2 * time.Minute,
	}
}

func externalTokenCachePath(cfg *config.TrinoConfig) string {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		log.Printf("WARNING: Could not determine home directory; external auth token caching disabled")
		return ""
	}

	hash := sha256.Sum256([]byte(fmt.Sprintf("%s|%s|%d|%s|external", cfg.Scheme, cfg.Host, cfg.Port, cfg.User)))
	filename := fmt.Sprintf("external-token-cache-%s.json", hex.EncodeToString(hash[:8]))
	return filepath.Join(homeDir, ".config", "trino", filename)
}

func (m *externalAuthTokenManager) CurrentToken() string {
	m.mu.Lock()
	defer m.mu.Unlock()

	return m.cachedTokenLocked()
}

// cachedTokenLocked returns the in-memory or disk-cached token.
// Must be called with m.mu held.
func (m *externalAuthTokenManager) cachedTokenLocked() string {
	if m.token != "" {
		return m.token
	}

	if cache := m.loadCache(); cache != nil {
		m.token = cache.AccessToken
	}
	return m.token
}

func (m *externalAuthTokenManager) AcquireToken(ctx context.Context, challenge bearerAuthChallenge, rejectedToken string) (string, error) {
	m.mu.Lock()

	// Return the cached token if it already differs from the one that was rejected.
	// This avoids a redundant browser popup when another goroutine refreshed first.
	if token := m.cachedTokenLocked(); token != "" && token != rejectedToken {
		m.mu.Unlock()
		return token, nil
	}

	if m.inFlight != nil {
		inFlight := m.inFlight
		m.mu.Unlock()
		select {
		case <-inFlight.done:
		case <-ctx.Done():
			return "", ctx.Err()
		}
		return inFlight.token, inFlight.err
	}

	if challenge.TokenURL == "" {
		m.mu.Unlock()
		return "", fmt.Errorf("missing Trino external auth token URL")
	}

	gen := m.generation
	inFlight := &tokenAcquisition{done: make(chan struct{})}
	m.inFlight = inFlight
	m.mu.Unlock()

	token, err := m.acquireFreshToken(ctx, challenge)

	m.mu.Lock()
	// Only persist if no InvalidateToken call raced with our acquisition.
	if err == nil && m.generation == gen {
		m.token = token
		m.saveCache(&externalTokenCache{AccessToken: token})
	}
	inFlight.token = token
	inFlight.err = err
	close(inFlight.done)
	if m.inFlight == inFlight {
		m.inFlight = nil
	}
	m.mu.Unlock()

	return token, err
}

func (m *externalAuthTokenManager) InvalidateToken() {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.token = ""
	m.generation++
	if m.cachePath != "" {
		if err := os.Remove(m.cachePath); err != nil && !os.IsNotExist(err) {
			log.Printf("WARNING: failed to remove external auth token cache: %v", err)
		}
	}
}

func parseBearerAuthChallenge(headers http.Header) (bearerAuthChallenge, error) {
	values := headers.Values("WWW-Authenticate")
	if len(values) == 0 {
		return bearerAuthChallenge{}, fmt.Errorf("trino returned no WWW-Authenticate header")
	}

	joined := strings.Join(values, ", ")

	redirectMatch := redirectServerPattern.FindStringSubmatch(joined)
	tokenMatch := tokenServerPattern.FindStringSubmatch(joined)
	if len(tokenMatch) < 2 {
		return bearerAuthChallenge{}, fmt.Errorf("no Trino external auth challenge found in WWW-Authenticate header")
	}

	challenge := bearerAuthChallenge{
		TokenURL: tokenMatch[1],
	}
	if len(redirectMatch) >= 2 {
		challenge.RedirectURL = redirectMatch[1]
	}
	return challenge, nil
}

func (m *externalAuthTokenManager) waitForToken(ctx context.Context, initialTokenURL string) (string, error) {
	deadline := time.Now().Add(m.pollTimeout)
	tokenURL := initialTokenURL

	for time.Now().Before(deadline) {
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, tokenURL, nil)
		if err != nil {
			return "", fmt.Errorf("failed to create token polling request: %w", err)
		}
		req.Header.Set("User-Agent", trinoAuthUserAgent)

		resp, err := m.httpClient.Do(req)
		if err != nil {
			return "", fmt.Errorf("failed to poll Trino token endpoint: %w", err)
		}

		var poll externalTokenPollResponse
		decodeErr := json.NewDecoder(resp.Body).Decode(&poll)
		_ = resp.Body.Close()

		if resp.StatusCode >= 400 {
			if decodeErr == nil && poll.Error != "" {
				return "", fmt.Errorf("trino token polling failed: %s", poll.Error)
			}
			return "", fmt.Errorf("trino token polling failed with status %d", resp.StatusCode)
		}
		if decodeErr != nil {
			return "", fmt.Errorf("failed to decode Trino token response: %w", decodeErr)
		}

		if poll.Token != "" {
			m.cleanupTokenURL(tokenURL)
			return poll.Token, nil
		}
		if poll.Error != "" {
			return "", fmt.Errorf("trino token polling failed: %s", poll.Error)
		}
		if poll.NextURI != "" {
			tokenURL = poll.NextURI
		}

		select {
		case <-time.After(m.pollInterval):
		case <-ctx.Done():
			return "", ctx.Err()
		}
	}

	return "", fmt.Errorf("timed out waiting for Trino access token after %v", m.pollTimeout)
}

func (m *externalAuthTokenManager) cleanupTokenURL(tokenURL string) {
	req, err := http.NewRequestWithContext(context.Background(), http.MethodDelete, tokenURL, nil)
	if err != nil {
		return
	}
	req.Header.Set("User-Agent", trinoAuthUserAgent)
	resp, err := m.httpClient.Do(req)
	if err == nil && resp != nil {
		_ = resp.Body.Close()
	}
}

func (m *externalAuthTokenManager) acquireFreshToken(ctx context.Context, challenge bearerAuthChallenge) (string, error) {
	if challenge.RedirectURL != "" {
		fmt.Fprintln(os.Stderr, "\nOpening browser for Trino authentication...")
		fmt.Fprintln(os.Stderr, challenge.RedirectURL)
		fmt.Fprintln(os.Stderr)
		if err := m.openBrowser(challenge.RedirectURL); err != nil {
			log.Printf("WARNING: Could not open browser: %v", err)
		}
	}

	return m.waitForToken(ctx, challenge.TokenURL)
}

func (m *externalAuthTokenManager) loadCache() *externalTokenCache {
	if m.cachePath == "" {
		return nil
	}

	data, err := os.ReadFile(m.cachePath)
	if err != nil {
		return nil
	}

	var cache externalTokenCache
	if err := json.Unmarshal(data, &cache); err != nil {
		return nil
	}
	if cache.AccessToken == "" {
		return nil
	}
	return &cache
}

func (m *externalAuthTokenManager) saveCache(cache *externalTokenCache) {
	if m.cachePath == "" || cache == nil || cache.AccessToken == "" {
		return
	}

	data, err := json.MarshalIndent(cache, "", "  ")
	if err != nil {
		log.Printf("WARNING: Failed to marshal external auth token cache: %v", err)
		return
	}

	if err := os.MkdirAll(filepath.Dir(m.cachePath), 0700); err != nil {
		log.Printf("WARNING: Failed to create external auth token cache directory: %v", err)
		return
	}
	if err := os.WriteFile(m.cachePath, data, 0600); err != nil {
		log.Printf("WARNING: Failed to write external auth token cache: %v", err)
	}
}

func openBrowserDefault(targetURL string) error {
	switch runtime.GOOS {
	case "windows":
		return exec.Command("rundll32", "url.dll,FileProtocolHandler", targetURL).Start()
	case "darwin":
		return exec.Command("open", targetURL).Start()
	default:
		return exec.Command("xdg-open", targetURL).Start()
	}
}
