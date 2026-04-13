package mcp_test

import (
	"context"
	"encoding/json"
	"os"
	"strings"
	"testing"

	"github.com/thuongh2/git-mimir/internal/graph"
	"github.com/thuongh2/git-mimir/internal/registry"
	"github.com/thuongh2/git-mimir/internal/store"
	"github.com/thuongh2/git-mimir/mcp"
)

// seedTestRepo creates an isolated store in a temp HOME, seeds it with data,
// and returns a Tools instance pointing at that repo.
func seedTestRepo(t *testing.T, nodes []graph.Node, edges []graph.Edge) *mcp.Tools {
	t.Helper()
	tmpHome := t.TempDir()
	t.Setenv("HOME", tmpHome)

	dbPath, err := registry.DBPath("testrepo")
	if err != nil {
		t.Fatalf("DBPath: %v", err)
	}
	s, err := store.OpenStore(dbPath)
	if err != nil {
		t.Fatalf("OpenStore: %v", err)
	}
	if len(nodes) > 0 {
		if err := s.BatchUpsertNodes(nodes); err != nil {
			t.Fatalf("BatchUpsertNodes: %v", err)
		}
	}
	if len(edges) > 0 {
		if err := s.BatchUpsertEdges(edges); err != nil {
			t.Fatalf("BatchUpsertEdges: %v", err)
		}
	}
	s.Close()

	reg := &registry.Registry{Repos: []registry.RepoInfo{{Name: "testrepo", Path: tmpHome}}}
	return mcp.NewTools(reg)
}

func TestTools_ListTools(t *testing.T) {
	reg := &registry.Registry{}
	tools := mcp.NewTools(reg)

	result := tools.ListTools()
	toolList, ok := result["tools"]
	if !ok {
		t.Fatal("ListTools missing 'tools' key")
	}

	defs, ok := toolList.([]mcp.ToolDefinition)
	if !ok {
		t.Fatal("tools is not []ToolDefinition")
	}

	if len(defs) != 12 {
		t.Errorf("expected 12 tools, got %d", len(defs))
	}

	names := map[string]bool{}
	for _, d := range defs {
		names[d.Name] = true
		t.Logf("  tool: %s", d.Name)
	}

	for _, want := range []string{"query", "context", "impact", "detect_changes", "rename", "cypher", "list_repos", "find_referencing", "symbol_coordinates", "get_symbols_overview", "query_repo", "find_symbol_body"} {
		if !names[want] {
			t.Errorf("missing tool: %s", want)
		}
	}
}

func TestTools_ListRepos(t *testing.T) {
	reg := &registry.Registry{Repos: []registry.RepoInfo{
		{Name: "myapp", Path: "/src/myapp"},
	}}
	tools := mcp.NewTools(reg)

	args, _ := json.Marshal(map[string]any{
		"name":      "list_repos",
		"arguments": map[string]any{},
	})
	resp := tools.Call(context.Background(), args)

	if resp.Error != nil {
		t.Fatalf("list_repos error: %s", resp.Error.Message)
	}
	if resp.Result == nil {
		t.Fatal("list_repos returned nil result")
	}
}

func TestTools_FindReferencing(t *testing.T) {
	caller := graph.Node{UID: "n1", Name: "callMe", Kind: "Function", FilePath: "a.go", StartLine: 1, EndLine: 5}
	callee := graph.Node{UID: "n2", Name: "target", Kind: "Function", FilePath: "b.go", StartLine: 10, EndLine: 20}
	edge := graph.Edge{FromUID: "n1", ToUID: "n2", Type: graph.EdgeCalls, Confidence: 0.9}

	tools := seedTestRepo(t, []graph.Node{caller, callee}, []graph.Edge{edge})

	cases := []struct {
		name         string
		symbolName   string
		edgeTypes    []string
		minConf      *float64
		wantTotal    int
		wantNotFound bool
	}{
		{
			name:       "finds direct caller",
			symbolName: "target",
			wantTotal:  1,
		},
		{
			name:       "filters by edge type match",
			symbolName: "target",
			edgeTypes:  []string{"CALLS"},
			wantTotal:  1,
		},
		{
			name:       "filters by edge type no match",
			symbolName: "target",
			edgeTypes:  []string{"IMPORTS"},
			wantTotal:  0,
		},
		{
			name:         "symbol not found",
			symbolName:   "nonexistent",
			wantNotFound: true,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			input := map[string]any{"name": tc.symbolName}
			if len(tc.edgeTypes) > 0 {
				input["edge_types"] = tc.edgeTypes
			}
			if tc.minConf != nil {
				input["min_confidence"] = *tc.minConf
			}
			args, _ := json.Marshal(map[string]any{
				"name":      "find_referencing",
				"arguments": input,
			})
			resp := tools.Call(context.Background(), args)
			if resp.Error != nil {
				t.Fatalf("find_referencing error: %s", resp.Error.Message)
			}

			content := extractText(t, resp)
			t.Logf("find_referencing response: %s", content)

			var result map[string]any
			if err := json.Unmarshal([]byte(content), &result); err != nil {
				t.Fatalf("unmarshal response: %v", err)
			}

			if tc.wantNotFound {
				if result["symbol"] != nil {
					t.Errorf("expected symbol=nil, got %v", result["symbol"])
				}
				return
			}

			total, ok := result["total"].(float64)
			if !ok {
				t.Fatalf("total missing or wrong type: %v", result["total"])
			}
			if int(total) != tc.wantTotal {
				t.Errorf("total = %d, want %d", int(total), tc.wantTotal)
			}
		})
	}
}

