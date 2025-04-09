# Trino MCP Server in Go

Model Context Protocol (MCP) server for Trino, reimplemented in Go.

## Overview

This project is a reimplementation of the Trino MCP Server in Go using the MCP SDK from [mark3labs/mcp-go](https://github.com/mark3labs/mcp-go). It enables AI models to access Trino's distributed SQL query engine through the Model Context Protocol.

Trino (formerly known as PrestoSQL) is a powerful distributed SQL query engine designed for fast analytics on large datasets, particularly beneficial in the adtech industry.

## Features

- ✅ MCP server implementation in Go
- ✅ Trino SQL query execution through MCP tools
- ✅ Catalog, schema, and table discovery
- ✅ Docker container support
- ✅ Reliable STDIO transport for LLM integration
- ✅ HTTP API endpoints for querying

## Prerequisites

- Go 1.19 or later
- Docker and Docker Compose (for containerized usage)
- A running Trino server (or use the provided Docker setup)

## Installation

1. Clone the repository:

```bash
git clone https://github.com/tuannvm/mcp-trino.git
cd mcp-trino
```

2. Build the server:

```bash
./scripts/build.sh
```

## Usage

### Standalone Usage

Run the server with STDIO transport (for direct LLM integration):

```bash
./scripts/run.sh
```

Or with HTTP transport:

```bash
MCP_TRANSPORT=http MCP_PORT=9097 ./scripts/run.sh
```

### Docker Usage

Start the server with Docker Compose:

```bash
docker-compose up -d
```

Verify the API is working:

```bash
curl -X POST "http://localhost:9097/api/query" \
     -H "Content-Type: application/json" \
     -d '{"query": "SELECT 1 AS test"}'
```

## MCP Tools

The server provides the following MCP tools:

1. `execute_query` - Execute a SQL query against Trino
2. `list_catalogs` - List all catalogs available in the Trino server
3. `list_schemas` - List all schemas in a catalog
4. `list_tables` - List all tables in a schema
5. `get_table_schema` - Get the schema of a table

## Configuration

The server can be configured using the following environment variables:

| Variable       | Description                   | Default   |
| -------------- | ----------------------------- | --------- |
| TRINO_HOST     | Trino server hostname         | localhost |
| TRINO_PORT     | Trino server port             | 8080      |
| TRINO_USER     | Trino user                    | trino     |
| TRINO_PASSWORD | Trino password                | (empty)   |
| TRINO_CATALOG  | Default catalog               | memory    |
| TRINO_SCHEMA   | Default schema                | default   |
| MCP_TRANSPORT  | Transport method (stdio/http) | stdio     |
| MCP_PORT       | HTTP port for http transport  | 9097      |

### Using `mcp.json`

You can integrate this MCP server with Cursor IDE by configuring the `mcp.json` file in your Cursor IDE settings directory (typically `~/.cursor/`).

Example `mcp.json` for Cursor IDE:

```json
{
	"mcpServers": {
		"mcp-liftoff-trino-adhoc": {
			"command": "mcp-trino",
			"args": [],
			"env": {
				"TRINO_HOST": "<HOST>",
				"TRINO_PORT": "<PORT>",
				"TRINO_USER": "<USER>",
				"TRINO_PASSWORD": "<PASSWORD>",
			}
		}
	}
}
```

Replace the placeholders:
- `<HOST>`: Your Trino server hostname
- `<PORT>`: Your Trino server port
- `<USER>`: Your Trino username
- `<PASSWORD>`: Your Trino password

After configuring this file, Cursor's AI assistant will be able to directly query your Trino database using natural language.

**Note:** When using the MCP server standalone, environment variables will override the settings specified in the `mcp.json` file if both are present.

## Development

1. Setup Go environment
2. Install dependencies:

```bash
go mod download
```

3. Run tests:

```bash
go test ./...
```

## License

MIT

## CI/CD and Releases

This project uses GitHub Actions for continuous integration and GoReleaser for automated releases.

### Continuous Integration

The CI pipeline runs automatically on pushes and pull requests to the main branch, performing:
- Static code analysis with golangci-lint
- Go dependency verification 
- Build validation
- Test execution with code coverage reporting

All CI checks must pass before a PR can be merged to the main branch. The repository is configured with branch protection rules to enforce this requirement.

### Release Process

The project uses an automated release process with a sequential workflow:

1. When changes are merged to the `main` branch, the CI workflow runs first to validate the code.

2. After the CI workflow completes successfully, the release workflow automatically:
   - Calculates the next version (starting from 1.0.0 and incrementing)
   - Creates and pushes a new version tag
   - Builds binaries for multiple platforms (named "mcp-trino")
   - Creates and pushes Docker images to GitHub Container Registry (ghcr.io)
   - Publishes all binaries and assets to GitHub Releases

You can find:
- Released binaries at: `https://github.com/tuannvm/mcp-trino/releases`
- Docker images at: `ghcr.io/tuannvm/mcp-trino:latest` or `ghcr.io/tuannvm/mcp-trino:v1.0.0`

No manual version tagging is required - just merge your changes to `main` and the release will be created automatically.

### Makefile

For convenience, a Makefile is provided with common development commands:

```bash
# Build the application
make build

# Run tests
make test

# Run linters (same as CI)
make lint

# Clean build artifacts
make clean

# Run in development mode
make run-dev

# Test GoReleaser locally (creates snapshot)
make release-snapshot

# Run the application
make run

# Docker operations
make run-docker          # Build and run in Docker
make docker-compose-up   # Start with Docker Compose
make docker-compose-down # Stop Docker Compose services
```
