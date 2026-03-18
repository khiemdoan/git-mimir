package main

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/spf13/cobra"
	"github.com/thuongh2/git-mimir/internal/cluster"
	"github.com/thuongh2/git-mimir/internal/daemon"
	"github.com/thuongh2/git-mimir/internal/embedder"
	"github.com/thuongh2/git-mimir/internal/graph"
	"github.com/thuongh2/git-mimir/internal/incremental"
	"github.com/thuongh2/git-mimir/internal/parser"
	"github.com/thuongh2/git-mimir/internal/process"
	"github.com/thuongh2/git-mimir/internal/registry"
	"github.com/thuongh2/git-mimir/internal/resolver"
	"github.com/thuongh2/git-mimir/internal/setup"
	"github.com/thuongh2/git-mimir/internal/store"
	"github.com/thuongh2/git-mimir/internal/walker"
)

var (
	analyzeForce        bool
	analyzeSkipEmbeds   bool
	analyzeResolution   float64
	analyzeName         string
	analyzeIncremental  bool
	analyzeHint         string
	analyzeRepo         string
	analyzeSkipHooks    bool
	analyzeSkipSkills   bool
	analyzeSkipDaemon   bool
	analyzeQuiet        bool
)

func init() {
	analyzeCmd.RunE = runAnalyze
	analyzeCmd.Flags().BoolVarP(&analyzeForce, "force", "f", false, "Force full re-index even if up to date")
	analyzeCmd.Flags().BoolVar(&analyzeSkipEmbeds, "skip-embeddings", false, "Skip embedding generation")
	analyzeCmd.Flags().Float64Var(&analyzeResolution, "resolution", 1.0, "Louvain clustering resolution")
	analyzeCmd.Flags().StringVar(&analyzeName, "name", "", "Repository name (default: directory basename)")
	analyzeCmd.Flags().BoolVar(&analyzeIncremental, "incremental", false, "Force incremental mode (skip full index)")
	analyzeCmd.Flags().StringVar(&analyzeHint, "hint", "", "File path hint for faster patch planning")
	analyzeCmd.Flags().StringVar(&analyzeRepo, "repo", "", "Repository path (default: current directory)")
	analyzeCmd.Flags().BoolVar(&analyzeSkipHooks, "skip-hooks", false, "Don't install Claude Code hooks")
	analyzeCmd.Flags().BoolVar(&analyzeSkipSkills, "skip-skills", false, "Don't install agent skills")
	analyzeCmd.Flags().BoolVar(&analyzeSkipDaemon, "skip-daemon", false, "Don't start MCP daemon")
	analyzeCmd.Flags().BoolVar(&analyzeQuiet, "quiet", false, "Suppress progress output")
}

