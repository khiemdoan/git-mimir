package resolver

import (
	"strings"
	"sync"

	"github.com/thuongh2/git-mimir/internal/graph"
)

// Resolve performs the two-pass resolution algorithm:
//  1. Build a complete SymbolTable from all FileSymbols
//  2. Walk all call/import sites and emit edges
//
// Pass 2 MUST NOT start before Pass 1 finishes.
func Resolve(allFiles []graph.FileSymbols) []graph.Edge {
	table := NewSymbolTable()

	// Pass 1: build SymbolTable in parallel (read-only per goroutine, writes locked)
	var wg1 sync.WaitGroup
	for _, fs := range allFiles {
		fs := fs
		wg1.Add(1)
		go func() {
			defer wg1.Done()
			table.Register(fs)
		}()
	}
	wg1.Wait()

	// Pass 2: resolve call/import sites in parallel
	edgeCh := make(chan graph.Edge, len(allFiles)*4)
	var wg2 sync.WaitGroup
	for _, fs := range allFiles {
		fs := fs
		wg2.Add(1)
		go func() {
			defer wg2.Done()
			resolveFile(fs, table, edgeCh)
		}()
	}
	go func() {
		wg2.Wait()
		close(edgeCh)
	}()

	var edges []graph.Edge
	for e := range edgeCh {
		edges = append(edges, e)
	}
	return edges
}

func resolveFile(fs graph.FileSymbols, table *SymbolTable, out chan<- graph.Edge) {
	// Resolve MEMBER_OF edges (encoded as ImportRef with ImportPath="MEMBER_OF:uid")
	for _, imp := range fs.Imports {
		if strings.HasPrefix(imp.ImportPath, "MEMBER_OF:") {
			targetUID := strings.TrimPrefix(imp.ImportPath, "MEMBER_OF:")
			out <- graph.Edge{
				FromUID:    imp.FromUID,
				ToUID:      targetUID,
				Type:       graph.EdgeMemberOf,
				Confidence: 1.0,
			}
			continue
		}

		// Resolve EXTENDS/IMPLEMENTS (encoded as CallRef)
		// Handled below in calls section.

		// Import edges: fromUID (first node in file) → imported file nodes
		if imp.FromUID == "" {
			continue
		}
		targetPkg, conf := table.ResolveImport(fs.FilePath, imp.ImportPath)
		_ = targetPkg
		// Emit an IMPORTS edge from the containing symbol to the import path
		// We emit a placeholder edge (no to_uid yet); the actual to_uid is resolved via call refs.
		// For now, just record the import in the table (already done in pass 1).
		_ = conf
	}

	// Resolve EXTENDS/IMPLEMENTS edges (encoded as CallRef with prefix)
	for _, call := range fs.Calls {
		if strings.HasPrefix(call.CalleeName, "EXTENDS:") {
			target := strings.TrimPrefix(call.CalleeName, "EXTENDS:")
			if toUID, conf, ok := table.Resolve(fs.FilePath, target); ok {
				out <- graph.Edge{
					FromUID:    call.FromUID,
					ToUID:      toUID,
					Type:       graph.EdgeExtends,
					Confidence: conf,
				}
			}
			continue
		}
		if strings.HasPrefix(call.CalleeName, "IMPLEMENTS:") {
			target := strings.TrimPrefix(call.CalleeName, "IMPLEMENTS:")
			if toUID, conf, ok := table.Resolve(fs.FilePath, target); ok {
				out <- graph.Edge{
					FromUID:    call.FromUID,
					ToUID:      toUID,
					Type:       graph.EdgeImplements,
					Confidence: conf,
				}
			}
			continue
		}

		// Normal CALLS edge
		var toUID string
		var conf float64
		var ok bool

		if strings.Contains(call.CalleeName, ".") {
			toUID, conf, ok = table.ResolveQualified(fs.FilePath, call.CalleeName)
		} else {
			toUID, conf, ok = table.Resolve(fs.FilePath, call.CalleeName)
		}

		if ok && toUID != call.FromUID {
			out <- graph.Edge{
				FromUID:    call.FromUID,
				ToUID:      toUID,
				Type:       graph.EdgeCalls,
				Confidence: conf,
			}
		}
	}
}
