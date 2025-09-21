# Trino MCP Server in Go

A high-performance Model Context Protocol (MCP) server for Trino implemented in Go. This project enables AI assistants to seamlessly interact with Trino's distributed SQL query engine through standardized MCP tools.

[![GitHub Workflow Status](https://img.shields.io/github/actions/workflow/status/tuannvm/mcp-trino/build.yml?branch=main&label=CI%2FCD&logo=github)](https://github.com/tuannvm/mcp-trino/actions/workflows/build.yml)
[![Go Version](https://img.shields.io/github/go-mod/go-version/tuannvm/mcp-trino?logo=go)](https://github.com/tuannvm/mcp-trino/blob/main/go.mod)
[![Trivy Scan](https://img.shields.io/github/actions/workflow/status/tuannvm/mcp-trino/build.yml?branch=main&label=Trivy%20Security%20Scan&logo=aquasec)](https://github.com/tuannvm/mcp-trino/actions/workflows/build.yml)
[![SLSA 3](https://slsa.dev/images/gh-badge-level3.svg)](https://slsa.dev)
[![Go Report Card](https://goreportcard.com/badge/github.com/tuannvm/mcp-trino)](https://goreportcard.com/report/github.com/tuannvm/mcp-trino)
[![Go Reference](https://pkg.go.dev/badge/github.com/tuannvm/mcp-trino.svg)](https://pkg.go.dev/github.com/tuannvm/mcp-trino)
[![Docker Image](https://img.shields.io/github/v/release/tuannvm/mcp-trino?sort=semver&label=GHCR&logo=docker)](https://github.com/tuannvm/mcp-trino/pkgs/container/mcp-trino)
[![GitHub Release](https://img.shields.io/github/v/release/tuannvm/mcp-trino?sort=semver)](https://github.com/tuannvm/mcp-trino/releases/latest)
[![License: MIT](https://img.shields.io/badge/License-MIT-yellow.svg)](https://opensource.org/licenses/MIT)

[![Trust Score](https://archestra.ai/mcp-catalog/api/badge/quality/tuannvm/mcp-trino)](https://archestra.ai/mcp-catalog/tuannvm__mcp-trino)


## Overview

This project implements a Model Context Protocol (MCP) server for Trino in Go. It enables AI assistants to access Trino's distributed SQL query engine through standardized MCP tools.

Trino (formerly PrestoSQL) is a powerful distributed SQL query engine designed for fast analytics on large datasets.

## Architecture

```mermaid
graph TB
    subgraph "AI Clients"
        CC[Claude Code]
        CD[Claude Desktop]
        CR[Cursor]
        WS[Windsurf]
        CW[ChatWise]
    end
    
    subgraph "Authentication (Optional)"
        OP[OAuth Provider<br/>Okta/Google/Azure AD]
        JWT[JWT Tokens]
    end
    
    subgraph "MCP Server (mcp-trino)"
        HTTP[HTTP Transport<br/>/mcp endpoint]
        STDIO[STDIO Transport]
        AUTH[OAuth Middleware]
        TOOLS[MCP Tools<br/>• execute_query<br/>• list_catalogs<br/>• list_schemas<br/>• list_tables<br/>• get_table_schema<br/>• explain_query]
    end
    
    subgraph "Data Layer"
        TRINO[Trino Cluster<br/>Distributed SQL Engine]
        CATALOGS[Data Sources<br/>• PostgreSQL<br/>• MySQL<br/>• S3/Hive<br/>• BigQuery<br/>• MongoDB]
    end
    
    %% Connections
    CC -.->|OAuth Flow| OP
    OP -.->|JWT Token| JWT
    
    CC -->|HTTP + JWT| HTTP
    CD -->|STDIO| STDIO
    CR -->|HTTP + JWT| HTTP
    WS -->|STDIO| STDIO
    CW -->|HTTP + JWT| HTTP
    
    HTTP --> AUTH
    AUTH -->|Validated| TOOLS
    STDIO --> TOOLS
    
    TOOLS -->|SQL Queries| TRINO
    TRINO --> CATALOGS
    
    %% Styling
    classDef client fill:#e1f5fe
    classDef auth fill:#f3e5f5
    classDef server fill:#e8f5e8
    classDef data fill:#fff3e0
    
    class CC,CD,CR,WS,CW client
    class OP,JWT auth
    class HTTP,STDIO,AUTH,TOOLS server
    class TRINO,CATALOGS data
```

**Key Components:**
- **AI Clients**: Various MCP-compatible applications
- **Authentication**: Optional OAuth 2.0 with OIDC providers
- **MCP Server**: Go-based server with dual transport support
- **Data Layer**: Trino cluster connecting to multiple data sources

## Features

- ✅ MCP server implementation in Go
- ✅ Trino SQL query execution through MCP tools
- ✅ Catalog, schema, and table discovery
- ✅ Docker container support
- ✅ Supports both STDIO and HTTP transports
- ✅ OAuth 2.0 authentication with OIDC provider support (Okta, Google, Azure AD)
- ✅ StreamableHTTP support with JWT authentication (upgraded from SSE)
- ✅ Backward compatibility with SSE endpoints
- ✅ Compatible with Cursor, Claude Desktop, Windsurf, ChatWise, and any MCP-compatible clients.

## Installation

For detailed installation instructions, see [Installation Guide](docs/installation.md).

### Quick Start

**One-liner install (macOS/Linux):**
```bash
curl -fsSL https://raw.githubusercontent.com/tuannvm/mcp-trino/main/install.sh -o install.sh && chmod +x install.sh && ./install.sh
```

**Homebrew:**
```bash
brew install tuannvm/mcp/mcp-trino
```

**Manual download:** Get binaries from [GitHub Releases](https://github.com/tuannvm/mcp-trino/releases).

## Quick Start

### Local Development (No Authentication)
```bash
export TRINO_HOST=localhost
export TRINO_PORT=8080
export TRINO_USER=trino
mcp-trino
```

### Production with OAuth
```bash
export OAUTH_PROVIDER=okta
export OIDC_ISSUER=https://your-domain.okta.com
export OIDC_AUDIENCE=your-service-audience
export MCP_TRANSPORT=http
mcp-trino
```

For detailed deployment and authentication options, see [Deployment Guide](docs/deployment.md).

## MCP Client Integration

For detailed integration instructions with various MCP clients, see [Integration Guide](docs/integrations.md).

**Supported Clients:**
- Claude Desktop
- Claude Code
- Cursor
- Windsurf
- ChatWise

**Quick Example (Claude Desktop):**
```json
{
  "mcpServers": {
    "mcp-trino": {
      "command": "mcp-trino",
      "args": [],
      "env": {
        "TRINO_HOST": "<HOST>",
        "TRINO_PORT": "<PORT>",
        "TRINO_USER": "<USERNAME>",
        "TRINO_PASSWORD": "<PASSWORD>"
      }
    }
  }
}
```

## Available MCP Tools

For detailed tool documentation and examples, see [Tools Reference](docs/tools.md).

**Available Tools:**
- `execute_query` - Execute SQL queries
- `list_catalogs` - Discover data catalogs
- `list_schemas` - List schemas in catalogs
- `list_tables` - List tables in schemas
- `get_table_schema` - Get table structure
- `explain_query` - Analyze query execution plans

**Quick Example:**
```json
{
  "query": "SELECT region, COUNT(*) FROM tpch.tiny.customer GROUP BY region"
}
```

## Configuration

Key environment variables:

| Variable | Description | Default |
|----------|-------------|----------|
| TRINO_HOST | Trino server hostname | localhost |
| TRINO_PORT | Trino server port | 8080 |
| TRINO_USER | Trino user | trino |
| TRINO_SCHEME | Connection scheme (http/https) | https |
| MCP_TRANSPORT | Transport method (stdio/http) | stdio |
| OAUTH_PROVIDER | OAuth provider (hmac/okta/google/azure) | (empty) |
| OIDC_AUDIENCE | OIDC audience identifier | (empty) |

For complete configuration reference, see [Deployment Guide](docs/deployment.md).

## Contributing

Contributions are welcome! Please feel free to submit a Pull Request.

## License

This project is licensed under the MIT License - see the LICENSE file for details.

## CI/CD and Releases

This project uses GitHub Actions for continuous integration and GoReleaser for automated releases.

### Continuous Integration Checks

Our CI pipeline performs the following checks on all PRs and commits to the main branch:

#### Code Quality
- **Linting**: Using golangci-lint to check for common code issues and style violations
- **Go Module Verification**: Ensuring go.mod and go.sum are properly maintained
- **Formatting**: Verifying code is properly formatted with gofmt

#### Security
- **Vulnerability Scanning**: Using govulncheck to check for known vulnerabilities in dependencies
- **Dependency Scanning**: Using Trivy to scan for vulnerabilities in dependencies (CRITICAL, HIGH, and MEDIUM)
- **SBOM Generation**: Creating a Software Bill of Materials for dependency tracking
- **SLSA Provenance**: Creating verifiable build provenance for supply chain security

#### Testing
- **Unit Tests**: Running tests with race detection and code coverage reporting
- **Build Verification**: Ensuring the codebase builds successfully

#### CI/CD Security
- **Least Privilege**: Workflows run with minimum required permissions
- **Pinned Versions**: All GitHub Actions use specific versions to prevent supply chain attacks
- **Dependency Updates**: Automated dependency updates via Dependabot

### Release Process

When changes are merged to the main branch:

1. CI checks are run to validate code quality and security
2. If successful, a new release is automatically created with:
   - Semantic versioning based on commit messages
   - Binary builds for multiple platforms
   - Docker image publishing to GitHub Container Registry
   - SBOM and provenance attestation
