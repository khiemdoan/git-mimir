package mcp

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/thuongh2/git-mimir/internal/incremental"
	"github.com/thuongh2/git-mimir/internal/registry"
	"github.com/thuongh2/git-mimir/internal/store"
)

// Tools handles all 7 MCP tool calls.
type Tools struct {
	reg *registry.Registry
}

func NewTools(reg *registry.Registry) *Tools {
	return &Tools{reg: reg}
}

// ToolDefinition is the schema for one MCP tool.
type ToolDefinition struct {
	Name        string                 `json:"name"`
	Description string                 `json:"description"`
	InputSchema map[string]interface{} `json:"inputSchema"`
}

// ListTools returns all tool definitions.
func (t *Tools) ListTools() map[string]interface{} {
	return map[string]interface{}{
		"tools": []ToolDefinition{
			{
				Name:        "query",
				Description: "Hybrid search over the code knowledge graph. Returns process-grouped results.",
				InputSchema: schema(`{"type":"object","properties":{"query":{"type":"string"},"repo":{"type":"string"},"limit":{"type":"integer"}},"required":["query"]}`),
			},
			{
				Name:        "context",
				Description: "360-degree view of a symbol: definition, incoming/outgoing edges, processes.",
				InputSchema: schema(`{"type":"object","properties":{"name":{"type":"string"},"repo":{"type":"string"}},"required":["name"]}`),
			},
			{
				Name:        "impact",
				Description: "Blast radius analysis: what does changing this symbol break?",
				InputSchema: schema(`{"type":"object","properties":{"target":{"type":"string"},"direction":{"type":"string","enum":["downstream","upstream"]},"min_confidence":{"type":"number"},"max_depth":{"type":"integer"},"relation_types":{"type":"array","items":{"type":"string"}},"include_tests":{"type":"boolean"},"repo":{"type":"string"}},"required":["target"]}`),
			},
			{
				Name:        "detect_changes",
				Description: "Detect uncommitted/recent git changes and their impact on the codebase.",
				InputSchema: schema(`{"type":"object","properties":{"scope":{"type":"string"},"repo":{"type":"string"}}}`),
			},
			{
				Name:        "rename",
				Description: "Plan a coordinated multi-file rename of a symbol.",
				InputSchema: schema(`{"type":"object","properties":{"symbol_name":{"type":"string"},"new_name":{"type":"string"},"dry_run":{"type":"boolean"},"repo":{"type":"string"}},"required":["symbol_name","new_name"]}`),
			},
			{
				Name:        "cypher",
				Description: "Run a Cypher-subset query against the knowledge graph (translated to SQL).",
				InputSchema: schema(`{"type":"object","properties":{"query":{"type":"string"},"repo":{"type":"string"}},"required":["query"]}`),
			},
			{
				Name:        "list_repos",
				Description: "List all indexed repositories.",
				InputSchema: schema(`{"type":"object","properties":{}}`),
			},
			{
				Name:        "find_referencing",
				Description: "Find all symbols that directly reference (call, import, extend, implement) a given symbol. Lighter than impact — returns 1-hop inbound edges only.",
				InputSchema: schema(`{"type":"object","properties":{"name":{"type":"string"},"edge_types":{"type":"array","items":{"type":"string"},"description":"Filter by edge type. One of: CALLS, IMPORTS, EXTENDS, IMPLEMENTS, MEMBER_OF. Defaults to all."},"min_confidence":{"type":"number"},"repo":{"type":"string"}},"required":["name"]}`),
			},
			{
				Name:        "symbol_coordinates",
				Description: "Return the exact file path and line range for a symbol. Use before editing — gives the precise location to replace.",
				InputSchema: schema(`{"type":"object","properties":{"name":{"type":"string"},"repo":{"type":"string"}},"required":["name"]}`),
			},
			{
				Name:        "get_symbols_overview",
				Description: "Gets an overview of all top-level symbols defined in a given file, sorted by line number. Excludes nested methods and members. Use to understand file structure before editing.",
				InputSchema: schema(`{"type":"object","properties":{"file_path":{"type":"string"},"include_private":{"type":"boolean","description":"Include non-exported symbols. Defaults to true."},"repo":{"type":"string","description":"Name of the indexed repository to query. Required."}},"required":["file_path","repo"]}`),
			},
			{
				Name:        "find_symbol_body",
				Description: "Returns the exact source code body of a symbol (function, method, class) by name, including file_path, start_line, end_line, and the full implementation text. Use when you see a function name in logs or stack traces — fetches only the relevant lines instead of reading the whole file.",
				InputSchema: schema(`{"type":"object","properties":{"name":{"type":"string","description":"Function, method, or class name to look up"},"repo":{"type":"string"}},"required":["name"]}`),
			},
			{
				Name:        "query_repo",
				Description: "Execute a whitelisted read-only tool against a different indexed repository. Pass tool_name, arguments, target_repo, and optional current_repo. Allowed tools: query, context, find_referencing, symbol_coordinates, get_symbols_overview, impact.",
				InputSchema: schema(`{"type":"object","properties":{"tool_name":{"type":"string","description":"Tool to invoke on the target repo. One of: query, context, find_referencing, symbol_coordinates, get_symbols_overview, impact."},"arguments":{"type":"object","description":"Arguments to pass to the tool."},"target_repo":{"type":"string","description":"Name of the target repository to query."},"current_repo":{"type":"string","description":"Name of the current repository (optional, for context)."}},"required":["tool_name","arguments","target_repo"]}`),
			},
		},
	}
}

