package mcp_test

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/yourusername/mimir/internal/registry"
	"github.com/yourusername/mimir/mcp"
)

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

	if len(defs) != 7 {
		t.Errorf("expected 7 tools, got %d", len(defs))
	}

	names := map[string]bool{}
	for _, d := range defs {
		names[d.Name] = true
		t.Logf("  tool: %s", d.Name)
	}

	for _, want := range []string{"query", "context", "impact", "detect_changes", "rename", "cypher", "list_repos"} {
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
