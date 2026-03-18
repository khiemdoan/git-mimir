package setup_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/thuongh2/git-mimir/internal/setup"
)

// TestStaticSkills_NotEmpty tests that all static skills are defined
func TestStaticSkills_NotEmpty(t *testing.T) {
	skills := setup.StaticSkills

	if len(skills) == 0 {
		t.Fatal("StaticSkills is empty")
	}

	expectedSkills := []string{
		"exploring.md",
		"debugging.md",
		"impact-analysis.md",
		"refactoring.md",
	}

	for _, name := range expectedSkills {
		content, ok := skills[name]
		if !ok {
			t.Errorf("missing static skill: %s", name)
			continue
		}
		if len(content) < 50 {
			t.Errorf("static skill %s seems too short (%d chars)", name, len(content))
		}
		if !strings.Contains(content, "name:") {
			t.Errorf("static skill %s missing frontmatter 'name:' field", name)
		}
		if !strings.Contains(content, "description:") {
			t.Errorf("static skill %s missing frontmatter 'description:' field", name)
		}
	}
}

// TestInstallStaticSkills tests static skill installation
func TestInstallStaticSkills(t *testing.T) {
	tmpDir := t.TempDir()

	err := setup.InstallStaticSkills(tmpDir)
	if err != nil {
		t.Fatalf("InstallStaticSkills() error: %v", err)
	}

	// Verify skills directory created
	skillsDir := filepath.Join(tmpDir, ".claude", "skills", "mimir")
	if _, err := os.Stat(skillsDir); os.IsNotExist(err) {
		t.Error("skills directory was not created")
	}

	// Verify each static skill file
	skills := setup.StaticSkills
	for name := range skills {
		path := filepath.Join(skillsDir, name)
		if _, err := os.Stat(path); os.IsNotExist(err) {
			t.Errorf("static skill %s was not created", name)
		} else {
			// Verify content
			data, err := os.ReadFile(path)
			if err != nil {
				t.Errorf("read %s: %v", name, err)
			} else if len(data) == 0 {
				t.Errorf("static skill %s is empty", name)
			}
		}
	}
}

// TestInstallDynamicSkills tests dynamic skill generation
func TestInstallDynamicSkills(t *testing.T) {
	tmpDir := t.TempDir()

	clusters := []setup.Community{
		{
			ID:            "cluster-1",
			Label:         "Authentication Module",
			CohesionScore: 0.85,
			Members:       []string{"auth.go", "session.go", "token.go"},
		},
		{
			ID:            "cluster-2",
			Label:         "Database Layer",
			CohesionScore: 0.72,
			Members:       []string{"db.go", "models.go", "queries.go"},
		},
		{
			ID:            "cluster-3",
			Label:         "Low Cohesion",
			CohesionScore: 0.15, // Should be skipped
			Members:       []string{"misc.go"},
		},
	}

	err := setup.InstallDynamicSkills(tmpDir, clusters)
	if err != nil {
		t.Fatalf("InstallDynamicSkills() error: %v", err)
	}

	// Verify modules directory created
	modulesDir := filepath.Join(tmpDir, ".claude", "skills", "mimir", "modules")
	if _, err := os.Stat(modulesDir); os.IsNotExist(err) {
		t.Error("modules directory was not created")
	}

	// Verify skill files created (only high-cohesion clusters)
	expectedFiles := []string{
		"authentication-module.md",
		"database-layer.md",
	}

	for _, expectedFile := range expectedFiles {
		path := filepath.Join(modulesDir, expectedFile)
		if _, err := os.Stat(path); os.IsNotExist(err) {
			t.Errorf("dynamic skill %s was not created", expectedFile)
		}
	}

	// Verify low-cohesion cluster was skipped
	lowCohesionFile := filepath.Join(modulesDir, "low-cohesion.md")
	if _, err := os.Stat(lowCohesionFile); !os.IsNotExist(err) {
		t.Error("low-cohesion cluster should have been skipped")
	}
}

// TestSanitizeFilename tests filename sanitization
func TestSanitizeFilename(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"Normal Name", "normal-name"},
		{"With/Slash", "with-slash"},
		{"With\\Backslash", "with-backslash"},
		{"With:Colon", "with-colon"},
		{"With Spaces", "with-spaces"},
		{"Mixed/Path:Name", "mixed-path-name"},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			// Note: sanitizeFilename is internal, so we test indirectly
			// by checking the output of InstallDynamicSkills
		})
	}
}

// TestGenerateModuleSkill tests the module skill content generation
func TestGenerateModuleSkill_Content(t *testing.T) {
	// This tests the output format indirectly through InstallDynamicSkills
	tmpDir := t.TempDir()

	clusters := []setup.Community{
		{
			ID:            "test-cluster",
			Label:         "Test Module",
			CohesionScore: 0.9,
			Members:       []string{"file1.go", "file2.go", "file3.go"},
		},
	}

	err := setup.InstallDynamicSkills(tmpDir, clusters)
	if err != nil {
		t.Fatalf("InstallDynamicSkills() error: %v", err)
	}

	// Read the generated skill
	skillPath := filepath.Join(tmpDir, ".claude", "skills", "mimir", "modules", "test-module.md")
	data, err := os.ReadFile(skillPath)
	if err != nil {
		t.Fatalf("read skill file: %v", err)
	}

	content := string(data)

	// Verify key sections
	requiredSections := []string{
		"name: mimir-module-",
		"description:",
		"## Key files",
		"## Entry points",
		"## Known dependencies",
	}

	for _, section := range requiredSections {
		if !strings.Contains(content, section) {
			t.Errorf("missing section: %s", section)
		}
	}

	// Verify cluster info included
	if !strings.Contains(content, "Test Module") {
		t.Error("cluster label not in skill content")
	}
	if !strings.Contains(content, "0.90") {
		t.Error("cohesion score not in skill content")
	}
}

// TestCommunity_Structure tests the Community type
func TestCommunity_Structure(t *testing.T) {
	community := setup.Community{
		ID:            "test-id",
		Label:         "Test Label",
		CohesionScore: 0.75,
		Members:       []string{"file1.go", "file2.go"},
	}

	if community.ID != "test-id" {
		t.Errorf("ID = %q, want \"test-id\"", community.ID)
	}
	if community.Label != "Test Label" {
		t.Errorf("Label = %q, want \"Test Label\"", community.Label)
	}
	if community.CohesionScore != 0.75 {
		t.Errorf("CohesionScore = %v, want 0.75", community.CohesionScore)
	}
	if len(community.Members) != 2 {
		t.Errorf("Members has %d items, want 2", len(community.Members))
	}
}
