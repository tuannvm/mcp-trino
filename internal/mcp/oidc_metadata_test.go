package mcp

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// The discovery fetch must not be able to hang forever: http.DefaultClient has
// no timeout, so a provider that accepts the connection and stalls would block
// the handler goroutine and every request queued behind it.
func TestDiscoveryClientHasTimeout(t *testing.T) {
	if discoveryClient.Timeout <= 0 {
		t.Fatal("discoveryClient must have a finite timeout")
	}
}

func TestDocumentFailsOnStalledIssuer(t *testing.T) {
	// A server that accepts the connection and answers far too late.
	//
	// A sleep rather than a channel the test closes: httptest's Close waits for
	// outstanding handlers, and with `defer close(ch)` registered first the
	// deferred Close runs BEFORE it (defers are LIFO) and the two deadlock.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(2 * time.Second)
	}))
	defer srv.Close()

	// A short timeout for the test only; the production value is 10s.
	original := discoveryClient
	discoveryClient = &http.Client{Timeout: 150 * time.Millisecond}
	defer func() { discoveryClient = original }()

	h := &oidcDiscoveryHandler{issuer: srv.URL, cacheTTL: time.Minute}
	start := time.Now()
	_, err := h.document()
	if err == nil {
		t.Fatal("expected a timeout error from a stalled issuer")
	}
	// Assert the error is the TIMEOUT, not merely that one occurred: document()
	// wraps the client error with %w, and http.Client's deadline is reported as
	// context.DeadlineExceeded. Without this a later parse failure would satisfy
	// the test while the timeout did nothing.
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("error = %v, want it to wrap context.DeadlineExceeded", err)
	}
	// Below the server's 2s sleep, so an ignored timeout FAILS here. A bound
	// above that sleep would pass whether or not the deadline was honoured,
	// which is the whole thing being tested.
	if elapsed := time.Since(start); elapsed >= time.Second {
		t.Fatalf("document() took %s — it did not honour the client timeout", elapsed)
	}
}

// An issuer that returns an enormous document must be refused rather than read
// into memory. The client timeout bounds duration, not bytes.
func TestDocumentRejectsOversizedResponse(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		// Valid JSON, so a failure here can only come from the size cap and not
		// from the parser.
		_, _ = w.Write([]byte(`{"issuer":"` + strings.Repeat("x", maxDiscoveryBytes) + `"}`))
	}))
	defer srv.Close()

	h := &oidcDiscoveryHandler{issuer: srv.URL, cacheTTL: time.Minute}
	if _, err := h.document(); err == nil {
		t.Fatal("expected an oversized-document error")
	} else if !strings.Contains(err.Error(), "more than") {
		t.Fatalf("error = %v, want it to name the size limit", err)
	}
	if h.cached != nil {
		t.Fatal("an oversized document must not be cached")
	}
}
