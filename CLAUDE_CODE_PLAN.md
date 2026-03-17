# Mimir — Claude Code Implementation Plan

> Paste this entire file into Claude Code and say:
> **"Read this plan carefully and implement phase by phase. Start with Phase 0."**

---

## Project overview

**Mimir** — the well of code intelligence.
Rebuild GitNexus (Node.js) in Go with a better algorithm stack:

- Parallel AST parsing via goroutine pool (no Worker thread overhead)
- Scope-aware import/call resolver — full AST walk, no regex fallback
- Incremental indexing via git-diff patch (re-index only changed files)
- Async batched embedding pipeline (indexing never blocks on embeddings)
- **SQLite + sqlite-vec** — pure Go, zero CGO, single static binary
- Full MCP stdio server + HTTP bridge API
- All 7 original MCP tools preserved with identical API surface

---

## Repository structure (target)

```
mimir/
├── cmd/
│   └── mimir/              # main CLI entrypoint
├── internal/
│   ├── walker/             # parallel file tree walker
│   ├── parser/             # tree-sitter bindings per language
│   ├── resolver/           # scope-aware import/call resolver
│   ├── graph/              # graph node/edge types
│   ├── store/              # SQLite store adapter (read/write)
│   ├── embedder/           # async batched embedder
│   ├── search/             # BM25 + vector + RRF hybrid search
│   ├── cluster/            # Louvain community detection
│   ├── process/            # execution flow tracer (BFS)
│   ├── incremental/        # git-diff → patch planner
│   └── registry/           # multi-repo registry (~/.mimir)
├── mcp/
│   ├── server.go           # MCP stdio transport (JSON-RPC 2.0)
│   ├── tools.go            # 7 MCP tool definitions
│   ├── resources.go        # 7 MCP resource handlers
│   └── prompts.go          # 2 MCP prompts
├── api/
│   └── http.go             # HTTP bridge for web UI
├── .claude/
│   └── skills/mimir/
│       └── SKILL.md        # agent skill for this project
├── go.mod
├── go.sum
└── AGENTS.md
```

---

## Tech stack

| Concern | Choice | Why |
|---|---|---|
| Language | Go 1.22+ | Goroutines, single binary, no GC pauses during indexing |
| AST parsing | smacker/go-tree-sitter | Official bindings, 12 languages |
| **Storage** | **modernc.org/sqlite** | **Pure Go, zero CGO, cross-compile anywhere** |
| **Vector search** | **sqlite-vec (asg017/sqlite-vec)** | **HNSW cosine, embedded into binary, ~1.5MB** |
| BM25 | Custom inverted index (bm25_index table) | ~200 lines, no dep |
| Embedding | HTTP client → ollama or OpenAI | Async, batched, cached |
| Community detection | Louvain in pure Go | see louvain.md reference |
| MCP transport | stdio JSON-RPC 2.0 | see mcp_spec.md reference |
| CLI | cobra + viper | Standard Go CLI |
| Git ops | go-git/go-git | Incremental indexing, no system git required |
| Config | ~/.mimir/registry.json | Same schema as GitNexus, compatible |

---

## Phase 0 — Bootstrap (Day 1)

**Goal:** Runnable binary that walks a repo and prints file stats.

```
Tasks:
1. go mod init github.com/yourname/mimir
2. go get: cobra, modernc.org/sqlite, github.com/asg017/sqlite-vec/go/vec0,
           smacker/go-tree-sitter, go-git/go-git, sabhiram/go-gitignore
3. cmd/mimir/main.go
   - Subcommands: analyze, mcp, serve, list, status, clean, setup, wiki
4. internal/walker/walker.go
   - WalkRepo(root string, concurrency int) <-chan FileInfo
   - Respect .gitignore via go-gitignore
   - Skip: .git, .mimir, node_modules, vendor, dist, *.min.js, *.lock, *.pb.go
   - FileInfo{Path, Ext, Size, ModTime}
5. Wire: mimir analyze <path> → walker → print "Found N files in Xs"
6. walker_test.go with testdata/simple-ts/ fixture
```

**Acceptance:** `mimir analyze ./testdata/simple-ts` prints file count without panics.

---

## Phase 1 — Parallel AST Parser (Days 2–4)

