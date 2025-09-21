# Authentication and Deployment Guide

## Transport Methods

The server supports two transport methods:

### STDIO Transport (Default)
- Direct integration with MCP clients
- Ideal for desktop applications like Claude Desktop
- Uses standard input/output for communication

### HTTP Transport with StreamableHTTP
- **Modern approach**: Uses the `/mcp` endpoint with StreamableHTTP protocol
- **Legacy support**: Maintains `/sse` endpoint for backward compatibility with SSE
- Supports web-based MCP clients
- Enables JWT authentication for secure access

## OAuth 2.0 Authentication

✅ **Production-Ready**: Complete OAuth 2.0 implementation with OIDC provider support for secure remote deployments.

**Supported Authentication Modes:**

1. **OIDC Provider Mode** (Production - Recommended)
   ```bash
   # Configure with OAuth provider (Okta example)
   export TRINO_OAUTH_ENABLED=true
   export OAUTH_PROVIDER=okta
   export OIDC_ISSUER=https://your-domain.okta.com
   export OIDC_AUDIENCE=your-service-audience
   export MCP_TRANSPORT=http
   mcp-trino
   ```

2. **HMAC Mode** (Development/Testing)
   ```bash
   # Simple JWT with shared secret
   export TRINO_OAUTH_ENABLED=true
   export OAUTH_PROVIDER=hmac
   export JWT_SECRET=your-secret-key-here
   export MCP_TRANSPORT=http
   mcp-trino
   ```

**Key Features:**
- **Multiple Providers**: Okta, Google, Azure AD, and custom OIDC providers
- **JWKS Validation**: Automatic key rotation and signature verification
- **Token Caching**: Performance optimization with 5-minute cache expiration
- **MCP Compliance**: Full OAuth 2.1 and MCP authorization specification support

Client requests must include the JWT token in the Authorization header:
```http
Authorization: Bearer <your-jwt-token>
```

For detailed OAuth configuration, deployment examples, and browser-based MCP client compatibility lessons learned, see [oauth.md](oauth.md).

## HTTPS Support

For production deployments with authentication, HTTPS is strongly recommended:

```bash
export HTTPS_CERT_FILE=/path/to/certificate.pem
export HTTPS_KEY_FILE=/path/to/private-key.pem
export TRINO_OAUTH_ENABLED=true
export MCP_TRANSPORT=http
mcp-trino
```

The server will automatically start with HTTPS when certificate files are provided.

## Remote MCP Server Deployment

Since the server supports JWT authentication and HTTP transport, you can deploy it as a remote MCP server accessible to multiple clients over the network.

> **Important**: When deploying a remote MCP server (behind a load balancer, reverse proxy, or with a public domain), you must set `MCP_URL` to the public base URL of your MCP server (including scheme and port if non-standard). This value is used in OAuth metadata and printed endpoints so clients discover the correct URLs.

### Production Deployment Example

```bash
# Deploy with HTTPS and JWT authentication
export MCP_TRANSPORT=http
export MCP_PORT=443
export MCP_URL=https://your-mcp-server.com
export TRINO_OAUTH_ENABLED=true
export HTTPS_CERT_FILE=/etc/ssl/certs/mcp-trino.pem
export HTTPS_KEY_FILE=/etc/ssl/private/mcp-trino.key
export TRINO_HOST=your-trino-cluster.com
export TRINO_PORT=443
export TRINO_USER=service-account
export TRINO_PASSWORD=service-password

# Start the server
mcp-trino
```

### Client Configuration for Remote Server

**With JWT Authentication:**
```json
{
  "mcpServers": {
    "remote-trino": {
      "url": "https://your-mcp-server.com/mcp",
      "headers": {
        "Authorization": "Bearer YOUR_JWT_TOKEN"
      }
    }
  }
}
```

**Load Balancer/Proxy Configuration:**
```nginx
server {
    listen 443 ssl;
    server_name your-mcp-server.com;

    ssl_certificate /etc/ssl/certs/mcp-trino.pem;
    ssl_certificate_key /etc/ssl/private/mcp-trino.key;

    location /mcp {
        proxy_pass http://localhost:8080;
        proxy_set_header Host $host;
        proxy_set_header X-Real-IP $remote_addr;
        proxy_set_header Authorization $http_authorization;
        proxy_http_version 1.1;
        proxy_set_header Upgrade $http_upgrade;
        proxy_set_header Connection "upgrade";
    }
}
```

### Docker Deployment

For containerized deployment:

```dockerfile
FROM ghcr.io/tuannvm/mcp-trino:latest

ENV MCP_TRANSPORT=http
ENV MCP_PORT=8080
ENV TRINO_OAUTH_ENABLED=true
ENV TRINO_HOST=your-trino-cluster.com
ENV TRINO_PORT=443
ENV TRINO_USER=service-account
ENV TRINO_PASSWORD=service-password

EXPOSE 8080

CMD ["mcp-trino"]
```

```bash
# Build and run with Docker
docker build -t mcp-trino-server .
docker run -d -p 8080:8080 \
  -e HTTPS_CERT_FILE=/certs/cert.pem \
  -e HTTPS_KEY_FILE=/certs/key.pem \
  -v /path/to/certs:/certs \
  mcp-trino-server
```

## Security Considerations

