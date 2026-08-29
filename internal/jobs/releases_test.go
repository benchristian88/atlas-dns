package jobs

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"sync/atomic"
	"testing"
	"time"

	"github.com/benchristian88/atlas-dns/internal/operationalhealth"
)

type releaseRefresherFake struct {
	calls atomic.Int64
	err   error
}

func (r *releaseRefresherFake) Refresh(context.Context) error {
	r.calls.Add(1)
	return r.err
}

type releaseTickerFake struct {
	ticks   chan time.Time
	stopped atomic.Bool
}

func (t *releaseTickerFake) Ticks() <-chan time.Time { return t.ticks }
func (t *releaseTickerFake) Stop()                   { t.stopped.Store(true) }

func TestRunReleaseChecksRunsImmediatelyTracksWakeCadenceAndStops(t *testing.T) {
	start := time.Date(2026, 8, 9, 0, 0, 0, 0, time.UTC)
	var clock atomic.Int64
	clock.Store(start.UnixNano())
	now := func() time.Time { return time.Unix(0, clock.Load()).UTC() }
	refresher := &releaseRefresherFake{}
	ticker := &releaseTickerFake{ticks: make(chan time.Time, 1)}
	tracker := operationalhealth.NewTracker()
	tracker.Register("adguard_release_check", false)
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		runReleaseChecks(ctx, refresher, slog.New(slog.NewTextHandler(io.Discard, nil)), tracker, now, func(interval time.Duration) releaseCheckTicker {
			if interval != releaseCheckInterval {
				t.Errorf("ticker interval=%s want %s", interval, releaseCheckInterval)
			}
			return ticker
		})
		close(done)
	}()

	waitForReleaseRuns(t, tracker, 1)
	assertReleaseNextRun(t, tracker, start.Add(releaseCheckInterval))
	clock.Store(start.Add(releaseCheckInterval).UnixNano())
	ticker.ticks <- now()
	waitForReleaseRuns(t, tracker, 2)
	assertReleaseNextRun(t, tracker, start.Add(2*releaseCheckInterval))

	cancel()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("release worker did not exit after cancellation")
	}
	if !ticker.stopped.Load() || refresher.calls.Load() != 2 {
		t.Fatalf("ticker stopped=%t refresh calls=%d", ticker.stopped.Load(), refresher.calls.Load())
	}
}

func TestRunReleaseChecksTracksFailureAtWorkerWakeCadence(t *testing.T) {
	start := time.Date(2026, 8, 9, 0, 0, 0, 0, time.UTC)
	refresher := &releaseRefresherFake{err: errors.New("upstream unavailable")}
	ticker := &releaseTickerFake{ticks: make(chan time.Time)}
	tracker := operationalhealth.NewTracker()
	tracker.Register("adguard_release_check", false)
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		runReleaseChecks(ctx, refresher, slog.New(slog.NewTextHandler(io.Discard, nil)), tracker, func() time.Time { return start }, func(time.Duration) releaseCheckTicker { return ticker })
		close(done)
	}()
	waitForReleaseRuns(t, tracker, 1)
	cancel()
	<-done
	worker := tracker.Snapshot()[0]
	if worker.State != operationalhealth.Degraded || worker.ErrorCode != "RELEASE_CHECK_UNAVAILABLE" || worker.NextScheduledAt == nil || !worker.NextScheduledAt.Equal(start.Add(releaseCheckInterval)) {
		t.Fatalf("worker=%#v", worker)
	}
}

func TestReleaseCheckCadenceClosesLegacyStaleAndFailureWindows(t *testing.T) {
	start := time.Date(2026, 8, 9, 0, 0, 0, 0, time.UTC)
	const legacyCadence = 6 * time.Hour
	successExpiry := start.Add(6*time.Hour + time.Second)
	legacySuccessWake := firstWakeAtOrAfter(start, successExpiry, legacyCadence)
	if staleFor := legacySuccessWake.Sub(successExpiry); staleFor < 5*time.Hour+59*time.Minute {
		t.Fatalf("legacy success stale window=%s", staleFor)
	}
	newSuccessWake := firstWakeAtOrAfter(start, successExpiry, releaseCheckInterval)
	if staleFor := newSuccessWake.Sub(successExpiry); staleFor < 0 || staleFor > releaseCheckInterval {
		t.Fatalf("new success stale window=%s", staleFor)
	}

	failureExpiry := start.Add(15 * time.Minute)
	legacyFailureWake := firstWakeAtOrAfter(start, failureExpiry, legacyCadence)
	if retryDelay := legacyFailureWake.Sub(failureExpiry); retryDelay != 5*time.Hour+45*time.Minute {
		t.Fatalf("legacy failure retry delay=%s", retryDelay)
	}
	newFailureWake := firstWakeAtOrAfter(start, failureExpiry, releaseCheckInterval)
	if retryDelay := newFailureWake.Sub(failureExpiry); retryDelay < 0 || retryDelay > releaseCheckInterval {
		t.Fatalf("new failure retry delay=%s", retryDelay)
	}
	if releaseCheckInterval >= 15*time.Minute || releaseCheckInterval >= 6*time.Hour {
		t.Fatalf("worker cadence=%s does not fit both cache deadlines", releaseCheckInterval)
	}
}

func firstWakeAtOrAfter(start, deadline time.Time, cadence time.Duration) time.Time {
	for wake := start.Add(cadence); ; wake = wake.Add(cadence) {
		if !wake.Before(deadline) {
			return wake
		}
	}
}

func waitForReleaseRuns(t *testing.T, tracker *operationalhealth.Tracker, want uint64) {
	t.Helper()
	deadline := time.After(time.Second)
	for {
		workers := tracker.Snapshot()
		if len(workers) == 1 && workers[0].RunsTotal >= want {
			return
		}
		select {
		case <-deadline:
			t.Fatalf("release worker runs did not reach %d: %#v", want, workers)
		default:
		}
	}
}

func assertReleaseNextRun(t *testing.T, tracker *operationalhealth.Tracker, want time.Time) {
	t.Helper()
	worker := tracker.Snapshot()[0]
	if worker.NextScheduledAt == nil || !worker.NextScheduledAt.Equal(want) {
		t.Fatalf("next run=%v want %s", worker.NextScheduledAt, want)
	}
}
