package jobs

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"testing"
	"time"

	"github.com/benchristian88/atlas-dns/internal/domain"
	"github.com/benchristian88/atlas-dns/internal/telemetry"
)

type statisticsStoreFake struct {
	attempt   telemetry.PollAttempt
	snapshots []telemetry.Snapshot
}

func (s *statisticsStoreFake) PollableNodes(context.Context) ([]domain.NodeRecord, error) {
	return nil, nil
}
func (s *statisticsStoreFake) RecordStatisticsPoll(_ context.Context, attempt telemetry.PollAttempt, snapshots []telemetry.Snapshot) error {
	s.attempt, s.snapshots = attempt, snapshots
	return nil
}
func (s *statisticsStoreFake) CleanupStatistics(context.Context, time.Time) error { return nil }

type statisticsReaderFake struct {
	config      telemetry.SourceConfig
	configReads int
	rangeReads  []time.Duration
}

func (r *statisticsReaderFake) ReadStatisticsConfig(context.Context, domain.NodeProbeRequest) (telemetry.SourceConfig, error) {
	r.configReads++
	return r.config, nil
}

func (r *statisticsReaderFake) ReadStatistics(_ context.Context, _ domain.NodeProbeRequest, recent time.Duration) (telemetry.SourceSnapshot, error) {
	r.rangeReads = append(r.rangeReads, recent)
	if recent != telemetry.Range24Hours.Duration() {
		return telemetry.SourceSnapshot{}, &domain.Error{Kind: domain.ErrorCapability, Message: "range unavailable", Cause: errors.New("retention too short")}
	}
	return telemetry.SourceSnapshot{TimeUnit: "hours", DNSQueries: 10, DNSQueriesSeries: []int64{10}, BlockedFilteringSeries: []int64{0}, ReplacedSafeBrowsingSeries: []int64{0}, ReplacedParentalSeries: []int64{0}}, nil
}

func TestStatisticsPollerRecordsPartialRangeFailures(t *testing.T) {
	store := &statisticsStoreFake{}
	reader := &statisticsReaderFake{config: telemetry.SourceConfig{Enabled: true, Retention: 30 * 24 * time.Hour}}
	poller := NewStatisticsPoller(store, decrypterFake{}, reader, time.Hour, time.Second, slog.New(slog.NewTextHandler(io.Discard, nil)))
	now := time.Date(2026, 8, 9, 12, 30, 0, 0, time.UTC)
	poller.now = func() time.Time { return now }
	poller.pollNode(context.Background(), domain.NodeRecord{Node: domain.Node{
		ID: "22222222-2222-4222-8222-222222222222", ClusterID: "11111111-1111-4111-8111-111111111111", Name: "Primary", Version: "v0.107.78",
	}})
	if store.attempt.Status != "partial" || store.attempt.CollectedRanges != 1 || len(store.snapshots) != 1 {
		t.Fatalf("attempt=%+v snapshots=%d", store.attempt, len(store.snapshots))
	}
	if store.attempt.RangeErrors[telemetry.Range7Days] != string(domain.ErrorCapability) || store.attempt.RangeErrors[telemetry.Range30Days] != string(domain.ErrorCapability) {
		t.Fatalf("range errors = %+v", store.attempt.RangeErrors)
	}
	if store.snapshots[0].SourceEndedAt != time.Date(2026, 8, 9, 13, 0, 0, 0, time.UTC) {
		t.Fatalf("SourceEndedAt = %v", store.snapshots[0].SourceEndedAt)
	}
}