func runAnalyze(cmd *cobra.Command, args []string) error {
	repoPath, err := absPath(args[0])
	if err != nil {
		return fmt.Errorf("resolve path: %w", err)
	}

	// Open/create registry
	reg, err := registry.Load()
	if err != nil {
		return fmt.Errorf("load registry: %w", err)
	}

	repoName := analyzeName
	if repoName == "" {
		if info := reg.FindByPath(repoPath); info != nil {
			repoName = info.Name
		}
	}

	// Register the repo (uses basename if name is empty)
	if err := reg.Register(repoPath, repoName); err != nil {
		return fmt.Errorf("register repo: %w", err)
	}
	// Ensure we have the name after registration
	if repoName == "" {
		info := reg.FindByPath(repoPath)
		if info != nil {
			repoName = info.Name
		} else {
			repoName = filepath.Base(repoPath)
		}
	}

	// Open store
	dbPath, err := registry.DBPath(repoName)
	if err != nil {
		return fmt.Errorf("db path: %w", err)
	}
	s, err := store.OpenStore(dbPath)
	if err != nil {
		return fmt.Errorf("open store: %w", err)
	}
	defer s.Close()

	ctx := context.Background()

	// Check staleness
	stale, reason, headSHA, err := incremental.IsStale(repoPath, s)
	if err != nil {
		return fmt.Errorf("staleness check: %w", err)
	}

	if !stale && !analyzeForce {
		fmt.Printf("Index is current (%s)\n", reason)
		return nil
	}

	if analyzeForce {
		fmt.Println("Force re-index requested — clearing existing data...")
		if err := clearRepo(s, repoPath); err != nil {
			return fmt.Errorf("clear repo: %w", err)
		}
	} else {
		fmt.Printf("Stale: %s\n", reason)
	}

	// Select embed provider
	var emb embedder.Embedder
	if analyzeSkipEmbeds {
		emb = &embedder.NoopEmbedder{}
	} else {
		emb = embedder.NewEmbedder()
	}

	// Determine which files to process
	var allSymbols []graph.FileSymbols
	var indexedFiles int

	if analyzeForce || !isIncrementalCandidate(s) {
		// Full index
		allSymbols, indexedFiles, err = fullIndex(ctx, repoPath, s)
	} else {
		// Incremental patch
		lastCommit, _ := s.GetMeta("last_commit")
		changed, err2 := incremental.GetChangedFiles(repoPath, lastCommit)
		if err2 != nil {
			// Fall back to full index
			allSymbols, indexedFiles, err = fullIndex(ctx, repoPath, s)
		} else {
			plan := incremental.PlanPatch(changed)
			allSymbols, err = incremental.ApplyPatch(ctx, plan, s)
			indexedFiles = len(changed)
		}
	}
	if err != nil {
		return err
	}

	// Upsert nodes
	var allNodes []graph.Node
	for _, fs := range allSymbols {
		allNodes = append(allNodes, fs.Nodes...)
	}
	if err := s.BatchUpsertNodes(allNodes); err != nil {
		return fmt.Errorf("upsert nodes: %w", err)
	}

	// Build BM25 index
	if err := s.IndexNodesBM25(allNodes); err != nil {
		return fmt.Errorf("index bm25: %w", err)
	}

	fmt.Printf("Parsed %s files\n", fmtCount(indexedFiles))

	// Resolve edges
	resolveStart := time.Now()
	edges := resolver.Resolve(allSymbols)
	if err := s.BatchUpsertEdges(edges); err != nil {
		return fmt.Errorf("upsert edges: %w", err)
	}
	fmt.Printf("Resolved %s edges (%s)\n", fmtCount(len(edges)), time.Since(resolveStart).Round(time.Millisecond))

	// Cluster
	clusterStart := time.Now()
	if err := s.DeleteAllClusters(); err != nil {
		return fmt.Errorf("clear clusters: %w", err)
	}
	allNodesDB, _ := s.AllNodes()
	allEdgesDB, _ := s.AllEdges()
	g := cluster.BuildGraph(allNodesDB, allEdgesDB)
	communities := cluster.Louvain(g, analyzeResolution)

	nodeMap := make(map[string]graph.Node, len(allNodesDB))
	for _, n := range allNodesDB {
		nodeMap[n.UID] = n
	}

	for _, c := range communities {
		members := make([]graph.Node, 0, len(c.Members))
		for _, uid := range c.Members {
			if n, ok := nodeMap[uid]; ok {
				members = append(members, n)
			}
		}
		label := cluster.LabelCommunity(members)
		c.Label = label

		if err := s.UpsertCluster(c.ID, label, c.CohesionScore); err != nil {
			return fmt.Errorf("upsert cluster: %w", err)
		}
		if err := s.UpsertClusterMembers(c.ID, c.Members); err != nil {
			return fmt.Errorf("upsert cluster members: %w", err)
		}
		if err := s.SetClusterForNodes(c.Members, c.ID); err != nil {
			return fmt.Errorf("set cluster for nodes: %w", err)
		}
	}
	fmt.Printf("Clustered %d communities (%s)\n", len(communities), time.Since(clusterStart).Round(time.Millisecond))

	// Trace processes
	processStart := time.Now()
	allNodesDB, _ = s.AllNodes()
	entryPoints := process.FindEntryPoints(allNodesDB, s)
	processCount := 0
	for _, entry := range entryPoints {
		proc := process.TraceProcess(entry, s, 10)
		processType := process.ClassifyProcess(proc, s)
		proc.ProcessType = processType

		if err := s.UpsertProcess(proc.ID, proc.Name, proc.EntryUID, proc.ProcessType); err != nil {
			return fmt.Errorf("upsert process: %w", err)
		}
		if err := s.UpsertProcessSteps(proc.ID, proc.Steps); err != nil {
			return fmt.Errorf("upsert process steps: %w", err)
		}
		processCount++
	}
	fmt.Printf("Traced %d processes (%s)\n", processCount, time.Since(processStart).Round(time.Millisecond))

	// Record commit SHA
	if headSHA != "" {
		s.SetMeta("last_commit", headSHA)
	}
	s.SetMeta("indexed_at", time.Now().UTC().Format(time.RFC3339))
	s.SetMeta("repo_path", repoPath)

	// Start async embed worker
	if !analyzeSkipEmbeds {
		w := embedder.NewWorker(s, emb)
		w.Start(ctx)
		for _, n := range allNodes {
			w.Enqueue(embedder.EmbedJob{UID: n.UID, Text: n.Name + " " + n.FilePath + " " + n.Kind})
		}
		fmt.Println("Indexing complete. Embeddings generating in background.")
		fmt.Println("Run `mimir status` to check embedding progress.")
	} else {
		fmt.Println("Indexing complete (embeddings skipped).")
	}

	// Phase E: Full Auto Flow

	// 4. Start MCP daemon (if not already running)
	if !analyzeSkipDaemon {
		bin, err := os.Executable()
		if err != nil {
			bin = "mimir"
		}
		if err := daemon.Start(bin); err != nil {
			fmt.Printf("⚠ Could not start MCP daemon: %v\n", err)
		} else {
			pid, _ := daemon.ReadPID()
			fmt.Printf("✓ MCP daemon running (PID %d)\n", pid)
		}
	}

	// 5. Setup all editors (write MCP config)
	editorResults := setup.SetupAll()
	configuredEditors := []string{}
	for _, r := range editorResults {
		switch r.Status {
		case "configured":
			fmt.Printf("✓ %s — MCP configured\n", r.Editor)
			configuredEditors = append(configuredEditors, r.Editor)
		case "not installed":
			// silent
		case "error":
			fmt.Printf("⚠ %s — %s\n", r.Editor, r.Error)
		}
	}

	// 6. Install Claude Code hooks
	if !analyzeSkipHooks {
		if err := setup.InstallClaudeCodeHooks(repoPath); err != nil {
			fmt.Printf("⚠ Hooks: %v\n", err)
		} else {
			fmt.Println("✓ Claude Code hooks installed")
		}
	}

	// 7. Install agent skills
	if !analyzeSkipSkills {
		dbClusters, _ := s.AllClusters()
		// Convert store.Community to setup.Community
		setupClusters := make([]setup.Community, len(dbClusters))
		for i, c := range dbClusters {
			setupClusters[i] = setup.Community{
				ID:            c.ID,
				Label:         c.Label,
				CohesionScore: c.CohesionScore,
				Members:       c.Members,
			}
		}
		setup.InstallStaticSkills(repoPath)
		setup.InstallDynamicSkills(repoPath, setupClusters)
		fmt.Printf("✓ Agent skills installed (%d modules)\n", len(dbClusters))
	}

	// 8. Generate AGENTS.md + CLAUDE.md
	wrapper := &storeWrapper{s}
	if err := setup.GenerateContextFiles(repoPath, wrapper); err != nil {
		fmt.Printf("⚠ Context files: %v\n", err)
	} else {
		fmt.Println("✓ AGENTS.md + CLAUDE.md written")
	}

	// 9. Print summary
	stats, _ := wrapper.Stats()
	fmt.Printf("\n✓ Done — %d symbols, %d edges, %d clusters, %d processes\n",
		stats.Nodes, stats.Edges, stats.Clusters, stats.Processes)
	fmt.Println("  MCP tools: query | context | impact | detect_changes | rename | cypher")

	// Show detected editors and MCP guidance
	if len(configuredEditors) > 0 {
		fmt.Printf("  Detected editors: %s\n", strings.Join(configuredEditors, ", "))
	}
	fmt.Println("  \n  Next steps:")
	fmt.Println("  1. Start MCP daemon: mimir daemon start")
	fmt.Println("  2. Restart your editor to load MCP server configuration")
	fmt.Println("  3. Use MCP tools (not CLI commands) in your agent conversations")

	return nil
}