// Call dispatches a tools/call request.
func (t *Tools) Call(ctx context.Context, params json.RawMessage) Response {
	var p struct {
		Name      string          `json:"name"`
		Arguments json.RawMessage `json:"arguments"`
	}
	if err := json.Unmarshal(params, &p); err != nil {
		return errResp(ErrInvalidParams, "invalid params: "+err.Error())
	}

	logToolCall(p.Name, "")

	switch p.Name {
	case "list_repos":
		return t.listRepos(p.Arguments)
	case "query":
		return t.query(ctx, p.Arguments)
	case "context":
		return t.context(ctx, p.Arguments)
	case "impact":
		return t.impact(ctx, p.Arguments)
	case "detect_changes":
		return t.detectChanges(ctx, p.Arguments)
	case "rename":
		return t.rename(ctx, p.Arguments)
	case "cypher":
		return t.cypher(ctx, p.Arguments)
	case "find_referencing":
		return t.findReferencing(ctx, p.Arguments)
	case "symbol_coordinates":
		return t.symbolCoordinates(ctx, p.Arguments)
	case "get_symbols_overview":
		return t.getSymbolsOverview(ctx, p.Arguments)
	case "query_repo":
		return t.queryRepo(ctx, p.Arguments)
	case "find_symbol_body":
		return t.findSymbolBody(ctx, p.Arguments)
	default:
		return errResp(ErrMethodNotFound, "unknown tool: "+p.Name)
	}
}

func (t *Tools) listRepos(_ json.RawMessage) Response {
	repos := t.reg.List()
	logDebug("listRepos: found %d repos", len(repos))
	return toolResult(map[string]interface{}{"repos": repos})
}

func (t *Tools) query(ctx context.Context, args json.RawMessage) Response {
	var input struct {
		Query string  `json:"query"`
		Repo  *string `json:"repo"`
		Limit *int    `json:"limit"`
	}
	if err := json.Unmarshal(args, &input); err != nil {
		return errResp(ErrInvalidParams, err.Error())
	}

	logDebug("query: query=%q repo=%v limit=%v", input.Query, input.Repo, input.Limit)

	s, err := t.openStore(input.Repo)
	if err != nil {
		logError("query.openStore", err)
		return errResp(ErrInternal, err.Error())
	}
	defer s.Close()

	limit := 20
	if input.Limit != nil {
		limit = *input.Limit
	}

	terms := tokenizeQuery(input.Query)
	results, err := s.HybridSearch(terms, nil, limit)
	if err != nil {
		logError("query.HybridSearch", err)
		return errResp(ErrInternal, err.Error())
	}

	logDebug("query: found %d results", len(results))
	return toolResult(map[string]interface{}{
		"definitions": results,
		"query":       input.Query,
	})
}

func (t *Tools) context(ctx context.Context, args json.RawMessage) Response {
	var input struct {
		Name string  `json:"name"`
		Repo *string `json:"repo"`
	}
	if err := json.Unmarshal(args, &input); err != nil {
		return errResp(ErrInvalidParams, err.Error())
	}

	logDebug("context: name=%q repo=%v", input.Name, input.Repo)

	s, err := t.openStore(input.Repo)
	if err != nil {
		logError("context.openStore", err)
		return errResp(ErrInternal, err.Error())
	}
	defer s.Close()

	nodes, err := s.QuerySymbol(input.Name)
	if err != nil {
		logError("context.QuerySymbol", err)
		return errResp(ErrInternal, err.Error())
	}
	if len(nodes) == 0 {
		logDebug("context: symbol not found: %s", input.Name)
		return toolResult(map[string]interface{}{"symbol": nil, "message": "symbol not found"})
	}

	node := nodes[0]
	outEdges, _ := s.QueryEdgesFrom(node.UID)
	inEdges, _ := s.QueryEdgesTo(node.UID)

	logDebug("context: found %d outgoing edges, %d incoming edges", len(outEdges), len(inEdges))
	return toolResult(map[string]interface{}{
		"symbol":   node,
		"outgoing": outEdges,
		"incoming": inEdges,
	})
}

