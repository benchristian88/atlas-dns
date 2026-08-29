package operationalhealth

import (
	"testing"
	"time"
)

func TestTrackerFailureAndRecovery(t *testing.T) {
	now := time.Date(2026, 8, 9, 1, 0, 0, 0, time.UTC)
	tracker := NewTracker()
	tracker.now = func() time.Time { return now }
	tracker.Register("statistics", false)
	for range 3 {
		tracker.Start("statistics", now.Add(time.Minute))
		tracker.Failure("statistics", "STATISTICS_COLLECTION_FAILED", now.Add(time.Minute))
	}
	failed := tracker.Snapshot()[0]
	if failed.State != Failed || failed.ConsecutiveFailures != 3 || failed.ErrorCode == "" {
		t.Fatalf("unexpected failed state: %#v", failed)
	}
	tracker.Start("statistics", now.Add(time.Minute))
	tracker.Success("statistics", now.Add(time.Minute))
	recovered := tracker.Snapshot()[0]
	if recovered.State != Healthy || recovered.ConsecutiveFailures != 0 || recovered.ErrorCode != "" {
		t.Fatalf("unexpected recovery state: %#v", recovered)
	}
}

func TestTrackerCanPauseAndResumeDynamicCollection(t *testing.T) {
	now := time.Date(2026, 8, 24, 1, 0, 0, 0, time.UTC)
	tracker := NewTracker()
	tracker.now = func() time.Time { return now }
	tracker.Register("query_log_collection", false)
	tracker.Start("query_log_collection", now.Add(time.Minute))
	tracker.Failure("query_log_collection", "QUERY_LOG_FAILED", now.Add(time.Minute))
	tracker.Pause("query_log_collection", now.Add(2*time.Minute))
	paused := tracker.Snapshot()[0]
	if paused.State != Paused || paused.Running || paused.ErrorCode != "" || paused.ConsecutiveFailures != 0 {
		t.Fatalf("unexpected paused state: %#v", paused)
	}
	tracker.Start("query_log_collection", now.Add(time.Minute))
	tracker.Success("query_log_collection", now.Add(time.Minute))
	if resumed := tracker.Snapshot()[0]; resumed.State != Healthy {
		t.Fatalf("unexpected resumed state: %#v", resumed)
	}
}
