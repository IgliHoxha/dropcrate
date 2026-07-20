package cmd

import (
	"github.com/spf13/cobra"
)

var sweepCmd = &cobra.Command{
	Use:   "sweep",
	Short: "Reclaim expired files once and exit",
	Long: `sweep deletes every file whose expiry has passed — bytes, cache entry, and
metadata — then exits. Useful as a cron job when you would rather not run the
in-process background reaper.`,
	RunE: func(cmd *cobra.Command, _ []string) error {
		ctx := cmd.Context()

		d, cleanup, err := buildDeps(ctx)
		if err != nil {
			return err
		}
		defer cleanup()

		total := 0
		for {
			n, err := d.svc.PurgeExpired(ctx, d.cfg.SweepBatch)
			total += n
			if err != nil {
				return err
			}
			if n < d.cfg.SweepBatch {
				break
			}
		}

		cmd.Printf("swept %d expired file(s)\n", total)
		return nil
	},
}