func (t *Tools) impact(ctx context.Context, args json.RawMessage) Response {
	var input struct {
		Target        string   `json:"target"`
		Direction     string   `json:"direction"`
		MinConfidence *float64 `json:"min_confidence"`
		MaxDepth      *int     `json:"max_depth"`
		Repo          *string  `json:"repo"`
	}
	if err := json.Unmarshal(args, &input); err != nil {
		return errResp(ErrInvalidParams, err.Error())
	}

	logDebug("impact: target=%q direction=%q minConf=%v maxDepth=%v", input.Target, input.Direction, input.MinConfidence, input.MaxDepth)

	s, err := t.openStore(input.Repo)
	if err != nil {
		logError("impact.openStore", err)
		return errResp(ErrInternal, err.Error())
	}
	defer s.Close()

	direction := input.Direction
	if direction == "" {
		direction = "downstream"
	}
	minConf := 0.5
	if input.MinConfidence != nil {
		minConf = *input.MinConfidence
	}
	maxDepth := 5
	if input.MaxDepth != nil {
		maxDepth = *input.MaxDepth
	}

	// Find the target node
	nodes, err := s.QuerySymbol(input.Target)
	if err != nil {
		logError("impact.QuerySymbol", err)
		return errResp(ErrInternal, err.Error())
	}
	if len(nodes) == 0 {
		logDebug("impact: target not found: %s", input.Target)
		return toolResult(map[string]interface{}{"target": nil, "message": "symbol not found"})
	}

	target := nodes[0]
	rows, err := s.QueryImpact(target.UID, direction, minConf, maxDepth)
	if err != nil {
		logError("impact.QueryImpact", err)
		return errResp(ErrInternal, err.Error())
	}

	logDebug("impact: found %d affected nodes", len(rows))
	return toolResult(map[string]interface{}{
		"target":    target,
		"direction": direction,
		"affected":  rows,
	})
}

func (t *Tools) detectChanges(ctx context.Context, args json.RawMessage) Response {
	var input struct {
		Scope *string `json:"scope"`
		Repo  *string `json:"repo"`
	}
	if err := json.Unmarshal(args, &input); err != nil {
		return errResp(ErrInvalidParams, err.Error())
	}

	logDebug("detectChanges: scope=%v repo=%v", input.Scope, input.Repo)

	repoName := t.resolveRepoName(input.Repo)
	repoInfo := t.reg.Get(repoName)
	if repoInfo == nil {
		logError("detectChanges.repo", fmt.Errorf("repo not found: %s", repoName))
		return errResp(ErrInternal, "repo not found: "+repoName)
	}

	s, err := t.openStore(input.Repo)
	if err != nil {
		logError("detectChanges.openStore", err)
		return errResp(ErrInternal, err.Error())
	}
	defer s.Close()

	lastCommit, _ := s.GetMeta("last_commit")
	changed, err := incremental.GetChangedFiles(repoInfo.Path, lastCommit)
	if err != nil {
		logError("detectChanges.GetChangedFiles", err)
		return errResp(ErrInternal, err.Error())
	}

	logDebug("detectChanges: %d changed files", len(changed))
	return toolResult(map[string]interface{}{
		"changed_files": changed,
		"total":         len(changed),
	})
}

