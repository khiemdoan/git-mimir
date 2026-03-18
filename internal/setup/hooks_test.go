package setup_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/thuongh2/git-mimir/internal/setup"
)

// TestPreSearchHook_NotEmpty tests that the pre-search hook script is defined
func TestPreSearchHook_NotEmpty(t *testing.T) {
	if setup.PreSearchHook == "" {
		t.Error("PreSearchHook is empty")
	}
	if len(setup.PreSearchHook) < 100 {
		t.Errorf("PreSearchHook seems too short (%d chars), may be incomplete", len(setup.PreSearchHook))
	}
}

// TestPostWriteHook_NotEmpty tests that the post-write hook script is defined
func TestPostWriteHook_NotEmpty(t *testing.T) {
	if setup.PostWriteHook == "" {
		t.Error("PostWriteHook is empty")
	}
	if len(setup.PostWriteHook) < 100 {
		t.Errorf("PostWriteHook seems too short (%d chars), may be incomplete", len(setup.PostWriteHook))
	}
}

// TestPreSearchHook_BashShebang tests that hooks have proper shebang
func TestPreSearchHook_Shebang(t *testing.T) {
	if !filepath.IsAbs(setup.PreSearchHook) && len(setup.PreSearchHook) > 2 {
		if setup.PreSearchHook[:2] != "#!" {
			t.Error("PreSearchHook doesn't start with shebang")
		}
	}
}

func TestPostWriteHook_Shebang(t *testing.T) {
	if !filepath.IsAbs(setup.PostWriteHook) && len(setup.PostWriteHook) > 2 {
		if setup.PostWriteHook[:2] != "#!" {
			t.Error("PostWriteHook doesn't start with shebang")
		}
	}
}

// TestInstallClaudeCodeHooks tests hook installation
func TestInstallClaudeCodeHooks(t *testing.T) {
	tmpDir := t.TempDir()

	// Create a fake repo structure
	err := setup.InstallClaudeCodeHooks(tmpDir)
	if err != nil {
		t.Fatalf("InstallClaudeCodeHooks() error: %v", err)
	}

	// Verify hooks directory created
	hooksDir := filepath.Join(tmpDir, ".claude", "hooks")
	if _, err := os.Stat(hooksDir); os.IsNotExist(err) {
		t.Error("hooks directory was not created")
	}

	// Verify hook scripts created
	scripts := []string{"mimir-pre-search.sh", "mimir-post-write.sh"}
	for _, script := range scripts {
		path := filepath.Join(hooksDir, script)
		if _, err := os.Stat(path); os.IsNotExist(err) {
			t.Errorf("%s was not created", script)
		} else {
			// Verify script is executable
			info, err := os.Stat(path)
			if err == nil && info.Mode().Perm()&0111 == 0 {
				t.Errorf("%s is not executable", script)
			}
		}
	}

	// Verify settings.json created
	settingsPath := filepath.Join(tmpDir, ".claude", "settings.json")
	if _, err := os.Stat(settingsPath); os.IsNotExist(err) {
		t.Error("settings.json was not created")
	}
}

// TestInstallClaudeCodeHooks_MergeExisting tests merging with existing settings
func TestInstallClaudeCodeHooks_MergeExisting(t *testing.T) {
	tmpDir := t.TempDir()
	claudeDir := filepath.Join(tmpDir, ".claude")
	os.MkdirAll(claudeDir, 0755)

	// Create existing settings with other hooks
	existing := map[string]any{
		"hooks": map[string]any{
			"PreToolUse": []any{
				map[string]any{
					"matcher": "Glob",
					"hooks": []any{
						map[string]any{
							"type":    "command",
							"command": "some-other-hook.sh",
						},
					},
				},
			},
		},
	}
	settingsPath := filepath.Join(claudeDir, "settings.json")
	data, _ := json.MarshalIndent(existing, "", "  ")
	os.WriteFile(settingsPath, data, 0644)

	err := setup.InstallClaudeCodeHooks(tmpDir)
	if err != nil {
		t.Fatalf("InstallClaudeCodeHooks() error: %v", err)
	}

	// Read back and verify merge
	data, err = os.ReadFile(settingsPath)
	if err != nil {
		t.Fatalf("read settings: %v", err)
	}

	var cfg map[string]any
	json.Unmarshal(data, &cfg)

	// Check hooks were merged
	hooks, _ := cfg["hooks"].(map[string]any)
	if hooks == nil {
		t.Fatal("hooks not found in settings")
	}

	preToolUse, ok := hooks["PreToolUse"].([]any)
	if !ok {
		t.Fatal("PreToolUse not found or not an array")
	}

	// Should have at least 2 matchers (existing + mimir)
	if len(preToolUse) < 2 {
		t.Errorf("PreToolUse has %d matchers, want at least 2 (merged)", len(preToolUse))
	}

	// Check existing hook preserved
	foundExisting := false
	for _, m := range preToolUse {
		if m, ok := m.(map[string]any); ok {
			if m["matcher"] == "Glob" {
				foundExisting = true
			}
		}
	}
	if !foundExisting {
		t.Error("existing Glob matcher was lost")
	}
}

// TestInstallClaudeCodeHooks_NoRepoPath tests behavior with empty repo path
func TestInstallClaudeCodeHooks_EmptyPath(t *testing.T) {
	// Should not crash with empty path
	err := setup.InstallClaudeCodeHooks("")
	if err != nil {
		t.Logf("InstallClaudeCodeHooks(\"\") returned error (may be expected): %v", err)
	}
}

// TestHookSettings_Structure tests the hook settings types
func TestHookSettings_Structure(t *testing.T) {
	settings := setup.HookSettings{
		Hooks: setup.HookEvents{
			PreToolUse: []setup.HookMatcher{
				{
					Matcher: "Bash",
					Hooks: []setup.HookHandler{
						{
							Type:    "command",
							Command: "test.sh",
							Async:   true,
						},
					},
				},
			},
		},
	}

	if len(settings.Hooks.PreToolUse) != 1 {
		t.Errorf("PreToolUse has %d matchers, want 1", len(settings.Hooks.PreToolUse))
	}

	matcher := settings.Hooks.PreToolUse[0]
	if matcher.Matcher != "Bash" {
		t.Errorf("matcher = %q, want \"Bash\"", matcher.Matcher)
	}

	if len(matcher.Hooks) != 1 {
		t.Errorf("matcher has %d hooks, want 1", len(matcher.Hooks))
	}

	hook := matcher.Hooks[0]
	if hook.Type != "command" {
		t.Errorf("hook type = %q, want \"command\"", hook.Type)
	}
	if hook.Command != "test.sh" {
		t.Errorf("hook command = %q, want \"test.sh\"", hook.Command)
	}
	if !hook.Async {
		t.Error("hook.Async = false, want true")
	}
}
