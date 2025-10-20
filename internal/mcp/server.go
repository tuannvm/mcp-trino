package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	mcpserver "github.com/mark3labs/mcp-go/server"
	oauth "github.com/tuannvm/oauth-mcp-proxy"
	"github.com/tuannvm/mcp-trino/internal/config"
	"github.com/tuannvm/mcp-trino/internal/trino"
)

// Server represents the MCP server with all components
type Server struct {
	mcpServer   *mcpserver.MCPServer
	config      *config.TrinoConfig
	version     string
	oauthServer *oauth.Server // oauth-mcp-proxy Server (nil if OAuth disabled)
}

// NewServer creates a new MCP server instance with all components
func NewServer(trinoClient *trino.Client, trinoConfig *config.TrinoConfig, version string) *Server {
	// Create MCP server with OAuth if enabled
	mcpServer, oauthServer := createMCPServer(trinoClient, trinoConfig, version)

	return &Server{
		mcpServer:   mcpServer,
		config:      trinoConfig,
		version:     version,
		oauthServer: oauthServer,
	}
}

// createMCPServer creates the core MCP server with tools and authentication
func createMCPServer(trinoClient *trino.Client, trinoConfig *config.TrinoConfig, version string) (*mcpserver.MCPServer, *oauth.Server) {
	// Build server options
	options := []mcpserver.ServerOption{
		mcpserver.WithToolCapabilities(true),
	}

	// Setup OAuth if enabled
	var oauthServer *oauth.Server
	if trinoConfig.OAuthEnabled {
		// Convert TrinoConfig to oauth.Config
		oauthCfg := trinoConfigToOAuthConfig(trinoConfig)

		// Create OAuth server
		var err error
		oauthServer, err = oauth.NewServer(oauthCfg)
		if err != nil {
			log.Printf("ERROR: Failed to create OAuth server: %v", err)
		} else {
			// Add OAuth middleware to server options
			options = append(options, mcpserver.WithToolHandlerMiddleware(oauthServer.Middleware()))
			log.Printf("INFO: OAuth enabled with provider: %s, mode: %s", trinoConfig.OAuthProvider, trinoConfig.OAuthMode)
		}
	}

	mcpServer := mcpserver.NewMCPServer("Trino MCP Server", version, options...)

	// Initialize tool handlers
	trinoHandlers := &TrinoHandlers{TrinoClient: trinoClient}
	RegisterTrinoTools(mcpServer, trinoHandlers)

	return mcpServer, oauthServer
}

// ServeStdio starts the MCP server with STDIO transport
func (s *Server) ServeStdio() error {
	return mcpserver.ServeStdio(s.mcpServer)
}