func (t *Tools) rename(ctx context.Context, args json.RawMessage) Response {
	var input struct {
		SymbolName string  `json:"symbol_name"`
		NewName    string  `json:"new_name"`
		DryRun     *bool   `json:"dry_run"`
		Repo       *string `json:"repo"`
	}
	if err := json.Unmarshal(args, &input); err != nil {
		return errResp(ErrInvalidParams, err.Error())
	}

	logDebug("rename: symbol=%q newName=%q dryRun=%v", input.SymbolName, input.NewName, input.DryRun)

	s, err := t.openStore(input.Repo)
	if err != nil {
		logError("rename.openStore", err)
		return errResp(ErrInternal, err.Error())
	}
	defer s.Close()

	nodes, err := s.QuerySymbol(input.SymbolName)
	if err != nil {
		logError("rename.QuerySymbol", err)
		return errResp(ErrInternal, err.Error())
	}

	dryRun := true
	if input.DryRun != nil {
		dryRun = *input.DryRun
	}

	changes := make([]map[string]interface{}, 0, len(nodes))
	for _, n := range nodes {
		changes = append(changes, map[string]interface{}{
			"file":     n.FilePath,
			"line":     n.StartLine,
			"old_name": input.SymbolName,
			"new_name": input.NewName,
		})
	}

	logDebug("rename: %d files affected", len(nodes))
	return toolResult(map[string]interface{}{
		"status":         "planned",
		"dry_run":        dryRun,
		"files_affected": len(nodes),
		"changes":        changes,
	})
}

func (t *Tools) cypher(ctx context.Context, args json.RawMessage) Response {
	var input struct {
		Query string  `json:"query"`
		Repo  *string `json:"repo"`
	}
	if err := json.Unmarshal(args, &input); err != nil {
		return errResp(ErrInvalidParams, err.Error())
	}

	logDebug("cypher: query=%q repo=%v", input.Query, input.Repo)

	s, err := t.openStore(input.Repo)
	if err != nil {
		logError("cypher.openStore", err)
		return errResp(ErrInternal, err.Error())
	}
	defer s.Close()

	sqlQuery, err := translateCypher(input.Query)
	if err != nil {
		logError("cypher.translateCypher", err)
		return toolResult(map[string]interface{}{
			"error": fmt.Sprintf("Unsupported Cypher: %s. Use SQL instead.", err.Error()),
		})
	}

	rows, columns, err := runRawQuery(s, sqlQuery)
	if err != nil {
		logError("cypher.runRawQuery", err)
		return errResp(ErrInternal, err.Error())
	}

	logDebug("cypher: returned %d rows, %d columns", len(rows), len(columns))

	return toolResult(map[string]interface{}{
		"columns": columns,
		"rows":    rows,
	})
}

func (t *Tools) findReferencing(ctx context.Context, args json.RawMessage) Response {
	var input struct {
		Name          string   `json:"name"`
		EdgeTypes     []string `json:"edge_types"`
		MinConfidence *float64 `json:"min_confidence"`
		Repo          *string  `json:"repo"`
	}
	if err := json.Unmarshal(args, &input); err != nil {
		return errResp(ErrInvalidParams, err.Error())
	}

	logDebug("findReferencing: name=%q edgeTypes=%v minConf=%v", input.Name, input.EdgeTypes, input.MinConfidence)

	s, err := t.openStore(input.Repo)
	if err != nil {
		logError("findReferencing.openStore", err)
		return errResp(ErrInternal, err.Error())
	}
	defer s.Close()

	nodes, err := s.QuerySymbol(input.Name)
	if err != nil {
		logError("findReferencing.QuerySymbol", err)
		return errResp(ErrInternal, err.Error())
	}
	if len(nodes) == 0 {
		logDebug("findReferencing: symbol not found: %s", input.Name)
		return toolResult(map[string]interface{}{"symbol": nil, "message": "symbol not found"})
	}

	target := nodes[0]

	minConf := 0.0
	if input.MinConfidence != nil {
		minConf = *input.MinConfidence
	}

	// Build edge-type filter set.
	edgeTypeSet := map[string]bool{}
	for _, et := range input.EdgeTypes {
		edgeTypeSet[strings.ToUpper(et)] = true
	}

	inEdges, err := s.QueryEdgesTo(target.UID)
	if err != nil {
		logError("findReferencing.QueryEdgesTo", err)
		return errResp(ErrInternal, err.Error())
	}

	type refEntry struct {
		Symbol     interface{} `json:"symbol"`
		EdgeType   string      `json:"edge_type"`
		Confidence float64     `json:"confidence"`
	}

	refs := make([]refEntry, 0, len(inEdges))
	for _, e := range inEdges {
		if len(edgeTypeSet) > 0 && !edgeTypeSet[e.Type] {
			continue
		}
		if e.Confidence < minConf {
			continue
		}
		caller, err := s.QueryNodeByUID(e.FromUID)
		if err != nil || caller == nil {
			continue
		}
		refs = append(refs, refEntry{
			Symbol:     caller,
			EdgeType:   e.Type,
			Confidence: e.Confidence,
		})
	}

	logDebug("findReferencing: %d references found for %s", len(refs), input.Name)
	return toolResult(map[string]interface{}{
		"target":     target.Name,
		"total":      len(refs),
		"references": refs,
	})
}

