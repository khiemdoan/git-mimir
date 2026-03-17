package parser_test

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/yourusername/mimir/internal/parser"
)

func testdataPath(rel string) string {
	_, thisFile, _, _ := runtime.Caller(0)
	return filepath.Join(filepath.Dir(thisFile), "..", "..", "testdata", rel)
}

func TestParseFile_TypeScript(t *testing.T) {
	path := testdataPath("simple-ts/src/greeter.ts")
	src, err := os.ReadFile(path)
	if err != nil {
		t.Skipf("testdata not found: %v", err)
	}

	fs, err := parser.ParseFile(path, src)
	if err != nil {
		t.Fatalf("ParseFile: %v", err)
	}

	if fs.FilePath != path {
		t.Errorf("FilePath = %q, want %q", fs.FilePath, path)
	}

	// Should find: greet (Function), Greeter (Class), greet method inside Greeter
	names := map[string]bool{}
	kinds := map[string]string{}
	for _, n := range fs.Nodes {
		names[n.Name] = true
		kinds[n.Name] = n.Kind
		t.Logf("  node: %s (%s) lines %d-%d exported=%v", n.Name, n.Kind, n.StartLine, n.EndLine, n.Exported)
	}

	if !names["greet"] {
		t.Error("expected to find function 'greet'")
	}
	if !names["Greeter"] {
		t.Error("expected to find class 'Greeter'")
	}
	if kinds["Greeter"] != "Class" {
		t.Errorf("Greeter kind = %q, want Class", kinds["Greeter"])
	}
}

func TestParseFile_TypeScript_Math(t *testing.T) {
	path := testdataPath("simple-ts/src/math.ts")
	src, err := os.ReadFile(path)
	if err != nil {
		t.Skipf("testdata not found: %v", err)
	}

	fs, err := parser.ParseFile(path, src)
	if err != nil {
		t.Fatalf("ParseFile: %v", err)
	}

	names := map[string]bool{}
	for _, n := range fs.Nodes {
		names[n.Name] = true
		t.Logf("  node: %s (%s) exported=%v", n.Name, n.Kind, n.Exported)
	}

	for _, want := range []string{"add", "multiply", "subtract"} {
		if !names[want] {
			t.Errorf("expected to find function %q", want)
		}
	}
}

func TestParseFile_TypeScript_Index(t *testing.T) {
	path := testdataPath("simple-ts/src/index.ts")
	src, err := os.ReadFile(path)
	if err != nil {
		t.Skipf("testdata not found: %v", err)
	}

	fs, err := parser.ParseFile(path, src)
	if err != nil {
		t.Fatalf("ParseFile: %v", err)
	}

	// Should find imports and calls
	if len(fs.Imports) == 0 {
		t.Error("expected imports to be extracted")
	}
	t.Logf("imports: %d, calls: %d, nodes: %d", len(fs.Imports), len(fs.Calls), len(fs.Nodes))

	// Should find the main() function
	found := false
	for _, n := range fs.Nodes {
		if n.Name == "main" {
			found = true
		}
	}
	if !found {
		t.Error("expected to find function 'main'")
	}
}

func TestParseFile_TypeScript_Class(t *testing.T) {
	path := testdataPath("simple-ts/src/controller.ts")
	src, err := os.ReadFile(path)
	if err != nil {
		t.Skipf("testdata not found: %v", err)
	}

	fs, err := parser.ParseFile(path, src)
	if err != nil {
		t.Fatalf("ParseFile: %v", err)
	}

	foundClass := false
	foundMethod := false
	for _, n := range fs.Nodes {
		t.Logf("  node: %s (%s)", n.Name, n.Kind)
		if n.Name == "UserController" && n.Kind == "Class" {
			foundClass = true
		}
		if n.Name == "handleGetUser" && n.Kind == "Method" {
			foundMethod = true
		}
	}
	if !foundClass {
		t.Error("expected to find class UserController")
	}
	if !foundMethod {
		t.Error("expected to find method handleGetUser")
	}
}

func TestParseFile_UnsupportedExt(t *testing.T) {
	_, err := parser.ParseFile("/tmp/foo.txt", []byte("hello"))
	if err == nil {
		t.Error("expected error for unsupported extension")
	}
}

func TestParseFile_Go(t *testing.T) {
	// Parse one of our own files
	path := testdataPath("../internal/graph/types.go")
	src, err := os.ReadFile(path)
	if err != nil {
		t.Skipf("file not found: %v", err)
	}

	fs, err := parser.ParseFile(path, src)
	if err != nil {
		t.Fatalf("ParseFile: %v", err)
	}

	names := map[string]bool{}
	for _, n := range fs.Nodes {
		names[n.Name] = true
		t.Logf("  node: %s (%s) exported=%v", n.Name, n.Kind, n.Exported)
	}

	if !names["NodeUID"] {
		t.Error("expected to find function NodeUID")
	}
	if !names["EdgeID"] {
		t.Error("expected to find function EdgeID")
	}
}
