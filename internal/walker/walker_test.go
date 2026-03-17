package walker_test

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/yourusername/mimir/internal/walker"
)

func TestWalkRepo_CountsFiles(t *testing.T) {
	// Find testdata/simple-ts relative to this test file.
	_, thisFile, _, _ := runtime.Caller(0)
	root := filepath.Join(filepath.Dir(thisFile), "..", "..", "testdata", "simple-ts")

	if _, err := os.Stat(root); err != nil {
		t.Skipf("testdata/simple-ts not found: %v", err)
	}

	files := walker.WalkRepo(root, 4)

	count := 0
	for f := range files {
		t.Logf("found: %s (ext=%s, size=%d)", f.Path, f.Ext, f.Size)
		count++
	}

	// simple-ts has 3 .ts files + 1 .gitignore = 4 files
	if count < 3 {
		t.Errorf("expected at least 3 files, got %d", count)
	}
}

func TestWalkRepo_SkipsNodeModules(t *testing.T) {
	dir := t.TempDir()

	// Create a src file and a node_modules file.
	if err := os.MkdirAll(filepath.Join(dir, "src"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(dir, "node_modules", "pkg"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "src", "main.ts"), []byte("export {}"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "node_modules", "pkg", "index.js"), []byte("module.exports={}"), 0o644); err != nil {
		t.Fatal(err)
	}

	files := walker.WalkRepo(dir, 2)

	var paths []string
	for f := range files {
		paths = append(paths, f.Path)
	}

	for _, p := range paths {
		if filepath.Base(filepath.Dir(p)) == "node_modules" || containsSegment(p, "node_modules") {
			t.Errorf("walker returned file inside node_modules: %s", p)
		}
	}

	found := false
	for _, p := range paths {
		if filepath.Base(p) == "main.ts" {
			found = true
		}
	}
	if !found {
		t.Errorf("expected to find src/main.ts, got paths: %v", paths)
	}
}

func TestWalkRepo_SkipsMinJS(t *testing.T) {
	dir := t.TempDir()

	if err := os.WriteFile(filepath.Join(dir, "app.js"), []byte("console.log(1)"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "app.min.js"), []byte("console.log(1)"), 0o644); err != nil {
		t.Fatal(err)
	}

	files := walker.WalkRepo(dir, 2)

	var paths []string
	for f := range files {
		paths = append(paths, filepath.Base(f.Path))
	}

	for _, name := range paths {
		if name == "app.min.js" {
			t.Errorf("walker should skip *.min.js files, but found: %s", name)
		}
	}
}

func TestWalkRepo_RespectsGitignore(t *testing.T) {
	dir := t.TempDir()

	if err := os.WriteFile(filepath.Join(dir, ".gitignore"), []byte("secret.ts\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "main.ts"), []byte("export {}"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "secret.ts"), []byte("password=123"), 0o644); err != nil {
		t.Fatal(err)
	}

	files := walker.WalkRepo(dir, 2)

	var names []string
	for f := range files {
		names = append(names, filepath.Base(f.Path))
	}

	for _, name := range names {
		if name == "secret.ts" {
			t.Errorf("walker should have skipped gitignored file secret.ts")
		}
	}
}

// containsSegment reports whether path has a component equal to segment.
func containsSegment(path, segment string) bool {
	for _, part := range filepath.SplitList(path) {
		if part == segment {
			return true
		}
	}
	// Also check by splitting on separator.
	parts := filepath.SplitList(filepath.ToSlash(path))
	_ = parts
	clean := filepath.ToSlash(path)
	for _, part := range splitSlash(clean) {
		if part == segment {
			return true
		}
	}
	return false
}

func splitSlash(s string) []string {
	var parts []string
	cur := ""
	for _, c := range s {
		if c == '/' {
			if cur != "" {
				parts = append(parts, cur)
			}
			cur = ""
		} else {
			cur += string(c)
		}
	}
	if cur != "" {
		parts = append(parts, cur)
	}
	return parts
}
