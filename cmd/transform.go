package cmd

import (
	"fmt"
	"os"
	"strings"

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
	maxParallelDBQueries int
	batchSize            int
	cleanUp              bool
	noPrivacy            bool
)

var transformCmd = &cobra.Command{
	Use:   "transform",
	Short: "Transforms the CSV files into database records",
	Long:  transformHelper,
	RunE: func(_ *cobra.Command, _ []string) error {
		db, err := loadDatabase()
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
		return transform.Transform(dir, db, batchSize, maxParallelDBQueries, !noPrivacy)
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
		&maxParallelDBQueries,
		"max-parallel-db-queries",
		"m",
		transform.MaxParallelDBQueries,
		"maximum parallel database queries",
	)

	u := os.Getenv("DATABASE_URL")
	batchSize = min(transform.MongoDBBatchSize, transform.PostgresBatchSize) // start with the lower one
	if strings.HasPrefix(u, "postgres://") || strings.HasPrefix(u, "postgresql://") {
		batchSize = transform.PostgresBatchSize
	}

	transformCmd.Flags().BoolVarP(&cleanUp, "clean-up", "c", cleanUp, "drop & recreate the database table before starting")
	transformCmd.Flags().IntVarP(&batchSize, "batch-size", "b", batchSize, "size of the batch to save to the database")
	transformCmd.Flags().BoolVarP(&noPrivacy, "no-privacy", "p", noPrivacy, "include email addresses, CPF and other PII in the JSON data")
	return transformCmd
}
