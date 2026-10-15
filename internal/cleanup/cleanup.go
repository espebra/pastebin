package cleanup

import (
	"context"
	"log/slog"
	"time"

	"github.com/espebra/pastebin/internal/metrics"
	"github.com/espebra/pastebin/internal/paste"
	"github.com/espebra/pastebin/internal/storage"
)

// Cleaner handles periodic cleanup of expired pastes
type Cleaner struct {
	storage  *storage.S3Storage
	interval time.Duration
}

// New creates a new Cleaner
func New(storage *storage.S3Storage, interval time.Duration) *Cleaner {
	return &Cleaner{
		storage:  storage,
		interval: interval,
	}
}

// Start begins the cleanup loop. It blocks until the context is cancelled.
func (c *Cleaner) Start(ctx context.Context) {
	slog.Info("starting cleanup routine", "interval", c.interval.String())

	ticker := time.NewTicker(c.interval)
	defer ticker.Stop()

	// Run immediately on start
	c.cleanup(ctx)

	for {
		select {
		case <-ctx.Done():
			slog.Info("cleanup routine stopped")
			return
		case <-ticker.C:
			c.cleanup(ctx)
		}
	}
}

func (c *Cleaner) cleanup(ctx context.Context) {
	slog.Debug("running cleanup")
	start := time.Now()

	var deleted, available int
	err := c.storage.ForEachMeta(ctx, func(meta *paste.Meta) error {
		if meta.IsExpired() {
			if err := c.storage.Delete(ctx, meta.Checksum); err != nil {
				slog.Error("failed to delete expired paste", "checksum", meta.Checksum, "error", err)
				return nil // Continue to next item
			}
			deleted++
			metrics.PastesDeleted.WithLabelValues("expired").Inc()
			slog.Info("deleted expired paste", "checksum", meta.Checksum)
			return nil
		}
		available++
		return nil
	})

	metrics.CleanupDuration.Observe(time.Since(start).Seconds())
	if err != nil {
		metrics.CleanupRuns.WithLabelValues("error").Inc()
		slog.Error("failed during cleanup iteration", "error", err)
	} else {
		metrics.CleanupRuns.WithLabelValues("success").Inc()
		metrics.CleanupLastSuccess.SetToCurrentTime()
		// Only a complete traversal gives a full count
		metrics.PastesAvailable.Set(float64(available))
	}

	slog.Info("cleanup complete", "deleted", deleted, "available", available)
}
