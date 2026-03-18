package setup_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/thuongh2/git-mimir/internal/setup"
)

// MockStore implements setup.StoreInterface for testing
type MockStore struct {
	StatsFunc      func() (setup.Stats, error)
	AllClustersFunc func() ([]setup.Community, error)
	AllProcessesFunc func() ([]setup.Process, error)
}

func (m *MockStore) Stats() (setup.Stats, error) {
	if m.StatsFunc != nil {
		return m.StatsFunc()
	}
	return setup.Stats{}, nil
}

func (m *MockStore) AllClusters() ([]setup.Community, error) {
	if m.AllClustersFunc != nil {
		return m.AllClustersFunc()
	}
	return []setup.Community{}, nil
}

func (m *MockStore) AllProcesses() ([]setup.Process, error) {
	if m.AllProcessesFunc != nil {
		return m.AllProcessesFunc()
	}
	return []setup.Process{}, nil
}

// TestGenerateContextFiles tests context file generation
func TestGenerateContextFiles(t *testing.T) {
	tmpDir := t.TempDir()

	store := &MockStore{
		StatsFunc: func() (setup.Stats, error) {
			return setup.Stats{
				Nodes:     100,
				Edges:     250,
				Clusters:  15,
				Processes: 8,
			}, nil
		},
		AllClustersFunc: func() ([]setup.Community, error) {
			return []setup.Community{
				{
					ID:            "cluster-1",
					Label:         "API Handlers",
					CohesionScore: 0.82,
					Members:       []string{"handler.go", "routes.go"},
				},
			}, nil
		},
		AllProcessesFunc: func() ([]setup.Process, error) {
			return []setup.Process{
				{Name: "HTTP Request Flow"},
				{Name: "Database Migration"},
			}, nil
		},
	}

	err := setup.GenerateContextFiles(tmpDir, store)
	if err != nil {
		t.Fatalf("GenerateContextFiles() error: %v", err)
	}

	// Verify AGENTS.md created
	agentsPath := filepath.Join(tmpDir, "AGENTS.md")
	if _, err := os.Stat(agentsPath); os.IsNotExist(err) {
		t.Error("AGENTS.md was not created")
	}

	// Verify CLAUDE.md created
	claudePath := filepath.Join(tmpDir, "CLAUDE.md")
	if _, err := os.Stat(claudePath); os.IsNotExist(err) {
		t.Error("CLAUDE.md was not created")
	}

	// Verify AGENTS.md content
	data, err := os.ReadFile(agentsPath)
	if err != nil {
		t.Fatalf("read AGENTS.md: %v", err)
	}

	content := string(data)

	// Check required sections
	requiredSections := []string{
		"# Mimir Index",
		"## Repository stats",
		"Symbols indexed: 100",
		"Edges (relationships): 250",
		"Functional clusters: 15",
		"Execution flows traced: 8",
		"## MCP Setup",
		"mimir daemon start",
		"## Functional clusters",
		"## Execution flows",
		"## Usage for agents",
	}

	for _, section := range requiredSections {
		if !strings.Contains(content, section) {
			t.Errorf("missing section: %s", section)
		}
	}

	// Check cluster info
	if !strings.Contains(content, "API Handlers") {
		t.Error("cluster label not in content")
	}
	if !strings.Contains(content, "0.82") {
		t.Error("cohesion score not in content")
	}

	// Check process info
	if !strings.Contains(content, "HTTP Request Flow") {
		t.Error("process name not in content")
	}
}

// TestGenerateContextFiles_EmptyStats tests with empty stats
func TestGenerateContextFiles_EmptyStats(t *testing.T) {
	tmpDir := t.TempDir()

	store := &MockStore{
		StatsFunc: func() (setup.Stats, error) {
			return setup.Stats{}, nil
		},
		AllClustersFunc: func() ([]setup.Community, error) {
			return []setup.Community{}, nil
		},
		AllProcessesFunc: func() ([]setup.Process, error) {
			return []setup.Process{}, nil
		},
	}

	err := setup.GenerateContextFiles(tmpDir, store)
	if err != nil {
		t.Fatalf("GenerateContextFiles() error: %v", err)
	}

	// Verify files created even with empty data
	agentsPath := filepath.Join(tmpDir, "AGENTS.md")
	data, err := os.ReadFile(agentsPath)
	if err != nil {
		t.Fatalf("read AGENTS.md: %v", err)
	}

	content := string(data)
	if !strings.Contains(content, "Symbols indexed: 0") {
		t.Error("empty stats not handled correctly")
	}
}

// TestGenerateContextFiles_StoreError tests error handling from store
func TestGenerateContextFiles_StoreError(t *testing.T) {
	tmpDir := t.TempDir()

	store := &MockStore{
		StatsFunc: func() (setup.Stats, error) {
			return setup.Stats{}, os.ErrNotExist
		},
	}

	err := setup.GenerateContextFiles(tmpDir, store)
	if err == nil {
		t.Error("GenerateContextFiles() should return error when Stats fails")
	}
}

// TestGenerateContextFiles_ClustersError tests error handling for clusters
func TestGenerateContextFiles_ClustersError(t *testing.T) {
	tmpDir := t.TempDir()

	store := &MockStore{
		StatsFunc: func() (setup.Stats, error) {
			return setup.Stats{Nodes: 10}, nil
		},
		AllClustersFunc: func() ([]setup.Community, error) {
			return nil, os.ErrPermission
		},
	}

	err := setup.GenerateContextFiles(tmpDir, store)
	if err == nil {
		t.Error("GenerateContextFiles() should return error when AllClusters fails")
	}
}

// TestStats_Structure tests the Stats type
func TestStats_Structure(t *testing.T) {
	stats := setup.Stats{
		Nodes:     500,
		Edges:     1200,
		Clusters:  25,
		Processes: 12,
	}

	if stats.Nodes != 500 {
		t.Errorf("Nodes = %d, want 500", stats.Nodes)
	}
	if stats.Edges != 1200 {
		t.Errorf("Edges = %d, want 1200", stats.Edges)
	}
	if stats.Clusters != 25 {
		t.Errorf("Clusters = %d, want 25", stats.Clusters)
	}
	if stats.Processes != 12 {
		t.Errorf("Processes = %d, want 12", stats.Processes)
	}
}

// TestProcess_Structure tests the Process type
func TestProcess_Structure(t *testing.T) {
	process := setup.Process{
		ID:   "proc-1",
		Name: "Test Process",
	}

	if process.ID != "proc-1" {
		t.Errorf("ID = %q, want \"proc-1\"", process.ID)
	}
	if process.Name != "Test Process" {
		t.Errorf("Name = %q, want \"Test Process\"", process.Name)
	}
}

// TestStoreInterface_Compliance tests that MockStore implements StoreInterface
func TestStoreInterface_Compliance(t *testing.T) {
	var _ setup.StoreInterface = (*MockStore)(nil)
}