func TestTools_SymbolCoordinates(t *testing.T) {
	fn := graph.Node{UID: "n1", Name: "MyFunc", Kind: "Function", FilePath: "pkg/foo.go", StartLine: 42, EndLine: 67, Exported: true, PackagePath: "pkg"}
	tools := seedTestRepo(t, []graph.Node{fn}, nil)

	cases := []struct {
		name          string
		symbolName    string
		wantNotFound  bool
		wantStartLine uint
		wantEndLine   uint
	}{
		{
			name:          "returns coordinates for known symbol",
			symbolName:    "MyFunc",
			wantStartLine: 42,
			wantEndLine:   67,
		},
		{
			name:         "symbol not found",
			symbolName:   "UnknownFunc",
			wantNotFound: true,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			args, _ := json.Marshal(map[string]any{
				"name":      "symbol_coordinates",
				"arguments": map[string]any{"name": tc.symbolName},
			})
			resp := tools.Call(context.Background(), args)
			if resp.Error != nil {
				t.Fatalf("symbol_coordinates error: %s", resp.Error.Message)
			}

			content := extractText(t, resp)
			t.Logf("symbol_coordinates response: %s", content)

			var result map[string]any
			if err := json.Unmarshal([]byte(content), &result); err != nil {
				t.Fatalf("unmarshal response: %v", err)
			}

			if tc.wantNotFound {
				if result["symbol"] != nil {
					t.Errorf("expected symbol=nil, got %v", result["symbol"])
				}
				return
			}

			matches, ok := result["matches"].([]any)
			if !ok || len(matches) == 0 {
				t.Fatalf("matches missing or empty: %v", result["matches"])
			}
			m := matches[0].(map[string]any)

			if got := m["file_path"]; got != fn.FilePath {
				t.Errorf("file_path = %v, want %v", got, fn.FilePath)
			}
			if got := m["start_line"].(float64); uint(got) != tc.wantStartLine {
				t.Errorf("start_line = %v, want %v", got, tc.wantStartLine)
			}
			if got := m["end_line"].(float64); uint(got) != tc.wantEndLine {
				t.Errorf("end_line = %v, want %v", got, tc.wantEndLine)
			}
		})
	}
}

