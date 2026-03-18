# The "Perfect Usage" Guide

To get 100% value out of Mimir, follow this workflow for every project you work on.

## 1. Initial Deep Indexing
When you first start working on a repo, run a full analysis. This builds the knowledge graph that powers all other tools.

```bash
# We recommend using embeddings for better semantic search
mimir analyze /path/to/repo
```

## 2. Keep it Fresh (Incremental Indexing)
Mimir is extremely fast at updating. You don't need to re-index the whole repo. Run the analyze command after you make significant changes or pull new code:

```bash
mimir analyze .
# This takes < 2 seconds because it only looks at git diffs!
```

## 3. The "Force-Directed" Discovery
Before writing code, run `mimir serve` and open `http://localhost:7842`.
-   **Visual Context**: See which files are "God Objects" (large clusters).
-   **Isolation**: Click a symbol to see only its direct callers. This helps you understand the "blast radius" visually.

## 4. Mastering MCP Tools in Claude Code

When chatting with Claude, you can prompt it to use Mimir effectively:

*   **"Query the graph"**: Instead of letting Claude search files with `grep`, tell it: *"Use Mimir to find all processes related to 'user authentication'."* Mimir will return logical flows, not just lines of code.
*   **"Check Impact"**: Before asking Claude to refactor, say: *"Run a Mimir impact analysis on the `AuthService` interface."* This prevents Claude from making breaking changes in distant parts of the repo.
*   **"Trace the process"**: If a bug happens in a specific flow, say: *"Mimir, show me the full process trace starting from the `login` endpoint."*

## 5. Advanced: Raw Graph Queries (Cypher)
For power users, Mimir supports a subset of Cypher. You can ask Claude: *"Run a Cypher query to find all exported functions in the 'internal/store' package that have no incoming call edges."*

This is perfect for finding **dead code** or **untested entry points**.

```cypher
MATCH (n:Function)
WHERE n.package_path = 'internal/store'
AND n.exported = 1
RETURN n.name
```

## 6. Pro Tip: Continuous Analysis
Add a post-commit or post-merge hook to your repo:
```bash
# .git/hooks/post-commit
mimir analyze . > /dev/null 2>&1 &
```
This ensures your knowledge graph is always in sync with your `HEAD` without you thinking about it.
