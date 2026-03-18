package resolver

import (
	"path/filepath"
	"strings"
	"sync"

	"github.com/thuongh2/git-mimir/internal/graph"
)

// Candidate is a potential resolution target.
type Candidate struct {
	NodeUID  string
	FilePath string
}

// SymbolTable maps package/file paths to exported symbol names to their candidates.
type SymbolTable struct {
	mu sync.RWMutex
	// pkg → name → []Candidate
	byPkg map[string]map[string][]Candidate
	// filePath → name → []Candidate (for same-file resolution)
	byFile map[string]map[string][]Candidate
}

// NewSymbolTable creates an empty symbol table.
func NewSymbolTable() *SymbolTable {
	return &SymbolTable{
		byPkg:  make(map[string]map[string][]Candidate),
		byFile: make(map[string]map[string][]Candidate),
	}
}

// Register adds all symbols from a FileSymbols into the table.
func (t *SymbolTable) Register(fs graph.FileSymbols) {
	t.mu.Lock()
	defer t.mu.Unlock()

	dir := filepath.Dir(fs.FilePath)

	for _, n := range fs.Nodes {
		pkg := n.PackagePath
		if pkg == "" {
			pkg = dir
		}
		if t.byPkg[pkg] == nil {
			t.byPkg[pkg] = make(map[string][]Candidate)
		}
		t.byPkg[pkg][n.Name] = append(t.byPkg[pkg][n.Name], Candidate{
			NodeUID:  n.UID,
			FilePath: n.FilePath,
		})

		if t.byFile[fs.FilePath] == nil {
			t.byFile[fs.FilePath] = make(map[string][]Candidate)
		}
		t.byFile[fs.FilePath][n.Name] = append(t.byFile[fs.FilePath][n.Name], Candidate{
			NodeUID:  n.UID,
			FilePath: n.FilePath,
		})
	}
}

// Resolve attempts to find the target node UID for a callee from a given file.
// It returns (uid, confidence, ok).
func (t *SymbolTable) Resolve(fromFile, calleeName string) (string, float64, bool) {
	t.mu.RLock()
	defer t.mu.RUnlock()

	// 1. Same-file lookup (highest priority, same package)
	if fileSyms, ok := t.byFile[fromFile]; ok {
		if cands, ok := fileSyms[calleeName]; ok && len(cands) == 1 {
			return cands[0].NodeUID, 0.95, true
		} else if ok && len(cands) > 1 {
			return cands[0].NodeUID, 0.60, true
		}
	}

	// 2. Same directory (same package for Go, barrel for TS)
	dir := filepath.Dir(fromFile)
	if pkgSyms, ok := t.byPkg[dir]; ok {
		if cands, ok := pkgSyms[calleeName]; ok && len(cands) == 1 {
			return cands[0].NodeUID, 0.95, true
		} else if ok && len(cands) > 1 {
			return cands[0].NodeUID, 0.60, true
		}
	}

	// 3. Global search (any package) — lower confidence
	var found []Candidate
	for _, pkgSyms := range t.byPkg {
		if cands, ok := pkgSyms[calleeName]; ok {
			found = append(found, cands...)
		}
	}
	if len(found) == 1 {
		return found[0].NodeUID, 0.75, true
	}
	if len(found) > 1 {
		return found[0].NodeUID, 0.60, true
	}

	return "", 0, false
}

// ResolveImport resolves a relative import path from a source file to a target file/package.
// Returns the package path and a confidence score.
func (t *SymbolTable) ResolveImport(fromFile, importPath string) (string, float64) {
	if strings.HasPrefix(importPath, ".") {
		// Relative import
		dir := filepath.Dir(fromFile)
		abs := filepath.Clean(filepath.Join(dir, importPath))
		return abs, 0.95
	}
	// Absolute/package import
	return importPath, 0.85
}

// ResolveQualified resolves "Alias.Method" or "pkg.Func" style calls.
// Returns (uid, confidence, ok).
func (t *SymbolTable) ResolveQualified(fromFile, qualified string) (string, float64, bool) {
	parts := strings.SplitN(qualified, ".", 2)
	if len(parts) != 2 {
		return t.Resolve(fromFile, qualified)
	}
	// Try "parts[1]" in all packages
	methodName := parts[1]
	return t.Resolve(fromFile, methodName)
}