func (t *Tools) symbolCoordinates(ctx context.Context, args json.RawMessage) Response {
	var input struct {
		Name string  `json:"name"`
		Repo *string `json:"repo"`
	}
	if err := json.Unmarshal(args, &input); err != nil {
		return errResp(ErrInvalidParams, err.Error())
	}

	logDebug("symbolCoordinates: name=%q repo=%v", input.Name, input.Repo)

	s, err := t.openStore(input.Repo)
	if err != nil {
		logError("symbolCoordinates.openStore", err)
		return errResp(ErrInternal, err.Error())
	}
	defer s.Close()

	nodes, err := s.QuerySymbol(input.Name)
	if err != nil {
		logError("symbolCoordinates.QuerySymbol", err)
		return errResp(ErrInternal, err.Error())
	}
	if len(nodes) == 0 {
		logDebug("symbolCoordinates: symbol not found: %s", input.Name)
		return toolResult(map[string]interface{}{"symbol": nil, "message": "symbol not found"})
	}

	results := make([]map[string]interface{}, 0, len(nodes))
	for _, n := range nodes {
		results = append(results, map[string]interface{}{
			"name":         n.Name,
			"kind":         n.Kind,
			"file_path":    n.FilePath,
			"start_line":   n.StartLine,
			"end_line":     n.EndLine,
			"package_path": n.PackagePath,
			"exported":     n.Exported,
		})
	}

	logDebug("symbolCoordinates: found %d locations for %s", len(results), input.Name)
	return toolResult(map[string]interface{}{
		"name":    input.Name,
		"matches": results,
	})
}

func (t *Tools) getSymbolsOverview(ctx context.Context, args json.RawMessage) Response {
	var input struct {
		FilePath       string  `json:"file_path"`
		IncludePrivate *bool   `json:"include_private"`
		Repo           *string `json:"repo"`
	}
	if err := json.Unmarshal(args, &input); err != nil {
		return errResp(ErrInvalidParams, err.Error())
	}

	logDebug("getSymbolsOverview: file_path=%q repo=%v", input.FilePath, input.Repo)

	s, err := t.openStore(input.Repo)
	if err != nil {
		logError("getSymbolsOverview.openStore", err)
		return errResp(ErrInternal, err.Error())
	}
	defer s.Close()

	// Nodes are stored with absolute paths. Normalize a relative input path
	// using the repo root stored in index_meta so agents can pass relative paths.
	filePath := input.FilePath
	if !filepath.IsAbs(filePath) {
		if repoRoot, err := s.GetMeta("repo_path"); err == nil && repoRoot != "" {
			filePath = filepath.Join(repoRoot, filePath)
		}
	}

	nodes, err := s.QueryTopLevelByFile(filePath)
	if err != nil {
		logError("getSymbolsOverview.QueryTopLevelByFile", err)
		return errResp(ErrInternal, err.Error())
	}

	// By default include all symbols; filter private only when explicitly disabled.
	includePrivate := true
	if input.IncludePrivate != nil {
		includePrivate = *input.IncludePrivate
	}

	results := make([]map[string]interface{}, 0, len(nodes))
	for _, n := range nodes {
		if !includePrivate && !n.Exported {
			continue
		}
		results = append(results, map[string]interface{}{
			"name":         n.Name,
			"kind":         n.Kind,
			"start_line":   n.StartLine,
			"end_line":     n.EndLine,
			"exported":     n.Exported,
			"package_path": n.PackagePath,
		})
	}

	logDebug("getSymbolsOverview: %d top-level symbols in %s", len(results), filePath)
	return toolResult(map[string]interface{}{
		"file_path": input.FilePath,
		"total":     len(results),
		"symbols":   results,
	})
}

// queryRepoWhitelist contains tools safe to execute cross-repo.
var queryRepoWhitelist = map[string]bool{
	"query":                true,
	"context":              true,
	"find_referencing":     true,
	"symbol_coordinates":   true,
	"get_symbols_overview": true,
	"impact":               true,
}

