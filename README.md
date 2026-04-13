# Mimir — Code Intelligence for AI Agents

Mimir is a **Go-based code intelligence engine** that indexes your repository into a knowledge graph and exposes it to AI agents via MCP, HTTP API, and an interactive web UI.

---

## What is Mimir?

Mimir builds a searchable knowledge graph of your codebase that helps AI agents (and humans) understand it faster.

| Use Case | How Mimir Helps |
|---|---|
| **AI Agents (MCP)** | 7 tools for semantic search, impact analysis, and blast radius queries |
| **Humans (Web UI)** | Interactive force-directed graph to explore code structure visually |
| **Teams (Refactors)** | Impact analysis before editing — see what breaking changes affect |

### Why Mimir?

| Metric | GitNexus (Node.js) | Mimir (Go) |
|---|---|---|
| Cold index (1,000 files) | ~45s | **< 8s** |
| Incremental re-index | ~45s (full) | **< 2s** |
| Hybrid search latency | ~300ms | **< 80ms** |
| Binary size | 350 MB | **< 20 MB** |
| RAM during index | ~800 MB | **< 120 MB** |

**Key improvements:** Pure Go (zero CGO), 8-language support (TS, JS, Go, Python, Rust, Java, C, C++), incremental indexing via git-diff, Louvain community detection, embedded web UI.

---

## Quick Start

### Step 1: Install

```bash
# One-line (Linux/macOS)
curl -fsSL https://raw.githubusercontent.com/thuongh2/git-mimir/main/install.sh | sh

# Or via Go
go install github.com/thuongh2/git-mimir/cmd/mimir@latest

# Or download binary from GitHub Releases
```

### Step 2: Index Your Code

```bash
# Full index with embeddings (requires ollama or OpenAI)
mimir analyze /path/to/your-project

# Faster — graph + BM25 only (no embeddings)
mimir analyze --skip-embeddings /path/to/your-project

# Skip auto-setup for manual control
mimir analyze --skip-daemon --skip-hooks --skip-skills /path/to/your-project
```

### Step 3: Use It

**Option A: Web UI (Humans)**
```bash
mimir serve
# Open http://localhost:7842
```

**Option B: MCP (AI Agents)**
```bash
mimir daemon start    # Start background MCP server
mimir setup           # Auto-configure Claude Code, VS Code, Cursor, etc.
```

---

## Installation

