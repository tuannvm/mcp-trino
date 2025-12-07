# MCP Client Integrations

This MCP server can be integrated with several AI applications. Choose the integration method that best suits your needs.

## Using Docker Image

To use the Docker image instead of a local binary:

```json
{
  "mcpServers": {
    "mcp-trino": {
      "command": "docker",
      "args": ["run", "--rm", "-i",
               "-e", "TRINO_HOST=<HOST>",
               "-e", "TRINO_PORT=<PORT>",
               "-e", "TRINO_USER=<USERNAME>",
               "-e", "TRINO_PASSWORD=<PASSWORD>",
               "-e", "TRINO_SCHEME=http",
               "ghcr.io/tuannvm/mcp-trino:latest"],
      "env": {}
    }
  }
}
```

> **Note**: The `host.docker.internal` special DNS name allows the container to connect to services running on the host machine. If your Trino server is running elsewhere, replace with the appropriate host.

This Docker configuration can be used in any of the below applications.

## Cursor

To use with [Cursor](https://cursor.sh/), create or edit `~/.cursor/mcp.json`:

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

Replace the environment variables with your specific Trino configuration.

For HTTP+StreamableHTTP transport mode (recommended for web clients):

```json
{
  "mcpServers": {
    "mcp-trino-http": {
      "url": "http://localhost:8080/mcp"
    }
  }
}
```

For remote MCP server with JWT authentication:

```json
{
  "mcpServers": {
    "mcp-trino-remote": {
      "url": "https://your-mcp-server.com/mcp",
      "headers": {
        "Authorization": "Bearer YOUR_JWT_TOKEN"
      }
    }
  }
}
```

For backward compatibility with SSE (legacy endpoint):

```json
{
  "mcpServers": {
    "mcp-trino-sse": {
      "url": "http://localhost:8080/sse"
    }
  }
}
```

Then start the server in a separate terminal with:

```bash
# Basic HTTP transport
MCP_TRANSPORT=http TRINO_HOST=<HOST> TRINO_PORT=<PORT> TRINO_USER=<USERNAME> TRINO_PASSWORD=<PASSWORD> mcp-trino

# With JWT authentication enabled
MCP_TRANSPORT=http OAUTH_ENABLED=true TRINO_HOST=<HOST> TRINO_PORT=<PORT> TRINO_USER=<USERNAME> TRINO_PASSWORD=<PASSWORD> mcp-trino

# Production deployment with HTTPS
MCP_TRANSPORT=http OAUTH_ENABLED=true HTTPS_CERT_FILE=/path/to/cert.pem HTTPS_KEY_FILE=/path/to/key.pem TRINO_HOST=<HOST> TRINO_PORT=<PORT> TRINO_USER=<USERNAME> TRINO_PASSWORD=<PASSWORD> mcp-trino
```

## Claude Desktop

To use with [Claude Desktop](https://claude.ai/desktop), the easiest way is to use the install script which will automatically configure it for you. Alternatively, you can manually edit your Claude configuration file:

- macOS: `~/Library/Application Support/Claude/claude_desktop_config.json`
- Windows: `%APPDATA%\Claude\claude_desktop_config.json`
- Linux: `~/.config/Claude/claude_desktop_config.json`

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

After updating the configuration, restart Claude Desktop. You should see the MCP tools available in the tools menu.

## Claude Code

To use with [Claude Code](https://claude.ai/code), the install script will automatically configure it for you. Alternatively, you can manually add the MCP server:

```bash
claude mcp add mcp-trino mcp-trino
```

Then set your environment variables:

```bash
export TRINO_HOST=<HOST>
export TRINO_PORT=<PORT>
export TRINO_USER=<USERNAME>
export TRINO_PASSWORD=<PASSWORD>
```

Restart Claude Code to see the MCP tools available.

## Windsurf

To use with [Windsurf](https://windsurf.com/refer?referral_code=sjqdvqozgx2wyi7r), create or edit your `mcp_config.json`:

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

Restart Windsurf to apply the changes. The Trino MCP tools will be available to the Cascade AI.

## ChatWise

To use with [ChatWise](https://chatwise.app?atp=uo1wzc), follow these steps:

### Local MCP Server:
1. Open ChatWise and go to Settings
2. Navigate to the Tools section
3. Click the "+" icon to add a new tool
4. Select "Command Line MCP"
5. Configure with the following details:
   - ID: `mcp-trino` (or any name you prefer)
   - Command: `mcp-trino`
   - Args: (leave empty)
   - Env: Add the following environment variables:
     ```
     TRINO_HOST=<HOST>
     TRINO_PORT=<PORT>
     TRINO_USER=<USERNAME>
     TRINO_PASSWORD=<PASSWORD>
     ```

### Remote MCP Server:
For remote MCP servers with JWT authentication:

1. Copy this JSON to your clipboard:
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
2. In ChatWise Settings > Tools, click the "+" icon
3. Select "Import JSON from Clipboard"
4. Toggle the switch next to the tool to enable it

Alternatively, you can import the local configuration from JSON:

1. Copy this JSON to your clipboard:
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
2. In ChatWise Settings > Tools, click the "+" icon
3. Select "Import JSON from Clipboard"
4. Toggle the switch next to the tool to enable it

Once enabled, click the hammer icon below the input box in ChatWise to access Trino MCP tools.