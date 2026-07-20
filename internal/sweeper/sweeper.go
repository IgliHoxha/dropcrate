// Package sweeper periodically reclaims files whose expiry has passed.
package sweeper

import (
	"context"
	"log/slog"
	"time"
)

// Purger deletes a bounded batch of expired files and reports how many it
// removed. *service.Service satisfies this.
type Purger interface {
	PurgeExpired(ctx context.Context, batch int) (int, error)
}

// Sweeper drains expired files on a fixed interval.
type Sweeper struct {
	purger   Purger
	interval time.Duration
	batch    int
	log      *slog.Logger
}

// New builds a Sweeper. A non-positive interval disables sweeping.
func New(p Purger, interval time.Duration, batch int, log *slog.Logger) *Sweeper {
	return &Sweeper{purger: p, interval: interval, batch: batch, log: log}
}

// Run blocks, sweeping every interval until ctx is cancelled. It is intended to
// be launched in its own goroutine.
func (s *Sweeper) Run(ctx context.Context) {
	if s.interval <= 0 {
		s.log.Info("sweeper disabled")
		return
	}

	ticker := time.NewTicker(s.interval)
	defer ticker.Stop()

	s.log.Info("sweeper started", "interval", s.interval.String(), "batch", s.batch)

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			s.sweep(ctx)
		}
	}
}

// sweep repeatedly purges batches until one comes back short, meaning no
// expired files remain.
func (s *Sweeper) sweep(ctx context.Context) {
	total := 0
	for {
		n, err := s.purger.PurgeExpired(ctx, s.batch)
		total += n
		if err != nil {
			s.log.Error("sweep failed", "error", err, "purged_before_error", total)
			return
		}
		if n < s.batch {
			break
		}
	}
	if total > 0 {
		s.log.Info("swept expired files", "count", total)
	}
}
