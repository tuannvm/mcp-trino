package mcp

// Generic OIDC discovery for the OAuth authorization-server metadata.
//
// oauth-mcp-proxy supports four providers — hmac, okta, google, azure — and no
// generic OIDC one. Its metadata handler builds the endpoints by string
// concatenation onto the issuer, in OKTA's shape:
//
//	metadata["authorization_endpoint"] = issuer + "/oauth2/v1/authorize"
//	metadata["token_endpoint"]         = issuer + "/oauth2/v1/token"
//
// Against Keycloak (or any other non-Okta provider) those paths do not exist —
// the real ones are under /protocol/openid-connect/. A client that follows the
// WWW-Authenticate `resource_metadata` hint therefore reaches a 404 and cannot
// complete the login, even though TOKEN VALIDATION works perfectly. That split
// is what makes it confusing: presenting a token by hand succeeds while the
// automatic flow fails.
//
// This handler answers the same document from the provider's OWN discovery
// endpoint (/.well-known/openid-configuration), so the advertised endpoints are
// whatever the provider actually serves. Enable with:
//
//	OAUTH_METADATA_FROM_DISCOVERY=true
//
// Off by default: without it the upstream behaviour is untouched, which keeps
// this a drop-in image for an Okta deployment.

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"
	"sync"
	"time"
)

// oidcDiscoveryHandler serves the authorization-server metadata by proxying the
// issuer's discovery document, with a small cache so every unauthenticated
// request does not hit the identity provider.
type oidcDiscoveryHandler struct {
	issuer string

	mu       sync.Mutex
	cached   []byte
	fetched  time.Time
	cacheTTL time.Duration
}

func newOIDCDiscoveryHandler(issuer string) *oidcDiscoveryHandler {
	return &oidcDiscoveryHandler{
		issuer:   strings.TrimRight(issuer, "/"),
		cacheTTL: 5 * time.Minute,
	}
}

// discoveryClient fetches the issuer's metadata.
//
// An EXPLICIT timeout, because http.DefaultClient has none: an identity
// provider that accepts the connection and then stalls would block this
// goroutine indefinitely, and every MCP client asking for the metadata
// document piles up behind it. The handler is on the request path of the
// OAuth flow, so that is an outage rather than a slow response.
//
// It keeps http.DefaultTransport, which is what honours SSL_CERT_FILE — how a
// deployment trusts a self-signed identity provider.
var discoveryClient = &http.Client{Timeout: 10 * time.Second}

// maxDiscoveryBytes caps the metadata document.
//
// The timeout above bounds how LONG the fetch may take, not how MUCH it may
// return: an issuer that streams quickly enough can exhaust memory well before
// the deadline. A discovery document is a few kilobytes, so 1 MiB is generous
// and still refuses a response that could not be a legitimate one.
const maxDiscoveryBytes = 1 << 20

func (h *oidcDiscoveryHandler) document() ([]byte, error) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.cached != nil && time.Since(h.fetched) < h.cacheTTL {
		return h.cached, nil
	}

	url := h.issuer + "/.well-known/openid-configuration"
	resp, err := discoveryClient.Get(url)
	if err != nil {
		return nil, fmt.Errorf("fetching %s: %w", url, err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("%s returned %s", url, resp.Status)
	}
	// Read one byte past the cap so hitting it is distinguishable from a
	// document that merely happens to be exactly maxDiscoveryBytes long.
	body, err := io.ReadAll(io.LimitReader(resp.Body, maxDiscoveryBytes+1))
	if err != nil {
		return nil, fmt.Errorf("reading %s: %w", url, err)
	}
	if len(body) > maxDiscoveryBytes {
		// Rejected BEFORE unmarshalling and before caching: an oversized
		// document must not be parsed and must not be served for the whole TTL.
		return nil, fmt.Errorf("%s returned more than %d bytes", url, maxDiscoveryBytes)
	}
	// Parse and re-encode so a malformed document fails HERE, with the URL in
	// the message, rather than in the client as an unexplained protocol error.
	var doc map[string]any
	if err := json.Unmarshal(body, &doc); err != nil {
		return nil, fmt.Errorf("parsing %s: %w", url, err)
	}
	out, err := json.Marshal(doc)
	if err != nil {
		return nil, fmt.Errorf("re-encoding %s: %w", url, err)
	}

	h.cached, h.fetched = out, time.Now()
	return out, nil
}

func (h *oidcDiscoveryHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	doc, err := h.document()
	if err != nil {
		log.Printf("ERROR: OIDC discovery passthrough: %v", err)
		http.Error(w, `{"error":"discovery_unavailable"}`, http.StatusBadGateway)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	// CORS: browser-based MCP clients read this cross-origin before they hold
	// any credential, so the document has to be publicly readable.
	w.Header().Set("Access-Control-Allow-Origin", "*")
	_, _ = w.Write(doc)
}
