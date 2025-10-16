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
	OAuthEnabled  bool   // Enable OAuth 2.1 authentication
	OAuthMode     string // OAuth operational mode: "native" or "proxy"
	OAuthProvider string // OAuth provider: "hmac", "okta", "google", "azure"
	JWTSecret     string // JWT signing secret for HMAC provider

	// OIDC provider configuration
	OIDCIssuer        string // OIDC issuer URL
	OIDCAudience      string // OIDC audience
	OIDCClientID      string // OIDC client ID
	OIDCClientSecret  string // OIDC client secret
	OAuthRedirectURIs string // OAuth redirect URIs - single URI or comma-separated list

	// Allowlist configuration for filtering catalogs, schemas, and tables
	AllowedCatalogs []string // List of allowed catalogs (empty means no filtering)
	AllowedSchemas  []string // List of allowed schemas in catalog.schema format
	AllowedTables   []string // List of allowed tables in catalog.schema.table format

	// Impersonation configuration
	EnableImpersonation bool   // Enable Trino user impersonation via X-Trino-User header
	ImpersonationField  string // JWT field to use for impersonation: "username", "email", or "subject" (default: "username")

	// Query attribution
	TrinoSource string // Value for X-Trino-Source header (identifies query source to Trino)
}

// NewTrinoConfig creates a new TrinoConfig with values from environment variables or defaults
func NewTrinoConfig() (*TrinoConfig, error) {
	return NewTrinoConfigWithVersion("dev")
}

// NewTrinoConfigWithVersion creates a new TrinoConfig with a specific version for X-Trino-Source
func NewTrinoConfigWithVersion(version string) (*TrinoConfig, error) {
	port, _ := strconv.Atoi(getEnv("TRINO_PORT", "8080"))
	ssl, _ := strconv.ParseBool(getEnv("TRINO_SSL", "true"))
	sslInsecure, _ := strconv.ParseBool(getEnv("TRINO_SSL_INSECURE", "true"))
	scheme := getEnv("TRINO_SCHEME", "https")
	allowWriteQueries, _ := strconv.ParseBool(getEnv("TRINO_ALLOW_WRITE_QUERIES", "false"))

	// OAuth configuration - OAUTH_ENABLED is the single source of truth
	oauthEnabled, _ := strconv.ParseBool(getEnv("OAUTH_ENABLED", "false"))
	oauthMode := strings.ToLower(getEnv("OAUTH_MODE", "native"))
	oauthProvider := strings.ToLower(getEnv("OAUTH_PROVIDER", "hmac"))
	jwtSecret := getEnv("JWT_SECRET", "")

	// OIDC configuration with secure defaults
	oidcIssuer := getEnv("OIDC_ISSUER", "")
	oidcAudience := getEnv("OIDC_AUDIENCE", "") // No default - must be explicitly configured
	oidcClientID := getEnv("OIDC_CLIENT_ID", "")
	oidcClientSecret := getEnv("OIDC_CLIENT_SECRET", "")

	// Redirect URI configuration with backward compatibility
	oauthRedirectURIs := getEnv("OAUTH_ALLOWED_REDIRECT_URIS", "")
	if oauthRedirectURIs == "" {
		deprecatedURI := getEnv("OAUTH_REDIRECT_URI", "")
		if deprecatedURI != "" {
			log.Println("WARNING: OAUTH_REDIRECT_URI is deprecated. Use OAUTH_ALLOWED_REDIRECT_URIS instead.")
			oauthRedirectURIs = deprecatedURI
		}
	}

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

	// Parse allowlist configuration
	allowedCatalogs := parseAllowlist(getEnv("TRINO_ALLOWED_CATALOGS", ""))
	allowedSchemas := parseAllowlist(getEnv("TRINO_ALLOWED_SCHEMAS", ""))
	allowedTables := parseAllowlist(getEnv("TRINO_ALLOWED_TABLES", ""))

	// Parse impersonation configuration
	enableImpersonation, _ := strconv.ParseBool(getEnv("TRINO_ENABLE_IMPERSONATION", "false"))
	impersonationField := strings.ToLower(getEnv("TRINO_IMPERSONATION_FIELD", "username"))

	// Parse Trino source configuration with default
	trinoSource := getEnv("TRINO_SOURCE", fmt.Sprintf("mcp-trino/%s", version))
	if trinoSource == "" {
		// If explicitly set to empty, use default
		trinoSource = fmt.Sprintf("mcp-trino/%s", version)
	}

	// Validate allowlist formats
	if err := validateAllowlist("TRINO_ALLOWED_SCHEMAS", allowedSchemas, 1); err != nil { // Must have catalog.schema format
		return nil, err
	}
	if err := validateAllowlist("TRINO_ALLOWED_TABLES", allowedTables, 2); err != nil { // Must have catalog.schema.table format
		return nil, err
	}

	// If using HTTPS, force SSL to true
	if strings.EqualFold(scheme, "https") {
		ssl = true
	}

	// Log a warning if write queries are allowed
	if allowWriteQueries {
		log.Println("WARNING: Write queries are enabled (TRINO_ALLOW_WRITE_QUERIES=true). SQL injection protection is bypassed.")
	}

	// Log OAuth status - detailed validation delegated to oauth-mcp-proxy
	if oauthEnabled {
		log.Printf("INFO: OAuth 2.1 enabled (mode: %s, provider: %s)", oauthMode, oauthProvider)

		// Keep helpful setup warnings for user experience
		if oauthProvider != "hmac" && oidcIssuer == "" {
			log.Printf("WARNING: OIDC_ISSUER not set for %s provider. OAuth may fail.", oauthProvider)
		}
		if oauthMode == "proxy" && oauthProvider != "hmac" && oidcClientSecret == "" {
			log.Printf("WARNING: OIDC_CLIENT_SECRET not set for proxy mode with %s provider.", oauthProvider)
		}
		if oauthMode == "proxy" && oauthRedirectURIs == "" {
			log.Printf("WARNING: No OAuth redirect URIs configured for proxy mode.")
		}
	} else {
		log.Println("INFO: OAuth disabled. Set OAUTH_ENABLED=true to activate.")
	}

	// Log allowlist configuration
	logAllowlistConfiguration(allowedCatalogs, allowedSchemas, allowedTables)

	// Validate impersonation field
	validFields := map[string]bool{"username": true, "email": true, "subject": true}
	if !validFields[impersonationField] {
		return nil, fmt.Errorf("invalid TRINO_IMPERSONATION_FIELD '%s'. Supported fields: username, email, subject", impersonationField)
	}

	// Log impersonation configuration
	if enableImpersonation {
		log.Printf("INFO: Trino user impersonation enabled (TRINO_ENABLE_IMPERSONATION=true)")
		log.Printf("INFO: Impersonation principal field: %s", impersonationField)
		if !oauthEnabled {
			log.Println("WARNING: Impersonation is enabled but OAuth is disabled. Impersonation requires OAuth to extract user information.")
		}
	} else {
		log.Println("INFO: Trino user impersonation disabled (TRINO_ENABLE_IMPERSONATION=false)")
	}

	// Log query attribution configuration
	log.Printf("INFO: Trino query source attribution: %s", trinoSource)

	return &TrinoConfig{
		Host:                getEnv("TRINO_HOST", "localhost"),
		Port:                port,
		User:                getEnv("TRINO_USER", "trino"),
		Password:            getEnv("TRINO_PASSWORD", ""),
		Catalog:             getEnv("TRINO_CATALOG", "memory"),
		Schema:              getEnv("TRINO_SCHEMA", "default"),
		Scheme:              scheme,
		SSL:                 ssl,
		SSLInsecure:         sslInsecure,
		AllowWriteQueries:   allowWriteQueries,
		QueryTimeout:        queryTimeout,
		OAuthEnabled:        oauthEnabled,
		OAuthMode:           oauthMode,
		OAuthProvider:       oauthProvider,
		JWTSecret:           jwtSecret,
		OIDCIssuer:          oidcIssuer,
		OIDCAudience:        oidcAudience,
		OIDCClientID:        oidcClientID,
		OIDCClientSecret:    oidcClientSecret,
		OAuthRedirectURIs:   oauthRedirectURIs,
		AllowedCatalogs:     allowedCatalogs,
		AllowedSchemas:      allowedSchemas,
		AllowedTables:       allowedTables,
		EnableImpersonation: enableImpersonation,
		ImpersonationField:  impersonationField,
		TrinoSource:         trinoSource,
	}, nil
}

