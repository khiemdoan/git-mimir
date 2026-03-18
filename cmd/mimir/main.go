package main

import (
	"fmt"
	"os"
	"strconv"
	"time"

	"github.com/spf13/cobra"
	"github.com/thuongh2/git-mimir/internal/daemon"
	"github.com/thuongh2/git-mimir/internal/registry"
	"github.com/thuongh2/git-mimir/mcp"
)

var version = "dev"

var rootCmd = &cobra.Command{
	Use:     "mimir",
	Short:   "Mimir — the well of code intelligence",
	Long:    "Mimir indexes code repositories into a knowledge graph and exposes it via MCP tools.",
	Version: version,
}

var analyzeCmd = &cobra.Command{
	Use:   "analyze <path>",
	Short: "Index a repository into Mimir's knowledge graph",
	Args:  cobra.ExactArgs(1),
}

var mcpCmd = &cobra.Command{
	Use:   "mcp",
	Short: "Start the MCP stdio server",
	RunE: func(cmd *cobra.Command, args []string) error {
		reg, err := registry.Load()
		if err != nil {
			return fmt.Errorf("load registry: %w", err)
		}
		return mcp.Serve(reg)
	},
}

var serveCmd = &cobra.Command{
	Use:   "serve",
	Short: "Start the HTTP bridge server",
	RunE:  runServe,
}

var listCmd = &cobra.Command{
	Use:   "list",
	Short: "List all indexed repositories",
	RunE: func(cmd *cobra.Command, args []string) error {
		reg, err := registry.Load()
		if err != nil {
			return err
		}
		repos := reg.List()
		if len(repos) == 0 {
			fmt.Println("No repositories indexed. Run `mimir analyze <path>` to get started.")
			return nil
		}
		fmt.Printf("%-20s  %-50s  %s\n", "NAME", "PATH", "INDEXED AT")
		for _, r := range repos {
			fmt.Printf("%-20s  %-50s  %s\n", r.Name, r.Path, r.IndexedAt)
		}
		return nil
	},
}

var statusCmd = &cobra.Command{
	Use:   "status [name]",
	Short: "Show index status for the current or specified repository",
	RunE:  runStatus,
}

var cleanCmd = &cobra.Command{
	Use:   "clean <name>",
	Short: "Remove the index for a repository",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		reg, err := registry.Load()
		if err != nil {
			return err
		}
		name := args[0]
		dbPath, err := registry.DBPath(name)
		if err != nil {
			return err
		}
		if err := os.Remove(dbPath); err != nil && !os.IsNotExist(err) {
			return fmt.Errorf("remove db: %w", err)
		}
		if err := reg.Unregister(name); err != nil {
			fmt.Fprintf(os.Stderr, "warning: %v\n", err)
		}
		fmt.Printf("Cleaned index for %q\n", name)
		return nil
	},
}

var setupCmd = &cobra.Command{
	Use:   "setup",
	Short: "Configure MCP settings in supported editors",
	RunE:  runSetup,
}

var (
	daemonStartCmd = &cobra.Command{
		Use:   "start",
		Short: "Start the MCP daemon",
		RunE:  runDaemonStart,
	}
	daemonStopCmd = &cobra.Command{
		Use:   "stop",
		Short: "Stop the MCP daemon",
		RunE:  runDaemonStop,
	}
	daemonStatusCmd = &cobra.Command{
		Use:   "status",
		Short: "Show daemon status",
		RunE:  runDaemonStatus,
	}
	daemonLogsCmd = &cobra.Command{
		Use:   "logs [lines]",
		Short: "View MCP daemon logs",
		RunE:  runDaemonLogs,
	}
	daemonRestartCmd = &cobra.Command{
		Use:   "restart",
		Short: "Restart the MCP daemon",
		RunE:  runDaemonRestart,
	}
	daemonCmd = &cobra.Command{
		Use:   "daemon",
		Short: "Manage the MCP daemon",
	}
)

