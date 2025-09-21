# Installation Guide

## Quick Install (One-liner)

For macOS and Linux, install with a single command:

```bash
curl -fsSL https://raw.githubusercontent.com/tuannvm/mcp-trino/main/install.sh -o install.sh && chmod +x install.sh && ./install.sh
```

## Homebrew (macOS and Linux)

The easiest way to install mcp-trino is using Homebrew:

```bash
# Install mcp-trino
brew install tuannvm/mcp/mcp-trino
```

To update to the latest version:

```bash
brew update && brew upgrade mcp-trino
```

## Alternative Installation Methods

### Manual Download

1. Download the appropriate binary for your platform from the [GitHub Releases](https://github.com/tuannvm/mcp-trino/releases) page.
2. Place the binary in a directory included in your PATH (e.g., `/usr/local/bin` on Linux/macOS)
3. Make it executable (`chmod +x mcp-trino` on Linux/macOS)

### From Source

```bash
git clone https://github.com/tuannvm/mcp-trino.git
cd mcp-trino
make build
# Binary will be in ./bin/
```

## Downloads

You can download pre-built binaries for your platform:

| Platform | Architecture | Download Link |
|----------|--------------|---------------|
| macOS | x86_64 (Intel) | [Download](https://github.com/tuannvm/mcp-trino/releases/latest/download/mcp-trino-darwin-amd64) |
| macOS | ARM64 (Apple Silicon) | [Download](https://github.com/tuannvm/mcp-trino/releases/latest/download/mcp-trino-darwin-arm64) |
| Linux | x86_64 | [Download](https://github.com/tuannvm/mcp-trino/releases/latest/download/mcp-trino-linux-amd64) |
| Linux | ARM64 | [Download](https://github.com/tuannvm/mcp-trino/releases/latest/download/mcp-trino-linux-arm64) |
| Windows | x86_64 | [Download](https://github.com/tuannvm/mcp-trino/releases/latest/download/mcp-trino-windows-amd64.exe) |

Or see all available downloads on the [GitHub Releases](https://github.com/tuannvm/mcp-trino/releases) page.

## Installation Troubleshooting

If you encounter issues during installation:

**Common Issues:**
- **Binary not found in PATH**: The install script installs to `~/.local/bin` by default. Make sure this directory is in your PATH:
  ```bash
  export PATH="$HOME/.local/bin:$PATH"
  ```
  Add this to your shell profile (`.bashrc`, `.zshrc`, etc.) to make it permanent.

- **Permission denied**: If you get permission errors, ensure the install directory is writable:
  ```bash
  mkdir -p ~/.local/bin
  chmod 755 ~/.local/bin
  ```

- **Claude configuration not found**: If the install script doesn't detect your Claude installation:
  - For Claude Desktop: Check if the config file exists at the expected location
  - For Claude Code: Verify the `claude` command is available in PATH
  - Use the manual configuration instructions provided by the script

- **GitHub API rate limiting**: If you're hitting GitHub API rate limits:
  ```bash
  export GITHUB_TOKEN=your_github_token
  curl -fsSL https://raw.githubusercontent.com/tuannvm/mcp-trino/main/install.sh | bash
  ```

**Getting Help:**
- Check the [GitHub Issues](https://github.com/tuannvm/mcp-trino/issues) for similar problems
- Run the install script with `--help` for usage information
- Use manual installation methods if the automated script fails