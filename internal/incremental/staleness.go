package incremental

import (
	"fmt"

	gogit "github.com/go-git/go-git/v5"
	"github.com/yourusername/mimir/internal/store"
)

// IsStale checks whether the index is out of date with the repo's HEAD.
// Returns (isStale, reason, headSHA, error).
func IsStale(repoPath string, s *store.Store) (bool, string, string, error) {
	repo, err := gogit.PlainOpen(repoPath)
	if err != nil {
		// Not a git repo — always consider stale (force full index)
		return true, "not a git repo", "", nil
	}

	head, err := repo.Head()
	if err != nil {
		return true, "cannot read HEAD", "", err
	}
	headSHA := head.Hash().String()

	lastCommit, err := s.GetMeta("last_commit")
	if err != nil {
		return true, "no previous index", headSHA, nil
	}
	if lastCommit == "" {
		return true, "no previous index", headSHA, nil
	}
	if lastCommit == headSHA {
		return false, fmt.Sprintf("up to date (HEAD %s)", headSHA[:8]), headSHA, nil
	}

	// Count changed files for the message
	changed, err := GetChangedFiles(repoPath, lastCommit)
	if err != nil {
		// If we can't diff, fall back to full re-index
		return true, fmt.Sprintf("changed since %s (diff failed)", lastCommit[:8]), headSHA, nil
	}
	return true, fmt.Sprintf("%d file(s) changed since %s", len(changed), lastCommit[:8]), headSHA, nil
}