var wikiCmd = &cobra.Command{
	Use:   "wiki [name]",
	Short: "Generate a wiki from the knowledge graph",
	RunE:  runWiki,
}

func main() {
	// Wire up daemon subcommands
	daemonCmd.AddCommand(daemonStartCmd, daemonStopCmd, daemonStatusCmd, daemonLogsCmd, daemonRestartCmd)

	rootCmd.AddCommand(analyzeCmd, mcpCmd, serveCmd, listCmd, statusCmd, cleanCmd, setupCmd, daemonCmd, wikiCmd)

	if err := rootCmd.Execute(); err != nil {
		os.Exit(1)
	}
}

// Daemon command handlers

func runDaemonStart(cmd *cobra.Command, args []string) error {
	bin, err := os.Executable()
	if err != nil {
		return fmt.Errorf("get executable path: %w", err)
	}

	if err := daemon.Start(bin); err != nil {
		return fmt.Errorf("start daemon: %w", err)
	}

	pid, _ := daemon.ReadPID()
	fmt.Printf("MCP daemon started (PID %d)\n", pid)
	fmt.Printf("Log file: %s\n", daemon.LogPath())
	return nil
}

func runDaemonStop(cmd *cobra.Command, args []string) error {
	if !daemon.IsRunning() {
		fmt.Println("MCP daemon is not running")
		return nil
	}

	pid, _ := daemon.ReadPID()
	if err := daemon.Stop(); err != nil {
		return fmt.Errorf("stop daemon: %w", err)
	}

	fmt.Printf("MCP daemon stopped (was PID %d)\n", pid)
	return nil
}

func runDaemonStatus(cmd *cobra.Command, args []string) error {
	if !daemon.IsRunning() {
		fmt.Println("MCP daemon: not running")
		return nil
	}

	pid, err := daemon.ReadPID()
	if err != nil {
		fmt.Println("MCP daemon: running (PID file unreadable)")
		return nil
	}

	uptime, err := daemon.GetUptime()
	if err != nil {
		fmt.Printf("MCP daemon: running (PID %d, uptime unknown)\n", pid)
	} else {
		fmt.Printf("MCP daemon: running (PID %d, uptime %v)\n", pid, uptime.Round(time.Second))
	}

	// Show last few log lines
	lines, err := daemon.LogTail(5)
	if err != nil {
		fmt.Printf("Log file: %s (unreadable)\n", daemon.LogPath())
	} else if len(lines) > 0 {
		fmt.Printf("Log file: %s\n", daemon.LogPath())
		fmt.Println("Recent logs:")
		for _, line := range lines {
			fmt.Printf("  %s\n", line)
		}
	}

	return nil
}

func runDaemonRestart(cmd *cobra.Command, args []string) error {
	bin, err := os.Executable()
	if err != nil {
		return fmt.Errorf("get executable path: %w", err)
	}

	if err := daemon.Stop(); err != nil {
		// Ignore error - daemon might not be running
		fmt.Println("Stopping daemon...")
	}

	// Brief pause to ensure port is freed
	time.Sleep(500 * time.Millisecond)

	if err := daemon.Start(bin); err != nil {
		return fmt.Errorf("start daemon: %w", err)
	}

	pid, _ := daemon.ReadPID()
	fmt.Printf("MCP daemon restarted (PID %d)\n", pid)
	return nil
}

func runDaemonLogs(cmd *cobra.Command, args []string) error {
	lines := 30 // default
	if len(args) > 0 {
		n, err := strconv.Atoi(args[0])
		if err != nil {
			return fmt.Errorf("invalid lines argument: %w", err)
		}
		lines = n
	}

	logLines, err := daemon.LogTail(lines)
	if err != nil {
		return fmt.Errorf("read log: %w", err)
	}

	if len(logLines) == 0 {
		fmt.Println("No log entries found")
		return nil
	}

	fmt.Printf("=== Last %d lines of MCP log ===\n", len(logLines))
	for _, line := range logLines {
		fmt.Println(line)
	}
	return nil
}