func fullIndex(ctx context.Context, repoPath string, s *store.Store) ([]graph.FileSymbols, int, error) {
	concurrency := runtime.GOMAXPROCS(0)
	start := time.Now()

	fileCh := walker.WalkRepo(repoPath, concurrency)

	// Count files while forwarding to parser pool
	counted := make(chan walker.FileInfo, concurrency*16)
	fileCount := 0
	go func() {
		for f := range fileCh {
			counted <- f
			fileCount++
		}
		close(counted)
	}()

	pool := parser.NewParserPool(concurrency)
	symsCh := pool.Run(ctx, counted)

	var allSymbols []graph.FileSymbols
	for fs := range symsCh {
		allSymbols = append(allSymbols, fs)
	}

	fmt.Printf("Walked %s in %s\n", repoPath, time.Since(start).Round(time.Millisecond))
	return allSymbols, fileCount, nil
}

func isIncrementalCandidate(s *store.Store) bool {
	last, _ := s.GetMeta("last_commit")
	return last != ""
}

func clearRepo(s *store.Store, repoPath string) error {
	return s.Write(func(tx *sql.Tx) error {
		for _, q := range []string{
			"DELETE FROM process_steps",
			"DELETE FROM processes",
			"DELETE FROM cluster_members",
			"DELETE FROM clusters",
			"DELETE FROM bm25_index",
			"DELETE FROM node_embeddings",
			"DELETE FROM edges",
			"DELETE FROM nodes",
			"DELETE FROM index_meta WHERE key != 'repo_path'",
		} {
			if _, err := tx.Exec(q); err != nil {
				return err
			}
		}
		return nil
	})
}