- **JWT Audience Validation**: The server enforces JWT audience claims to prevent cross-service token reuse
  - Audience must be explicitly configured via `OIDC_AUDIENCE` environment variable
  - Tokens must include the correct audience claim to be accepted
  - Prevents unauthorized access from other services using the same OAuth provider
- **JWT Token Management**: Implement proper token rotation and validation
- **Network Security**: Use HTTPS in production and consider network-level security
- **Access Control**: Implement proper authentication and authorization mechanisms
- **Monitoring**: Set up logging and monitoring for security events
- **Token Security**:
  - Never commit JWT secrets to version control
  - Use strong, randomly generated secrets (minimum 256 bits)
  - Implement short token expiration times with refresh mechanisms
  - Store tokens securely in client applications
- **Production Recommendations**:
  - Use asymmetric algorithms (RS256, ES256) instead of HS256
  - Implement proper issuer (`iss`) and audience (`aud`) validation
  - Use established OAuth 2.1/OpenID Connect providers
  - Implement token revocation mechanisms

## Quick Start with OAuth

**For Production (OIDC):**
```bash
# Configure OAuth provider
export TRINO_OAUTH_ENABLED=true
export OAUTH_PROVIDER=okta
export OIDC_ISSUER=https://your-domain.okta.com
export OIDC_AUDIENCE=https://your-domain.okta.com
export MCP_TRANSPORT=http

# Start server
mcp-trino
```

**For Development (HMAC):**
```bash
# Simple JWT testing
export TRINO_OAUTH_ENABLED=true
export OAUTH_PROVIDER=hmac
export JWT_SECRET="your-test-secret"
export MCP_TRANSPORT=http

# Start server
mcp-trino
```

**Client Configuration:**
```json
{
  "mcpServers": {
    "trino-oauth": {
      "url": "https://your-mcp-server.com/mcp",
      "headers": {
        "Authorization": "Bearer YOUR_JWT_TOKEN"
      }
    }
  }
}
```

## Configuration Reference

| Variable               | Description                       | Default   |
| ---------------------- | --------------------------------- | --------- |
| TRINO_HOST             | Trino server hostname             | localhost |
| TRINO_PORT             | Trino server port                 | 8080      |
| TRINO_USER             | Trino user                        | trino     |
| TRINO_PASSWORD         | Trino password                    | (empty)   |
| TRINO_CATALOG          | Default catalog                   | memory    |
| TRINO_SCHEMA           | Default schema                    | default   |
| TRINO_SCHEME           | Connection scheme (http/https)    | https     |
| TRINO_SSL              | Enable SSL                        | true      |
| TRINO_SSL_INSECURE     | Allow insecure SSL                | true      |
| TRINO_ALLOW_WRITE_QUERIES | Allow non-read-only SQL queries | false     |
| TRINO_QUERY_TIMEOUT    | Query timeout in seconds          | 30        |
| MCP_TRANSPORT          | Transport method (stdio/http)     | stdio     |
| MCP_PORT               | HTTP port for http transport      | 8080      |
| MCP_HOST               | Host for HTTP callbacks           | localhost |
| MCP_URL                | Public base URL of MCP server (used for OAuth metadata and client discovery); required for remote deployments | http://localhost:8080 |
| TRINO_OAUTH_ENABLED    | Enable OAuth authentication (auto-enabled when OAUTH_PROVIDER is set) | false |
| OAUTH_PROVIDER         | OAuth provider (hmac/okta/google/azure) - setting this enables OAuth | (empty) |
| JWT_SECRET             | JWT secret for HMAC mode          | (empty)   |
| OIDC_ISSUER            | OIDC provider issuer URL          | (empty)   |
| OIDC_AUDIENCE          | OIDC audience identifier (required for OIDC providers) | (empty - must be set) |
| OIDC_CLIENT_ID         | OIDC client ID                     | (empty)   |
| HTTPS_CERT_FILE        | Path to HTTPS certificate file    | (empty)   |
| HTTPS_KEY_FILE         | Path to HTTPS private key file    | (empty)   |

> **Note**: When `TRINO_SCHEME` is set to "https", `TRINO_SSL` is automatically set to true regardless of the provided value.

> **Important**: The default connection mode is HTTPS. If you're using an HTTP-only Trino server, you must set `TRINO_SCHEME=http` in your environment variables.

> **Security Note**: By default, only read-only queries (SELECT, SHOW, DESCRIBE, EXPLAIN) are allowed to prevent SQL injection. If you need to execute write operations or other non-read queries, set `TRINO_ALLOW_WRITE_QUERIES=true`, but be aware this bypasses this security protection.

> **For Web Client Integration**: When using with web clients, set `MCP_TRANSPORT=http` and connect to the `/mcp` endpoint for StreamableHTTP support. The `/sse` endpoint is maintained for backward compatibility.

> **OAuth Authentication**: When `TRINO_OAUTH_ENABLED=true`, the server supports multiple OAuth providers including OIDC-compliant providers (Okta, Google, Azure AD) for production use and HMAC mode for development/testing.

> **HTTPS Support**: For production deployments, configure HTTPS by setting `HTTPS_CERT_FILE` and `HTTPS_KEY_FILE` environment variables. This is strongly recommended when using JWT authentication.