**Goal:** Extract all symbols from a repo in parallel using a goroutine pool.

```
Tasks:
1. internal/parser/parser.go
   - ParseFile(path string, src []byte) (*FileSymbols, error)
   - Uses go-tree-sitter grammars
   - Extracts per node: name, kind, startLine, endLine, filePath, exported bool
   - Symbol kinds: Function, Method, Class, Interface, Variable, Constant, Type
   - Languages (priority order): TypeScript, JavaScript, Go, Python, Java, Rust, C, C++

2. internal/parser/pool.go
   - NewParserPool(size int) *ParserPool  — size = runtime.GOMAXPROCS(0)
   - Each worker owns one pre-allocated tree-sitter parser (never create per-file)
   - Run(ctx, files <-chan FileInfo) <-chan FileSymbols
   - On parse error: log to stderr, skip file (never crash)

3. internal/parser/languages.go
   - Registry: ext → grammar loader (lazy, only loads grammars found in repo)
   - Supported: .ts .tsx .js .jsx .go .py .java .rs .c .cpp .cs .kt .swift .php

4. internal/graph/types.go
   - Node{UID, Name, Kind, FilePath, StartLine, EndLine, Exported, PackagePath}
   - Edge{FromUID, ToUID, Type, Confidence}
   - EdgeType constants: CALLS IMPORTS EXTENDS IMPLEMENTS MEMBER_OF
   - UID = sha256(filePath + ":" + name + ":" + kind)[:16]

5. Wire: analyze → walker → parser pool → collect []FileSymbols → print symbol count
```

**Goroutine pool pattern (use everywhere in Mimir):**
```go
func (p *ParserPool) Run(ctx context.Context, files <-chan FileInfo) <-chan FileSymbols {
    out := make(chan FileSymbols, p.size*4)
    var wg sync.WaitGroup
    for _, w := range p.workers {
        wg.Add(1)
        go func(worker *treeParser) {
            defer wg.Done()
            for f := range files {
                select {
                case <-ctx.Done():
                    return
                default:
                }
                if syms, err := worker.parse(f); err == nil {
                    out <- syms
                }
            }
        }(w)
    }
    go func() { wg.Wait(); close(out) }()
    return out
}
```

**Acceptance:** Parse 1000-file TypeScript repo in under 5 seconds.

---

## Phase 2 — SQLite Store (Days 5–6)

**Goal:** Persist nodes and edges to SQLite with vector search support.

```
Tasks:
1. internal/store/schema.go
   - Full schema (see sqlite_go.md reference for exact DDL)
   - Tables: nodes, edges, clusters, cluster_members, processes,
             process_steps, bm25_index, embed_cache, index_meta
   - WAL mode + performance PRAGMAs on open
   - sqlite-vec virtual table: vec_nodes (uid TEXT, embedding float[384])

2. internal/store/store.go
   - OpenStore(path string) (*Store, error)
   - Store has: write *sql.DB (MaxOpenConns=1) + read *sql.DB (MaxOpenConns=8)
   - Write(fn func(*sql.Tx) error) — mutex + transaction
   - Read(fn func(*sql.DB) error) — concurrent reads
   - BatchUpsertNodes([]graph.Node) error — prepared stmt, ON CONFLICT DO UPDATE
   - BatchUpsertEdges([]graph.Edge) error — prepared stmt, edge ID = sha256(from+to+type)
   - DeleteNodesByFile(filePath string) error — cascade deletes edges via FK

3. internal/store/query.go
   - QuerySymbol(name string) ([]graph.Node, error)
   - QueryByFile(filePath string) ([]graph.Node, error)
   - QueryCallers(uid string, minConf float64, depth int) ([]ImpactRow, error)
     → recursive CTE (see sqlite_go.md for query)
   - HybridSearch(terms []string, emb []float32, k int) ([]SearchResult, error)
     → BM25 from bm25_index + VectorSearch + RRF fusion (k=60)

4. internal/store/embeddings.go
   - UpsertEmbedding(uid string, emb []float32) error
     → encode float32 slice as little-endian bytes for sqlite-vec
   - VectorSearch(emb []float32, k int) ([]VectorResult, error)
     → sqlite-vec MATCH query
   - BatchUpdateEmbeddings(map[string][]float32) error

5. store_test.go
   - TestOpenStore, TestBatchUpsert, TestImpactQuery, TestHybridSearch
   - Use in-memory DB: sql.Open("sqlite", ":memory:")
```