func TestStatisticsPollerOnlyExpectsRangesWithinNodeRetention(t *testing.T) {
	store := &statisticsStoreFake{}
	reader := &statisticsReaderFake{config: telemetry.SourceConfig{Enabled: true, Retention: 24 * time.Hour}}
	poller := NewStatisticsPoller(store, decrypterFake{}, reader, time.Hour, time.Second, slog.New(slog.NewTextHandler(io.Discard, nil)))
	poller.pollNode(context.Background(), domain.NodeRecord{Node: domain.Node{
		ID: "22222222-2222-4222-8222-222222222222", ClusterID: "11111111-1111-4111-8111-111111111111", Name: "Primary", Version: "v0.107.78",
	}})
	if store.attempt.Status != "succeeded" || store.attempt.ExpectedRanges != 1 || store.attempt.CollectedRanges != 1 || len(store.snapshots) != 1 {
		t.Fatalf("attempt=%+v snapshots=%d", store.attempt, len(store.snapshots))
	}
	if len(reader.rangeReads) != 1 || reader.rangeReads[0] != telemetry.Range24Hours.Duration() {
		t.Fatalf("range reads = %v", reader.rangeReads)
	}
	if store.attempt.RangeErrors[telemetry.Range7Days] != telemetry.ErrorRangeExceedsNodeRetention || store.attempt.RangeErrors[telemetry.Range30Days] != telemetry.ErrorRangeExceedsNodeRetention {
		t.Fatalf("range errors = %+v", store.attempt.RangeErrors)
	}
}

func TestStatisticsPollerDoesNotCallUnsupportedNode(t *testing.T) {
	store := &statisticsStoreFake{}
	reader := &statisticsReaderFake{config: telemetry.SourceConfig{Enabled: true, Retention: 30 * 24 * time.Hour}}
	poller := NewStatisticsPoller(store, decrypterFake{}, reader, time.Hour, time.Second, slog.New(slog.NewTextHandler(io.Discard, nil)))
	poller.pollNode(context.Background(), domain.NodeRecord{Node: domain.Node{ID: "22222222-2222-4222-8222-222222222222", ClusterID: "11111111-1111-4111-8111-111111111111", Version: "v0.107.71"}})
	if store.attempt.Status != "unsupported" || store.attempt.RangeErrors[telemetry.Range24Hours] != "STATISTICS_EXACT_RANGE_UNSUPPORTED" {
		t.Fatalf("attempt = %+v", store.attempt)
	}
	if reader.configReads != 0 || len(reader.rangeReads) != 0 {
		t.Fatalf("unsupported node was read: config=%d ranges=%v", reader.configReads, reader.rangeReads)
	}
}

func TestStatisticsPollerAllowsNewerCompatiblePatchAndSeparatesUnknownGeneration(t *testing.T) {
	store := &statisticsStoreFake{}
	reader := &statisticsReaderFake{config: telemetry.SourceConfig{Enabled: true, Retention: 24 * time.Hour}}
	poller := NewStatisticsPoller(store, decrypterFake{}, reader, time.Hour, time.Second, slog.New(slog.NewTextHandler(io.Discard, nil)))
	poller.pollNode(context.Background(), domain.NodeRecord{Node: domain.Node{ID: "22222222-2222-4222-8222-222222222222", ClusterID: "11111111-1111-4111-8111-111111111111", Version: "v0.107.80"}})
	if store.attempt.Status != "succeeded" || reader.configReads != 1 {
		t.Fatalf("newer compatible patch attempt=%+v reads=%d", store.attempt, reader.configReads)
	}

	store = &statisticsStoreFake{}
	reader = &statisticsReaderFake{}
	poller = NewStatisticsPoller(store, decrypterFake{}, reader, time.Hour, time.Second, slog.New(slog.NewTextHandler(io.Discard, nil)))
	poller.pollNode(context.Background(), domain.NodeRecord{Node: domain.Node{ID: "22222222-2222-4222-8222-222222222222", ClusterID: "11111111-1111-4111-8111-111111111111", Version: "v0.108.0"}})
	if store.attempt.Status != "failed" || store.attempt.ErrorCode != "STATISTICS_CAPABILITY_UNKNOWN" || reader.configReads != 0 {
		t.Fatalf("unknown generation attempt=%+v reads=%d", store.attempt, reader.configReads)
	}
}
