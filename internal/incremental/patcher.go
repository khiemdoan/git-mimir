package incremental

import (
	"context"
	"log"
	"os"

	"github.com/thuongh2/git-mimir/internal/graph"
	"github.com/thuongh2/git-mimir/internal/parser"
	"github.com/thuongh2/git-mimir/internal/store"
	"github.com/thuongh2/git-mimir/internal/walker"
)

// PatchPlan describes what needs to be re-indexed.
type PatchPlan struct {
	ToAdd    []string
	ToUpdate []string
	ToDelete []string
}

// PlanPatch converts a list of changed files into a patch plan.
func PlanPatch(changed []ChangedFile) *PatchPlan {
	p := &PatchPlan{}
	for _, f := range changed {
		switch f.Status {
		case StatusAdded:
			p.ToAdd = append(p.ToAdd, f.Path)
		case StatusModified:
			p.ToUpdate = append(p.ToUpdate, f.Path)
		case StatusDeleted:
			p.ToDelete = append(p.ToDelete, f.Path)
		}
	}
	return p
}

// ApplyPatch applies a patch plan to the store.
// It deletes stale nodes, re-parses and re-resolves changed files.
func ApplyPatch(ctx context.Context, plan *PatchPlan, s *store.Store) ([]graph.FileSymbols, error) {
	// Delete stale nodes (cascade removes edges)
	for _, path := range plan.ToUpdate {
		if err := s.DeleteNodesByFile(path); err != nil {
			log.Printf("incremental: delete %s: %v", path, err)
		}
	}
	for _, path := range plan.ToDelete {
		if err := s.DeleteNodesByFile(path); err != nil {
			log.Printf("incremental: delete %s: %v", path, err)
		}
	}

	// Re-parse added + updated files
	toProcess := append(plan.ToAdd, plan.ToUpdate...)
	if len(toProcess) == 0 {
		return nil, nil
	}

	// Convert to FileInfo channel for the parser pool
	fileCh := make(chan walker.FileInfo, len(toProcess))
	for _, path := range toProcess {
		info, err := os.Stat(path)
		if err != nil {
			log.Printf("incremental: stat %s: %v", path, err)
			continue
		}
		fileCh <- walker.FileInfo{
			Path:    path,
			Ext:     extOf(path),
			Size:    info.Size(),
			ModTime: info.ModTime(),
		}
	}
	close(fileCh)

	pool := parser.NewParserPool(0)
	symsCh := pool.Run(ctx, fileCh)

	var allSymbols []graph.FileSymbols
	for fs := range symsCh {
		allSymbols = append(allSymbols, fs)
	}
	return allSymbols, nil
}

func extOf(path string) string {
	for i := len(path) - 1; i >= 0; i-- {
		if path[i] == '.' {
			return path[i:]
		}
		if path[i] == '/' {
			break
		}
	}
	return ""
}
