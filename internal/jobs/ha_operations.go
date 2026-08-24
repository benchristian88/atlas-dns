package jobs

import (
	"context"
	"log/slog"
	"time"

	"github.com/benchristian88/atlas-dns/internal/operationalhealth"
)

type HAOperationsPoller interface {
	PollAll(context.Context) error
}

func RunHAOperations(ctx context.Context, service HAOperationsPoller, interval time.Duration, logger *slog.Logger, tracker *operationalhealth.Tracker, providers ...RuntimeSettingsProvider) {
	if interval <= 0 {
		interval = 30 * time.Second
	}
	currentInterval := func() time.Duration {
		if len(providers) > 0 && providers[0] != nil {
			return providers[0].RuntimeSettings().NodeHealthInterval
		}
		return interval
	}
	run := func() {
		interval = currentInterval()
		next := time.Now().UTC().Add(interval)
		if tracker != nil {
			tracker.Start("dns_service_health", next)
		}
		if err := service.PollAll(ctx); err != nil {
			logger.Error("DNS service health pass failed", "subsystem", "dns_service_health", "error_code", "DNS_HEALTH_PASS_FAILED")
			if tracker != nil {
				tracker.Failure("dns_service_health", "DNS_HEALTH_PASS_FAILED", next)
			}
			return
		}
		if tracker != nil {
			tracker.Success("dns_service_health", next)
		}
	}
	run()
	for {
		timer := time.NewTimer(currentInterval())
		select {
		case <-ctx.Done():
			timer.Stop()
			return
		case <-timer.C:
			run()
		}
	}
}
