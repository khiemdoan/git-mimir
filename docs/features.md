# Features & MCP Server

Mimir provides a suite of features designed specifically for AI agents through the **Model Context Protocol (MCP)**.

## MCP Tools

| Tool | Description |
|---|---|
| `query` | Hybrid search (BM25 + vector + centrality) that returns results grouped by logical processes. |
| `context` | Returns a "360-degree" view of a specific symbol: definition, outgoing calls, all incoming edges, and cluster membership. |
| `find_referencing` | Lightweight 1-hop lookup of all symbols that directly call, import, extend, or implement a given symbol. Accepts an optional `edge_types` filter (`CALLS`, `IMPORTS`, `EXTENDS`, `IMPLEMENTS`, `MEMBER_OF`) and `min_confidence` threshold. Faster than `context` when you only need the caller list. |
| `symbol_coordinates` | Returns the exact `file_path`, `start_line`, and `end_line` for every definition of a symbol. Always run this before editing a symbol body so the agent knows the precise location to modify. |
| `get_symbols_overview` | Returns all top-level symbols (functions, classes, interfaces, variables, constants) defined in a given file, sorted by line number. Excludes nested methods and members that belong to a class. Accepts `include_private` (default: `true`) to filter to exported symbols only. Use this to understand the structure of a file before editing. |
| `impact` | Performs blast-radius analysis (BFS up to configurable depth) to show what might break if a specific function or class is modified. Supports `direction` (upstream/downstream) and `min_confidence`. |
| `detect_changes` | Analyzes uncommitted git changes and identifies which high-level processes are affected. |
| `rename` | Plans a coordinated rename across the graph and the filesystem. Use `dry_run=true` first. |
| `cypher` | Allows advanced users to run graph queries using a subset of the Cypher query language (`MATCH` only). |
| `list_repos` | Lists all indexed repositories available for querying. |
| `query_repo` | Executes a read-only tool against a different indexed repository without switching context. Accepts `tool_name` (one of: `query`, `context`, `find_referencing`, `symbol_coordinates`, `get_symbols_overview`, `impact`), `arguments`, `target_repo`, and optional `current_repo` for correlation. The response includes a `meta` field with `queried_repo`, `current_repo`, and `tool_used`. |

## Key Features

-   **Zero-Config Indexing**: Run `mimir analyze .` and get a graph in seconds.
-   **Incremental Updates**: Uses git diffs to only re-index files that have changed.
-   **Multi-Language**: Native support for Go and TypeScript (extensible AST framework).
-   **Local-First**: Everything runs on your machine. No code leaves your environment.
-   **Agentic-Optimized**: Output formats are designed for LLMs to consume, with high signal-to-noise ratios.