**Key: float32 ↔ bytes encoding** (required by sqlite-vec):
```go
import "encoding/binary"
import "math"

func float32ToBytes(f []float32) []byte {
    b := make([]byte, len(f)*4)
    for i, v := range f {
        binary.LittleEndian.PutUint32(b[i*4:], math.Float32bits(v))
    }
    return b
}
```

**Acceptance:** Store + retrieve 50k nodes < 500ms. Hybrid search < 100ms.

---

## Phase 3 — Scope-Aware Resolver (Days 7–9)

**Goal:** Build typed, confidence-scored edges across files. No regex.

```
Tasks:
1. internal/resolver/symbol_table.go
   - SymbolTable: map[packagePath]map[symbolName][]Candidate
   - Candidate{NodeUID, FilePath, Confidence}
   - RegisterExport(pkg, name, uid, filePath)
   - Resolve(fromFile, importPath, symbolName string) (uid, confidence, error)
   - Confidence rules (from skill/SKILL.md):
     * Direct import + exact name match   → 0.95
     * Same package, unambiguous          → 0.95
     * Re-export chain (1 hop)            → 0.85
     * Re-export chain (2+ hops)          → 0.70
     * Interface dispatch                 → 0.75
     * Multiple candidates (best picked)  → 0.60
     * Unresolved (placeholder edge)      → 0.00

2. internal/resolver/resolver.go
   TWO-PASS ALGORITHM — never combine into one pass:
   - Pass 1: Build complete SymbolTable from all FileSymbols (parallel, file-level goroutines)
   - Pass 2: Walk all call/import sites, resolve → emit Edges (parallel, file-level goroutines)
   - Pass 2 MUST wait for Pass 1 to fully complete before starting

3. internal/resolver/languages/ts.go
   - TypeScript: handle export default, re-exports, barrel files (index.ts)
   - Track import aliases: import { foo as bar } → alias map per file
   - Detect: namespace imports (import * as X), type-only imports (import type)

4. internal/resolver/languages/go_lang.go
   - Go: use import path + exported symbol (capital letter = exported)
   - Handle: method receivers, interface implementation detection

5. resolver_test.go
   - Use testdata/simple-ts/ with known expected edges
   - Assert edge count, spot-check specific from→to pairs and confidence
```

**Acceptance:** ≥90% edge accuracy on testdata/simple-ts known graph.

---

## Phase 4 — Incremental Indexer (Days 10–11)

**Goal:** Only re-parse changed files. Transforms daily UX from 45s → <2s.

```
Tasks:
1. internal/incremental/diffplanner.go
   - GetChangedFiles(repoPath, sinceCommit string) ([]ChangedFile, error)
   - Uses go-git: open repo, diff sinceCommit..HEAD
   - ChangedFile{Path string, Status: added|modified|deleted}
   - Fall back to full index if sinceCommit == "" (first run)

2. internal/incremental/patcher.go
   - PlanPatch(changed []ChangedFile) *PatchPlan
   - PatchPlan{ToAdd []string, ToUpdate []string, ToDelete []string}
   - For ToUpdate + ToDelete: call store.DeleteNodesByFile (CASCADE removes edges)
   - Then re-parse ToAdd + ToUpdate files through normal parser pool
   - Re-resolve ONLY files in the patch (not full repo)
   - After re-resolve: re-cluster affected communities only

3. internal/incremental/staleness.go
   - IsStale(repoPath string) (bool, string, error)
   - Read last_commit from store index_meta table
   - Get current HEAD SHA via go-git
   - Return: (true, "12 files changed since abc1234", nil) or (false, "up to date", nil)

4. Wire into analyze command:
   if --force flag → full index (delete all nodes for repo, re-index)
   else if IsStale  → PlanPatch → apply patch
   else             → print "Index is current (HEAD abc1234)" and exit 0

5. Store last_commit SHA in index_meta after every successful index:
   store.SetMeta("last_commit", headSHA)
   store.SetMeta("indexed_at", time.Now().UTC().Format(time.RFC3339))
```