func (t *Tools) queryRepo(ctx context.Context, args json.RawMessage) Response {
	var input struct {
		ToolName    string          `json:"tool_name"`
		Arguments   json.RawMessage `json:"arguments"`
		TargetRepo  string          `json:"target_repo"`
		CurrentRepo *string         `json:"current_repo"`
	}
	if err := json.Unmarshal(args, &input); err != nil {
		return errResp(ErrInvalidParams, err.Error())
	}
	if !queryRepoWhitelist[input.ToolName] {
		return errResp(ErrInvalidParams, fmt.Sprintf("tool %q is not allowed in query_repo; allowed: query, context, find_referencing, symbol_coordinates, get_symbols_overview, impact", input.ToolName))
	}

	// Inject target_repo into arguments.
	var argMap map[string]interface{}
	if len(input.Arguments) > 0 {
		if err := json.Unmarshal(input.Arguments, &argMap); err != nil {
			return errResp(ErrInvalidParams, "invalid arguments: "+err.Error())
		}
	} else {
		argMap = map[string]interface{}{}
	}
	argMap["repo"] = input.TargetRepo
	injectedArgs, _ := json.Marshal(argMap)

	// Delegate to existing dispatcher via Call().
	delegateParams, _ := json.Marshal(map[string]interface{}{
		"name":      input.ToolName,
		"arguments": json.RawMessage(injectedArgs),
	})
	resp := t.Call(ctx, delegateParams)
	if resp.Error != nil {
		return resp
	}

	// Attach meta inside the content text so the agent can read it.
	currentRepo := ""
	if input.CurrentRepo != nil {
		currentRepo = *input.CurrentRepo
	}
	meta := map[string]interface{}{
		"queried_repo": input.TargetRepo,
		"current_repo": currentRepo,
		"tool_used":    input.ToolName,
	}
	if resultMap, ok := resp.Result.(map[string]interface{}); ok {
		if content, ok := resultMap["content"].([]map[string]interface{}); ok && len(content) > 0 {
			if text, ok := content[0]["text"].(string); ok {
				var innerData map[string]interface{}
				if err := json.Unmarshal([]byte(text), &innerData); err == nil {
					innerData["meta"] = meta
					if b, err := json.Marshal(innerData); err == nil {
						content[0]["text"] = string(b)
					}
				}
			}
		}
	}
	logDebug("queryRepo: tool=%s target=%s", input.ToolName, input.TargetRepo)
	return resp
}

func (t *Tools) findSymbolBody(ctx context.Context, args json.RawMessage) Response {
	var input struct {
		Name string  `json:"name"`
		Repo *string `json:"repo"`
	}
	if err := json.Unmarshal(args, &input); err != nil {
		return errResp(ErrInvalidParams, err.Error())
	}

	logDebug("findSymbolBody: name=%q repo=%v", input.Name, input.Repo)

	s, err := t.openStore(input.Repo)
	if err != nil {
		logError("findSymbolBody.openStore", err)
		return errResp(ErrInternal, err.Error())
	}
	defer s.Close()

	nodes, err := s.QuerySymbol(input.Name)
	if err != nil {
		logError("findSymbolBody.QuerySymbol", err)
		return errResp(ErrInternal, err.Error())
	}
	if len(nodes) == 0 {
		logDebug("findSymbolBody: symbol not found: %s", input.Name)
		return toolResult(map[string]interface{}{"symbol": nil, "message": "symbol not found"})
	}

	type bodyResult struct {
		Name        string `json:"name"`
		Kind        string `json:"kind"`
		FilePath    string `json:"file_path"`
		StartLine   uint   `json:"start_line"`
		EndLine     uint   `json:"end_line"`
		PackagePath string `json:"package_path"`
		Exported    bool   `json:"exported"`
		Body        string `json:"body"`
	}

	results := make([]bodyResult, 0, len(nodes))
	for _, n := range nodes {
		body, readErr := readLines(n.FilePath, n.StartLine, n.EndLine)
		if readErr != nil {
			logDebug("findSymbolBody: read %s: %v", n.FilePath, readErr)
			body = ""
		}
		results = append(results, bodyResult{
			Name:        n.Name,
			Kind:        n.Kind,
			FilePath:    n.FilePath,
			StartLine:   n.StartLine,
			EndLine:     n.EndLine,
			PackagePath: n.PackagePath,
			Exported:    n.Exported,
			Body:        body,
		})
	}

	logDebug("findSymbolBody: %d matches for %s", len(results), input.Name)
	return toolResult(map[string]interface{}{
		"name":    input.Name,
		"matches": results,
	})
}

