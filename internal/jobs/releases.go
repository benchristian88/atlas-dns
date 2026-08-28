package jobs

import (
	"context"
	"log/slog"
	"time"

	"github.com/benchristian88/atlas-dns/internal/operationalhealth"
)

type ReleaseRefresher interface{ Refresh(context.Context) error }

const releaseCheckInterval = 5 * time.Minute

type releaseCheckTicker interface {
	Ticks() <-chan time.Time
	Stop()
}

type systemReleaseCheckTicker struct{ ticker *time.Ticker }

func (t systemReleaseCheckTicker) Ticks() <-chan time.Time { return t.ticker.C }
func (t systemReleaseCheckTicker) Stop()                   { t.ticker.Stop() }

func RunReleaseChecks(ctx context.Context, checker ReleaseRefresher, logger *slog.Logger, tracker *operationalhealth.Tracker) {
	runReleaseChecks(ctx, checker, logger, tracker, time.Now, func(interval time.Duration) releaseCheckTicker {
		return systemReleaseCheckTicker{ticker: time.NewTicker(interval)}
	})
}

func runReleaseChecks(ctx context.Context, checker ReleaseRefresher, logger *slog.Logger, tracker *operationalhealth.Tracker, now func() time.Time, newTicker func(time.Duration) releaseCheckTicker) {
	run := func() {
		next := now().UTC().Add(releaseCheckInterval)
		if tracker != nil {
			tracker.Start("adguard_release_check", next)
		}
		if err := checker.Refresh(ctx); err != nil {
			logger.Warn("AdGuard Home release check failed", "subsystem", "adguard_release_check", "error_code", "RELEASE_CHECK_UNAVAILABLE")
			if tracker != nil {
				tracker.Failure("adguard_release_check", "RELEASE_CHECK_UNAVAILABLE", next)
			}
			return
		}
		if tracker != nil {
			tracker.Success("adguard_release_check", next)
		}
	}
	ticker := newTicker(releaseCheckInterval)
	defer ticker.Stop()
	run()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.Ticks():
			run()
		}
	}
}
