package jobs

import (
	"context"
	"log/slog"
	"time"

	"github.com/benchristian88/atlas-dns/internal/operationalhealth"
)

type OperationalHistoryStore interface {
	PruneOperationalHistory(context.Context, time.Time, int) (int64, error)
}

func RunOperationalHistoryRetention(ctx context.Context, store OperationalHistoryStore, settings RuntimeSettingsProvider, logger *slog.Logger, tracker *operationalhealth.Tracker) {
	const interval = time.Hour
	prune := func() {
		now := time.Now().UTC()
		if tracker != nil {
			tracker.Start("operational_history_retention", now.Add(interval))
		}
		retention := settings.RuntimeSettings().OperationalHistoryRetention
		deleted, err := store.PruneOperationalHistory(ctx, now.Add(-retention), 10000)
		if err != nil {
			logger.Error("Operational History retention failed", "subsystem", "operational_history_retention", "error", err)
			if tracker != nil {
				tracker.Failure("operational_history_retention", "OPERATIONAL_HISTORY_RETENTION_FAILED", now.Add(interval))
			}
			return
		}
		if tracker != nil {
			tracker.Success("operational_history_retention", now.Add(interval))
		}
		if deleted > 0 {
			logger.Info("Operational History retention pruned events", "count", deleted)
		}
	}
	prune()
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			prune()
		}
	}
}
