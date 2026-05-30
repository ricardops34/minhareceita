package cmd

import (
	"fmt"
	"os"

	"codeberg.org/cuducos/minha-receita/api"
	"github.com/spf13/cobra"
)

var skipCreate bool

var graphCmd = &cobra.Command{
	Use:   "graph",
	Short: "Creates the graph table and spins up the graph web API",
	RunE: func(_ *cobra.Command, _ []string) error {
		if port == "" {
			port = os.Getenv("PORT")
		}
		if port == "" {
			port = defaultPort
		}
		db, err := loadDatabase()
		if err != nil {
			return fmt.Errorf("could not find database: %w", err)
		}
		defer db.Close()

		if !skipCreate {
			if err := db.CreateGraphTable(); err != nil {
				return fmt.Errorf("could not create graph table: %w", err)
			}
		}

		return api.ServeGraph(db, port)
	},
}

func graphCLI() *cobra.Command {
	graphCmd.Flags().StringVarP(
		&port,
		"port",
		"p",
		"",
		fmt.Sprintf("web server port (default PORT environment variable or %s)", defaultPort),
	)
	graphCmd.Flags().BoolVar(&skipCreate, "skip-create", false, "skip creating the graph table/collection")
	addDatabase(graphCmd)
	return graphCmd
}
