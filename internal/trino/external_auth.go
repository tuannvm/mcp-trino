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
	"sync"
	"time"

	"github.com/tuannvm/mcp-trino/internal/config"
)

var (
	redirectServerPattern = regexp.MustCompile(`x_redirect_server="([^"]+)"`)
	tokenServerPattern    = regexp.MustCompile(`x_token_server="([^"]+)"`)
)

type bearerTokenManager interface {
	CurrentToken() string
	AcquireToken(challenge bearerAuthChallenge) (string, error)
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

	mu    sync.Mutex
	token string
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
	if cfg.AuthMode != config.AuthModeExternal {
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

	if m.token != "" {
		return m.token
	}

	cache := m.loadCache()
	if cache == nil || cache.AccessToken == "" {
		return ""
	}

	m.token = cache.AccessToken
	return m.token
}

func (m *externalAuthTokenManager) AcquireToken(challenge bearerAuthChallenge) (string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.token != "" {
		return m.token, nil
	}

	if cache := m.loadCache(); cache != nil && cache.AccessToken != "" {
		m.token = cache.AccessToken
		return m.token, nil
	}

	if challenge.RedirectURL == "" || challenge.TokenURL == "" {
		return "", fmt.Errorf("missing Trino external auth challenge URLs")
	}

	fmt.Fprintln(os.Stderr, "\nOpening browser for Trino authentication...")
	fmt.Fprintln(os.Stderr, challenge.RedirectURL)
	fmt.Fprintln(os.Stderr)
	if err := m.openBrowser(challenge.RedirectURL); err != nil {
		log.Printf("WARNING: Could not open browser: %v", err)
	}

	token, err := m.waitForToken(challenge.TokenURL)
	if err != nil {
		return "", err
	}

	m.token = token
	m.saveCache(&externalTokenCache{AccessToken: token})
	return token, nil
}

func (m *externalAuthTokenManager) InvalidateToken() {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.token = ""
	if m.cachePath != "" {
		_ = os.Remove(m.cachePath)
	}
}

func parseBearerAuthChallenge(headers http.Header) (bearerAuthChallenge, error) {
	values := headers.Values("WWW-Authenticate")
	if len(values) == 0 {
		return bearerAuthChallenge{}, fmt.Errorf("trino returned no WWW-Authenticate header")
	}

	joined := ""
	for i, value := range values {
		if i > 0 {
			joined += ", "
		}
		joined += value
	}

	redirectMatch := redirectServerPattern.FindStringSubmatch(joined)
	tokenMatch := tokenServerPattern.FindStringSubmatch(joined)
	if len(redirectMatch) < 2 || len(tokenMatch) < 2 {
		return bearerAuthChallenge{}, fmt.Errorf("no Trino external auth challenge found in WWW-Authenticate header")
	}

	return bearerAuthChallenge{
		RedirectURL: redirectMatch[1],
		TokenURL:    tokenMatch[1],
	}, nil
}

func (m *externalAuthTokenManager) waitForToken(initialTokenURL string) (string, error) {
	deadline := time.Now().Add(m.pollTimeout)
	tokenURL := initialTokenURL

	for time.Now().Before(deadline) {
		req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, tokenURL, nil)
		if err != nil {
			return "", fmt.Errorf("failed to create token polling request: %w", err)
		}
		req.Header.Set("User-Agent", "mcp-trino-external-auth")

		resp, err := m.httpClient.Do(req)
		if err != nil {
			return "", fmt.Errorf("failed to poll Trino token endpoint: %w", err)
		}

		var poll externalTokenPollResponse
		decodeErr := json.NewDecoder(resp.Body).Decode(&poll)
		_ = resp.Body.Close()
		if decodeErr != nil {
			return "", fmt.Errorf("failed to decode Trino token response: %w", decodeErr)
		}
		if resp.StatusCode >= 400 {
			if poll.Error != "" {
				return "", fmt.Errorf("trino token polling failed: %s", poll.Error)
			}
			return "", fmt.Errorf("trino token polling failed with status %d", resp.StatusCode)
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

		time.Sleep(m.pollInterval)
	}

	return "", fmt.Errorf("timed out waiting for Trino access token after %v", m.pollTimeout)
}

func (m *externalAuthTokenManager) cleanupTokenURL(tokenURL string) {
	req, err := http.NewRequestWithContext(context.Background(), http.MethodDelete, tokenURL, nil)
	if err != nil {
		return
	}
	req.Header.Set("User-Agent", "mcp-trino-external-auth")
	resp, err := m.httpClient.Do(req)
	if err == nil && resp != nil {
		_ = resp.Body.Close()
	}
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
