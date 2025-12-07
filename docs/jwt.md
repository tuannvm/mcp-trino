# JWT Authentication Implementation

> **Implementation:** JWT authentication is provided by [oauth-mcp-proxy](https://github.com/tuannvm/oauth-mcp-proxy).
>
> **For JWT configuration, validation logic, and security details**, see the [oauth-mcp-proxy documentation](https://github.com/tuannvm/oauth-mcp-proxy#readme).

This document describes the JWT-based authentication architecture for mcp-trino server, providing secure access control at the server level.

## Overview

The mcp-trino server implements JWT Bearer token authentication with server-level request interception, ensuring **complete API protection** for all MCP methods. This approach provides security for the entire API surface, not just individual tools.

## Architecture

### Authentication Flow

```mermaid
sequenceDiagram
    participant Client
    participant HTTPServer as HTTP Server
    participant AuthHook as Auth Hook
    participant MCPServer as MCP Server
    participant TrinoClient as Trino Client

    Client->>HTTPServer: POST /mcp<br/>Authorization: Bearer <jwt-token>
    HTTPServer->>HTTPServer: Extract token from headers
    HTTPServer->>AuthHook: OnRequestInitialization(ctx, id, message)
    AuthHook->>AuthHook: Validate JWT token
    
    alt Valid Token
        AuthHook->>AuthHook: Extract user claims<br/>(sub, preferred_username, email)
        AuthHook-->>HTTPServer: ✅ Authentication Success
        HTTPServer->>MCPServer: Process MCP Request
        MCPServer->>TrinoClient: Execute operations
        TrinoClient-->>Client: Results
    else Invalid/Missing Token
        AuthHook-->>HTTPServer: ❌ Authentication Failed
        HTTPServer-->>Client: 400 Bad Request<br/>{"error": "authentication required"}
    end
```

**Key Flow Steps:**

1. **HTTP Request**: Client sends request with `Authorization: Bearer <jwt-token>` header
2. **Token Extraction**: Server extracts token from headers into request context
3. **Server-Level Authentication**: Authentication hook validates token before any processing
4. **Request Processing**: If authenticated, request proceeds to appropriate MCP handler

## Security Model

### Complete API Protection
- **All MCP Methods Protected**: Every API endpoint requires authentication
- **Server-Level Enforcement**: Authentication applied before method-specific processing
- **Early Termination**: Invalid requests rejected immediately
- **Context Propagation**: User information available throughout request lifecycle

### JWT Validation Features
- **Signature Verification**: Proper HMAC-SHA256 signature validation
- **Claims Validation**: Required claims checking (sub, exp, iat)
- **Token Caching**: Performance optimization with secure secret caching
- **Secure Token Logging**: JWT tokens logged as SHA256 hashes to prevent exposure
- **Mandatory JWT_SECRET**: Server fails to start without JWT_SECRET in HMAC mode

## Configuration

### Environment Variables

```bash
# Authentication Configuration
OAUTH_ENABLED=true        # Default: true (secure by default)
JWT_SECRET=your-secret-key      # JWT signing secret (REQUIRED - server fails without it)

# Transport Configuration
MCP_TRANSPORT=http              # Enable HTTP transport
MCP_PORT=8080                   # Server port
```

### Required JWT Claims

JWT tokens must include the following claims:
- **sub** (subject): Required user identifier
- **preferred_username**: Username for logging and display
- **email**: User email address
- **exp** (expiration): Token expiration timestamp
- **iat** (issued at): Token issuance timestamp

## Protected API Surface

With server-level authentication, **ALL MCP methods** are protected:

- ✅ `initialize` - Session establishment  
- ✅ `tools/list` - List available tools
- ✅ `tools/call` - Execute tools
- ✅ `resources/list` - List available resources
- ✅ `resources/read` - Read resources
- ✅ `prompts/list` - List available prompts
- ✅ `prompts/get` - Get prompt templates
- ✅ **All other MCP methods**

## Transport Endpoints

### Dual Endpoint Support

The server supports both modern and legacy endpoints for backward compatibility:

| Endpoint | Status | Description |
|----------|---------|-------------|
| `/mcp` | ✅ **Recommended** | Modern StreamableHTTP endpoint |
| `/sse` | ✅ **Legacy Support** | Backward compatibility for existing clients |

### Client Configuration

**Modern Endpoint (Recommended):**
```json
{
  "mcpServers": {
    "trino-jwt": {
      "url": "https://your-server.com/mcp",
      "headers": {
        "Authorization": "Bearer YOUR_JWT_TOKEN"
      }
    }
  }
}
```

**Legacy Endpoint (Backward Compatibility):**
```json
{
  "mcpServers": {
    "trino-jwt": {
      "url": "https://your-server.com/sse",
      "headers": {
        "Authorization": "Bearer YOUR_JWT_TOKEN"
      }
    }
  }
}
```

## Security Features

### Authentication Enforcement
- **Server-Level Security**: Authentication applied before any request processing
- **No Bypass Routes**: Every MCP method requires authentication
- **Proper Error Handling**: Clear error messages for authentication failures
- **Debug Logging**: Comprehensive logging for troubleshooting

### Token Security
- **Hash-Based Logging**: JWT tokens logged as SHA256 hashes to prevent sensitive data exposure
- **Secret Enforcement**: Server startup blocked without proper JWT_SECRET configuration
- **Secret Caching**: Efficient JWT secret management with sync.Once pattern
- **Signature Verification**: Proper HMAC-SHA256 validation
- **Claims Validation**: Required claims verification
- **Context Management**: Secure token and user information storage

## Testing and Validation

### Authentication Testing
- **Valid Token Test**: Authenticated requests should succeed
- **Invalid Token Test**: Unauthenticated requests should be blocked
- **Missing Token Test**: Requests without tokens should be rejected
- **Malformed Token Test**: Corrupted tokens should be handled gracefully

### Expected Behavior
- ❌ **Unauthenticated requests**: Blocked with "authentication required"
- ✅ **Authenticated requests**: Allowed with proper JWT token
- 🔒 **All API methods**: Protected uniformly across the entire surface

## Migration Considerations

### From Tool-Only Middleware
If migrating from tool-specific middleware:
1. Remove tool-specific middleware configuration
2. Add server-level hooks for complete API protection
3. Test all MCP methods for proper authentication
4. Update client configurations to include authentication headers

### From SSE Transport
If migrating from Server-Sent Events:
1. Replace SSE server with StreamableHTTP server
2. Update client endpoints from `/sse` to `/mcp` (optional)
3. Maintain backward compatibility if needed
4. Test session management compatibility

## Production Considerations

### Security Requirements
- **HTTPS Required**: JWT authentication should always use HTTPS in production
- **Strong Secrets**: Use cryptographically strong JWT secrets (minimum 256 bits)
- **Mandatory Configuration**: JWT_SECRET required for HMAC mode (server fails without it)
- **Secure Logging**: JWT tokens logged as hashes to prevent sensitive data exposure
- **Token Expiration**: Implement appropriate token lifetimes
- **Rate Limiting**: Consider adding rate limiting middleware
- **Audit Logging**: Log authentication attempts and failures

### Performance Optimizations
- **Secret Caching**: JWT secret cached for performance
- **Context Efficiency**: Minimal overhead for token validation
- **Early Termination**: Invalid requests rejected quickly
- **Session Management**: Proper MCP session handling

## Troubleshooting

### Common Issues
- **"authentication required"**: Missing or malformed Authorization header
- **"failed to parse token"**: JWT token corrupted or invalid format
- **"missing subject in token"**: JWT missing required `sub` claim
- **"unexpected signing method"**: Token signed with unsupported algorithm

### Debug Information
Enable detailed logging to see:
- Token extraction from headers
- JWT validation results (tokens logged as secure hashes)
- User authentication status
- Request processing flow
- SHA256 token hashes for debugging without exposing sensitive data

## Implementation Status

✅ **Complete JWT Implementation**
- Server-level authentication with complete API protection
- Secure JWT validation with proper signature verification
- Modern StreamableHTTP transport with backward compatibility
- Comprehensive testing framework and client integration
- Production-ready security features and error handling

The JWT authentication implementation provides robust, server-level security for the mcp-trino server with modern transport protocols and comprehensive API protection.