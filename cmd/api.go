package cmd

import (
	"fmt"
	"os"
	"strconv"

	"codeberg.org/cuducos/minha-receita/api"
	"github.com/spf13/cobra"
)

const (
	defaultPort      = "8000"
	defaultCacheSize = 1 << 5 // 32MB
	defaultBloomSize = 1 << 6 // 64MB
	apiHelper        = `
Starts the web API.

Using GODEBUG environment variable changes the HTTP server verbosity (for
example: http2debug=1 is verbose and http2debug=2 is more verbose, as in
https://golang.org/pkg/net/http/

The HTTP server is prepared to do a host header validation against the value of
ALLOWED_HOST environment variable. If this variable is not set, this validation
is skipped.`
)

var (
	port      string
	cacheSize int
	bloomSize int
)

var apiCmd = &cobra.Command{
	Use:   "api",
	Short: "Spins up the web API",
	Long:  apiHelper,
	RunE: func(cmd *cobra.Command, _ []string) error {
		var err error
		if port == "" {
			port = os.Getenv("PORT")
		}
		if port == "" {
			port = defaultPort
		}
		if !cmd.Flags().Changed("cache") {
			if v := os.Getenv("CACHE_SIZE"); v != "" {
				n, err := strconv.Atoi(v)
				if err != nil {
					return fmt.Errorf("invalid CACHE_SIZE: %w", err)
				}
				cacheSize = n
			}
		}
		if !cmd.Flags().Changed("bloom-filter") {
			if v := os.Getenv("BLOOM_FILTER_SIZE"); v != "" {
				n, err := strconv.Atoi(v)
				if err != nil {
					return fmt.Errorf("invalid BLOOM_FILTER_SIZE: %w", err)
				}
				bloomSize = n
			}
		}
		db, err := loadDatabase()
		if err != nil {
			return fmt.Errorf("could not find database: %w", err)
		}
		defer db.Close()
		return api.Serve(db, port, cacheSize, bloomSize)
	},
}

func apiCLI() *cobra.Command {
	apiCmd.Flags().StringVarP(
		&port,
		"port",
		"p",
		"",
		fmt.Sprintf("web server port (default PORT environment variable or %s)", defaultPort),
	)
	apiCmd.Flags().IntVarP(
		&cacheSize,
		"cache",
		"c",
		defaultCacheSize,
		fmt.Sprintf("max size in MB for the cache (default CACHE_SIZE environment variable or %d MB)", defaultCacheSize),
	)
	apiCmd.Flags().IntVarP(
		&bloomSize,
		"bloom-filter",
		"b",
		defaultBloomSize,
		fmt.Sprintf("max size in MB for the bloom filter (default BLOOM_FILTER_SIZE environment variable or %d MB)", defaultBloomSize),
	)
	return apiCmd
}
