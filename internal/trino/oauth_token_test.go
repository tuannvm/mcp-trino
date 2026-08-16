package trino

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/tuannvm/mcp-trino/internal/config"
)

// jwtWithExp builds an unsigned JWT carrying only an `exp` claim. Nothing here
// verifies signatures, so a valid header and payload are enough.
func jwtWithExp(exp time.Time) string {
	enc := func(v any) string {
		b, _ := json.Marshal(v)
		return base64.RawURLEncoding.EncodeToString(b)
	}
	return enc(map[string]string{"alg": "RS256", "typ": "JWT"}) + "." +
		enc(map[string]int64{"exp": exp.Unix()}) + ".signature"
}

func TestJwtExpiry(t *testing.T) {
	want := time.Now().Add(90 * time.Minute).Truncate(time.Second)
	got, ok := jwtExpiry(jwtWithExp(want))
	if !ok || !got.Equal(want) {
		t.Fatalf("jwtExpiry = %v, %v; want %v, true", got, ok, want)
	}

	// Anything that is not a three-part JWT falls back to expires_in.
	for _, bad := range []string{"", "opaque-token", "a.b", "a.!!!.c"} {
		if _, ok := jwtExpiry(bad); ok {
			t.Errorf("jwtExpiry(%q) reported success on a non-JWT", bad)
		}
	}
}

// The regression this patch exists for: a provider that promises a long
// expires_in while the token itself expires sooner. Trusting expires_in pins a
// dead token and every query 401s until the process restarts.
func TestTokenExpiryPrefersJwtExpOverExpiresIn(t *testing.T) {
	realExp := time.Now().Add(2 * time.Minute)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintf(w, `{"access_token":%q,"expires_in":86400}`, jwtWithExp(realExp))
	}))
	defer srv.Close()

	ts := &tokenSource{tokenURL: srv.URL, clientID: "id", secret: "sec", scope: "openid"}
	if _, err := ts.Token(context.Background()); err != nil {
		t.Fatalf("Token: %v", err)
	}
	if diff := ts.expires.Sub(realExp); diff > time.Second || diff < -time.Second {
		t.Fatalf("cached expiry %v; want the token's own exp %v, not expires_in", ts.expires, realExp)
	}
}

func TestTokenFallsBackToExpiresInForOpaqueTokens(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `{"access_token":"opaque","expires_in":600}`)
	}))
	defer srv.Close()

	ts := &tokenSource{tokenURL: srv.URL, clientID: "id", secret: "sec", scope: "openid"}
	if _, err := ts.Token(context.Background()); err != nil {
		t.Fatalf("Token: %v", err)
	}
	if d := time.Until(ts.expires); d < 9*time.Minute || d > 10*time.Minute {
		t.Fatalf("expiry in %v; want ~10m from expires_in", d)
	}
}

func TestInvalidateForcesRefetch(t *testing.T) {
	var fetches int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fetches++
		fmt.Fprintf(w, `{"access_token":%q,"expires_in":3600}`,
			jwtWithExp(time.Now().Add(time.Hour)))
	}))
	defer srv.Close()

	ts := &tokenSource{tokenURL: srv.URL, clientID: "id", secret: "sec", scope: "openid"}
	ctx := context.Background()
	_, _ = ts.Token(ctx)
	_, _ = ts.Token(ctx) // cached — must not hit the endpoint again
	if fetches != 1 {
		t.Fatalf("fetches = %d after two Token calls; want 1 (second should be cached)", fetches)
	}
	ts.Invalidate()
	_, _ = ts.Token(ctx)
	if fetches != 2 {
		t.Fatalf("fetches = %d after Invalidate; want 2", fetches)
	}
}

