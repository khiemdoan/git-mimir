# Using Mimir with Claude Code

To get the most out of Mimir in Claude Code, you should register it as an MCP server and use the specialized skill.

## 1. Setup MCP Server
Add Mimir to your Claude Code configuration:

```bash
mimir setup
```
This will add the Mimir executable to your editor's MCP settings (e.g., VS Code or Claude Desktop).

## 2. Recommended Skill for Claude Code

When using Claude Code to develop or query Mimir, use the following guidelines:

-   **Always start with `query`**: If you don't know where to look, use `mcp__mimir__query` to find the relevant code paths.
-   **Use `context` for deep dives**: Once you find a symbol, use `mcp__mimir__context` to see its full relationship tree.
-   **Use `find_referencing` for lightweight caller lookup**: When you only need to know who calls or imports a symbol (not the full 360° view), `mcp__mimir__find_referencing` is faster. Supports `edge_types` filter — e.g., `["CALLS"]` or `["IMPORTS"]`.
-   **Always run `symbol_coordinates` before editing**: Before replacing a function or class body, call `mcp__mimir__symbol_coordinates` to get the exact `file_path`, `start_line`, and `end_line`. This avoids reading entire files just to locate the target.
-   **Check `impact` before refactoring**: If you are about to change a core interface, run `mcp__mimir__impact` to see the blast radius.
-   **Analyze on save**: You can configure a git hook to run `mimir analyze` automatically on commit to keep the knowledge graph fresh.

## 3. Example Workflow — Understanding a Symbol
1.  **User**: "How does the resolver handle interfaces?"
2.  **Claude**: Calls `mcp__mimir__query(query="interface resolution")`.
3.  **Mimir**: Returns relevant symbols and the "Resolve" process.
4.  **Claude**: Calls `mcp__mimir__context(name="Resolve")` to see the logic.
5.  **Claude**: Explains the logic to the user using the high-fidelity graph data.

## 4. Example Workflow — Editing a Symbol Safely
1.  **User**: "Refactor `ProcessOrder` to accept a context parameter."
2.  **Claude**: Calls `mcp__mimir__find_referencing(name="ProcessOrder", edge_types=["CALLS"])` → sees 4 direct callers.
3.  **Claude**: Calls `mcp__mimir__symbol_coordinates(name="ProcessOrder")` → gets `{file_path: "internal/order/service.go", start_line: 42, end_line: 67}`.
4.  **Claude**: Calls `mcp__mimir__impact(target="ProcessOrder")` → confirms blast radius before changing the signature.
5.  **Claude**: Edits lines 42–67 in `internal/order/service.go` and updates the 4 call sites.