**Acceptance:** Re-indexing after 10-file commit on 5000-file repo < 2s.

---

## Phase 5 — Clustering + Process Tracing (Days 12–14)

**Goal:** Community detection and call-chain execution flow extraction.

```
Tasks:
1. internal/cluster/louvain.go  (see louvain.md reference for full implementation)
   - BuildGraph(nodes []graph.Node, edges []graph.Edge) *Graph
     → Only CALLS and IMPORTS edges, weighted by Confidence
   - Louvain(g *Graph, resolution float64) []Community
     → resolution=1.0 default, expose as --resolution flag
   - Community{ID, Members []string, CohesionScore float64, Label string}
   - LabelCommunity(members []graph.Node) string
     → Most common directory prefix heuristic

2. internal/cluster/store_integration.go
   - After Louvain: store.UpsertClusters(communities)
   - store.SetClusterForNodes(members, clusterID)
   - store.UpsertClusterMembers(clusterID, members)

3. internal/process/tracer.go
   - FindEntryPoints(nodes []graph.Node, store *Store) []graph.Node
     Heuristics (in order):
     a. Exported functions named: main, handler, Handle*, Serve*, Route*, Controller*
     b. Exported functions with 0 incoming CALLS edges but ≥1 outgoing
     c. Functions in files named: main.go, server.go, router.go, handler*.go
   - TraceProcess(entry graph.Node, store *Store, maxDepth int) *Process
     → BFS through CALLS edges
     → Visited set to break cycles
     → Stop at: maxDepth reached | external/unresolved edge | no outgoing CALLS
   - Process{ID, Name, EntryUID, Steps []ProcessStep, ProcessType}
   - ProcessStep{Index, SymbolUID, Depth}

4. internal/process/classifier.go
   - ClassifyProcess(p *Process, store *Store) ProcessType
     → single_community: all steps in same cluster
     → cross_community: steps span ≥2 clusters
     → external: any step resolves to unindexed node

5. Wire into analyze after resolver:
   communities = cluster.Louvain(graph, 1.0)
   store.UpsertClusters(communities)
   entryPoints = process.FindEntryPoints(allNodes, store)
   for each entry → TraceProcess → store.UpsertProcess
```

**Acceptance:** On a web API repo, finds HTTP handler entry points and traces to DB calls.

---

## Phase 6 — Async Embedder (Days 15–16)

**Goal:** Generate embeddings without blocking the indexing pipeline.

```
Tasks:
1. internal/embedder/embedder.go
   Interface:
   - type Embedder interface { Embed(texts []string) ([][]float32, error) }
   Implementations:
   a. OllamaEmbedder  — POST http://localhost:11434/api/embed, model: nomic-embed-text
   b. OpenAIEmbedder  — POST api.openai.com/v1/embeddings, text-embedding-3-small
   c. NoopEmbedder    — returns zero vectors (for --skip-embeddings flag)
   Select via: MIMIR_EMBED_PROVIDER env var or config file

2. internal/embedder/worker.go
   - EmbedWorker struct with: queue chan EmbedJob, store *Store, embedder Embedder
   - EmbedJob{UID, Text string}  — Text = name + " " + filePath + " " + kind
   - Start(ctx context.Context)
     → background goroutine, reads from queue
     → Batch: accumulate up to 64 jobs OR 100ms timeout (whichever first)
     → Call embedder.Embed(batch texts)
     → Write results to store.BatchUpdateEmbeddings
     → Write to embed_cache (key=sha256(text), skip if already cached)

3. internal/embedder/cache.go
   - TextHash(text string) string — sha256 hex of text
   - IsCached(db *Store, hash string) bool
   - GetCached(db *Store, hash string) ([]float32, error)

4. Wire into analyze:
   embedWorker := embedder.NewWorker(store, selectedEmbedder)
   go embedWorker.Start(ctx)
   // After store.BatchUpsertNodes:
   for _, node := range allNodes {
       embedWorker.Enqueue(EmbedJob{UID: node.UID, Text: node.Name + " " + node.FilePath})
   }
   // analyze returns immediately — do NOT wait for embedder
   fmt.Println("Indexing complete. Embeddings generating in background.")
   fmt.Println("Run `mimir status` to check embedding progress.")
```