// ServeHTTP starts the MCP server with HTTP transport
func (s *Server) ServeHTTP(port string) error {
	addr := fmt.Sprintf(":%s", port)

	// Create StreamableHTTP server instance
	log.Println("Setting up StreamableHTTP server...")

	// Build streamable server with OAuth context extraction if enabled
	var streamableServer *mcpserver.StreamableHTTPServer
	if s.config.OAuthEnabled {
		streamableServer = mcpserver.NewStreamableHTTPServer(
			s.mcpServer,
			mcpserver.WithEndpointPath("/mcp"),
			mcpserver.WithHTTPContextFunc(oauth.CreateHTTPContextFunc()),
			mcpserver.WithStateLess(false),
		)
		log.Println("INFO: OAuth context extraction enabled")
	} else {
		streamableServer = mcpserver.NewStreamableHTTPServer(
			s.mcpServer,
			mcpserver.WithEndpointPath("/mcp"),
			mcpserver.WithStateLess(false),
		)
	}

	// Create HTTP mux for routing
	mux := http.NewServeMux()

	// Add status endpoint
	mux.HandleFunc("/status", s.handleStatus)

	// Register OAuth endpoints if enabled
	if s.config.OAuthEnabled && s.oauthServer != nil {
		// oauth-mcp-proxy automatically registers all OAuth endpoints:
		// - /.well-known/oauth-authorization-server
		// - /.well-known/oauth-protected-resource
		// - /.well-known/openid-configuration
		// - /.well-known/jwks.json
		// - /oauth/authorize, /oauth/callback, /oauth/token (proxy mode only)
		s.oauthServer.RegisterHandlers(mux)
		log.Printf("INFO: OAuth endpoints registered via oauth-mcp-proxy")

		// Add compatibility endpoints for mcp-remote 0.1.19+
		// mcp-remote incorrectly appends /.well-known/* to the full MCP endpoint URL
		// TODO: Remove after mcp-remote is fixed
		// Note: These need to use the internal handler from oauth-mcp-proxy
		// For now, we'll skip these - clients should use correct endpoints
		log.Printf("INFO: OAuth mode: %s, provider: %s", s.config.OAuthMode, s.config.OAuthProvider)
	}

	// Shared MCP handler function for both endpoints
	mcpHandler := s.createMCPHandler(streamableServer)

	// Add MCP endpoint (modern)
	mux.HandleFunc("/mcp", mcpHandler)

	// Add SSE endpoint (backward compatibility)
	mux.HandleFunc("/sse", mcpHandler)

	httpServer := &http.Server{
		Addr:    addr,
		Handler: mux,
	}

	// Graceful shutdown
	done := make(chan bool, 1)
	go s.handleSignals(done)

	go func() {
		// Check for HTTPS certificates
		certFile := getEnv("HTTPS_CERT_FILE", "")
		keyFile := getEnv("HTTPS_KEY_FILE", "")

		// Determine MCP server URL for status endpoint
		mcpHost := getEnv("MCP_HOST", "localhost")
		mcpPort := getEnv("MCP_PORT", "8080")
		scheme := s.getScheme()
		mcpURL := getEnv("MCP_URL", fmt.Sprintf("%s://%s:%s", scheme, mcpHost, mcpPort))

		if certFile != "" && keyFile != "" {
			// Start HTTPS server
			oauthStatus := s.getOAuthStatus()

			log.Printf("Starting HTTPS server on %s%s", addr, oauthStatus)
			log.Printf("  - Modern endpoint: %s/mcp", mcpURL)
			log.Printf("  - Legacy endpoint: %s/sse (backward compatibility)", mcpURL)
			log.Printf("  - OAuth metadata: %s/.well-known/oauth-authorization-server", mcpURL)
			log.Printf("  - OAuth metadata (legacy): %s/.well-known/oauth-metadata", mcpURL)
			if s.config.OAuthEnabled {
				log.Printf("  - OAuth callback: %s/oauth/callback", mcpURL)
				log.Printf("  - OAuth callback (Claude Code): %s/callback (redirects to /oauth/callback)", mcpURL)
			}

			if err := httpServer.ListenAndServeTLS(certFile, keyFile); err != nil && err != http.ErrServerClosed {
				log.Fatalf("HTTPS server error: %v", err)
			}
		} else {
			// Start HTTP server
			oauthStatus := s.getOAuthStatusWithWarning()

			log.Printf("Starting HTTP server on %s%s", addr, oauthStatus)
			log.Printf("  - Modern endpoint: %s/mcp", mcpURL)
			log.Printf("  - Legacy endpoint: %s/sse (backward compatibility)", mcpURL)
			log.Printf("  - OAuth metadata: %s/.well-known/oauth-authorization-server", mcpURL)
			log.Printf("  - OAuth metadata (legacy): %s/.well-known/oauth-metadata", mcpURL)
			if s.config.OAuthEnabled {
				log.Printf("  - OAuth callback: %s/oauth/callback", mcpURL)
				log.Printf("  - OAuth callback (Claude Code): %s/callback (redirects to /oauth/callback)", mcpURL)
			}

			if err := httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
				log.Fatalf("HTTP server error: %v", err)
			}
		}
	}()

	<-done
	log.Println("Shutting down HTTP server...")

	// Allow 30 seconds for graceful shutdown
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	log.Println("Waiting for active connections to finish (max 30 seconds)...")
	if err := httpServer.Shutdown(ctx); err != nil {
		log.Printf("HTTP server forced shutdown after timeout: %v", err)
		return httpServer.Close()
	}
	log.Println("HTTP server shutdown completed gracefully")
	return nil
}

