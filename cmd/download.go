package cmd

import (
	"codeberg.org/cuducos/minha-receita/download"
	"github.com/spf13/cobra"
)

const downloadHelper = `
Downloads the source files for a given month from the Federal Revenue via
WebDAV, plus the tax regime files and the IBGE municipalities CSV from the
National Treasure.

Usage:
  minha-receita download YYYY-MM

The CNPJ files are bundled into a single YYYY-MM.zip in the target directory.`

var downloadCmd = &cobra.Command{
	Use:   "download YYYY-MM",
	Short: "Downloads the source files for a given month",
	Long:  downloadHelper,
	Args:  cobra.ExactArgs(1),
	RunE: func(_ *cobra.Command, args []string) error {
		return download.Download(dir, args[0])
	},
}
