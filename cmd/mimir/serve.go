package main

import (
	"github.com/spf13/cobra"
	"github.com/thuongh2/git-mimir/api"
	"github.com/thuongh2/git-mimir/internal/registry"
)

var servePort int

func init() {
	serveCmd.Flags().IntVarP(&servePort, "port", "p", 7842, "HTTP server port")
}

func runServe(cmd *cobra.Command, args []string) error {
	reg, err := registry.Load()
	if err != nil {
		return err
	}
	srv := api.NewServer(reg, servePort)
	return srv.ListenAndServe()
}
