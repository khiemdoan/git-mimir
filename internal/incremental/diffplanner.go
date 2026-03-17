package incremental

import (
	"fmt"

	gogit "github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/object"
	"github.com/yourusername/mimir/internal/parser"
)

// ChangeStatus represents the status of a changed file.
type ChangeStatus string

const (
	StatusAdded    ChangeStatus = "added"
	StatusModified ChangeStatus = "modified"
	StatusDeleted  ChangeStatus = "deleted"
)

// ChangedFile is a file that changed between two commits.
type ChangedFile struct {
	Path   string
	Status ChangeStatus
}

// GetChangedFiles returns files changed between sinceCommit and HEAD.
// If sinceCommit is "", returns all tracked files (first run).
func GetChangedFiles(repoPath, sinceCommit string) ([]ChangedFile, error) {
	repo, err := gogit.PlainOpen(repoPath)
	if err != nil {
		return nil, fmt.Errorf("open repo: %w", err)
	}

	head, err := repo.Head()
	if err != nil {
		return nil, fmt.Errorf("read HEAD: %w", err)
	}

	headCommit, err := repo.CommitObject(head.Hash())
	if err != nil {
		return nil, fmt.Errorf("read head commit: %w", err)
	}

	if sinceCommit == "" {
		// First run: return all supported files in the tree
		return allFilesInCommit(headCommit)
	}

	sinceHash := plumbing.NewHash(sinceCommit)
	sinceCommitObj, err := repo.CommitObject(sinceHash)
	if err != nil {
		return nil, fmt.Errorf("read since commit %s: %w", sinceCommit, err)
	}

	sinceTree, err := sinceCommitObj.Tree()
	if err != nil {
		return nil, err
	}
	headTree, err := headCommit.Tree()
	if err != nil {
		return nil, err
	}

	changes, err := sinceTree.Diff(headTree)
	if err != nil {
		return nil, fmt.Errorf("diff trees: %w", err)
	}

	var result []ChangedFile
	for _, change := range changes {
		from, to, err := change.Files()
		if err != nil {
			continue
		}

		switch {
		case from == nil && to != nil:
			if isSupportedFile(to.Name) {
				result = append(result, ChangedFile{Path: repoPath + "/" + to.Name, Status: StatusAdded})
			}
		case from != nil && to == nil:
			if isSupportedFile(from.Name) {
				result = append(result, ChangedFile{Path: repoPath + "/" + from.Name, Status: StatusDeleted})
			}
		case from != nil && to != nil:
			if isSupportedFile(to.Name) {
				result = append(result, ChangedFile{Path: repoPath + "/" + to.Name, Status: StatusModified})
			}
		}
	}
	return result, nil
}

func allFilesInCommit(commit *object.Commit) ([]ChangedFile, error) {
	tree, err := commit.Tree()
	if err != nil {
		return nil, err
	}

	var files []ChangedFile
	err = tree.Files().ForEach(func(f *object.File) error {
		if isSupportedFile(f.Name) {
			files = append(files, ChangedFile{Path: f.Name, Status: StatusAdded})
		}
		return nil
	})
	return files, err
}

func isSupportedFile(name string) bool {
	for ext := range map[string]bool{
		".ts": true, ".tsx": true, ".js": true, ".jsx": true,
		".go": true, ".py": true, ".java": true, ".rs": true,
		".cpp": true, ".cc": true, ".cxx": true,
	} {
		if len(name) > len(ext) && name[len(name)-len(ext):] == ext {
			return parser.LangForExt(ext) != ""
		}
	}
	return false
}
