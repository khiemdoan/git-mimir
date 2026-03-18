package setup

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// SetupResult represents the outcome of setting up an editor
type SetupResult struct {
	Editor     string
	ConfigPath string
	Status     string // "configured" | "not installed" | "error"
	Error      string
}

// MimirMCPEntry returns the JSON block for mimir MCP server
func MimirMCPEntry() map[string]any {
	bin, err := exec.LookPath("mimir")
	if err != nil || bin == "" {
		// Fallback: use the currently running binary path
		bin, _ = os.Executable()
	}
	return map[string]any{
		"command": bin,
		"args":    []string{"mcp"},
		"env":     map[string]string{},
	}
}

// SetupAll writes mimir MCP config to all detected editors
func SetupAll() []SetupResult {
	var results []SetupResult
	for _, e := range Editors {
		if !e.Detect() {
			results = append(results, SetupResult{
				Editor: e.Name,
				Status: "not installed",
			})
			continue
		}

		err := WriteToEditor(e)
		if err != nil {
			results = append(results, SetupResult{
				Editor: e.Name,
				Status: "error",
				Error:  err.Error(),
			})
		} else {
			results = append(results, SetupResult{
				Editor:     e.Name,
				ConfigPath: e.ConfigPath(),
				Status:     "configured",
			})
		}
	}
	return results
}

// WriteToEditor merges the mimir MCP entry into editor's config file.
// Creates the file if it doesn't exist. Never overwrites other mcpServers.
func WriteToEditor(e Editor) error {
	path := e.ConfigPath()

	// Ensure directory exists
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return fmt.Errorf("mkdir %s: %w", filepath.Dir(path), err)
	}

	// Read existing config (or start fresh)
	cfg := map[string]any{}
	if data, err := os.ReadFile(path); err == nil {
		json.Unmarshal(data, &cfg) // ignore parse error — we'll overwrite safely
	}

	// Navigate nested key path (e.g. "github.copilot.mcpServers")
	parent, finalKey := navigateKeyPath(cfg, e.ConfigKey)

	// Get or create mcpServers map
	servers, _ := parent[finalKey].(map[string]any)
	if servers == nil {
		servers = map[string]any{}
	}
	servers["mimir"] = MimirMCPEntry()
	parent[finalKey] = servers

	// Write back
	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0644)
}

// navigateKeyPath navigates or creates a nested key path like "github.copilot.mcpServers"
// Returns the parent map and the final key to set
func navigateKeyPath(cfg map[string]any, keyPath string) (map[string]any, string) {
	parts := strings.Split(keyPath, ".")
	current := cfg

	for i := 0; i < len(parts)-1; i++ {
		key := parts[i]
		if current[key] == nil {
			current[key] = map[string]any{}
		}
		nested, ok := current[key].(map[string]any)
		if !ok {
			// Key exists but isn't a map, create new map
			nested = map[string]any{}
			current[key] = nested
		}
		current = nested
	}

	return current, parts[len(parts)-1]
}

// NavigateKeyPathForTest exports navigateKeyPath for testing
func NavigateKeyPathForTest(cfg map[string]any, keyPath string) (map[string]any, string) {
	return navigateKeyPath(cfg, keyPath)
}