// createMCPHandler creates the shared MCP handler function
func (s *Server) createMCPHandler(streamableServer *mcpserver.StreamableHTTPServer) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// Add CORS headers
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, DELETE, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")

		if r.Method == "OPTIONS" {
			w.WriteHeader(http.StatusOK)
			return
		}

		log.Printf("MCP %s %s from %s", r.Method, r.URL.Path, r.RemoteAddr)

		// Check if OAuth is enabled and no token is provided
		if s.config.OAuthEnabled {
			authHeader := r.Header.Get("Authorization")
			if authHeader == "" || !strings.HasPrefix(authHeader, "Bearer ") {
				// Return 401 with OAuth discovery information
				log.Printf("OAuth: No bearer token provided, returning 401 with discovery info")

				// Calculate MCP URL for OAuth discovery
				mcpHost := getEnv("MCP_HOST", "localhost")
				mcpPort := getEnv("MCP_PORT", "8080")
				scheme := s.getScheme()
				mcpURL := getEnv("MCP_URL", fmt.Sprintf("%s://%s:%s", scheme, mcpHost, mcpPort))

				// Multiple WWW-Authenticate headers for broad client compatibility (RFC 7235)
				// Standard OAuth 2.0 Bearer challenge for traditional clients
				w.Header().Add("WWW-Authenticate", `Bearer realm="OAuth", error="invalid_token", error_description="Missing or invalid access token"`)
				// MCP-compliant resource metadata discovery for Claude.ai/Perplexity
				w.Header().Add("WWW-Authenticate", fmt.Sprintf(`resource_metadata="%s/.well-known/oauth-protected-resource"`, mcpURL))
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusUnauthorized)

				errorResponse := map[string]string{
					"error":             "invalid_token",
					"error_description": "Missing or invalid access token",
				}
				if err := json.NewEncoder(w).Encode(errorResponse); err != nil {
					log.Printf("Error encoding OAuth error response: %v", err)
				}
				return
			}

			// Add OAuth context
			contextFunc := oauth.CreateHTTPContextFunc()
			ctx := contextFunc(r.Context(), r)
			r = r.WithContext(ctx)
		}

		// Handle MCP request using StreamableHTTP server
		streamableServer.ServeHTTP(w, r)
	}
}

// handleStatus handles the status endpoint
func (s *Server) handleStatus(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_, _ = fmt.Fprintf(w, `{"status":"ok","version":"%s"}`, s.version)
}

// handleSignals handles graceful shutdown signals
func (s *Server) handleSignals(done chan<- bool) {
	ch := make(chan os.Signal, 1)
	signal.Notify(ch, syscall.SIGINT, syscall.SIGTERM)
	<-ch
	done <- true
}

// getOAuthStatus returns OAuth status string
// getScheme returns the appropriate URL scheme (http or https) based on server configuration
func (s *Server) getScheme() string {
	certFile := getEnv("HTTPS_CERT_FILE", "")
	keyFile := getEnv("HTTPS_KEY_FILE", "")

	if certFile != "" && keyFile != "" {
		return "https"
	}
	return "http"
}

func (s *Server) getOAuthStatus() string {
	if s.config.OAuthEnabled {
		return " (OAuth enabled)"
	}
	return " (OAuth disabled)"
}

// getOAuthStatusWithWarning returns OAuth status with warning for HTTP
func (s *Server) getOAuthStatusWithWarning() string {
	if s.config.OAuthEnabled {
		return " (OAuth enabled - WARNING: HTTPS recommended for production)"
	}
	return " (OAuth disabled)"
}


// trinoConfigToOAuthConfig converts TrinoConfig to oauth.Config for oauth-mcp-proxy
func trinoConfigToOAuthConfig(cfg *config.TrinoConfig) *oauth.Config {
	// Determine server URL for proxy mode
	serverURL := getEnv("MCP_URL", "")
	if serverURL == "" {
		mcpHost := getEnv("MCP_HOST", "localhost")
		mcpPort := getEnv("MCP_PORT", "8080")
		scheme := "http"
		if getEnv("HTTPS_CERT_FILE", "") != "" && getEnv("HTTPS_KEY_FILE", "") != "" {
			scheme = "https"
		}
		serverURL = fmt.Sprintf("%s://%s:%s", scheme, mcpHost, mcpPort)
	}

	return &oauth.Config{
		Mode:         cfg.OAuthMode,
		Provider:     cfg.OAuthProvider,
		RedirectURIs: cfg.OAuthRedirectURIs,
		Issuer:       cfg.OIDCIssuer,
		Audience:     cfg.OIDCAudience,
		ClientID:     cfg.OIDCClientID,
		ClientSecret: cfg.OIDCClientSecret,
		ServerURL:    serverURL,
		JWTSecret:    []byte(cfg.JWTSecret),
	}
}

// getEnv gets environment variable with default value
func getEnv(key, def string) string {
	if v, ok := os.LookupEnv(key); ok {
		return v
	}
	return def
}
