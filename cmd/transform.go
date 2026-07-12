package cmd

import (
	"fmt"
	"os"
	"strings"

	"codeberg.org/cuducos/minha-receita/db"
	"codeberg.org/cuducos/minha-receita/graph"
	"codeberg.org/cuducos/minha-receita/transform"
	"github.com/spf13/cobra"
)

const transformHelper = `
Convert the CSV files from the Federal Revenue for venues (Estabelecimentos*.zip
group of files) into records in the database, 1 record per CNPJ, joining
information from all other source CSV files.

The transformation process is divided into three steps:
1. Unarchive the bundled ZIP file for a given month/year
2. Load relational data to a key-value store
3. Load the full database using the key-value store
`

var (
	batchSize int
	cleanUp   bool
	noPrivacy bool
	skipGraph bool
	graphOnly bool
)

var transformCmd = &cobra.Command{
	Use:   "transform",
	Short: "Transforms the CSV files into database records",
	Long:  transformHelper,
	RunE: func(_ *cobra.Command, _ []string) error {
		if skipGraph && graphOnly {
			return fmt.Errorf("cannot use both --skip-graph and --graph-only")
		}
		if graphOnly {
			g, err := graph.NewWriter(graphPath)
			if err != nil {
				return err
			}
			return transform.Transform(dir, nil, g, batchSize, !noPrivacy)
		}
		args.SetURI(uri)
		db, err := loadDatabase(&args)
		if err != nil {
			return fmt.Errorf("could not find database: %w", err)
		}
		defer db.Close()
		if cleanUp {
			if err := db.Drop(); err != nil {
				return err
			}
			if err := db.Create(); err != nil {
				return err
			}
		}
		if skipGraph {
			return transform.Transform(dir, db, nil, batchSize, !noPrivacy)
		}
		g, err := graph.NewWriter(graphPath)
		if err != nil {
			return err
		}
		return transform.Transform(dir, db, g, batchSize, !noPrivacy)
	},
}

var cleanupTempCmd = &cobra.Command{
	Use:   "clean-up",
	Short: "Clean-up temporary ETL files",
	RunE: func(_ *cobra.Command, _ []string) error {
		return transform.Cleanup()
	},
}

func transformCLI() *cobra.Command {
	transformCmd.Flags().IntVarP(
		&args.MaxConns,
		"max-db-connections",
		"m",
		args.MaxConns,
		"maximum parallel database connections",
	)

	batchSize = min(db.MongoDBBatchSize, db.SQLBatchSize)
	if !strings.HasPrefix(args.URI, "mongodb://") {
		batchSize = db.SQLBatchSize
	}

	transformCmd.Flags().BoolVarP(&cleanUp, "clean-up", "c", cleanUp, "drop & recreate the database table before starting")
	pth := graph.DefaultGraphPath
	if env := os.Getenv("GRAPH_PATH"); env != "" {
		pth = env
	}
	transformCmd.Flags().StringVarP(&graphPath, "graph", "g", pth, "path for the graph data (directory or .tar.gz archive)")
	transformCmd.Flags().BoolVar(&skipGraph, "skip-graph", skipGraph, "skip creating the graph database")
	transformCmd.Flags().BoolVar(&graphOnly, "graph-only", graphOnly, "only create the graph database")
	transformCmd.Flags().IntVarP(&batchSize, "batch-size", "b", batchSize, "size of the batch to save to the database")
	transformCmd.Flags().BoolVarP(&noPrivacy, "no-privacy", "p", noPrivacy, "include email addresses, CPF and other PII in the JSON data")
	transformCmd.Flags().BoolVarP(&args.PostgresLogged, "logged", "l", args.PostgresLogged, "avoids the disk overhead but writes slowly to the table (PostgreSQL only)")
	return transformCmd
}