// readLines reads lines [start, end] (1-based, inclusive) from a file.
func readLines(filePath string, start, end uint) (string, error) {
	f, err := os.Open(filePath)
	if err != nil {
		return "", fmt.Errorf("open %s: %w", filePath, err)
	}
	defer f.Close()

	var sb strings.Builder
	scanner := bufio.NewScanner(f)
	var lineNum uint = 1
	for scanner.Scan() {
		if lineNum >= start && lineNum <= end {
			sb.WriteString(scanner.Text())
			sb.WriteByte('\n')
		}
		if lineNum > end {
			break
		}
		lineNum++
	}
	if err := scanner.Err(); err != nil {
		return "", fmt.Errorf("scan %s: %w", filePath, err)
	}
	return sb.String(), nil
}

// openStore opens the store for the given repo name.
func (t *Tools) openStore(repoName *string) (*store.Store, error) {
	name := t.resolveRepoName(repoName)
	dbPath, err := registry.DBPath(name)
	if err != nil {
		return nil, err
	}
	return store.OpenStore(dbPath)
}

func (t *Tools) resolveRepoName(repoName *string) string {
	if repoName != nil && *repoName != "" {
		return *repoName
	}
	repos := t.reg.List()
	if len(repos) == 1 {
		return repos[0].Name
	}
	return ""
}

func schema(s string) map[string]interface{} {
	var m map[string]interface{}
	json.Unmarshal([]byte(s), &m)
	return m
}

func errResp(code int, msg string) Response {
	return Response{Error: &RPCError{Code: code, Message: msg}}
}

func toolResult(data interface{}) Response {
	b, _ := json.Marshal(data)
	return Response{
		Result: map[string]interface{}{
			"content": []map[string]interface{}{
				{"type": "text", "text": string(b)},
			},
		},
	}
}

func tokenizeQuery(q string) []string {
	words := strings.Fields(strings.ToLower(q))
	var result []string
	for _, w := range words {
		// Strip punctuation
		w = strings.Trim(w, ".,;:!?\"'()")
		if len(w) >= 2 {
			result = append(result, w)
		}
	}
	return result
}

// translateCypher translates a limited Cypher subset to SQLite SQL.
func translateCypher(cypher string) (string, error) {
	q := strings.TrimSpace(cypher)

	// Pattern: MATCH (n:Kind) WHERE n.prop = 'val' RETURN n.col LIMIT k
	if strings.HasPrefix(strings.ToUpper(q), "MATCH") {
		return translateMatchQuery(q)
	}
	return "", fmt.Errorf("only MATCH queries are supported")
}

func translateMatchQuery(cypher string) (string, error) {
	upper := strings.ToUpper(cypher)

	// Detect edge pattern: MATCH (a)-[r:TYPE]->(b)
	if strings.Contains(upper, "]-[") || strings.Contains(upper, "]->(") || strings.Contains(upper, ")<-[") {
		return translateEdgeMatch(cypher)
	}

	// Node-only pattern: MATCH (n:Kind ...) WHERE ... RETURN ... LIMIT ...
	return translateNodeMatch(cypher)
}

func translateNodeMatch(cypher string) (string, error) {
	// Extract kind from (n:Kind) or (n:Kind {name: 'val'})
	kind := extractBetween(cypher, ":", "{")
	if kind == "" {
		kind = extractBetween(cypher, ":", ")")
	}
	kind = strings.TrimSpace(kind)

	var whereClauses []string
	if kind != "" {
		whereClauses = append(whereClauses, fmt.Sprintf("kind = '%s'", escapeSQLString(kind)))
	}

	// Extract inline props: {name: 'val'}
	if strings.Contains(cypher, "{") {
		prop, val := extractInlineProp(cypher)
		if prop != "" && val != "" {
			whereClauses = append(whereClauses, fmt.Sprintf("%s = '%s'", mapProp(prop), escapeSQLString(val)))
		}
	}

	// Extract WHERE clause
	if i := strings.Index(strings.ToUpper(cypher), " WHERE "); i >= 0 {
		whereStr := cypher[i+7:]
		if j := strings.Index(strings.ToUpper(whereStr), " RETURN "); j >= 0 {
			whereStr = whereStr[:j]
		}
		clause := translateWhereClause(whereStr)
		if clause != "" {
			whereClauses = append(whereClauses, clause)
		}
	}

	// Extract LIMIT
	limit := "100"
	if i := strings.Index(strings.ToUpper(cypher), " LIMIT "); i >= 0 {
		limitStr := strings.TrimSpace(cypher[i+7:])
		limit = strings.Fields(limitStr)[0]
	}

	// Extract RETURN columns
	cols := extractReturnCols(cypher)

	where := ""
	if len(whereClauses) > 0 {
		where = " WHERE " + strings.Join(whereClauses, " AND ")
	}

	return fmt.Sprintf("SELECT %s FROM nodes%s LIMIT %s", cols, where, limit), nil
}

