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
	mcpServer, oauthServer := createMCPServer(trinoClient, trinoConfig, version)

	return &Server{
		mcpServer:   mcpServer,
		config:      trinoConfig,
		version:     version,
		oauthServer: oauthServer,
	}
}

func createMCPServer(trinoClient *trino.Client, trinoConfig *config.TrinoConfig, version string) (*mcpserver.MCPServer, *oauth.Server) {
	options := []mcpserver.ServerOption{mcpserver.WithToolCapabilities(true)}

	var oauthServer *oauth.Server
	if trinoConfig.OAuthEnabled {
		oauthCfg := trinoConfigToOAuthConfig(trinoConfig)
		var err error
		oauthServer, err = oauth.NewServer(oauthCfg)
		if err != nil {
			log.Printf("ERROR: Failed to create OAuth server: %v", err)
		} else {
			options = append(options, mcpserver.WithToolHandlerMiddleware(oauthServer.Middleware()))
			log.Printf("INFO: OAuth enabled with provider: %s, mode: %s", trinoConfig.OAuthProvider, trinoConfig.OAuthMode)
		}
	}

	mcpServer := mcpserver.NewMCPServer("Trino MCP Server", version, options...)

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

	log.Println("Setting up StreamableHTTP server...")

	var streamableServer *mcpserver.StreamableHTTPServer
	if s.config.OAuthEnabled {
		streamableServer = mcpserver.NewStreamableHTTPServer(
			s.mcpServer,
			mcpserver.WithEndpointPath("/mcp"),
			mcpserver.WithHTTPContextFunc(oauth.CreateHTTPContextFunc()),
			mcpserver.WithStateLess(false),
		)
	} else {
		streamableServer = mcpserver.NewStreamableHTTPServer(
			s.mcpServer,
			mcpserver.WithEndpointPath("/mcp"),
			mcpserver.WithStateLess(false),
		)
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/status", s.handleStatus)

	if s.config.OAuthEnabled && s.oauthServer != nil {
		s.oauthServer.RegisterHandlers(mux)
		log.Printf("INFO: OAuth enabled - mode: %s, provider: %s", s.config.OAuthMode, s.config.OAuthProvider)
	}

	mcpHandler := s.createMCPHandler(streamableServer)
	mux.HandleFunc("/mcp", mcpHandler)
	mux.HandleFunc("/sse", mcpHandler)

	httpServer := &http.Server{Addr: addr, Handler: mux}

	done := make(chan bool, 1)
	go s.handleSignals(done)

	go func() {
		certFile := getEnv("HTTPS_CERT_FILE", "")
		keyFile := getEnv("HTTPS_KEY_FILE", "")

		mcpHost := getEnv("MCP_HOST", "localhost")
		mcpPort := getEnv("MCP_PORT", "8080")
		scheme := s.getScheme()
		mcpURL := getEnv("MCP_URL", fmt.Sprintf("%s://%s:%s", scheme, mcpHost, mcpPort))

		if certFile != "" && keyFile != "" {
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
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, DELETE, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")

		if r.Method == "OPTIONS" {
			w.WriteHeader(http.StatusOK)
			return
		}

		log.Printf("MCP %s %s from %s", r.Method, r.URL.Path, r.RemoteAddr)

		if s.config.OAuthEnabled {
			authHeader := r.Header.Get("Authorization")
			if authHeader == "" || !strings.HasPrefix(authHeader, "Bearer ") {
				log.Printf("OAuth: No bearer token provided, returning 401 with discovery info")

				mcpHost := getEnv("MCP_HOST", "localhost")
				mcpPort := getEnv("MCP_PORT", "8080")
				scheme := s.getScheme()
				mcpURL := getEnv("MCP_URL", fmt.Sprintf("%s://%s:%s", scheme, mcpHost, mcpPort))

				w.Header().Add("WWW-Authenticate", `Bearer realm="OAuth", error="invalid_token", error_description="Missing or invalid access token"`)
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

			contextFunc := oauth.CreateHTTPContextFunc()
			ctx := contextFunc(r.Context(), r)
			r = r.WithContext(ctx)
		}

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


func trinoConfigToOAuthConfig(cfg *config.TrinoConfig) *oauth.Config {
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