func TestTools_GetSymbolsOverview(t *testing.T) {
	class := graph.Node{UID: "c1", Name: "MyClass", Kind: "Class", FilePath: "src/foo.go", StartLine: 1, EndLine: 50, Exported: true, PackagePath: "src"}
	method := graph.Node{UID: "m1", Name: "doWork", Kind: "Method", FilePath: "src/foo.go", StartLine: 10, EndLine: 20, Exported: false, PackagePath: "src"}
	topFn := graph.Node{UID: "f1", Name: "HelperFunc", Kind: "Function", FilePath: "src/foo.go", StartLine: 55, EndLine: 60, Exported: true, PackagePath: "src"}
	privateFn := graph.Node{UID: "f2", Name: "internalFn", Kind: "Function", FilePath: "src/foo.go", StartLine: 62, EndLine: 70, Exported: false, PackagePath: "src"}
	memberEdge := graph.Edge{FromUID: "m1", ToUID: "c1", Type: graph.EdgeMemberOf, Confidence: 1.0}

	tools := seedTestRepo(t,
		[]graph.Node{class, method, topFn, privateFn},
		[]graph.Edge{memberEdge},
	)

	cases := []struct {
		name           string
		filePath       string
		includePrivate *bool
		wantTotal      int
		wantNames      []string
		wantAbsent     []string
	}{
		{
			name:       "all top-level symbols (default includes private)",
			filePath:   "src/foo.go",
			wantTotal:  3, // class + topFn + privateFn (method excluded via MEMBER_OF)
			wantNames:  []string{"MyClass", "HelperFunc", "internalFn"},
			wantAbsent: []string{"doWork"},
		},
		{
			name:           "only exported symbols",
			filePath:       "src/foo.go",
			includePrivate: boolPtr(false),
			wantTotal:      2,
			wantNames:      []string{"MyClass", "HelperFunc"},
			wantAbsent:     []string{"internalFn", "doWork"},
		},
		{
			name:      "empty file returns zero symbols",
			filePath:  "nonexistent/file.go",
			wantTotal: 0,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			input := map[string]any{"file_path": tc.filePath}
			if tc.includePrivate != nil {
				input["include_private"] = *tc.includePrivate
			}
			args, _ := json.Marshal(map[string]any{
				"name":      "get_symbols_overview",
				"arguments": input,
			})
			resp := tools.Call(context.Background(), args)
			if resp.Error != nil {
				t.Fatalf("get_symbols_overview error: %s", resp.Error.Message)
			}

			content := extractText(t, resp)
			t.Logf("get_symbols_overview response: %s", content)

			var result map[string]any
			if err := json.Unmarshal([]byte(content), &result); err != nil {
				t.Fatalf("unmarshal response: %v", err)
			}

			total, ok := result["total"].(float64)
			if !ok {
				t.Fatalf("total missing or wrong type: %v", result["total"])
			}
			if int(total) != tc.wantTotal {
				t.Errorf("total = %d, want %d", int(total), tc.wantTotal)
			}

			syms, _ := result["symbols"].([]any)
			gotNames := map[string]bool{}
			for _, sym := range syms {
				if m, ok := sym.(map[string]any); ok {
					gotNames[m["name"].(string)] = true
				}
			}
			for _, want := range tc.wantNames {
				if !gotNames[want] {
					t.Errorf("expected symbol %q in results, got %v", want, gotNames)
				}
			}
			for _, absent := range tc.wantAbsent {
				if gotNames[absent] {
					t.Errorf("symbol %q should not appear in results", absent)
				}
			}
		})
	}
}

func boolPtr(b bool) *bool { return &b }

// TestTools_GetSymbolsOverview_LikeSuffix verifies that a relative file_path
// matches nodes stored with absolute paths via the LIKE suffix fallback in SQL,
// even when there is NO repo_path stored in index_meta (handler normalization
// cannot help). This is the primary regression test for the empty-results bug.
func TestTools_GetSymbolsOverview_LikeSuffix(t *testing.T) {
	tmpHome := t.TempDir()
	t.Setenv("HOME", tmpHome)

	dbPath, err := registry.DBPath("testrepo")
	if err != nil {
		t.Fatalf("DBPath: %v", err)
	}
	s, err := store.OpenStore(dbPath)
	if err != nil {
		t.Fatalf("OpenStore: %v", err)
	}
	// Store nodes with absolute paths, no repo_path meta set.
	nodes := []graph.Node{
		{UID: "b1", Name: "FlashcardHandler", Kind: "Class", FilePath: "/abs/repo/internal/handler/flashcard_handler.go", StartLine: 1, EndLine: 40, Exported: true, PackagePath: "handler"},
		{UID: "b2", Name: "NewFlashcardHandler", Kind: "Function", FilePath: "/abs/repo/internal/handler/flashcard_handler.go", StartLine: 42, EndLine: 50, Exported: true, PackagePath: "handler"},
	}
	if err := s.BatchUpsertNodes(nodes); err != nil {
		t.Fatalf("BatchUpsertNodes: %v", err)
	}
	// Intentionally no SetMeta("repo_path", ...) — handler normalization won't fire.
	s.Close()

	reg := &registry.Registry{Repos: []registry.RepoInfo{{Name: "testrepo", Path: tmpHome}}}
	tools := mcp.NewTools(reg)

	args, _ := json.Marshal(map[string]any{
		"name":      "get_symbols_overview",
		"arguments": map[string]any{"file_path": "internal/handler/flashcard_handler.go"},
	})
	resp := tools.Call(context.Background(), args)
	if resp.Error != nil {
		t.Fatalf("get_symbols_overview error: %s", resp.Error.Message)
	}

	content := extractText(t, resp)
	t.Logf("response: %s", content)

	var result map[string]any
	if err := json.Unmarshal([]byte(content), &result); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	total, _ := result["total"].(float64)
	if int(total) != 2 {
		t.Errorf("total = %d, want 2 (LIKE suffix should match absolute stored paths)", int(total))
	}
	syms, _ := result["symbols"].([]any)
	found := map[string]bool{}
	for _, sym := range syms {
		if m, ok := sym.(map[string]any); ok {
			found[m["name"].(string)] = true
		}
	}
	if !found["FlashcardHandler"] || !found["NewFlashcardHandler"] {
		t.Errorf("expected FlashcardHandler and NewFlashcardHandler, got %v", found)
	}
}

