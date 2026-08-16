package trino

// Service-to-service authentication to TRINO with an OAuth2 access token.
//
// Upstream mcp-trino authenticates to Trino with basic auth only: NewClient
// builds the DSN with url.UserPassword and never sets the Go driver's
// `accessToken` parameter. That forces a Trino coordinator to keep
// PASSWORD authentication enabled purely for this one client, even when every
// human and every other service authenticates with a Keycloak JWT.
//
// This adds the missing leg: fetch a token with the client_credentials grant and
// present it as `Authorization: Bearer` on every request to Trino.
//
// Why the header and not the DSN's accessToken parameter: the DSN is fixed when
// the pool is opened, so a token baked in there expires and every later query
// fails. Setting the header per request means the token is refreshed
// transparently for the life of the process.

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"
)

// tokenSource caches a client_credentials token and refreshes it before expiry.
type tokenSource struct {
	tokenURL string
	clientID string
	secret   string
	scope    string

	mu      sync.Mutex
	token   string
	expires time.Time
}

// newTokenSourceFromEnv returns (nil, nil) when Trino token auth is not
// configured at all, so the client falls back to the upstream basic-auth
// behaviour unchanged.
//
//	TRINO_OAUTH2_TOKEN_URL   token endpoint of the identity provider
//	TRINO_OAUTH2_CREDENTIAL  "<client-id>:<client-secret>" (one string, matching
//	                         the form Iceberg REST clients use)
//	TRINO_OAUTH2_SCOPE       optional, defaults to "openid"
//	TRINO_OAUTH2_ALLOW_INSECURE_TOKEN_URL
//	                         optional, permits a plaintext http:// token URL
//
// It FAILS CLOSED on a partial or malformed configuration rather than returning
// nil. Returning nil would silently fall back to basic auth while the DSN still
// carries the basic credentials, so a coordinator that permits password
// authentication would accept the connection — an unintended downgrade that
// looks like a working deployment. A half-configured client is a mistake, and a
// mistake here should stop the process, not quietly authenticate a different
// way.
func newTokenSourceFromEnv() (*tokenSource, error) {
	tokenURL := strings.TrimSpace(os.Getenv("TRINO_OAUTH2_TOKEN_URL"))
	credential := strings.TrimSpace(os.Getenv("TRINO_OAUTH2_CREDENTIAL"))

	// Neither set: token auth is simply not in use.
	if tokenURL == "" && credential == "" {
		return nil, nil
	}
	if tokenURL == "" || credential == "" {
		return nil, fmt.Errorf(
			"TRINO_OAUTH2_TOKEN_URL and TRINO_OAUTH2_CREDENTIAL must be set together")
	}

	endpoint, err := url.ParseRequestURI(tokenURL)
	if err != nil {
		return nil, fmt.Errorf("TRINO_OAUTH2_TOKEN_URL is not a valid URL: %w", err)
	}
	// Token requests carry the client secret in the request body, so plaintext
	// http:// leaks it to anything on the path. Loopback is exempt because it
	// cannot leave the host, and the opt-out exists for the in-cluster case:
	// a token endpoint reached over a service address inside a trusted network,
	// where TLS is terminated elsewhere.
	if endpoint.Scheme != "https" {
		allowInsecure, _ := strconv.ParseBool(
			os.Getenv("TRINO_OAUTH2_ALLOW_INSECURE_TOKEN_URL"))
		switch {
		case endpoint.Scheme != "http":
			return nil, fmt.Errorf(
				"TRINO_OAUTH2_TOKEN_URL must be http or https, got %q", endpoint.Scheme)
		case isLoopback(endpoint.Hostname()), allowInsecure:
			// Permitted.
		default:
			return nil, fmt.Errorf(
				"TRINO_OAUTH2_TOKEN_URL must use https (it carries the client secret); "+
					"set TRINO_OAUTH2_ALLOW_INSECURE_TOKEN_URL=true to allow %q", tokenURL)
		}
	}

	id, secret, found := strings.Cut(credential, ":")
	if !found {
		return nil, fmt.Errorf(
			"TRINO_OAUTH2_CREDENTIAL must be \"<client-id>:<client-secret>\"")
	}
	// An empty half is as broken as a missing colon, and easier to produce: a
	// Secret key that resolved to nothing yields ":" or "id:".
	if strings.TrimSpace(id) == "" || strings.TrimSpace(secret) == "" {
		return nil, fmt.Errorf(
			"TRINO_OAUTH2_CREDENTIAL must have a non-empty client id and secret")
	}

	scope := strings.TrimSpace(os.Getenv("TRINO_OAUTH2_SCOPE"))
	if scope == "" {
		scope = "openid"
	}
	return &tokenSource{tokenURL: tokenURL, clientID: id, secret: secret, scope: scope}, nil
}