// Trino answering 401 must refresh the token and replay the query exactly once,
// rather than surfacing the 401 and poisoning every later request.
func TestRoundTripRetriesOnceOn401(t *testing.T) {
	var issued int
	auth := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		issued++
		fmt.Fprintf(w, `{"access_token":"token-%d","expires_in":3600}`, issued)
	}))
	defer auth.Close()

	var seen []string
	var bodies []string
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seen = append(seen, r.Header.Get("Authorization"))
		// io.ReadAll, not one Read into a ContentLength-sized buffer: a single
		// Read may return short, so the body assertion below could fail
		// intermittently. And a chunked request has ContentLength -1, where
		// make([]byte, -1) panics outright.
		body, _ := io.ReadAll(r.Body)
		bodies = append(bodies, string(body))
		if len(seen) == 1 {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer upstream.Close()

	rt := &headerRoundTripper{
		base:   http.DefaultTransport,
		config: testTrinoConfig(),
		tokens: &tokenSource{tokenURL: auth.URL, clientID: "id", secret: "sec", scope: "openid"},
	}
	req, err := http.NewRequest(http.MethodPost, upstream.URL, strings.NewReader("SELECT 1"))
	if err != nil {
		t.Fatalf("NewRequest: %v", err)
	}
	resp, err := rt.RoundTrip(req)
	if err != nil {
		t.Fatalf("RoundTrip: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d; want 200 after the retry", resp.StatusCode)
	}
	if len(seen) != 2 {
		t.Fatalf("upstream saw %d requests; want exactly 2", len(seen))
	}
	if seen[0] == seen[1] {
		t.Fatalf("retry reused the rejected token %q", seen[0])
	}
	// The replay must carry the query, not an empty body consumed by attempt 1.
	if bodies[1] != "SELECT 1" {
		t.Fatalf("retry body = %q; want the original query", bodies[1])
	}
}

// A 401 with no token source configured is upstream's basic-auth path: return
// it untouched and do not retry.
func TestRoundTripDoesNotRetryWithoutTokenSource(t *testing.T) {
	var hits int
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits++
		w.WriteHeader(http.StatusUnauthorized)
	}))
	defer upstream.Close()

	rt := &headerRoundTripper{base: http.DefaultTransport, config: testTrinoConfig()}
	req, _ := http.NewRequest(http.MethodGet, upstream.URL, nil)
	resp, err := rt.RoundTrip(req)
	if err != nil {
		t.Fatalf("RoundTrip: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if hits != 1 || resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("hits = %d, status = %d; want 1 and 401 passed through", hits, resp.StatusCode)
	}
}

func testTrinoConfig() *config.TrinoConfig {
	return &config.TrinoConfig{}
}

