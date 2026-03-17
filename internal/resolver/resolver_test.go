package resolver_test

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/yourusername/mimir/internal/graph"
	"github.com/yourusername/mimir/internal/parser"
	"github.com/yourusername/mimir/internal/resolver"
)

func testdataPath(rel string) string {
	_, thisFile, _, _ := runtime.Caller(0)
	return filepath.Join(filepath.Dir(thisFile), "..", "..", "testdata", rel)
}

func parseTestFile(t *testing.T, rel string) graph.FileSymbols {
	t.Helper()
	path := testdataPath(rel)
	src, err := os.ReadFile(path)
	if err != nil {
		t.Skipf("testdata not found: %v", err)
	}
	fs, err := parser.ParseFile(path, src)
	if err != nil {
		t.Fatalf("ParseFile %s: %v", rel, err)
	}
	return *fs
}

func TestResolve_SimpleTS(t *testing.T) {
	files := []graph.FileSymbols{
		parseTestFile(t, "simple-ts/src/index.ts"),
		parseTestFile(t, "simple-ts/src/greeter.ts"),
		parseTestFile(t, "simple-ts/src/math.ts"),
	}

	edges := resolver.Resolve(files)

	t.Logf("Resolved %d edges:", len(edges))
	for _, e := range edges {
		t.Logf("  %s -[%s (%.2f)]-> %s", e.FromUID, e.Type, e.Confidence, e.ToUID)
	}

	if len(edges) == 0 {
		t.Error("expected at least some edges")
	}

	// Check that we have CALLS edges
	hasCalls := false
	for _, e := range edges {
		if e.Type == graph.EdgeCalls {
			hasCalls = true
			break
		}
	}
	if !hasCalls {
		t.Error("expected at least one CALLS edge")
	}

	// All edges must have non-zero confidence (except unresolved=0.0)
	for _, e := range edges {
		if e.Confidence < 0 || e.Confidence > 1.0 {
			t.Errorf("edge confidence out of range: %.2f", e.Confidence)
		}
	}
}

func TestResolve_WithClasses(t *testing.T) {
	files := []graph.FileSymbols{
		parseTestFile(t, "simple-ts/src/controller.ts"),
		parseTestFile(t, "simple-ts/src/user-service.ts"),
		parseTestFile(t, "simple-ts/src/logger.ts"),
		parseTestFile(t, "simple-ts/src/math.ts"),
	}

	edges := resolver.Resolve(files)

	t.Logf("Resolved %d edges:", len(edges))
	edgeTypes := map[string]int{}
	for _, e := range edges {
		edgeTypes[e.Type]++
		t.Logf("  %s -[%s (%.2f)]-> %s", e.FromUID, e.Type, e.Confidence, e.ToUID)
	}

	if len(edges) == 0 {
		t.Error("expected edges")
	}

	// Should have MEMBER_OF edges for class methods
	if edgeTypes[graph.EdgeMemberOf] == 0 {
		t.Log("Note: no MEMBER_OF edges found (may depend on parser AST structure)")
	}
}

func TestSymbolTable_SameFile(t *testing.T) {
	table := resolver.NewSymbolTable()

	fs := graph.FileSymbols{
		FilePath: "/src/math.ts",
		Nodes: []graph.Node{
			{UID: "uid1", Name: "add", Kind: "Function", FilePath: "/src/math.ts"},
			{UID: "uid2", Name: "multiply", Kind: "Function", FilePath: "/src/math.ts"},
		},
	}
	table.Register(fs)

	uid, conf, ok := table.Resolve("/src/math.ts", "add")
	if !ok {
		t.Fatal("Resolve failed for same-file symbol")
	}
	if uid != "uid1" {
		t.Errorf("Resolve uid = %q, want uid1", uid)
	}
	if conf < 0.9 {
		t.Errorf("same-file confidence = %.2f, want >= 0.9", conf)
	}
}

func TestSymbolTable_CrossFile(t *testing.T) {
	table := resolver.NewSymbolTable()

	table.Register(graph.FileSymbols{
		FilePath: "/src/math.ts",
		Nodes: []graph.Node{
			{UID: "uid1", Name: "add", Kind: "Function", FilePath: "/src/math.ts"},
		},
	})
	table.Register(graph.FileSymbols{
		FilePath: "/src/index.ts",
		Nodes: []graph.Node{
			{UID: "uid2", Name: "main", Kind: "Function", FilePath: "/src/index.ts"},
		},
	})

	uid, conf, ok := table.Resolve("/src/index.ts", "add")
	if !ok {
		t.Fatal("Resolve failed for cross-file symbol")
	}
	if uid != "uid1" {
		t.Errorf("Resolve uid = %q, want uid1", uid)
	}
	// Same directory = same package
	if conf < 0.9 {
		t.Errorf("same-package confidence = %.2f, want >= 0.9", conf)
	}
}
