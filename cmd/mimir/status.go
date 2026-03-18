package main

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
	"github.com/thuongh2/git-mimir/internal/incremental"
	"github.com/thuongh2/git-mimir/internal/registry"
	"github.com/thuongh2/git-mimir/internal/store"
)

func runStatus(cmd *cobra.Command, args []string) error {
	reg, err := registry.Load()
	if err != nil {
		return err
	}

	repos := reg.List()
	if len(repos) == 0 {
		fmt.Println("No repositories indexed.")
		return nil
	}

	// Filter to specific name if given
	if len(args) > 0 {
		name := args[0]
		info := reg.Get(name)
		if info == nil {
			return fmt.Errorf("repo %q not found", name)
		}
		repos = []registry.RepoInfo{*info}
	}

	for _, repo := range repos {
		fmt.Printf("Repository: %s\n", repo.Name)
		fmt.Printf("  Path:      %s\n", repo.Path)
		fmt.Printf("  IndexedAt: %s\n", repo.IndexedAt)

		dbPath, err := registry.DBPath(repo.Name)
		if err != nil {
			fmt.Fprintf(os.Stderr, "  Error: %v\n", err)
			continue
		}

		s, err := store.OpenStore(dbPath)
		if err != nil {
			fmt.Printf("  Status:    not indexed\n\n")
			continue
		}
		defer s.Close()

		nodeCount, _ := s.NodeCount()
		edgeCount, _ := s.EdgeCount()
		embedCount, _ := s.EmbeddingCount()
		lastCommit, _ := s.GetMeta("last_commit")
		indexedAt, _ := s.GetMeta("indexed_at")

		fmt.Printf("  Nodes:     %d\n", nodeCount)
		fmt.Printf("  Edges:     %d\n", edgeCount)
		if nodeCount > 0 {
			fmt.Printf("  Embeddings: %d/%d (%.0f%%)\n",
				embedCount, nodeCount, float64(embedCount)/float64(nodeCount)*100)
		}
		if indexedAt != "" {
			fmt.Printf("  Indexed:   %s\n", indexedAt)
		}
		if lastCommit != "" {
			fmt.Printf("  Commit:    %s\n", lastCommit[:min8(len(lastCommit))])
		}

		// Staleness check
		stale, reason, _, err := incremental.IsStale(repo.Path, s)
		if err == nil {
			if stale {
				fmt.Printf("  Staleness: STALE — %s\n", reason)
			} else {
				fmt.Printf("  Staleness: %s\n", reason)
			}
		}
		fmt.Println()
	}
	return nil
}

func min8(n int) int {
	if n < 8 {
		return n
	}
	return 8
}
