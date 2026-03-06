package cmd

import (
	"github.com/dcm-io/dcm/pkg/api"
	"github.com/dcm-io/dcm/pkg/store"
	"github.com/spf13/cobra"
)

var (
	serveAddr string
	serveDB   string
)

var serveCmd = &cobra.Command{
	Use:   "serve",
	Short: "Start the DCM API server",
	RunE:  runServe,
}

func init() {
	serveCmd.Flags().StringVar(&serveAddr, "addr", ":8080", "Address to listen on")
	serveCmd.Flags().StringVar(&serveDB, "db", "dcm.db", "Path to the SQLite database file")
	rootCmd.AddCommand(serveCmd)
}

func runServe(cmd *cobra.Command, args []string) error {
	db, err := store.New(serveDB)
	if err != nil {
		return err
	}
	defer db.Close()

	registry := buildRegistry()
	server := api.NewServer(db, registry)

	return server.Start(serveAddr)
}
