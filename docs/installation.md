# Installation & Setup Guide

Mimir is distributed as a single static binary. You can install it using several methods depending on your environment and preference.

## 1. Automated Installation (Recommended)

For Linux and macOS users, the fastest way is to use the one-line installer. This script automatically detects your OS and architecture (Intel or Apple Silicon/ARM) and installs the correct binary to `/usr/local/bin`.

```bash
curl -fsSL https://raw.githubusercontent.com/thuongh2/git-mimir/main/install.sh | sh
```

## 2. Manual Download

If you prefer to install manually or are on a system where the script doesn't work:

1.  Go to the [GitHub Releases](https://github.com/thuongh2/git-mimir/releases) page.
2.  Download the tarball corresponding to your system:
    -   `mimir-linux-amd64.tar.gz` (Standard Linux)
    -   `mimir-linux-arm64.tar.gz` (Linux on ARM/Raspberry Pi)
    -   `mimir-darwin-amd64.tar.gz` (Intel Mac)
    -   `mimir-darwin-arm64.tar.gz` (Apple Silicon Mac)
3.  Extract the archive: `tar -xzf mimir-*.tar.gz`.
4.  Move the `mimir` binary to a directory in your `$PATH` (e.g., `/usr/local/bin`).

## 3. Go Toolchain (For Developers)

If you have Go 1.22+ installed, you can compile from source or use `go install`:

```bash
# Global install
go install github.com/thuongh2/git-mimir/cmd/mimir@latest

# Or clone and build manually
git clone https://github.com/thuongh2/git-mimir.git
cd git-mimir
make build
```

## 4. Post-Installation Verification

Run the following command to verify the installation:

```bash
mimir --version
```

## 5. Setting up MCP for AI Agents

Mimir is designed to work perfectly with Claude Code and other MCP-compatible clients.

### Automatic Setup
Run the setup command to automatically configure your editor (VS Code, Claude Desktop, etc.):

```bash
mimir setup
```

### Manual Configuration
If automatic setup isn't supported for your tool, add the following to your MCP configuration file (usually `claude_desktop_config.json`):

```json
{
  "mcpServers": {
    "mimir": {
      "command": "mimir",
      "args": ["mcp"]
    }
  }
}
```

## 6. Required Dependencies for Full Features

-   **Search & Graph**: No extra dependencies.
-   **Semantic Search (Vector)**: Requires an embedding provider. Mimir supports:
    -   **Ollama (Local)**: Recommended for privacy. Just run `ollama run nomic-embed-text`.
    -   **OpenAI**: Set your `OPENAI_API_KEY` environment variable.
