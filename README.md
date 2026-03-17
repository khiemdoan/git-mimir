# Mimir — The Well of Code Intelligence

Mimir is a **Go-based code intelligence engine** that indexes any repository into a knowledge graph and exposes it via MCP tools, HTTP API, and an interactive web UI. It is a ground-up rewrite of [GitNexus](https://github.com/) (Node.js) with a fundamentally better algorithm stack.

```
mimir analyze ./my-project
mimir serve
# Open http://localhost:7842 → interactive force-directed graph
```

---

## Why Mimir over GitNexus?

### Performance

| Metric | GitNexus (Node.js) | Mimir (Go) |
|---|---|---|
| Cold index 1,000-file TS repo | ~45s | **< 8s** |
| Incremental re-index (10 files) | ~45s (full re-index) | **< 2s** |
| Hybrid search latency | ~300ms | **< 80ms** |
| MCP tool cold start | ~2s (npm startup) | **< 80ms** |
| Binary size | 350 MB (node_modules) | **< 20 MB** |
| RAM during index (1,000 files) | ~800 MB | **< 120 MB** |

### Architecture Improvements

| Area | GitNexus | Mimir |
|---|---|---|
| Parsing | Worker threads, single-lang | **Goroutine pool, 8 languages** (TS, JS, Go, Python, Rust, Java, C, C++) |
| Resolution | Regex-based fallback | **Full AST scope-aware resolver** — two-pass parallel resolution |
| Incremental indexing | None (full re-index) | **Git-diff patch planner** — only re-parses changed files |
| Storage | SQLite + Node bindings (CGO) | **Pure Go SQLite** (modernc.org) — zero CGO, single static binary |
| Vector search | External dependency | **sqlite-vec** — HNSW cosine embedded in binary |
| Text search | Basic FTS | **BM25 with camelCase-aware tokenization** |
| Clustering | Basic grouping | **Louvain community detection** on call graph |
| Process tracing | Manual | **Automatic entry point detection** with BFS flow tracing |
| Deployment | `npm install` + native deps | **Single binary**, zero dependencies |
| Web UI | Separate Vercel app | **Embedded in binary** — `mimir serve` does everything |

---

## Features

- **Knowledge Graph** — Functions, classes, methods, interfaces, variables, and types as nodes; CALLS, IMPORTS, EXTENDS, IMPLEMENTS, MEMBER_OF as edges
- **8-Language Support** — TypeScript, JavaScript, Go, Python, Rust, Java, C, C++
- **Hybrid Search** — BM25 text + vector embeddings + Reciprocal Rank Fusion
- **Impact Analysis** — Recursive upstream/downstream blast radius queries
- **Louvain Clustering** — Automatic module detection via community detection on call graphs
- **Process Tracing** — Heuristic entry point detection with BFS execution flow mapping
- **Incremental Indexing** — Git-diff based; only re-parses changed files
- **MCP Server** — 7 tools, 7 resources, 2 prompts via stdio JSON-RPC 2.0
- **HTTP API** — RESTful endpoints for graph, search, clusters, processes, impact
- **Interactive Web UI** — Force-directed graph visualization with Sigma.js (embedded, no build step)

---

## Installation

### From Source

```bash
git clone https://github.com/yourusername/mimir.git
cd mimir
go build -o mimir ./cmd/mimir
```

The result is a single static binary with zero external dependencies.

---

## Quick Start

### 1. Index a Repository

```bash
# Index with full features (requires ollama or OpenAI for embeddings)
mimir analyze /path/to/your/project

# Index without embeddings (faster, graph + BM25 search only)
mimir analyze --skip-embeddings /path/to/your/project
```

### 2. Explore via Web UI

```bash
mimir serve
# Open http://localhost:7842
```

The web UI provides:
- **Force-directed graph** — nodes colored by kind, sized by connection count
- **Search** — fuzzy filter to highlight matching symbols
- **Click-to-inspect** — detail panel with incoming/outgoing edges
- **Hover highlighting** — dims unrelated nodes, shows direct connections
- **Repo selector** — switch between indexed repositories
- **Zoom controls** — fit, zoom in/out

### 3. Use with AI Agents (MCP)

```bash
# Start the MCP stdio server
mimir mcp
```

Or configure in your editor's MCP settings:

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

**Auto-setup** for supported editors:

```bash
mimir setup
```

---

## CLI Reference

| Command | Description |
|---|---|
| `mimir analyze <path>` | Index a repository into the knowledge graph |
| `mimir serve` | Start the HTTP server + web UI (default: port 7842) |
| `mimir mcp` | Start the MCP stdio server for AI agent integration |
| `mimir list` | List all indexed repositories |
| `mimir status [name]` | Show index status for a repository |
| `mimir clean <name>` | Remove the index for a repository |
| `mimir setup` | Configure MCP settings in supported editors |
| `mimir wiki [name]` | Generate a wiki from the knowledge graph |

### Flags

```bash
mimir analyze --skip-embeddings <path>   # Skip embedding generation
mimir serve --port 8080                  # Custom port (default: 7842)
```

---

## HTTP API

All endpoints available at `http://localhost:7842`:

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

## MCP Tools

When connected via MCP, AI agents get access to 7 tools:

| Tool | Description |
|---|---|
| `query` | Hybrid search over the code knowledge graph |
| `context` | 360-degree view of a symbol: definition, edges, processes |
| `impact` | Blast radius analysis: what does changing this symbol break? |
| `detect_changes` | Detect uncommitted/recent git changes and their impact |
| `rename` | Plan a coordinated multi-file rename |
| `cypher` | Execute raw graph queries |
| `reindex` | Trigger incremental re-indexing |

---

## Web UI Screenshots

```
┌─────────────────────────────────────────────────────────┐
│ MIMIR  [Repo v]  [Search...]             42 nodes  67 edges │
├───────────────────────────────────┬─────────────────────┤
│                                   │ handleRequest       │
│     ● ──── ●                      │ Kind: Function      │
│    / \      \                     │ File: src/server.ts  │
│   ●   ●     ● ── ●               │ Lines: 45-82        │
│    \   \   /                      │ Cluster: api/server  │
│     ●   ● ●                       │                     │
│      \   |                        │ Incoming:           │
│       ●  ●                        │  main → CALLS      │
│                                   │  router → CALLS    │
│   Force-directed graph            │                     │
│   (Sigma.js canvas)               │ Outgoing:           │
│                                   │  validate → CALLS  │
│                                   │  respond → CALLS   │
├───────────────────────────────────┴─────────────────────┤
│ [Fit] [+] [-]  ● Func  ● Class  ● Method  ● Interface  │
└─────────────────────────────────────────────────────────┘
```

**Node colors:** Function (blue), Class (green), Method (cyan), Interface (purple), Variable (orange), Type (gray)

**Edge colors:** CALLS (gray), IMPORTS (blue), EXTENDS (green), IMPLEMENTS (purple), MEMBER_OF (orange)

---

## Tech Stack

| Concern | Choice |
|---|---|
| Language | Go 1.22+ |
| AST Parsing | smacker/go-tree-sitter |
| Storage | modernc.org/sqlite (pure Go, zero CGO) |
| Vector Search | sqlite-vec (HNSW cosine) |
| Text Search | Custom BM25 inverted index |
| Community Detection | Louvain algorithm (pure Go) |
| MCP Transport | stdio JSON-RPC 2.0 |
| CLI | cobra |
| Git Operations | go-git/go-git |
| Graph Visualization | Sigma.js v2 + Graphology |

---

## Project Structure

```
mimir/
├── cmd/mimir/          # CLI entrypoint (cobra commands)
├── api/                # HTTP server + embedded web UI
├── mcp/                # MCP stdio server (tools, resources, prompts)
├── internal/
│   ├── walker/         # Parallel file tree walker with .gitignore
│   ├── parser/         # Tree-sitter AST parser (8 languages)
│   ├── resolver/       # Scope-aware import/call resolver
│   ├── graph/          # Node/edge type definitions
│   ├── store/          # SQLite storage layer
│   ├── embedder/       # Async batched embedding pipeline
│   ├── cluster/        # Louvain community detection
│   ├── process/        # Execution flow tracer
│   ├── incremental/    # Git-diff based incremental indexing
│   └── registry/       # Multi-repo registry (~/.mimir)
└── testdata/           # Test fixtures
```

---

## License

MIT
