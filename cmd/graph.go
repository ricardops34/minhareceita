package cmd

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"strconv"

	"codeberg.org/cuducos/minha-receita/graph"
	"github.com/schollz/progressbar/v3"
	"github.com/spf13/cobra"
)

var (
	graphPath      string
	graphCacheSize int
)

var graphCmd = &cobra.Command{
	Use:   "graph",
	Short: "Manage the graph API",
}

var graphCreateCmd = &cobra.Command{
	Use:   "create",
	Short: "Creates the graph index from the database",
	RunE: func(_ *cobra.Command, _ []string) error {
		args.SetURI(uri)
		db, err := loadDatabase(&args)
		if err != nil {
			return fmt.Errorf("could not find database: %w", err)
		}
		defer db.Close()

		ctx := context.Background()

		slog.Info("Querying total relationship count from the main database")
		t, err := db.RelationshipCount(ctx)
		if err != nil {
			return fmt.Errorf("could not get relationship count: %w", err)
		}
		slog.Info("Found relationships to process", "count", t)

		bar := progressbar.NewOptions(
			int(t),
			progressbar.OptionSetDescription("Streaming & building relationships index"),
			progressbar.OptionShowCount(),
			progressbar.OptionShowElapsedTimeOnFinish(),
			progressbar.OptionFullWidth(),
			progressbar.OptionUseANSICodes(true),
			progressbar.OptionOnCompletion(func() { fmt.Println() }),
		)

		if err := graph.Create(ctx, db, t, graphPath, bar); err != nil {
			return fmt.Errorf("error creating graph index: %w", err)
		}

		slog.Info("Relationships index successfully created")
		return nil
	},
}

var graphApiCmd = &cobra.Command{
	Use:   "api",
	Short: "Spins up the graph API",
	RunE: func(cmd *cobra.Command, _ []string) error {
		if graphPath == "graph.db" {
			if env := os.Getenv("GRAPH_DIR"); env != "" {
				graphPath = env
			} else if env := os.Getenv("GRAPH_FILE"); env != "" {
				graphPath = env
			} else if env := os.Getenv("KEYS_DIR"); env != "" {
				graphPath = env
			}
		}
		if !cmd.Flags().Changed("port") {
			if env := os.Getenv("PORT"); env != "" {
				port = env
			}
		}
		if !cmd.Flags().Changed("cache") {
			if v := os.Getenv("GRAPH_CACHE_SIZE"); v != "" {
				n, err := strconv.Atoi(v)
				if err != nil {
					return fmt.Errorf("invalid GRAPH_CACHE_SIZE: %w", err)
				}
				graphCacheSize = n
			}
		}

		srv, err := graph.NewServer(graphPath, graphCacheSize)
		if err != nil {
			return fmt.Errorf("failed to initialize server: %w", err)
		}
		defer srv.Close()

		return graph.Serve(srv, port)
	},
}

func graphCLI() *cobra.Command {
	graphCreateCmd.Flags().StringVarP(
		&graphPath,
		"graph",
		"g",
		"graph.db",
		"directory path for the Badger key/value storage",
	)
	addDatabase(graphCreateCmd, &args)

	graphApiCmd.Flags().StringVarP(
		&graphPath,
		"graph",
		"g",
		"graph.db",
		"directory path for the Badger key/value storage",
	)
	graphApiCmd.Flags().StringVarP(
		&port,
		"port",
		"p",
		"8000",
		"HTTP server port",
	)
	graphApiCmd.Flags().IntVarP(
		&graphCacheSize,
		"cache",
		"c",
		graph.DefaultCacheSize,
		"max size in MB for the graph adjacency cache, use 0 to disable",
	)

	graphCmd.AddCommand(graphCreateCmd)
	graphCmd.AddCommand(graphApiCmd)
	return graphCmd
}