**Acceptance:** Indexing 1000-file repo completes in <10s regardless of embedding latency.

---

## Phase 7 — MCP Server (Days 17–19)

**Goal:** All 7 tools, 7 resources, 2 prompts. Spec-compliant JSON-RPC 2.0 stdio.

*(See mcp_spec.md reference for full message format, types, and Go skeleton)*

```
Tasks:
1. mcp/server.go
   - Serve(registry *registry.Registry) error
   - bufio.Scanner reads stdin line by line
   - json.NewEncoder(os.Stdout) writes responses
   - Notifications (no id field) → handle + no response
   - Graceful shutdown on SIGTERM / context cancel

2. mcp/tools.go — 7 tools with exact signatures from SKILL.md:
   a. list_repos     → registry.ListAll()
   b. query          → store.HybridSearch() → group results by process
   c. context        → store.QuerySymbol() + incoming/outgoing edges + processes
   d. impact         → store.QueryImpact() with direction + minConfidence + maxDepth
   e. detect_changes → incremental.GetChangedFiles() + impact analysis
   f. rename         → graph edges + text search → produce FileChange list (dry_run default)
   g. cypher         → Cypher subset → SQLite SQL translator (see below)

3. mcp/resources.go — 7 resource URIs (replace gitnexus:// → mimir://):
   mimir://repos
   mimir://repo/{name}/context
   mimir://repo/{name}/clusters
   mimir://repo/{name}/cluster/{id}
   mimir://repo/{name}/processes
   mimir://repo/{name}/process/{id}
   mimir://repo/{name}/schema

4. mcp/prompts.go
   - detect_impact: guides agent through detect_changes → summarise risk
   - generate_map: reads clusters + processes → produce mermaid architecture diagram

5. Cypher → SQLite translator (for tool g):
   Supported subset:
     MATCH (n:Function) WHERE n.name = 'foo' RETURN n.name, n.filePath LIMIT 10
     MATCH (a)-[r:CALLS]->(b) WHERE r.confidence > 0.8 RETURN a.name, b.name
     MATCH (n:Class {name: 'UserService'}) RETURN n
   Translate to: SELECT + JOIN on nodes/edges tables
   Reject unsupported clauses with: {"error": "Unsupported Cypher: <clause>. Use SQL instead."}
```

**Acceptance:** All 7 tools return valid responses on testdata/. `npx @modelcontextprotocol/inspector mimir mcp` passes.

---

## Phase 8 — CLI polish + registry (Days 20–21)

```
Tasks:
1. internal/registry/registry.go
   - File: ~/.mimir/registry.json
   - Schema: {"repos": [{"name": "myapp", "path": "/abs/path", "indexed_at": "..."}]}
   - Register(repoPath, name string) / Unregister(name) / List() / Get(name)
   - Auto-detect name from repo directory basename if not specified

2. cmd/mimir/analyze.go — full pipeline:
   walker → parser pool → pass1 (symbol table) → pass2 (resolver) →
   store.BatchUpsert → clusterer → process tracer → embed worker.Start
   Progress output:
     "Parsed 1,243 files (4.2s)"
     "Resolved 8,421 edges (1.1s)"
     "Clustered 14 communities"
     "Traced 23 processes"
     "Indexing complete. Embeddings generating in background."

3. cmd/mimir/setup.go
   - Detect editors: look for ~/.cursor/mcp.json, ~/.claude.json, ~/.config/windsurf/
   - Write MCP config entry: {"mimir": {"command": "mimir", "args": ["mcp"]}}
   - Print which configs were updated

4. cmd/mimir/status.go
   - Read index_meta from store: last_commit, indexed_at, node count, edge count
   - Count NULL embeddings: "Embeddings: 8,421/8,421 complete" or "4,210/8,421 (50%)"
   - Show staleness: "Index is current" or "Stale: 3 files modified since abc1234"

5. cmd/mimir/wiki.go
   - Read clusters + processes from store
   - For each cluster: call LLM API (openai-compatible, env: MIMIR_LLM_BASE_URL)
   - Generate per-cluster .md page → write to .mimir/wiki/
   - Generate WIKI.md overview with cross-references

6. Build config (Makefile or goreleaser):
   CGO_ENABLED=0 go build -ldflags="-s -w" -o mimir ./cmd/mimir
   Target: < 20MB binary (SQLite ~6MB + sqlite-vec ~1.5MB + tree-sitter grammars ~8MB)
```

