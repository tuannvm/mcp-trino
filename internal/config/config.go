package config

import (
	"fmt"
	"log"
	"os"
	"strconv"
	"strings"
	"time"
)

// TrinoConfig holds Trino connection parameters
type TrinoConfig struct {
	// Basic connection parameters
	Host              string
	Port              int
	User              string
	Password          string
	Catalog           string
	Schema            string
	Scheme            string
	SSL               bool
	SSLInsecure       bool
	AllowWriteQueries bool          // Controls whether non-read-only SQL queries are allowed
	QueryTimeout      time.Duration // Query execution timeout
	
	// OAuth mode configuration
	OAuthEnabled      bool   // Enable OAuth 2.1 authentication
	OAuthProvider     string // OAuth provider: "hmac", "okta", "google", "azure"
	JWTSecret         string // JWT signing secret for HMAC provider
	
	// OIDC provider configuration
	OIDCIssuer        string // OIDC issuer URL
	OIDCAudience      string // OIDC audience
	OIDCClientID      string // OIDC client ID
	OIDCClientSecret  string // OIDC client secret
	OAuthRedirectURI  string // Fixed OAuth redirect URI (overrides dynamic callback)
	// Custom Trino Source header
	TrinoSource       string // Value for X-Trino-Source header
}

// NewTrinoConfig creates a new TrinoConfig with values from environment variables or defaults
func NewTrinoConfig() (*TrinoConfig, error) {
	port, _ := strconv.Atoi(getEnv("TRINO_PORT", "8080"))
	ssl, _ := strconv.ParseBool(getEnv("TRINO_SSL", "true"))
	sslInsecure, _ := strconv.ParseBool(getEnv("TRINO_SSL_INSECURE", "true"))
	scheme := getEnv("TRINO_SCHEME", "https")
	allowWriteQueries, _ := strconv.ParseBool(getEnv("TRINO_ALLOW_WRITE_QUERIES", "false"))
	oauthEnabled, _ := strconv.ParseBool(getEnv("TRINO_OAUTH_ENABLED", "false"))
	oauthProvider := strings.ToLower(getEnv("OAUTH_PROVIDER", "hmac"))
	jwtSecret := getEnv("JWT_SECRET", "")
	
	// OIDC configuration
	oidcIssuer := getEnv("OIDC_ISSUER", "")
	oidcAudience := getEnv("OIDC_AUDIENCE", "")
	oidcClientID := getEnv("OIDC_CLIENT_ID", "")
	oidcClientSecret := getEnv("OIDC_CLIENT_SECRET", "")
	oauthRedirectURI := getEnv("OAUTH_REDIRECT_URI", "")

	// Parse query timeout from environment variable
	const defaultTimeout = 30
	timeoutStr := getEnv("TRINO_QUERY_TIMEOUT", strconv.Itoa(defaultTimeout))
	timeoutInt, err := strconv.Atoi(timeoutStr)

	// Validate timeout value
	switch {
	case err != nil:
		log.Printf("WARNING: Invalid TRINO_QUERY_TIMEOUT '%s': not an integer. Using default of %d seconds", timeoutStr, defaultTimeout)
		timeoutInt = defaultTimeout
	case timeoutInt <= 0:
		log.Printf("WARNING: Invalid TRINO_QUERY_TIMEOUT '%d': must be positive. Using default of %d seconds", timeoutInt, defaultTimeout)
		timeoutInt = defaultTimeout
	}

	queryTimeout := time.Duration(timeoutInt) * time.Second

	// If using HTTPS, force SSL to true
	if strings.EqualFold(scheme, "https") {
		ssl = true
	}

	// Log a warning if write queries are allowed
	if allowWriteQueries {
		log.Println("WARNING: Write queries are enabled (TRINO_ALLOW_WRITE_QUERIES=true). SQL injection protection is bypassed.")
	}

	// Validate and log OAuth mode status
	if oauthEnabled {
		// Validate OAuth provider
		validProviders := map[string]bool{"hmac": true, "okta": true, "google": true, "azure": true}
		if !validProviders[oauthProvider] {
			return nil, fmt.Errorf("invalid OAuth provider '%s'. Supported providers: hmac, okta, google, azure", oauthProvider)
		}
		
		log.Printf("INFO: OAuth 2.1 authentication enabled (TRINO_OAUTH_ENABLED=true) with provider: %s", oauthProvider)
		if oauthProvider == "hmac" && jwtSecret == "" {
			log.Println("WARNING: JWT_SECRET not set for HMAC provider. Using insecure default for development only.")
		}
		if oauthProvider != "hmac" && oidcIssuer == "" {
			log.Printf("WARNING: OIDC_ISSUER not set for %s provider. OAuth authentication may fail.", oauthProvider)
		}
		if oauthRedirectURI != "" {
			log.Printf("INFO: Fixed OAuth redirect URI configured: %s", oauthRedirectURI)
		}
	}

	// Get Trino Source from env/config (no default)
	trinoSource := getEnv("TRINO_SOURCE", "")

	return &TrinoConfig{
		Host:              getEnv("TRINO_HOST", "localhost"),
		Port:              port,
		User:              getEnv("TRINO_USER", "trino"),
		Password:          getEnv("TRINO_PASSWORD", ""),
		Catalog:           getEnv("TRINO_CATALOG", "memory"),
		Schema:            getEnv("TRINO_SCHEMA", "default"),
		Scheme:            scheme,
		SSL:               ssl,
		SSLInsecure:       sslInsecure,
		AllowWriteQueries: allowWriteQueries,
		QueryTimeout:      queryTimeout,
		TrinoSource:       trinoSource,
		OAuthEnabled:      oauthEnabled,
		OAuthProvider:     oauthProvider,
		JWTSecret:         jwtSecret,
		OIDCIssuer:        oidcIssuer,
		OIDCAudience:      oidcAudience,
		OIDCClientID:      oidcClientID,
		OIDCClientSecret:  oidcClientSecret,
		OAuthRedirectURI:  oauthRedirectURI,
	}, nil
}

// getEnv retrieves an environment variable or returns a default value
func getEnv(key, fallback string) string {
	if value, exists := os.LookupEnv(key); exists {
		return value
	}
	return fallback
}