// isLoopback reports whether a host is the local machine, where a plaintext
// token request cannot be observed from anywhere else.
func isLoopback(host string) bool {
	if host == "localhost" {
		return true
	}
	if ip := net.ParseIP(host); ip != nil {
		return ip.IsLoopback()
	}
	return false
}

// Invalidate drops the cached token so the next Token call fetches a new one.
//
// The safety net for the case this file cannot detect on its own: a token the
// provider called valid but the RESOURCE SERVER rejects. Trino answering 401 is
// ground truth about the token regardless of what the cache believes, so
// RoundTrip calls this and retries once rather than failing every query until
// the process is restarted.
func (t *tokenSource) Invalidate() {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.token = ""
	t.expires = time.Time{}
}

// Token returns a cached token, fetching a new one when it is missing or within
// 60 seconds of expiry.
func (t *tokenSource) Token(ctx context.Context) (string, error) {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.token != "" && time.Now().Before(t.expires.Add(-60*time.Second)) {
		return t.token, nil
	}

	form := url.Values{}
	form.Set("grant_type", "client_credentials")
	form.Set("client_id", t.clientID)
	form.Set("client_secret", t.secret)
	form.Set("scope", t.scope)

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, t.tokenURL,
		strings.NewReader(form.Encode()))
	if err != nil {
		return "", fmt.Errorf("building token request: %w", err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	// http.DefaultClient, deliberately: it honours SSL_CERT_FILE, which is how
	// this deployment trusts a self-signed identity provider.
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("requesting token from %s: %w", t.tokenURL, err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("token endpoint %s returned %s", t.tokenURL, resp.Status)
	}

	var payload struct {
		AccessToken string `json:"access_token"`
		ExpiresIn   int    `json:"expires_in"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return "", fmt.Errorf("decoding token response: %w", err)
	}
	if payload.AccessToken == "" {
		return "", fmt.Errorf("token endpoint %s returned no access_token", t.tokenURL)
	}

	t.token = payload.AccessToken
	// Default to 5 minutes when the provider omits expires_in, so a missing
	// field cannot pin a stale token forever.
	ttl := payload.ExpiresIn
	if ttl <= 0 {
		ttl = 300
	}
	t.expires = time.Now().Add(time.Duration(ttl) * time.Second)

	// Prefer the token's OWN `exp` when it is sooner than expires_in claims.
	// expires_in is the provider's promise about a token this process then
	// holds for hours; `exp` is what the resource server actually enforces, and
	// when the two disagree every query fails with
	//   401 JWT expired N milliseconds ago ... Invalid credentials
	// while the cache sits on a token it believes is fresh. Trusting the
	// smaller of the two costs one early refresh and cannot cost an outage.
	if exp, ok := jwtExpiry(payload.AccessToken); ok && exp.Before(t.expires) {
		t.expires = exp
	}
	return t.token, nil
}

// jwtExpiry reads the `exp` claim out of a JWT without verifying it. Signature
// verification is Trino's job — this only needs to know when to stop reusing
// the token, so a malformed or opaque token simply falls back to expires_in.
func jwtExpiry(token string) (time.Time, bool) {
	parts := strings.Split(token, ".")
	if len(parts) != 3 {
		return time.Time{}, false
	}
	// RawURLEncoding: JWT payloads are base64url with the padding stripped.
	raw, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return time.Time{}, false
	}
	var claims struct {
		Exp int64 `json:"exp"`
	}
	if err := json.Unmarshal(raw, &claims); err != nil || claims.Exp <= 0 {
		return time.Time{}, false
	}
	return time.Unix(claims.Exp, 0), true
}
