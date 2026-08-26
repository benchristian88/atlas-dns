package jobs

import (
	"context"
	"log/slog"
	"time"

	"github.com/benchristian88/atlas-dns/internal/operationalhealth"
)

type SessionStore interface {
	DeleteExpiredSessions(context.Context, time.Time) (int64, error)
	DeleteExpiredMFAChallenges(context.Context, time.Time) (int64, error)
}

func RunSessionCleanup(ctx context.Context, store SessionStore, logger *slog.Logger, trackers ...*operationalhealth.Tracker) {
	var tracker *operationalhealth.Tracker
	if len(trackers) > 0 {
		tracker = trackers[0]
	}
	cleanup := func() {
		if tracker != nil {
			tracker.Start("session_cleanup", time.Now().UTC().Add(time.Hour))
		}
		now := time.Now().UTC()
		deleted, err := store.DeleteExpiredSessions(ctx, now)
		if err != nil {
			logger.Error("expired session cleanup failed", "subsystem", "session_cleanup", "error", err, "retry_in", time.Hour)
			if tracker != nil {
				tracker.Failure("session_cleanup", "SESSION_CLEANUP_FAILED", time.Now().UTC().Add(time.Hour))
			}
			return
		}
		challenges, err := store.DeleteExpiredMFAChallenges(ctx, now)
		if err != nil {
			logger.Error("expired MFA challenge cleanup failed", "subsystem", "session_cleanup", "error", err, "retry_in", time.Hour)
			if tracker != nil {
				tracker.Failure("session_cleanup", "MFA_CHALLENGE_CLEANUP_FAILED", time.Now().UTC().Add(time.Hour))
			}
			return
		}
		if tracker != nil {
			tracker.Success("session_cleanup", time.Now().UTC().Add(time.Hour))
		}
		if deleted > 0 {
			logger.Info("expired sessions removed", "count", deleted)
		}
		if challenges > 0 {
			logger.Info("expired MFA challenges removed", "count", challenges)
		}
	}
	cleanup()
	ticker := time.NewTicker(time.Hour)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			cleanup()
		}
	}
}