// TestTools_GetSymbolsOverview_RelativePath verifies that the handler correctly
// resolves a relative file_path against the repo_path stored in index_meta.
// Nodes are stored with absolute paths (as the indexer does) and the agent
// supplies a relative path — this should still return results.
func TestTools_GetSymbolsOverview_RelativePath(t *testing.T) {
	tmpHome := t.TempDir()
	t.Setenv("HOME", tmpHome)

	const repoRoot = "/abs/fake/root"

	dbPath, err := registry.DBPath("testrepo")
	if err != nil {
		t.Fatalf("DBPath: %v", err)
	}
	s, err := store.OpenStore(dbPath)
	if err != nil {
		t.Fatalf("OpenStore: %v", err)
	}
	nodes := []graph.Node{
		{UID: "a1", Name: "Handler", Kind: "Function", FilePath: repoRoot + "/internal/handler/file.go", StartLine: 1, EndLine: 20, Exported: true, PackagePath: "handler"},
		{UID: "a2", Name: "helper", Kind: "Function", FilePath: repoRoot + "/internal/handler/file.go", StartLine: 22, EndLine: 30, Exported: false, PackagePath: "handler"},
	}
	if err := s.BatchUpsertNodes(nodes); err != nil {
		t.Fatalf("BatchUpsertNodes: %v", err)
	}
	if err := s.SetMeta("repo_path", repoRoot); err != nil {
		t.Fatalf("SetMeta repo_path: %v", err)
	}
	s.Close()

	reg := &registry.Registry{Repos: []registry.RepoInfo{{Name: "testrepo", Path: tmpHome}}}
	tools := mcp.NewTools(reg)

	args, _ := json.Marshal(map[string]any{
		"name":      "get_symbols_overview",
		"arguments": map[string]any{"file_path": "internal/handler/file.go"},
	})
	resp := tools.Call(context.Background(), args)
	if resp.Error != nil {
		t.Fatalf("get_symbols_overview error: %s", resp.Error.Message)
	}

	content := extractText(t, resp)
	t.Logf("response: %s", content)

	var result map[string]any
	if err := json.Unmarshal([]byte(content), &result); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	total, _ := result["total"].(float64)
	if int(total) != 2 {
		t.Errorf("total = %d, want 2 (relative path should resolve against repo_path)", int(total))
	}

	syms, _ := result["symbols"].([]any)
	found := map[string]bool{}
	for _, sym := range syms {
		if m, ok := sym.(map[string]any); ok {
			found[m["name"].(string)] = true
		}
	}
	if !found["Handler"] || !found["helper"] {
		t.Errorf("expected Handler and helper in results, got %v", found)
	}
}

