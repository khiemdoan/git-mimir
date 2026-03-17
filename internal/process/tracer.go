package process

import (
	"crypto/sha256"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/yourusername/mimir/internal/graph"
	"github.com/yourusername/mimir/internal/store"
)

// Process represents an execution flow starting from an entry point.
type Process struct {
	ID          string
	Name        string
	EntryUID    string
	Steps       []store.ProcessStep
	ProcessType string // single_community | cross_community | external
}

// FindEntryPoints detects likely process entry points from a list of nodes.
// Heuristics (in priority order):
// a. Exported functions named: main, handler, Handle*, Serve*, Route*, Controller*
// b. Exported functions with 0 incoming CALLS but ≥1 outgoing
// c. Functions in files named: main.go, server.go, router.go, handler*.go
func FindEntryPoints(nodes []graph.Node, s *store.Store) []graph.Node {
	// Build set of UIDs that have incoming CALLS
	incomingUIDs := map[string]bool{}
	outgoingCount := map[string]int{}

	for _, n := range nodes {
		edges, err := s.QueryEdgesFrom(n.UID)
		if err != nil {
			continue
		}
		for _, e := range edges {
			if e.Type == graph.EdgeCalls {
				incomingUIDs[e.ToUID] = true
				outgoingCount[n.UID]++
			}
		}
	}

	var entries []graph.Node
	seen := map[string]bool{}

	for _, n := range nodes {
		if n.Kind != "Function" && n.Kind != "Method" {
			continue
		}

		isEntry := false

		// Heuristic a: entry point by name pattern
		nameLower := strings.ToLower(n.Name)
		if n.Exported && (nameLower == "main" ||
			strings.HasPrefix(nameLower, "handle") ||
			strings.HasPrefix(nameLower, "serve") ||
			strings.HasPrefix(nameLower, "route") ||
			strings.HasPrefix(nameLower, "controller")) {
			isEntry = true
		}

		// Heuristic b: no incoming CALLS but has outgoing
		if !isEntry && n.Exported && !incomingUIDs[n.UID] && outgoingCount[n.UID] > 0 {
			isEntry = true
		}

		// Heuristic c: entry-point file names
		if !isEntry {
			base := strings.ToLower(filepath.Base(n.FilePath))
			if base == "main.go" || base == "server.go" || base == "router.go" ||
				strings.HasPrefix(base, "handler") {
				isEntry = true
			}
		}

		if isEntry && !seen[n.UID] {
			seen[n.UID] = true
			entries = append(entries, n)
		}
	}
	return entries
}

// TraceProcess performs a BFS through CALLS edges from an entry point.
func TraceProcess(entry graph.Node, s *store.Store, maxDepth int) *Process {
	id := processID(entry.UID)
	proc := &Process{
		ID:       id,
		Name:     entry.Name,
		EntryUID: entry.UID,
	}

	visited := map[string]bool{entry.UID: true}
	type qItem struct {
		uid   string
		depth int
	}
	queue := []qItem{{uid: entry.UID, depth: 0}}
	stepIdx := 0

	proc.Steps = append(proc.Steps, store.ProcessStep{
		Index:     stepIdx,
		SymbolUID: entry.UID,
		Depth:     0,
	})
	stepIdx++

	for len(queue) > 0 {
		item := queue[0]
		queue = queue[1:]

		if item.depth >= maxDepth {
			continue
		}

		edges, err := s.QueryEdgesFrom(item.uid)
		if err != nil {
			continue
		}

		for _, e := range edges {
			if e.Type != graph.EdgeCalls {
				continue
			}
			if visited[e.ToUID] {
				continue
			}
			visited[e.ToUID] = true
			proc.Steps = append(proc.Steps, store.ProcessStep{
				Index:     stepIdx,
				SymbolUID: e.ToUID,
				Depth:     item.depth + 1,
			})
			stepIdx++
			queue = append(queue, qItem{uid: e.ToUID, depth: item.depth + 1})
		}
	}

	return proc
}

func processID(entryUID string) string {
	h := sha256.Sum256([]byte("process:" + entryUID))
	return fmt.Sprintf("proc_%x", h[:8])
}
