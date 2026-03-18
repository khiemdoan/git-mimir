package main

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
	"github.com/thuongh2/git-mimir/internal/daemon"
	"github.com/thuongh2/git-mimir/internal/setup"
)

func runSetup(cmd *cobra.Command, args []string) error {
	fmt.Println("Mimir — global setup")
	fmt.Println()

	// Write MCP config to all detected editors
	results := setup.SetupAll()
	for _, r := range results {
		switch r.Status {
		case "configured":
			fmt.Printf("  ✓ %-14s — MCP configured (%s)\n", r.Editor, r.ConfigPath)
		case "not installed":
			fmt.Printf("  - %-14s — not installed\n", r.Editor)
		case "error":
			fmt.Printf("  ⚠ %-14s — %s\n", r.Editor, r.Error)
		}
	}

	// Start daemon
	bin, err := os.Executable()
	if err != nil {
		bin = "mimir"
	}
	if err := daemon.Start(bin); err != nil {
		fmt.Printf("\n⚠ Could not start MCP daemon: %v\n", err)
		fmt.Println("  Run 'mimir daemon start' manually after fixing the issue.")
	} else {
		pid, _ := daemon.ReadPID()
		fmt.Printf("\n✓ MCP daemon started (PID %d)\n", pid)
	}

	fmt.Println("\nDone. Run \"mimir analyze\" in any repo to start indexing.")
	return nil
}
