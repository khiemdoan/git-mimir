package setup_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/thuongh2/git-mimir/internal/setup"
)

// TestMimirMCPEntry tests that the MCP entry has required fields
func TestMimirMCPEntry(t *testing.T) {
	entry := setup.MimirMCPEntry()

	if entry == nil {
		t.Fatal("MimirMCPEntry() returned nil")
	}

	// Check required fields
	command, ok := entry["command"].(string)
	if !ok || command == "" {
		t.Errorf("command field missing or empty: %v", entry["command"])
	}

	args, ok := entry["args"].([]string)
	if !ok {
		t.Fatalf("args field is not []string: %T", entry["args"])
	}
	if len(args) != 1 || args[0] != "mcp" {
		t.Errorf("args = %v, want [\"mcp\"]", args)
	}

	env, ok := entry["env"].(map[string]string)
	if !ok {
		t.Fatalf("env field is not map[string]string: %T", entry["env"])
	}
	// env can be empty, but should exist
	if env == nil {
		t.Error("env field is nil, want empty map")
	}
}

// TestWriteToEditor tests writing MCP config to a file
func TestWriteToEditor(t *testing.T) {
	// Create temp directory
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "mcp.json")

	editor := setup.Editor{
		Name:       "Test Editor",
		ConfigPath: func() string { return configPath },
		ConfigKey:  "mcpServers",
		Detect:     func() bool { return true },
	}

	err := setup.WriteToEditor(editor)
	if err != nil {
		t.Fatalf("WriteToEditor() error: %v", err)
	}

	// Verify file was created
	data, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatalf("read config: %v", err)
	}

	// Parse JSON
	var cfg map[string]any
	if err := json.Unmarshal(data, &cfg); err != nil {
		t.Fatalf("parse JSON: %v", err)
	}

	// Check mcpServers.mimir exists
	mcpServers, ok := cfg["mcpServers"].(map[string]any)
	if !ok {
		t.Fatalf("mcpServers not found or not a map: %v", cfg["mcpServers"])
	}

	mimir, ok := mcpServers["mimir"].(map[string]any)
	if !ok {
		t.Fatalf("mimir server not found in mcpServers: %v", mcpServers)
	}

	// Verify mimir config
	if mimir["command"] == nil {
		t.Error("mimir.command is missing")
	}
	args, _ := mimir["args"].([]any)
	if len(args) != 1 || args[0].(string) != "mcp" {
		t.Errorf("mimir.args = %v, want [\"mcp\"]", args)
	}
}

// TestWriteToEditor_NestedKey tests writing to nested config keys
func TestWriteToEditor_NestedKey(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "settings.json")

	editor := setup.Editor{
		Name:       "VS Code",
		ConfigPath: func() string { return configPath },
		ConfigKey:  "github.copilot.mcpServers",
		Detect:     func() bool { return true },
	}

	err := setup.WriteToEditor(editor)
	if err != nil {
		t.Fatalf("WriteToEditor() error: %v", err)
	}

	data, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatalf("read config: %v", err)
	}

	var cfg map[string]any
	json.Unmarshal(data, &cfg)

	// Navigate nested path
	github, _ := cfg["github"].(map[string]any)
	if github == nil {
		t.Fatal("github key not created")
	}
	copilot, _ := github["copilot"].(map[string]any)
	if copilot == nil {
		t.Fatal("copilot key not created")
	}
	mcpServers, _ := copilot["mcpServers"].(map[string]any)
	if mcpServers == nil {
		t.Fatal("mcpServers key not created")
	}
	if _, ok := mcpServers["mimir"]; !ok {
		t.Error("mimir not added to nested mcpServers")
	}
}

// TestWriteToEditor_MergeExisting tests that existing config is preserved
func TestWriteToEditor_MergeExisting(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "mcp.json")

	// Create existing config with other servers
	existing := map[string]any{
		"mcpServers": map[string]any{
			"other-server": map[string]any{
				"command": "/other/bin",
			},
		},
		"otherSetting": "value",
	}
	data, _ := json.MarshalIndent(existing, "", "  ")
	os.WriteFile(configPath, data, 0644)

	editor := setup.Editor{
		Name:       "Test Editor",
		ConfigPath: func() string { return configPath },
		ConfigKey:  "mcpServers",
		Detect:     func() bool { return true },
	}

	err := setup.WriteToEditor(editor)
	if err != nil {
		t.Fatalf("WriteToEditor() error: %v", err)
	}

	// Read back and verify merge
	data, err = os.ReadFile(configPath)
	if err != nil {
		t.Fatalf("read config: %v", err)
	}

	var cfg map[string]any
	json.Unmarshal(data, &cfg)

	// Check existing server preserved
	mcpServers, _ := cfg["mcpServers"].(map[string]any)
	if _, ok := mcpServers["other-server"]; !ok {
		t.Error("existing other-server was overwritten")
	}

	// Check mimir added
	if _, ok := mcpServers["mimir"]; !ok {
		t.Error("mimir not added")
	}

	// Check otherSetting preserved
	if cfg["otherSetting"] != "value" {
		t.Error("otherSetting was lost")
	}
}

// TestWriteToEditor_CreateDir tests that parent directories are created
func TestWriteToEditor_CreateDir(t *testing.T) {
	tmpDir := t.TempDir()
	deepPath := filepath.Join(tmpDir, "a", "b", "c", "mcp.json")

	editor := setup.Editor{
		Name:       "Test Editor",
		ConfigPath: func() string { return deepPath },
		ConfigKey:  "mcpServers",
		Detect:     func() bool { return true },
	}

	err := setup.WriteToEditor(editor)
	if err != nil {
		t.Fatalf("WriteToEditor() error: %v", err)
	}

	// Verify file exists
	if _, err := os.Stat(deepPath); os.IsNotExist(err) {
		t.Error("config file was not created at deep path")
	}
}

// TestNavigateKeyPath tests the navigateKeyPath helper
func TestNavigateKeyPath(t *testing.T) {
	tests := []struct {
		name      string
		keyPath   string
		wantFinal string
	}{
		{"simple", "mcpServers", "mcpServers"},
		{"nested", "github.copilot.mcpServers", "mcpServers"},
		{"deep", "a.b.c.d", "d"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := map[string]any{}
			parent, final := setup.NavigateKeyPathForTest(cfg, tt.keyPath)

			if final != tt.wantFinal {
				t.Errorf("final key = %q, want %q", final, tt.wantFinal)
			}
			if parent == nil {
				t.Error("parent is nil")
			}
		})
	}
}