// parseAllowlist parses a comma-separated allowlist from an environment variable
func parseAllowlist(value string) []string {
	if value == "" {
		return nil
	}

	// Split by comma and clean up entries
	items := strings.Split(value, ",")
	var result []string
	for _, item := range items {
		cleaned := strings.TrimSpace(item)
		if cleaned != "" {
			result = append(result, cleaned)
		}
	}
	return result
}

// validateAllowlist validates the format of allowlist entries
func validateAllowlist(envVar string, allowlist []string, expectedDots int) error {
	for _, item := range allowlist {
		dots := strings.Count(item, ".")
		if dots != expectedDots {
			return fmt.Errorf("invalid format in %s: '%s' (expected %d dots, found %d)",
				envVar, item, expectedDots, dots)
		}
	}
	return nil
}

// logAllowlistConfiguration logs the current allowlist configuration
func logAllowlistConfiguration(catalogs, schemas, tables []string) {
	if len(catalogs) > 0 || len(schemas) > 0 || len(tables) > 0 {
		log.Println("INFO: Trino allowlist configuration:")
		if len(catalogs) > 0 {
			log.Printf("  - Allowed catalogs: %s (%d configured)", strings.Join(catalogs, ", "), len(catalogs))
		}
		if len(schemas) > 0 {
			log.Printf("  - Allowed schemas: %s (%d configured)", strings.Join(schemas, ", "), len(schemas))
		}
		if len(tables) > 0 {
			log.Printf("  - Allowed tables: %s (%d configured)", strings.Join(tables, ", "), len(tables))
		}
	} else {
		log.Println("INFO: No Trino allowlists configured - all catalogs, schemas, and tables are accessible")
	}
}

// getEnv retrieves an environment variable or returns a default value
func getEnv(key, fallback string) string {
	if value, exists := os.LookupEnv(key); exists {
		return value
	}
	return fallback
}
