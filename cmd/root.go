package cmd

import (
	"embed"
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

// MigrationsFS is wired up from main so the migrate command can read the
// embedded SQL files without importing the top-level package.
var MigrationsFS embed.FS

var rootCmd = &cobra.Command{
	Use:   "dropcrate",
	Short: "dropcrate is an expiring file-sharing and object-storage API",
	Long: `dropcrate stores uploaded files in an S3-compatible bucket, tracks their
metadata in MySQL, caches hot lookups in Redis, and serves them back over
HTTP through download links that can be configured to expire.`,
}

// Execute runs the root command and exits non-zero on failure.
func Execute() {
	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func init() {
	rootCmd.AddCommand(serveCmd)
	rootCmd.AddCommand(migrateCmd)
	rootCmd.AddCommand(sweepCmd)
}
