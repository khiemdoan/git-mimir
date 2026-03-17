package process

import (
	"github.com/yourusername/mimir/internal/store"
)

// ClassifyProcess determines the process type based on cluster membership.
func ClassifyProcess(proc *Process, s *store.Store) string {
	clusterIDs := map[string]bool{}
	hasExternal := false

	for _, step := range proc.Steps {
		node, err := s.QueryNodeByUID(step.SymbolUID)
		if err != nil || node == nil {
			hasExternal = true
			continue
		}
		if node.ClusterID == "" {
			hasExternal = true
		} else {
			clusterIDs[node.ClusterID] = true
		}
	}

	if hasExternal {
		return "external"
	}
	if len(clusterIDs) > 1 {
		return "cross_community"
	}
	return "single_community"
}