func absPath(p string) (string, error) {
	return filepath.Abs(p)
}

func fmtCount(n int) string {
	if n >= 1000 {
		return fmt.Sprintf("%d,%03d", n/1000, n%1000)
	}
	return fmt.Sprintf("%d", n)
}

// storeWrapper adapts store.Store to setup.StoreInterface
type storeWrapper struct {
	*store.Store
}

func (w *storeWrapper) Stats() (setup.Stats, error) {
	nodes, err := w.AllNodes()
	if err != nil {
		return setup.Stats{}, err
	}
	edges, err := w.AllEdges()
	if err != nil {
		return setup.Stats{}, err
	}
	clusters, err := w.AllClusters()
	if err != nil {
		return setup.Stats{}, err
	}
	processes, err := w.AllProcesses()
	if err != nil {
		return setup.Stats{}, err
	}
	return setup.Stats{
		Nodes:     len(nodes),
		Edges:     len(edges),
		Clusters:  len(clusters),
		Processes: len(processes),
	}, nil
}

func (w *storeWrapper) AllClusters() ([]setup.Community, error) {
	dbClusters, err := w.Store.AllClusters()
	if err != nil {
		return nil, err
	}
	clusters := make([]setup.Community, len(dbClusters))
	for i, c := range dbClusters {
		clusters[i] = setup.Community{
			ID:            c.ID,
			Label:         c.Label,
			CohesionScore: c.CohesionScore,
			Members:       c.Members,
		}
	}
	return clusters, nil
}

func (w *storeWrapper) AllProcesses() ([]setup.Process, error) {
	dbProcesses, err := w.Store.AllProcesses()
	if err != nil {
		return nil, err
	}
	processes := make([]setup.Process, len(dbProcesses))
	for i, p := range dbProcesses {
		processes[i] = setup.Process{ID: p.ID, Name: p.Name}
	}
	return processes, nil
}
