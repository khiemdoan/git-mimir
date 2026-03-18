package mcp

import (
	"context"
	"encoding/json"
	"fmt"
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
		"filePath": "file_path",
		"fileName": "file_path",
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