| Method | Command |
|---|---|
| **One-line (Linux/macOS)** | `curl -fsSL https://raw.githubusercontent.com/thuongh2/git-mimir/main/install.sh | sh` |
| **Go Install** | `go install github.com/thuongh2/git-mimir/cmd/mimir@latest` |
| **Download Binary** | [GitHub Releases](https://github.com/thuongh2/git-mimir/releases) |
| **From Source** | `git clone && cd git-mimir && make build` |

For detailed installation guide, see [docs/installation.md](docs/installation.md).

---

## CLI Reference

| Command | Description |
|---|---|
| `mimir analyze <path>` | Index repository with auto-setup |
| `mimir serve` | Start HTTP server + Web UI (default: port 7842) |
| `mimir mcp` | Start MCP stdio server |
| `mimir daemon start/stop/status` | Manage background MCP daemon |
| `mimir daemon logs [lines]` | View daemon logs (default: 30 lines) |
| `mimir list` | List indexed repositories |
| `mimir status [name]` | Show index status |
| `mimir clean <name>` | Remove index for a repository |
| `mimir setup` | Configure MCP in all supported editors |
| `mimir wiki [name]` | Generate wiki from knowledge graph |

### Analyze Flags

```bash
mimir analyze --skip-embeddings <path>   # Skip embeddings (faster)
mimir analyze --skip-daemon <path>       # Don't start MCP daemon
mimir analyze --skip-hooks <path>        # Don't install Claude Code hooks
mimir analyze --skip-skills <path>       # Don't install agent skills
mimir analyze --force <path>             # Force full re-index
mimir analyze --incremental <path>       # Force incremental mode
mimir analyze --hint <file> <path>       # Hint for faster patch planning
```

---

## MCP Tools

AI agents get access to 12 tools via MCP:

| Tool | Description | Example |
|---|---|---|
| `query` | Hybrid search (BM25 + vector) | "Find all auth-related processes" |
| `context` | 360-degree symbol view | "Show handleRequest definition and callers" |
| `find_referencing` | Who directly calls/imports/extends a symbol (1-hop) | "What calls UserService.GetUser?" |
| `symbol_coordinates` | Exact file path + line range for a symbol | "Where is ProcessOrder defined?" |
| `get_symbols_overview` | All top-level symbols in a file, sorted by line | "What's exported from store.go?" |
| `impact` | Blast radius analysis | "What breaks if I change UserService?" |
| `detect_changes` | Analyze uncommitted git changes | "What processes did my commit affect?" |
| `rename` | Plan coordinated multi-file rename | "Rename AuthController to SessionController" |
| `cypher` | Raw graph queries | "Find unused exported functions" |
| `list_repos` | List all registered repositories | — |
| `query_repo` | Run a read-only tool against a different repo | "Query symbols in repo B from repo A" |

### Recommended Workflow

1. **Discovery**: Use `query()` for semantic search
2. **Deep Dive**: Use `context()` to understand a symbol
3. **Find callers**: Use `find_referencing()` for a lightweight 1-hop caller list
4. **Understand a file**: Use `get_symbols_overview()` to see what's defined in a file
5. **Before Editing**: Run `symbol_coordinates()` to get the exact location, then `impact()` for blast radius
6. **Cross-repo**: Use `list_repos()` to discover repos, then `query_repo()` to query them

For detailed usage, see [docs/guide.md](docs/guide.md).

---

## Testing MCP Tools Locally

Use [MCP Inspector](https://github.com/modelcontextprotocol/inspector) — the official browser-based UI for exploring and calling MCP tools interactively:

```bash
# Build first
make build

# Index the current repo
./bin/mimir analyze .

# Launch MCP Inspector (requires Node.js)
npx @modelcontextprotocol/inspector ./bin/mimir mcp
```

Then open **http://localhost:5173** in your browser. You can browse all 12 tools, fill in arguments via a form, and see the JSON responses in real time.

Alternatively, test via raw stdin:

```bash
# List all tools
echo '{"jsonrpc":"2.0","id":1,"method":"tools/list","params":{}}' | ./bin/mimir mcp

# Call a tool
echo '{"jsonrpc":"2.0","id":2,"method":"tools/call","params":{"name":"list_repos","arguments":{}}}' | ./bin/mimir mcp

# Query a symbol in the current repo
echo '{"jsonrpc":"2.0","id":3,"method":"tools/call","params":{"name":"query","arguments":{"q":"handleRequest"}}}' | ./bin/mimir mcp

# Cross-repo query: search for a symbol in a different repo
echo '{"jsonrpc":"2.0","id":4,"method":"tools/call","params":{"name":"query_repo","arguments":{"tool_name":"query","arguments":{"query":"auth"},"target_repo":"other-project","current_repo":"git-mimir"}}}' | ./bin/mimir mcp
```

---

## Auto-Analyze Features

Running `mimir analyze` automatically sets up:

1. **MCP Daemon** — Background server for persistent MCP availability
2. **Editor Config** — Auto-configures Claude Code, VS Code, Cursor, Windsurf, Zed, OpenCode
3. **Claude Code Hooks** — Pre-search (graph-augmented search) and post-write (auto re-index)
4. **Agent Skills** — 4 static methodology skills + dynamic module skills per cluster
5. **Context Files** — AGENTS.md and CLAUDE.md with live index statistics

**Log location:** `~/.mimir/mimir-mcp.log`

For complete feature documentation, see [docs/features.md](docs/features.md).

---

## HTTP API

| Method | Endpoint | Description |
|---|---|---|
| GET | `/` | Web UI |
| GET | `/repos` | List indexed repos |
| GET | `/repo/{name}/graph` | Full graph (nodes + edges) |
| GET | `/repo/{name}/query?q=...` | Hybrid search |
| GET | `/repo/{name}/context/{symbol}` | Symbol detail + edges |
| GET | `/repo/{name}/impact/{target}` | Blast radius analysis |
| GET | `/repo/{name}/clusters` | Louvain clusters |
| GET | `/repo/{name}/processes` | Execution flow traces |
| POST | `/repo/{name}/cypher` | Raw SQL query |

---

## Documentation

| Guide | Description |
|---|---|
| [Installation](docs/installation.md) | Detailed installation and setup |
| [Usage Guide](docs/guide.md) | Perfect workflow for daily use |
| [Features](docs/features.md) | Complete feature reference |
| [Architecture](docs/architecture.md) | System design and components |
| [Algorithms](docs/algorithms.md) | Louvain, BM25, HNSW details |

---

## Tech Stack

| Component | Technology |
|---|---|
| Language | Go 1.22+ |
| AST Parsing | smacker/go-tree-sitter |
| Storage | modernc.org/sqlite (pure Go) |
| Vector Search | sqlite-vec (HNSW) |
| Text Search | Custom BM25 |
| Clustering | Louvain algorithm |
| CLI | cobra |
| Web UI | Sigma.js v2 + Graphology |

---

## Project Structure

```
mimir/
├── cmd/mimir/          # CLI entrypoint
├── api/                # HTTP server + embedded Web UI
├── mcp/                # MCP stdio server
├── internal/
│   ├── walker/         # File tree walker
│   ├── parser/         # AST parser (8 languages)
│   ├── resolver/       # Import/call resolver
│   ├── graph/          # Node/edge types
│   ├── store/          # SQLite layer
│   ├── embedder/       # Embedding pipeline
│   ├── cluster/        # Louvain detection
│   ├── process/        # Flow tracer
│   ├── incremental/    # Git-diff indexing
│   └── registry/       # Multi-repo registry
└── testdata/           # Test fixtures
```

---

## Contributing

Contributions welcome! See [GitHub Issues](https://github.com/thuongh2/git-mimir/issues) for open tasks.

**License:** MIT
