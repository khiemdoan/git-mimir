# Mimir Algorithms

## 1. Scope-Aware Symbol Resolution
Mimir does not rely on simple string matching. It implements a formal two-pass resolution algorithm:

1.  **Symbol Collection**: Every file is parsed into a local symbol tree. Exported symbols are added to a global Trie.
2.  **Resolution**: For every "unresolved" reference (e.g., a function call), Mimir:
    -   Checks the local file scope.
    -   Checks imported packages.
    -   Queries the global Symbol Table for candidates.
    -   Calculates a **Confidence Score** (0.0 - 1.0) based on how well the signature and scope match.

## 2. Louvain Community Detection
To help AI agents understand the high-level structure of a codebase, Mimir uses the Louvain algorithm:
-   It treats the call graph as a weighted undirected graph.
-   It iteratively optimizes **Modularity** to find dense clusters of highly-interconnected symbols.
-   Clusters are labeled based on their primary directory or package name.

## 3. Process Tracing (BFS)
Execution flows are identified by:
1.  Identifying **Entry Points** (e.g., `main` functions, HTTP handlers, CLI commands).
2.  Performing a Breadth-First Search (BFS) down the call graph.
3.  Linearizing the results into "Processes" that represent logical features of the application.

## 4. Hybrid Search (RRF)
Mimir combines multiple search strategies using **Reciprocal Rank Fusion (RRF)**:
-   **BM25**: Keyword relevance.
-   **Vector Search**: Semantic meaning using embeddings.
-   **Graph Centrality**: Boosting results that are highly connected (important) in the graph.
