package walker

import (
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	ignore "github.com/sabhiram/go-gitignore"
)

// FileInfo holds metadata for a file discovered during the walk.
type FileInfo struct {
	Path    string
	Ext     string
	Size    int64
	ModTime time.Time
}

// skipDirs are directory names that should always be skipped.
var skipDirs = map[string]bool{
	".git":         true,
	".mimir":       true,
	"node_modules": true,
	"vendor":       true,
	"dist":         true,
	".idea":        true,
	".vscode":      true,
}

// skipSuffixes are file name suffixes that should always be skipped.
var skipSuffixes = []string{
	".min.js",
	".lock",
	".pb.go",
}

// WalkRepo walks the repository rooted at root using a goroutine pool of
// concurrency workers and returns a channel of FileInfo. The caller must
// drain the channel; the channel is closed when the walk is complete.
func WalkRepo(root string, concurrency int) <-chan FileInfo {
	if concurrency < 1 {
		concurrency = 1
	}
	out := make(chan FileInfo, concurrency*16)

	go func() {
		defer close(out)

		// Load .gitignore if present.
		var gi *ignore.GitIgnore
		gitignorePath := filepath.Join(root, ".gitignore")
		if _, err := os.Stat(gitignorePath); err == nil {
			gi, _ = ignore.CompileIgnoreFile(gitignorePath)
		}

		// Collect all file paths first (fast, single goroutine).
		type entry struct {
			path string
			info fs.FileInfo
		}
		entries := make(chan entry, concurrency*32)

		var walkWg sync.WaitGroup
		walkWg.Add(1)
		go func() {
			defer walkWg.Done()
			defer close(entries)

			_ = filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
				if err != nil {
					return nil // skip unreadable entries
				}
				name := d.Name()
				if d.IsDir() {
					if skipDirs[name] {
						return filepath.SkipDir
					}
					return nil
				}
				// Apply gitignore.
				rel, _ := filepath.Rel(root, path)
				if gi != nil && gi.MatchesPath(rel) {
					return nil
				}
				// Skip unwanted suffixes.
				for _, suf := range skipSuffixes {
					if strings.HasSuffix(name, suf) {
						return nil
					}
				}
				info, err := d.Info()
				if err != nil {
					return nil
				}
				entries <- entry{path: path, info: info}
				return nil
			})
		}()

		// Fan-out workers to stat and send FileInfo.
		var workerWg sync.WaitGroup
		for i := 0; i < concurrency; i++ {
			workerWg.Add(1)
			go func() {
				defer workerWg.Done()
				for e := range entries {
					out <- FileInfo{
						Path:    e.path,
						Ext:     filepath.Ext(e.info.Name()),
						Size:    e.info.Size(),
						ModTime: e.info.ModTime(),
					}
				}
			}()
		}

		walkWg.Wait()
		workerWg.Wait()
	}()

	return out
}