func TestTools_QueryRepo(t *testing.T) {
	fn := graph.Node{UID: "n1", Name: "Greet", Kind: "Function", FilePath: "src/a.go", StartLine: 1, EndLine: 5, Exported: true}
	tools := seedTestRepo(t, []graph.Node{fn}, nil)

	t.Run("delegates query tool with meta", func(t *testing.T) {
		args, _ := json.Marshal(map[string]any{
			"name": "query_repo",
			"arguments": map[string]any{
				"tool_name":    "query",
				"arguments":    map[string]any{"query": "Greet"},
				"target_repo":  "testrepo",
				"current_repo": "otherrepo",
			},
		})
		resp := tools.Call(context.Background(), args)
		if resp.Error != nil {
			t.Fatalf("query_repo error: %s", resp.Error.Message)
		}
		content := extractText(t, resp)
		t.Logf("query_repo response: %s", content)

		var result map[string]any
		if err := json.Unmarshal([]byte(content), &result); err != nil {
			t.Fatalf("unmarshal: %v", err)
		}
		meta, ok := result["meta"].(map[string]any)
		if !ok {
			t.Fatalf("meta missing from response: %v", result)
		}
		if meta["queried_repo"] != "testrepo" {
			t.Errorf("meta.queried_repo = %v, want testrepo", meta["queried_repo"])
		}
		if meta["current_repo"] != "otherrepo" {
			t.Errorf("meta.current_repo = %v, want otherrepo", meta["current_repo"])
		}
		if meta["tool_used"] != "query" {
			t.Errorf("meta.tool_used = %v, want query", meta["tool_used"])
		}
	})

	t.Run("rejects non-whitelisted tool", func(t *testing.T) {
		args, _ := json.Marshal(map[string]any{
			"name": "query_repo",
			"arguments": map[string]any{
				"tool_name":   "rename",
				"arguments":   map[string]any{},
				"target_repo": "testrepo",
			},
		})
		resp := tools.Call(context.Background(), args)
		if resp.Error == nil {
			t.Fatal("expected error for non-whitelisted tool, got nil")
		}
		t.Logf("error (expected): %s", resp.Error.Message)
	})

	t.Run("rejects query_repo itself (recursion protection)", func(t *testing.T) {
		args, _ := json.Marshal(map[string]any{
			"name": "query_repo",
			"arguments": map[string]any{
				"tool_name":   "query_repo",
				"arguments":   map[string]any{},
				"target_repo": "testrepo",
			},
		})
		resp := tools.Call(context.Background(), args)
		if resp.Error == nil {
			t.Fatal("expected error for query_repo recursion, got nil")
		}
		t.Logf("error (expected): %s", resp.Error.Message)
	})
}

func TestTools_FindSymbolBody(t *testing.T) {
	fn := graph.Node{
		UID: "n1", Name: "ProcessOrder", Kind: "Function",
		FilePath: "src/order.go", StartLine: 3, EndLine: 5,
		Exported: true, PackagePath: "src",
	}
	tools := seedTestRepo(t, []graph.Node{fn}, nil)

	// Write a real file matching the stored coordinates.
	dir := t.TempDir()
	srcDir := dir + "/src"
	_ = os.MkdirAll(srcDir, 0755)
	filePath := srcDir + "/order.go"
	content := "package src\n\n// ProcessOrder handles orders\nfunc ProcessOrder() {}\n// end\n"
	if err := os.WriteFile(filePath, []byte(content), 0600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	// Update the node's FilePath to the real temp file.
	fn.FilePath = filePath
	dbPath, err := registry.DBPath("testrepo")
	if err != nil {
		t.Fatalf("DBPath: %v", err)
	}
	s2, err := store.OpenStore(dbPath)
	if err != nil {
		t.Fatalf("OpenStore: %v", err)
	}
	if err := s2.BatchUpsertNodes([]graph.Node{fn}); err != nil {
		t.Fatalf("BatchUpsertNodes: %v", err)
	}
	s2.Close()

	args, _ := json.Marshal(map[string]any{
		"name":      "find_symbol_body",
		"arguments": map[string]any{"name": "ProcessOrder"},
	})
	resp := tools.Call(context.Background(), args)
	if resp.Error != nil {
		t.Fatalf("find_symbol_body error: %s", resp.Error.Message)
	}

	text := extractText(t, resp)
	t.Logf("find_symbol_body response: %s", text)

	var result map[string]any
	if err := json.Unmarshal([]byte(text), &result); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	matches, _ := result["matches"].([]any)
	if len(matches) == 0 {
		t.Fatal("expected at least one match")
	}
	m := matches[0].(map[string]any)
	if m["name"] != "ProcessOrder" {
		t.Errorf("name = %v, want ProcessOrder", m["name"])
	}
	body, _ := m["body"].(string)
	if body == "" {
		t.Error("body is empty")
	}
	if !strings.Contains(body, "ProcessOrder") {
		t.Errorf("body does not contain 'ProcessOrder': %q", body)
	}
}

// extractText pulls the text content from a tool response.
func extractText(t *testing.T, resp mcp.Response) string {
	t.Helper()
	resultMap, ok := resp.Result.(map[string]interface{})
	if !ok {
		t.Fatalf("resp.Result wrong type: %T", resp.Result)
	}
	content, _ := resultMap["content"].([]map[string]interface{})
	if len(content) == 0 {
		t.Fatal("content is empty")
	}
	text, _ := content[0]["text"].(string)
	return text
}
