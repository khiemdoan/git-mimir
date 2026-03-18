package store_test

import (
	"testing"

	"github.com/thuongh2/git-mimir/internal/graph"
	"github.com/thuongh2/git-mimir/internal/store"
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

func TestOpenStore(t *testing.T) {
	s := openTestStore(t)
	// Verify tables exist by querying
	count, err := s.NodeCount()
	if err != nil {
		t.Fatalf("NodeCount: %v", err)
	}
	if count != 0 {
		t.Errorf("expected 0 nodes, got %d", count)
	}
}

func TestBatchUpsertNodes(t *testing.T) {
	s := openTestStore(t)

	nodes := []graph.Node{
		{UID: "n1", Name: "foo", Kind: "Function", FilePath: "a.ts", StartLine: 1, EndLine: 5, Exported: true},
		{UID: "n2", Name: "bar", Kind: "Class", FilePath: "b.ts", StartLine: 10, EndLine: 20, Exported: false},
		{UID: "n3", Name: "baz", Kind: "Method", FilePath: "b.ts", StartLine: 12, EndLine: 15},
	}

	if err := s.BatchUpsertNodes(nodes); err != nil {
		t.Fatalf("BatchUpsertNodes: %v", err)
	}

	count, _ := s.NodeCount()
	if count != 3 {
		t.Errorf("NodeCount = %d, want 3", count)
	}

	// Query by name
	found, err := s.QuerySymbol("foo")
	if err != nil {
		t.Fatalf("QuerySymbol: %v", err)
	}
	if len(found) != 1 || found[0].UID != "n1" {
		t.Errorf("QuerySymbol(foo) = %v, want [n1]", found)
	}

	// Query by file
	byFile, err := s.QueryByFile("b.ts")
	if err != nil {
		t.Fatalf("QueryByFile: %v", err)
	}
	if len(byFile) != 2 {
		t.Errorf("QueryByFile(b.ts) count = %d, want 2", len(byFile))
	}
}

func TestBatchUpsertEdges(t *testing.T) {
	s := openTestStore(t)

	nodes := []graph.Node{
		{UID: "n1", Name: "caller", Kind: "Function", FilePath: "a.ts"},
		{UID: "n2", Name: "callee", Kind: "Function", FilePath: "b.ts"},
	}
	s.BatchUpsertNodes(nodes)

	edges := []graph.Edge{
		{FromUID: "n1", ToUID: "n2", Type: graph.EdgeCalls, Confidence: 0.95},
	}
	if err := s.BatchUpsertEdges(edges); err != nil {
		t.Fatalf("BatchUpsertEdges: %v", err)
	}

	count, _ := s.EdgeCount()
	if count != 1 {
		t.Errorf("EdgeCount = %d, want 1", count)
	}

	// Verify edge queries
	from, _ := s.QueryEdgesFrom("n1")
	if len(from) != 1 || from[0].ToUID != "n2" {
		t.Errorf("QueryEdgesFrom(n1) = %v", from)
	}
	to, _ := s.QueryEdgesTo("n2")
	if len(to) != 1 || to[0].FromUID != "n1" {
		t.Errorf("QueryEdgesTo(n2) = %v", to)
	}
}

func TestDeleteNodesByFile(t *testing.T) {
	s := openTestStore(t)

	nodes := []graph.Node{
		{UID: "n1", Name: "a", Kind: "Function", FilePath: "x.ts"},
		{UID: "n2", Name: "b", Kind: "Function", FilePath: "x.ts"},
		{UID: "n3", Name: "c", Kind: "Function", FilePath: "y.ts"},
	}
	s.BatchUpsertNodes(nodes)
	s.BatchUpsertEdges([]graph.Edge{
		{FromUID: "n1", ToUID: "n3", Type: graph.EdgeCalls, Confidence: 0.9},
	})

	if err := s.DeleteNodesByFile("x.ts"); err != nil {
		t.Fatalf("DeleteNodesByFile: %v", err)
	}

	count, _ := s.NodeCount()
	if count != 1 {
		t.Errorf("NodeCount after delete = %d, want 1", count)
	}

	// Edge should be cascade-deleted
	edgeCount, _ := s.EdgeCount()
	if edgeCount != 0 {
		t.Errorf("EdgeCount after cascade delete = %d, want 0", edgeCount)
	}
}

func TestImpactQuery(t *testing.T) {
	s := openTestStore(t)

	// Build a simple call chain: n1 -> n2 -> n3
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

	// Downstream from n1: should find n2 and n3
	rows, err := s.QueryImpact("n1", "downstream", 0.5, 5)
	if err != nil {
		t.Fatalf("QueryImpact: %v", err)
	}
	if len(rows) < 2 {
		t.Errorf("QueryImpact downstream count = %d, want >= 2", len(rows))
	}

	// Upstream from n3: should find n2 and n1
	upRows, err := s.QueryImpact("n3", "upstream", 0.5, 5)
	if err != nil {
		t.Fatalf("QueryImpact upstream: %v", err)
	}
	if len(upRows) < 2 {
		t.Errorf("QueryImpact upstream count = %d, want >= 2", len(upRows))
	}
}

func TestHybridSearch(t *testing.T) {
	s := openTestStore(t)

	nodes := []graph.Node{
		{UID: "n1", Name: "handleUserRequest", Kind: "Function", FilePath: "handler.ts"},
		{UID: "n2", Name: "getUserData", Kind: "Function", FilePath: "service.ts"},
		{UID: "n3", Name: "parseConfig", Kind: "Function", FilePath: "config.ts"},
	}
	s.BatchUpsertNodes(nodes)
	s.IndexNodesBM25(nodes)

	results, err := s.HybridSearch([]string{"user"}, nil, 10)
	if err != nil {
		t.Fatalf("HybridSearch: %v", err)
	}

	// Should find nodes with "user" in their names
	if len(results) < 1 {
		t.Errorf("HybridSearch(user) count = %d, want >= 1", len(results))
	}

	for _, r := range results {
		t.Logf("  result: %s (%s) score=%.4f", r.Name, r.Kind, r.Score)
	}
}

func TestSetGetMeta(t *testing.T) {
	s := openTestStore(t)

	if err := s.SetMeta("last_commit", "abc1234"); err != nil {
		t.Fatalf("SetMeta: %v", err)
	}

	val, err := s.GetMeta("last_commit")
	if err != nil {
		t.Fatalf("GetMeta: %v", err)
	}
	if val != "abc1234" {
		t.Errorf("GetMeta = %q, want %q", val, "abc1234")
	}

	// Non-existent key
	val2, err := s.GetMeta("nonexistent")
	if err != nil {
		t.Fatalf("GetMeta nonexistent: %v", err)
	}
	if val2 != "" {
		t.Errorf("GetMeta nonexistent = %q, want empty", val2)
	}
}

func TestEmbeddings(t *testing.T) {
	s := openTestStore(t)

	nodes := []graph.Node{
		{UID: "n1", Name: "foo", Kind: "Function", FilePath: "a.ts"},
		{UID: "n2", Name: "bar", Kind: "Function", FilePath: "b.ts"},
	}
	s.BatchUpsertNodes(nodes)

	// Insert embeddings
	embs := map[string][]float32{
		"n1": {0.1, 0.2, 0.3, 0.4},
		"n2": {0.9, 0.8, 0.7, 0.6},
	}
	if err := s.BatchUpdateEmbeddings(embs); err != nil {
		t.Fatalf("BatchUpdateEmbeddings: %v", err)
	}

	count, _ := s.EmbeddingCount()
	if count != 2 {
		t.Errorf("EmbeddingCount = %d, want 2", count)
	}

	// Vector search
	results, err := s.VectorSearch([]float32{0.1, 0.2, 0.3, 0.4}, 5)
	if err != nil {
		t.Fatalf("VectorSearch: %v", err)
	}
	if len(results) != 2 {
		t.Errorf("VectorSearch count = %d, want 2", len(results))
	}
	// n1 should be closer (exact match)
	if len(results) > 0 && results[0].UID != "n1" {
		t.Errorf("VectorSearch closest = %s, want n1", results[0].UID)
	}
}

func TestClusters(t *testing.T) {
	s := openTestStore(t)

	nodes := []graph.Node{
		{UID: "n1", Name: "a", Kind: "Function", FilePath: "x.ts"},
		{UID: "n2", Name: "b", Kind: "Function", FilePath: "x.ts"},
	}
	s.BatchUpsertNodes(nodes)

	s.UpsertCluster("c1", "module-x", 0.85)
	s.UpsertClusterMembers("c1", []string{"n1", "n2"})
	s.SetClusterForNodes([]string{"n1", "n2"}, "c1")

	clusters, err := s.AllClusters()
	if err != nil {
		t.Fatalf("AllClusters: %v", err)
	}
	if len(clusters) != 1 {
		t.Fatalf("AllClusters count = %d, want 1", len(clusters))
	}
	if clusters[0].Label != "module-x" {
		t.Errorf("cluster label = %q, want module-x", clusters[0].Label)
	}
	if len(clusters[0].Members) != 2 {
		t.Errorf("cluster members = %d, want 2", len(clusters[0].Members))
	}
}

func TestProcesses(t *testing.T) {
	s := openTestStore(t)

	nodes := []graph.Node{
		{UID: "n1", Name: "handler", Kind: "Function", FilePath: "a.ts"},
		{UID: "n2", Name: "service", Kind: "Function", FilePath: "b.ts"},
	}
	s.BatchUpsertNodes(nodes)

	s.UpsertProcess("p1", "handleRequest", "n1", "single_community")
	s.UpsertProcessSteps("p1", []store.ProcessStep{
		{Index: 0, SymbolUID: "n1", Depth: 0},
		{Index: 1, SymbolUID: "n2", Depth: 1},
	})

	procs, err := s.AllProcesses()
	if err != nil {
		t.Fatalf("AllProcesses: %v", err)
	}
	if len(procs) != 1 {
		t.Fatalf("AllProcesses count = %d, want 1", len(procs))
	}
	if len(procs[0].Steps) != 2 {
		t.Errorf("process steps = %d, want 2", len(procs[0].Steps))
	}
}

func TestReadRaw(t *testing.T) {
	s := openTestStore(t)

	nodes := []graph.Node{
		{UID: "n1", Name: "foo", Kind: "Function", FilePath: "a.ts"},
	}
	s.BatchUpsertNodes(nodes)

	var cols []string
	var rows [][]interface{}
	err := s.ReadRaw("SELECT name, kind FROM nodes", func(c []string, r [][]interface{}) {
		cols = c
		rows = r
	})
	if err != nil {
		t.Fatalf("ReadRaw: %v", err)
	}
	if len(cols) != 2 || cols[0] != "name" {
		t.Errorf("columns = %v", cols)
	}
	if len(rows) != 1 {
		t.Errorf("rows = %d, want 1", len(rows))
	}
}
