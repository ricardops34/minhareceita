package cmd

import (
	"fmt"
	"os"
	"strconv"

	"codeberg.org/cuducos/minha-receita/graph"
	"github.com/spf13/cobra"
)

var (
	graphPath      string
	graphCacheSize int
)

var graphCmd = &cobra.Command{
	Use:   "graph",
	Short: "Spins up the graph API",
	RunE: func(cmd *cobra.Command, _ []string) error {
		if !cmd.Flags().Changed("graph") {
			if env := os.Getenv("GRAPH_PATH"); env != "" {
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
	pth := graph.DefaultGraphPath
	if env := os.Getenv("GRAPH_PATH"); env != "" {
		pth = env
	}
	graphCmd.Flags().StringVarP(
		&graphPath,
		"graph",
		"g",
		pth,
		"path for the graph data (directory or .tar.gz archive)",
	)
	graphCmd.Flags().StringVarP(
		&port,
		"port",
		"p",
		"8000",
		"HTTP server port",
	)
	graphCmd.Flags().IntVarP(
		&graphCacheSize,
		"cache",
		"c",
		graph.DefaultCacheSize,
		"max size in MB for the graph adjacency cache, use 0 to disable",
	)
	return graphCmd
}
