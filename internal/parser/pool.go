package parser

import (
	"context"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"runtime"
	"sync"

	sitter "github.com/smacker/go-tree-sitter"
	"github.com/yourusername/mimir/internal/graph"
	"github.com/yourusername/mimir/internal/walker"
)

// worker holds a pre-allocated set of parsers (one per supported language)
// so we never create parsers inside the hot parse loop.
type worker struct {
	parsers map[LangID]*sitter.Parser
}

func newWorker() *worker {
	w := &worker{parsers: make(map[LangID]*sitter.Parser)}
	for _, lang := range []LangID{
		LangTypeScript, LangTSX, LangJavaScript,
		LangGo, LangPython, LangJava, LangRust, LangCPP,
	} {
		g := grammarForLang(lang)
		if g == nil {
			continue
		}
		p := sitter.NewParser()
		p.SetLanguage(g)
		w.parsers[lang] = p
	}
	return w
}

func (w *worker) parse(path string, src []byte) (*graph.FileSymbols, error) {
	ext := filepath.Ext(path)
	lang := LangForExt(ext)
	if lang == "" {
		return nil, fmt.Errorf("unsupported extension: %s", ext)
	}

	tp := &treeParser{lang: lang, parser: w.parsers[lang]}
	if tp.parser == nil {
		return nil, fmt.Errorf("no parser for %s", lang)
	}
	return tp.parse(path, src)
}

// ParserPool manages a fixed set of worker goroutines, each with pre-allocated parsers.
type ParserPool struct {
	size int
}

// NewParserPool creates a pool with `size` workers.
// If size <= 0, defaults to runtime.GOMAXPROCS(0).
func NewParserPool(size int) *ParserPool {
	if size <= 0 {
		size = runtime.GOMAXPROCS(0)
	}
	return &ParserPool{size: size}
}

// Run processes files from the files channel and returns a channel of FileSymbols.
// The output channel is closed when all files have been processed.
func (p *ParserPool) Run(ctx context.Context, files <-chan walker.FileInfo) <-chan graph.FileSymbols {
	out := make(chan graph.FileSymbols, p.size*4)
	var wg sync.WaitGroup

	for i := 0; i < p.size; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			w := newWorker()
			for {
				select {
				case <-ctx.Done():
					return
				case fi, ok := <-files:
					if !ok {
						return
					}
					src, err := os.ReadFile(fi.Path)
					if err != nil {
						log.Printf("parser: read %s: %v", fi.Path, err)
						continue
					}
					fs, err := w.parse(fi.Path, src)
					if err != nil {
						// Unsupported or broken file — skip silently
						continue
					}
					select {
					case out <- *fs:
					case <-ctx.Done():
						return
					}
				}
			}
		}()
	}

	go func() {
		wg.Wait()
		close(out)
	}()

	return out
}
