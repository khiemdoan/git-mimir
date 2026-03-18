package setup_test

import (
	"testing"

	"github.com/thuongh2/git-mimir/internal/setup"
)

// TestEditors_Detection tests that editor detection doesn't crash
func TestEditors_Detection(t *testing.T) {
	editors := setup.GetEditors()

	if len(editors) == 0 {
		t.Fatal("GetEditors() returned empty slice, want at least one editor")
	}

	for _, e := range editors {
		t.Run(e.Name, func(t *testing.T) {
			// Detect should not panic
			detected := e.Detect()

			// If detected, ConfigPath should return non-empty
			if detected {
				configPath := e.ConfigPath()
				if configPath == "" {
					t.Errorf("ConfigPath() = \"\" for detected editor %s, want non-empty", e.Name)
				}
			}

			// ConfigKey should be non-empty
			if e.ConfigKey == "" {
				t.Errorf("ConfigKey = \"\" for %s, want non-empty", e.Name)
			}
		})
	}
}

// TestDetectBinary tests the DetectBinary helper function
func TestDetectBinary(t *testing.T) {
	tests := []struct {
		name     string
		binary   string
		expected bool
	}{
		{"go compiler", "go", true},    // go should exist
		{"fake binary", "fakebin12345", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			detected := setup.DetectBinary(tt.binary)
			if detected != tt.expected {
				t.Errorf("DetectBinary(%q) = %v, want %v", tt.binary, detected, tt.expected)
			}
		})
	}
}

// TestDetectBinary_NilError tests that DetectBinary handles nil exec.LookPath error
func TestDetectBinary_NilError(t *testing.T) {
	// This tests the edge case where exec.LookPath returns nil error
	// which should result in detected = true
	detected := setup.DetectBinary("go")
	if !detected {
		t.Error("DetectBinary(\"go\") = false, want true (go should exist)")
	}
}

// TestSetupResult_structure tests that SetupResult has expected fields
func TestSetupResult_Structure(t *testing.T) {
	result := setup.SetupResult{
		Editor:     "Test Editor",
		Status:     "configured",
		ConfigPath: "/test/path",
	}

	if result.Editor != "Test Editor" {
		t.Errorf("Editor = %q, want \"Test Editor\"", result.Editor)
	}
	if result.Status != "configured" {
		t.Errorf("Status = %q, want \"configured\"", result.Status)
	}
	if result.ConfigPath != "/test/path" {
		t.Errorf("ConfigPath = %q, want \"/test/path\"", result.ConfigPath)
	}
}