func translateEdgeMatch(cypher string) (string, error) {
	// MATCH (a)-[r:CALLS]->(b) WHERE r.confidence > 0.8 RETURN a.name, b.name
	edgeType := extractBetween(cypher, ":", "]")
	edgeType = strings.TrimSpace(edgeType)

	var whereClauses []string
	if edgeType != "" {
		whereClauses = append(whereClauses, fmt.Sprintf("e.type = '%s'", escapeSQLString(edgeType)))
	}

	if i := strings.Index(strings.ToUpper(cypher), " WHERE "); i >= 0 {
		whereStr := cypher[i+7:]
		if j := strings.Index(strings.ToUpper(whereStr), " RETURN "); j >= 0 {
			whereStr = whereStr[:j]
		}
		clause := translateEdgeWhereClause(whereStr)
		if clause != "" {
			whereClauses = append(whereClauses, clause)
		}
	}

	limit := "100"
	if i := strings.Index(strings.ToUpper(cypher), " LIMIT "); i >= 0 {
		limitStr := strings.TrimSpace(cypher[i+7:])
		limit = strings.Fields(limitStr)[0]
	}

	where := ""
	if len(whereClauses) > 0 {
		where = " AND " + strings.Join(whereClauses, " AND ")
	}

	return fmt.Sprintf(`
		SELECT a.name AS from_name, b.name AS to_name, e.type, e.confidence
		FROM edges e
		JOIN nodes a ON a.uid = e.from_uid
		JOIN nodes b ON b.uid = e.to_uid
		WHERE 1=1%s LIMIT %s`, where, limit), nil
}

func translateWhereClause(where string) string {
	// n.name = 'foo' → name = 'foo'
	// n.kind = 'Function' → kind = 'Function'
	where = strings.ReplaceAll(where, "n.", "")
	return translatePropRefs(where)
}

func translateEdgeWhereClause(where string) string {
	where = strings.ReplaceAll(where, "r.", "e.")
	where = strings.ReplaceAll(where, "a.", "a.")
	where = strings.ReplaceAll(where, "b.", "b.")
	return where
}

func translatePropRefs(s string) string {
	replacements := map[string]string{
		"filePath":  "file_path",
		"fileName":  "file_path",
		"startLine": "start_line",
		"endLine":   "end_line",
	}
	for from, to := range replacements {
		s = strings.ReplaceAll(s, from, to)
	}
	return s
}

func extractBetween(s, start, end string) string {
	i := strings.Index(s, start)
	if i < 0 {
		return ""
	}
	s = s[i+len(start):]
	j := strings.Index(s, end)
	if j < 0 {
		return s
	}
	return s[:j]
}

func extractInlineProp(cypher string) (string, string) {
	s := extractBetween(cypher, "{", "}")
	if s == "" {
		return "", ""
	}
	// "name: 'val'" or `name: "val"`
	parts := strings.SplitN(s, ":", 2)
	if len(parts) != 2 {
		return "", ""
	}
	prop := strings.TrimSpace(parts[0])
	val := strings.Trim(strings.TrimSpace(parts[1]), `"'`)
	return prop, val
}

func extractReturnCols(cypher string) string {
	i := strings.Index(strings.ToUpper(cypher), " RETURN ")
	if i < 0 {
		return "uid, name, kind, file_path"
	}
	ret := cypher[i+8:]
	if j := strings.Index(strings.ToUpper(ret), " LIMIT "); j >= 0 {
		ret = ret[:j]
	}
	// n.name → name, n.filePath → file_path
	ret = strings.ReplaceAll(ret, "n.", "")
	ret = translatePropRefs(ret)
	if strings.TrimSpace(ret) == "n" || strings.TrimSpace(ret) == "" {
		return "uid, name, kind, file_path"
	}
	return ret
}

func mapProp(prop string) string {
	switch prop {
	case "filePath", "file_path":
		return "file_path"
	case "startLine", "start_line":
		return "start_line"
	case "endLine", "end_line":
		return "end_line"
	default:
		return prop
	}
}

func escapeSQLString(s string) string {
	return strings.ReplaceAll(s, "'", "''")
}

// runRawQuery executes a raw SQL query and returns rows + column names.
func runRawQuery(s *store.Store, query string) ([][]interface{}, []string, error) {
	var rows [][]interface{}
	var columns []string
	err := s.ReadRaw(query, func(cols []string, rs [][]interface{}) {
		columns = cols
		rows = rs
	})
	return rows, columns, err
}
