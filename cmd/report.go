package cmd

import (
	"os"

	"codeberg.org/cuducos/minha-receita/report"
	"github.com/spf13/cobra"
)

const reportHelper = `Generates a formal FOI report from the ETL log
file, listing data inconsistencies.`

var reportCmd = &cobra.Command{
	Use:   "report <log-file>",
	Short: "Generates a FOI report from the ETL log file",
	Long:  reportHelper,
	Args:  cobra.ExactArgs(1),
	RunE: func(_ *cobra.Command, args []string) error {
		return report.Report(args[0], os.Stdout)
	},
}
