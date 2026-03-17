package registry

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

// RepoInfo holds metadata about a registered repository.
type RepoInfo struct {
	Name      string `json:"name"`
	Path      string `json:"path"`
	IndexedAt string `json:"indexed_at,omitempty"`
}

// Registry manages the list of indexed repositories.
type Registry struct {
	path  string
	Repos []RepoInfo `json:"repos"`
}

// DefaultPath returns the default registry file path (~/.mimir/registry.json).
func DefaultPath() string {
	home, err := os.UserHomeDir()
	if err != nil {
		home = "."
	}
	return filepath.Join(home, ".mimir", "registry.json")
}

// DBPath returns the SQLite database path for a registered repo.
func DBPath(repoName string) (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	dir := filepath.Join(home, ".mimir", "indexes")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}
	return filepath.Join(dir, repoName+".db"), nil
}

// Load reads the registry from disk (creates it if missing).
func Load() (*Registry, error) {
	path := DefaultPath()

	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return &Registry{path: path, Repos: []RepoInfo{}}, nil
	}
	if err != nil {
		return nil, fmt.Errorf("read registry: %w", err)
	}

	var reg Registry
	if err := json.Unmarshal(data, &reg); err != nil {
		return nil, fmt.Errorf("parse registry: %w", err)
	}
	reg.path = path
	return &reg, nil
}

// Save writes the registry to disk.
func (r *Registry) Save() error {
	if err := os.MkdirAll(filepath.Dir(r.path), 0o755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(r, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(r.path, data, 0o644)
}

// Register adds or updates a repo entry. Uses directory basename as default name.
func (r *Registry) Register(repoPath, name string) error {
	if name == "" {
		name = filepath.Base(repoPath)
	}
	abs, err := filepath.Abs(repoPath)
	if err != nil {
		return err
	}

	for i, repo := range r.Repos {
		if repo.Name == name || repo.Path == abs {
			r.Repos[i].Name = name
			r.Repos[i].Path = abs
			r.Repos[i].IndexedAt = time.Now().UTC().Format(time.RFC3339)
			return r.Save()
		}
	}
	r.Repos = append(r.Repos, RepoInfo{
		Name:      name,
		Path:      abs,
		IndexedAt: time.Now().UTC().Format(time.RFC3339),
	})
	return r.Save()
}

// Unregister removes a repo by name.
func (r *Registry) Unregister(name string) error {
	for i, repo := range r.Repos {
		if repo.Name == name {
			r.Repos = append(r.Repos[:i], r.Repos[i+1:]...)
			return r.Save()
		}
	}
	return fmt.Errorf("repo %q not found", name)
}

// Get returns the RepoInfo for a name (or nil if not found).
func (r *Registry) Get(name string) *RepoInfo {
	for _, repo := range r.Repos {
		if repo.Name == name {
			return &repo
		}
	}
	return nil
}

// List returns all registered repos.
func (r *Registry) List() []RepoInfo {
	return r.Repos
}

// FindByPath returns the RepoInfo for a given absolute path (or nil).
func (r *Registry) FindByPath(absPath string) *RepoInfo {
	for _, repo := range r.Repos {
		if repo.Path == absPath {
			return &repo
		}
	}
	return nil
}
