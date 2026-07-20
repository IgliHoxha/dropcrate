package cmd

import (
	"io/fs"

	"github.com/spf13/cobra"

	"github.com/IgliHoxha/dropcrate/internal/config"
	"github.com/IgliHoxha/dropcrate/internal/database"
)

var migrateCmd = &cobra.Command{
	Use:   "migrate",
	Short: "Apply pending database migrations",
	RunE: func(cmd *cobra.Command, _ []string) error {
		ctx := cmd.Context()

		cfg, err := config.Load()
		if err != nil {
			return err
		}

		db, err := database.Open(ctx, cfg.MySQLDSN)
		if err != nil {
			return err
		}
		defer db.Close()

		// The embedded FS is rooted at the module; scope it to migrations/.
		sub, err := fs.Sub(MigrationsFS, "migrations")
		if err != nil {
			return err
		}

		if err := database.Migrate(ctx, db, sub); err != nil {
			return err
		}

		cmd.Println("migrations applied")
		return nil
	},
}
