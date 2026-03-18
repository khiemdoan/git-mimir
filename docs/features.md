# Features & MCP Server

Mimir provides a suite of features designed specifically for AI agents through the **Model Context Protocol (MCP)**.

## MCP Tools

| Tool | Description |
|---|---|
| `query` | Hybrid search (Keyword + Vector) that returns results grouped by logical processes. |
| `context` | Returns a "360-degree" view of a specific symbol, including its definition, callers, called functions, and related documentation. |
| `impact` | Performs blast-radius analysis to show what might break if a specific function or class is modified. |
| `detect_changes` | Analyzes uncommitted git changes and identifies which high-level processes are affected. |
| `rename` | Plans a coordinated rename across the graph and the filesystem. |
| `cypher` | Allows advanced users to run graph queries using a subset of the Cypher query language. |
| `list_repos` | Lists all indexed repositories available for querying. |

## Key Features

-   **Zero-Config Indexing**: Run `mimir analyze .` and get a graph in seconds.
-   **Incremental Updates**: Uses git diffs to only re-index files that have changed.
-   **Multi-Language**: Native support for Go and TypeScript (extensible AST framework).
-   **Local-First**: Everything runs on your machine. No code leaves your environment.
-   **Agentic-Optimized**: Output formats are designed for LLMs to consume, with high signal-to-noise ratios.
