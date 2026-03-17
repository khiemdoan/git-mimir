package process_test

import (
	"testing"

	"github.com/yourusername/mimir/internal/graph"
	"github.com/yourusername/mimir/internal/process"
	"github.com/yourusername/mimir/internal/store"
)

func openTestStore(t *testing.T) *store.Store {
	t.Helper()
	s, err := store.OpenStore(":memory:")
	if err != nil {
		t.Fatalf("OpenStore: %v", err)
	}
	t.Cleanup(func() { s.Close() })
	return s
}

func TestFindEntryPoints(t *testing.T) {
	s := openTestStore(t)

	nodes := []graph.Node{
		{UID: "n1", Name: "handleRequest", Kind: "Function", FilePath: "handler.ts", Exported: true},
		{UID: "n2", Name: "processData", Kind: "Function", FilePath: "service.ts", Exported: true},
		{UID: "n3", Name: "saveToDb", Kind: "Function", FilePath: "repo.ts", Exported: true},
		{UID: "n4", Name: "helperInternal", Kind: "Function", FilePath: "util.ts", Exported: false},
	}
	s.BatchUpsertNodes(nodes)

	// n1 calls n2, n2 calls n3
	s.BatchUpsertEdges([]graph.Edge{
		{FromUID: "n1", ToUID: "n2", Type: graph.EdgeCalls, Confidence: 0.95},
		{FromUID: "n2", ToUID: "n3", Type: graph.EdgeCalls, Confidence: 0.90},
	})

	entries := process.FindEntryPoints(nodes, s)
	t.Logf("Found %d entry points:", len(entries))
	for _, e := range entries {
		t.Logf("  %s (%s)", e.Name, e.UID)
	}

	// handleRequest should be detected (name pattern)
	found := false
	for _, e := range entries {
		if e.Name == "handleRequest" {
			found = true
		}
	}
	if !found {
		t.Error("expected handleRequest to be detected as entry point")
	}
}

func TestTraceProcess(t *testing.T) {
	s := openTestStore(t)

	nodes := []graph.Node{
		{UID: "n1", Name: "handler", Kind: "Function", FilePath: "a.ts"},
		{UID: "n2", Name: "service", Kind: "Function", FilePath: "b.ts"},
		{UID: "n3", Name: "repo", Kind: "Function", FilePath: "c.ts"},
	}
	s.BatchUpsertNodes(nodes)
	s.BatchUpsertEdges([]graph.Edge{
		{FromUID: "n1", ToUID: "n2", Type: graph.EdgeCalls, Confidence: 0.95},
		{FromUID: "n2", ToUID: "n3", Type: graph.EdgeCalls, Confidence: 0.90},
	})

	proc := process.TraceProcess(nodes[0], s, 10)

	t.Logf("Process %s (%s): %d steps", proc.Name, proc.ID, len(proc.Steps))
	for _, step := range proc.Steps {
		t.Logf("  step %d: %s (depth %d)", step.Index, step.SymbolUID, step.Depth)
	}

	if len(proc.Steps) != 3 {
		t.Errorf("expected 3 steps, got %d", len(proc.Steps))
	}
	if proc.Steps[0].SymbolUID != "n1" {
		t.Errorf("first step = %s, want n1", proc.Steps[0].SymbolUID)
	}
}

func TestTraceProcess_CycleBreaking(t *testing.T) {
	s := openTestStore(t)

	nodes := []graph.Node{
		{UID: "n1", Name: "a", Kind: "Function", FilePath: "a.ts"},
		{UID: "n2", Name: "b", Kind: "Function", FilePath: "b.ts"},
	}
	s.BatchUpsertNodes(nodes)
	// Cycle: n1 -> n2 -> n1
	s.BatchUpsertEdges([]graph.Edge{
		{FromUID: "n1", ToUID: "n2", Type: graph.EdgeCalls, Confidence: 0.9},
		{FromUID: "n2", ToUID: "n1", Type: graph.EdgeCalls, Confidence: 0.9},
	})

	proc := process.TraceProcess(nodes[0], s, 10)

	// Should visit each node exactly once despite cycle
	if len(proc.Steps) != 2 {
		t.Errorf("expected 2 steps (cycle broken), got %d", len(proc.Steps))
	}
}

func TestClassifyProcess(t *testing.T) {
	s := openTestStore(t)

	nodes := []graph.Node{
		{UID: "n1", Name: "a", Kind: "Function", FilePath: "a.ts"},
		{UID: "n2", Name: "b", Kind: "Function", FilePath: "b.ts"},
	}
	s.BatchUpsertNodes(nodes)
	s.UpsertCluster("c1", "module-a", 0.9)
	s.SetClusterForNodes([]string{"n1", "n2"}, "c1")

	proc := &process.Process{
		Steps: []store.ProcessStep{
			{SymbolUID: "n1"},
			{SymbolUID: "n2"},
		},
	}

	ptype := process.ClassifyProcess(proc, s)
	if ptype != "single_community" {
		t.Errorf("expected single_community, got %s", ptype)
	}
}
