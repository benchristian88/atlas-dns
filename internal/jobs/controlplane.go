package jobs

import (
	"context"
	"log/slog"
	"time"

	"github.com/benchristian88/atlas-dns/internal/operationalhealth"
)

type DeploymentExecutor interface {
	RunOnce(context.Context) (bool, error)
}

type DriftReconciler interface {
	RunOnce(context.Context) error
}

func RunDeploymentExecutor(ctx context.Context, executor DeploymentExecutor, logger *slog.Logger, trackers ...*operationalhealth.Tracker) {
	var tracker *operationalhealth.Tracker
	if len(trackers) > 0 {
		tracker = trackers[0]
	}
	failures := 0
	for {
		if tracker != nil {
			tracker.Start("deployment", time.Now().UTC().Add(time.Second))
		}
		worked, err := executor.RunOnce(ctx)
		if err != nil && ctx.Err() == nil {
			failures++
			delay := workerBackoff(failures)
			logger.Error("deployment execution failed", "subsystem", "deployment", "error", err, "consecutive_failures", failures, "retry_in", delay)
			if tracker != nil {
				tracker.Failure("deployment", "DEPLOYMENT_WORKER_FAILED", time.Now().UTC().Add(delay))
			}
		} else {
			if failures > 0 {
				logger.Info("deployment execution recovered", "subsystem", "deployment", "previous_failures", failures)
			}
			failures = 0
			if tracker != nil {
				tracker.Success("deployment", time.Now().UTC().Add(time.Second))
			}
		}
		delay := time.Second
		if worked {
			delay = 10 * time.Millisecond
		} else if failures > 0 {
			delay = workerBackoff(failures)
		}
		timer := time.NewTimer(delay)
		select {
		case <-ctx.Done():
			timer.Stop()
			return
		case <-timer.C:
		}
	}
}

func RunOperationalCommandExecutor(ctx context.Context, executor DeploymentExecutor, logger *slog.Logger, trackers ...*operationalhealth.Tracker) {
	var tracker *operationalhealth.Tracker
	if len(trackers) > 0 {
		tracker = trackers[0]
	}
	failures := 0
	for {
		if tracker != nil {
			tracker.Start("operational_commands", time.Now().UTC().Add(time.Second))
		}
		worked, err := executor.RunOnce(ctx)
		if err != nil && ctx.Err() == nil {
			failures++
			delay := workerBackoff(failures)
			logger.Error("operational command execution failed", "subsystem", "operational_commands", "error", err, "consecutive_failures", failures, "retry_in", delay)
			if tracker != nil {
				tracker.Failure("operational_commands", "OPERATIONAL_COMMAND_WORKER_FAILED", time.Now().UTC().Add(delay))
			}
		} else {
			if failures > 0 {
				logger.Info("operational command execution recovered", "subsystem", "operational_commands", "previous_failures", failures)
			}
			failures = 0
			if tracker != nil {
				tracker.Success("operational_commands", time.Now().UTC().Add(time.Second))
			}
		}
		delay := time.Second
		if worked {
			delay = 10 * time.Millisecond
		} else if failures > 0 {
			delay = workerBackoff(failures)
		}
		timer := time.NewTimer(delay)
		select {
		case <-ctx.Done():
			timer.Stop()
			return
		case <-timer.C:
		}
	}
}

func RunReconciler(ctx context.Context, reconciler DriftReconciler, interval time.Duration, logger *slog.Logger, tracker *operationalhealth.Tracker, providers ...RuntimeSettingsProvider) {
	currentInterval := func() time.Duration {
		value := interval
		if len(providers) > 0 && providers[0] != nil {
			value = providers[0].RuntimeSettings().NodeHealthInterval
		}
		if value < 10*time.Second {
			return 10 * time.Second
		}
		return value
	}
	failures := 0
	run := func() {
		interval = currentInterval()
		if tracker != nil {
			tracker.Start("drift_reconciliation", time.Now().UTC().Add(interval))
		}
		if err := reconciler.RunOnce(ctx); err != nil && ctx.Err() == nil {
			failures++
			logger.Error("drift reconciliation pass failed", "subsystem", "drift_reconciliation", "error", err, "consecutive_failures", failures, "retry_in", interval)
			if tracker != nil {
				tracker.Failure("drift_reconciliation", "DRIFT_WORKER_FAILED", time.Now().UTC().Add(interval))
			}
		} else {
			if failures > 0 {
				logger.Info("drift reconciliation recovered", "subsystem", "drift_reconciliation", "previous_failures", failures)
			}
			failures = 0
			if tracker != nil {
				tracker.Success("drift_reconciliation", time.Now().UTC().Add(interval))
			}
		}
	}
	run()
	for {
		timer := time.NewTimer(currentInterval())
		var changed <-chan struct{}
		if len(providers) > 0 {
			changed = runtimeSettingsChanged(providers[0])
		}
		select {
		case <-ctx.Done():
			timer.Stop()
			return
		case <-changed:
			timer.Stop()
			continue
		case <-timer.C:
			run()
		}
	}
}

func workerBackoff(failures int) time.Duration {
	if failures < 1 {
		return time.Second
	}
	delay := time.Second << min(failures-1, 5)
	if delay > 30*time.Second {
		return 30 * time.Second
	}
	return delay
}