---

## Phase 9 — HTTP bridge + web UI (Day 22)

```
Tasks:
1. api/http.go  (net/http, no external framework)
   GET  /repos                          → registry.ListAll()
   GET  /repo/:name/query?q=&embed=     → store.HybridSearch()
   GET  /repo/:name/context/:symbol     → store.QuerySymbol() + relations
   GET  /repo/:name/impact/:target      → store.QueryImpact()
   GET  /repo/:name/clusters            → store.QueryClusters()
   POST /repo/:name/cypher              → mcp cypher translator
   GET  /repo/:name/graph               → full graph as JSON (nodes + edges)
   WS   /repo/:name/graph/stream        → stream graph for Sigma.js

2. CORS: allow https://gitnexus.vercel.app + localhost:*
3. Port: 7842 (same as current GitNexus serve)
4. cmd/mimir/serve.go → start HTTP server + print URL
```

---

## Testing strategy

```
testdata/
├── simple-ts/      # 10 files, fully known call graph (used in Phase 0–3 tests)
├── medium-api/     # 200 files Python/TS API server
└── large-mono/     # 3000+ files (script-generated, for benchmark only)

Per-package tests:
  walker/walker_test.go         — file enumeration, gitignore
  parser/parser_test.go         — symbol extraction per language
  resolver/resolver_test.go     — edge accuracy on simple-ts known graph
  store/store_test.go           — CRUD, impact query, hybrid search
  cluster/louvain_test.go       — community detection on synthetic graph
  process/tracer_test.go        — entry point detection + BFS trace
  incremental/patcher_test.go   — diff → patch plan correctness
  mcp/tools_test.go             — all 7 tools response shape

Benchmarks (go test -bench):
  BenchmarkParseRepo1000        — parser pool on 1000 TS files
  BenchmarkHybridSearch         — BM25 + vector + RRF on 50k nodes
  BenchmarkImpactQuery          — recursive CTE depth 5 on 50k edges
  BenchmarkIncrementalPatch     — 10-file diff on 5000-file indexed repo
```

---

## Performance targets

| Metric | GitNexus (Node.js) | Mimir (Go) target |
|---|---|---|
| Cold index 1000-file TS repo | ~45s | < 8s |
| Incremental re-index (10 files) | ~45s (full re-index) | < 2s |
| Hybrid search latency | ~300ms | < 80ms |
| MCP tool cold start | ~2s (npm startup) | < 80ms |
| Binary size | 350MB (node_modules) | < 20MB |
| RAM during index (1000 files) | ~800MB | < 120MB |

---

## Execution order for Claude Code

```
Phase 0 (bootstrap)
  → Phase 1 (parser)
  → Phase 2 (SQLite store)   ← do store before resolver (resolver writes to store)
  → Phase 3 (resolver)
  → Phase 4 (incremental)
  → Phase 5 (cluster + process)
  → Phase 6 (embedder)
  → Phase 7 (MCP server)     ← full end-to-end test here before continuing
  → Phase 8 (CLI polish)
  → Phase 9 (HTTP bridge)
```

After each phase: run `go test ./...` — fix all failures before next phase.
After Phase 7: test with `npx @modelcontextprotocol/inspector mimir mcp`.

---

## Key dependencies (go.mod)

```
require (
    github.com/spf13/cobra                    v1.8+
    github.com/spf13/viper                    v1.18+
    modernc.org/sqlite                        v1.29+
    github.com/asg017/sqlite-vec/go/vec0      v0.1+
    github.com/smacker/go-tree-sitter         v0.0.0-latest
    github.com/go-git/go-git/v5               v5.11+
    github.com/sabhiram/go-gitignore          v0.0.0-latest
)
```

---

## Reference files to read before each phase

| Phase | Read first |
|---|---|
| Phase 1 (parser) | skill/references/tree_sitter_go.md |
| Phase 2 (store) | skill/references/sqlite_go.md |
| Phase 5 (cluster) | skill/references/louvain.md |
| Phase 7 (MCP) | skill/references/mcp_spec.md |