// newTokenSourceFromEnv must FAIL CLOSED. Returning nil on a partial or
// malformed configuration would fall back to the basic credentials still in the
// DSN, so a coordinator permitting password auth would accept an unintended
// downgrade that looks like success.
func TestNewTokenSourceFromEnvFailsClosed(t *testing.T) {
	cases := []struct {
		name        string
		env         map[string]string
		wantSource  bool
		wantErr     bool
		errContains string
	}{
		{
			name:       "unset is not an error — basic auth is the documented fallback",
			env:        map[string]string{},
			wantSource: false,
		},
		{
			name:        "token url without credential",
			env:         map[string]string{"TRINO_OAUTH2_TOKEN_URL": "https://idp.example/token"},
			wantErr:     true,
			errContains: "must be set together",
		},
		{
			name:        "credential without token url",
			env:         map[string]string{"TRINO_OAUTH2_CREDENTIAL": "id:secret"},
			wantErr:     true,
			errContains: "must be set together",
		},
		{
			name: "credential with no colon",
			env: map[string]string{
				"TRINO_OAUTH2_TOKEN_URL":  "https://idp.example/token",
				"TRINO_OAUTH2_CREDENTIAL": "no-colon-here",
			},
			wantErr:     true,
			errContains: "<client-id>:<client-secret>",
		},
		{
			name: "empty client id",
			env: map[string]string{
				"TRINO_OAUTH2_TOKEN_URL":  "https://idp.example/token",
				"TRINO_OAUTH2_CREDENTIAL": ":secret",
			},
			wantErr:     true,
			errContains: "non-empty",
		},
		{
			name: "empty client secret",
			env: map[string]string{
				"TRINO_OAUTH2_TOKEN_URL":  "https://idp.example/token",
				"TRINO_OAUTH2_CREDENTIAL": "id:",
			},
			wantErr:     true,
			errContains: "non-empty",
		},
		{
			name: "malformed token url",
			env: map[string]string{
				"TRINO_OAUTH2_TOKEN_URL":  "not a url",
				"TRINO_OAUTH2_CREDENTIAL": "id:secret",
			},
			wantErr:     true,
			errContains: "not a valid URL",
		},
		{
			name: "non-http scheme",
			env: map[string]string{
				"TRINO_OAUTH2_TOKEN_URL":  "ftp://idp.example/token",
				"TRINO_OAUTH2_CREDENTIAL": "id:secret",
			},
			wantErr:     true,
			errContains: "must be http or https",
		},
		{
			// The secret travels in the request body, so plaintext to a remote
			// host leaks it to anything on the path.
			name: "plaintext http to a remote host is refused",
			env: map[string]string{
				"TRINO_OAUTH2_TOKEN_URL":  "http://idp.example/token",
				"TRINO_OAUTH2_CREDENTIAL": "id:secret",
			},
			wantErr:     true,
			errContains: "must use https",
		},
		{
			name: "plaintext http to loopback is allowed",
			env: map[string]string{
				"TRINO_OAUTH2_TOKEN_URL":  "http://127.0.0.1:8080/token",
				"TRINO_OAUTH2_CREDENTIAL": "id:secret",
			},
			wantSource: true,
		},
		{
			name: "plaintext http to localhost is allowed",
			env: map[string]string{
				"TRINO_OAUTH2_TOKEN_URL":  "http://localhost:8080/token",
				"TRINO_OAUTH2_CREDENTIAL": "id:secret",
			},
			wantSource: true,
		},
		{
			// The in-cluster case: a Service address inside a trusted network
			// where TLS is terminated elsewhere.
			name: "plaintext http elsewhere with the explicit opt-out",
			env: map[string]string{
				"TRINO_OAUTH2_TOKEN_URL":                "http://keycloak.keycloak.svc:8080/token",
				"TRINO_OAUTH2_CREDENTIAL":               "id:secret",
				"TRINO_OAUTH2_ALLOW_INSECURE_TOKEN_URL": "true",
			},
			wantSource: true,
		},
		{
			name: "https is always allowed",
			env: map[string]string{
				"TRINO_OAUTH2_TOKEN_URL":  "https://idp.example/token",
				"TRINO_OAUTH2_CREDENTIAL": "id:secret",
			},
			wantSource: true,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			for _, k := range []string{
				"TRINO_OAUTH2_TOKEN_URL",
				"TRINO_OAUTH2_CREDENTIAL",
				"TRINO_OAUTH2_SCOPE",
				"TRINO_OAUTH2_ALLOW_INSECURE_TOKEN_URL",
			} {
				t.Setenv(k, "")
				os.Unsetenv(k)
			}
			for k, v := range tc.env {
				t.Setenv(k, v)
			}

			got, err := newTokenSourceFromEnv()
			if tc.wantErr {
				if err == nil {
					t.Fatalf("expected an error, got source=%v", got != nil)
				}
				if !strings.Contains(err.Error(), tc.errContains) {
					t.Fatalf("error %q does not contain %q", err, tc.errContains)
				}
				// Failing closed means no usable source came back either.
				if got != nil {
					t.Fatal("returned a token source alongside an error")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if (got != nil) != tc.wantSource {
				t.Fatalf("source=%v, want %v", got != nil, tc.wantSource)
			}
		})
	}
}

// The default scope is applied, and an explicit one wins.
func TestNewTokenSourceFromEnvScope(t *testing.T) {
	t.Setenv("TRINO_OAUTH2_TOKEN_URL", "https://idp.example/token")
	t.Setenv("TRINO_OAUTH2_CREDENTIAL", "id:secret")

	// t.Setenv with an empty value, not os.Unsetenv: only t.Setenv registers the
	// cleanup that restores a pre-existing value, and without it this test would
	// leave the variable unset for every later test in the binary. An empty
	// value exercises the same path, since newTokenSourceFromEnv trims it and
	// falls back to the default scope.
	t.Setenv("TRINO_OAUTH2_SCOPE", "")
	got, err := newTokenSourceFromEnv()
	if err != nil || got == nil {
		t.Fatalf("unexpected: source=%v err=%v", got, err)
	}
	if got.scope != "openid" {
		t.Fatalf("default scope = %q, want openid", got.scope)
	}

	t.Setenv("TRINO_OAUTH2_SCOPE", "trino:all")
	got, err = newTokenSourceFromEnv()
	if err != nil || got == nil {
		t.Fatalf("unexpected: source=%v err=%v", got, err)
	}
	if got.scope != "trino:all" {
		t.Fatalf("scope = %q, want trino:all", got.scope)
	}
}
