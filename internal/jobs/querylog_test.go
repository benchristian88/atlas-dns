package jobs

import (
	"context"
	"io"
	"log/slog"
	"testing"
	"time"

	"github.com/benchristian88/atlas-dns/internal/domain"
	"github.com/benchristian88/atlas-dns/internal/querylog"
)

type queryLogStoreFake struct {
	checkpoint querylog.Checkpoint
	hasCheck   bool
	attempts   []querylog.Attempt
	events     []querylog.Event
	unique     map[string]bool
	cleanupErr error
}

func (s *queryLogStoreFake) PollableNodes(context.Context) ([]domain.NodeRecord, error) {
	return nil, nil
}
func (s *queryLogStoreFake) QueryLogCheckpoint(context.Context, string) (querylog.Checkpoint, bool, error) {
	return s.checkpoint, s.hasCheck, nil
}
func (s *queryLogStoreFake) RecordQueryLogPoll(_ context.Context, attempt querylog.Attempt, checkpoint querylog.Checkpoint, events []querylog.Event) (int, error) {
	s.attempts = append(s.attempts, attempt)
	s.checkpoint, s.hasCheck = checkpoint, true
	if s.unique == nil {
		s.unique = map[string]bool{}
	}
	inserted := 0
	for _, event := range events {
		key := event.NodeID + string(event.SourceFingerprint) + string(rune(event.SourceOccurrence))
		if !s.unique[key] {
			s.unique[key] = true
			s.events = append(s.events, event)
			inserted++
		}
	}
	return inserted, nil
}
func (s *queryLogStoreFake) CleanupQueryLog(context.Context, time.Time, time.Duration, int) (int64, error) {
	return 0, s.cleanupErr
}

type queryLogReaderFake struct {
	config    querylog.SourceConfig
	pages     []querylog.SourcePage
	cursors   []string
	readErr   error
	configErr error
}

func (r *queryLogReaderFake) ReadQueryLogConfig(context.Context, domain.NodeProbeRequest, string) (querylog.SourceConfig, error) {
	return r.config, r.configErr
}
func (r *queryLogReaderFake) ReadQueryLog(_ context.Context, _ domain.NodeProbeRequest, olderThan string, _ int) (querylog.SourcePage, error) {
	r.cursors = append(r.cursors, olderThan)
	if r.readErr != nil {
		return querylog.SourcePage{}, r.readErr
	}
	index := len(r.cursors) - 1
	if index >= len(r.pages) {
		return querylog.SourcePage{}, nil
	}
	return r.pages[index], nil
}

func TestQueryLogPollerResumesAndDeduplicatesOverlap(t *testing.T) {
	now := time.Date(2026, 8, 9, 3, 0, 0, 0, time.UTC)
	source := querylog.SourceEvent{Timestamp: now, QueryName: "example.org", QueryType: "A", ClientIdentifier: "192.0.2.1", ResponseStatus: querylog.StatusAllowed, ElapsedMS: 1}
	store := &queryLogStoreFake{}
	reader := &queryLogReaderFake{config: querylog.SourceConfig{Enabled: true}, pages: []querylog.SourcePage{{Events: []querylog.SourceEvent{source}}}}
	poller := queryLogPollerForTest(store, reader, now)
	record := queryLogNode()
	poller.pollNode(context.Background(), record)
	reader.cursors = nil
	poller.pollNode(context.Background(), record)
	if len(store.events) != 1 || len(store.attempts) != 2 {
		t.Fatalf("events=%d attempts=%d", len(store.events), len(store.attempts))
	}
	if store.checkpoint.HighWatermarkAt == nil || !store.checkpoint.HighWatermarkAt.Equal(now) {
		t.Fatalf("checkpoint = %+v", store.checkpoint)
	}
}

func TestQueryLogPollerUsesSourceCursorAcrossPages(t *testing.T) {
	now := time.Date(2026, 8, 9, 3, 0, 0, 0, time.UTC)
	first := make([]querylog.SourceEvent, queryLogPageSize)
	for index := range first {
		first[index] = querylog.SourceEvent{Timestamp: now.Add(-time.Duration(index) * time.Second), QueryName: "example.org", QueryType: "A", ResponseStatus: querylog.StatusAllowed, ElapsedMS: 1}
	}
	oldest := now.Add(-time.Duration(queryLogPageSize-1) * time.Second).Format(time.RFC3339Nano)
	reader := &queryLogReaderFake{config: querylog.SourceConfig{Enabled: true}, pages: []querylog.SourcePage{
		{Events: first, Oldest: oldest},
		{Events: []querylog.SourceEvent{{Timestamp: now.Add(-time.Hour), QueryName: "older.example", QueryType: "AAAA", ResponseStatus: querylog.StatusAllowed, ElapsedMS: 1}}},
	}}
	store := &queryLogStoreFake{}
	poller := queryLogPollerForTest(store, reader, now)
	poller.pollNode(context.Background(), queryLogNode())
	if len(reader.cursors) != 2 || reader.cursors[0] != "" || reader.cursors[1] != oldest {
		t.Fatalf("cursors = %#v", reader.cursors)
	}
	if len(store.events) != queryLogPageSize+1 || store.attempts[0].PageCount != 2 {
		t.Fatalf("events=%d attempt=%+v", len(store.events), store.attempts[0])
	}
}

func TestQueryLogPollerRecordsDisabledAndMalformedSourceStates(t *testing.T) {
	now := time.Date(2026, 8, 9, 3, 0, 0, 0, time.UTC)
	store := &queryLogStoreFake{}
	reader := &queryLogReaderFake{config: querylog.SourceConfig{Enabled: false}}
	poller := queryLogPollerForTest(store, reader, now)
	poller.pollNode(context.Background(), queryLogNode())
	if store.attempts[0].Status != "logging_disabled" || store.checkpoint.LoggingEnabled == nil || *store.checkpoint.LoggingEnabled {
		t.Fatalf("attempt/checkpoint = %+v %+v", store.attempts[0], store.checkpoint)
	}

	store = &queryLogStoreFake{}
	reader = &queryLogReaderFake{config: querylog.SourceConfig{Enabled: true}, pages: []querylog.SourcePage{{InvalidRecords: 1}}}
	poller = queryLogPollerForTest(store, reader, now)
	poller.pollNode(context.Background(), queryLogNode())
	if !store.attempts[0].GapDetected || store.attempts[0].GapReason != "QUERY_LOG_MALFORMED_RECORD" {
		t.Fatalf("attempt = %+v", store.attempts[0])
	}
}

func TestQueryLogPollerFailureRetainsCheckpointAndExistingEvents(t *testing.T) {
	now := time.Date(2026, 8, 9, 3, 0, 0, 0, time.UTC)
	previous := now.Add(-time.Minute)
	store := &queryLogStoreFake{
		hasCheck:   true,
		checkpoint: querylog.Checkpoint{NodeID: queryLogNode().Node.ID, ClusterID: queryLogNode().Node.ClusterID, LastSuccessAt: &previous, HighWatermarkAt: &previous},
		events:     []querylog.Event{{ID: "33333333-3333-4333-8333-333333333333"}},
	}
	reader := &queryLogReaderFake{config: querylog.SourceConfig{Enabled: true}, readErr: context.DeadlineExceeded}
	poller := queryLogPollerForTest(store, reader, now)
	poller.pollNode(context.Background(), queryLogNode())
	if len(store.events) != 1 || store.attempts[0].Status != "failed" || store.attempts[0].ErrorCode != "QUERY_LOG_TIMEOUT" || store.checkpoint.LastSuccessAt == nil || !store.checkpoint.LastSuccessAt.Equal(previous) {
		t.Fatalf("events=%d attempt=%+v checkpoint=%+v", len(store.events), store.attempts[0], store.checkpoint)
	}
}

func TestQueryLogPollerSupportsMixedTestedPatchesAndClassifiesMissingEndpoint(t *testing.T) {
	now := time.Date(2026, 8, 19, 3, 0, 0, 0, time.UTC)
	for _, version := range []string{"v0.107.78", "v0.107.79", "v0.107.80"} {
		store := &queryLogStoreFake{}
		reader := &queryLogReaderFake{config: querylog.SourceConfig{Enabled: true}}
		poller := queryLogPollerForTest(store, reader, now)
		record := queryLogNode()
		record.Node.Version = version
		poller.pollNode(context.Background(), record)
		if len(store.attempts) != 1 || store.attempts[0].Status != "succeeded" {
			t.Fatalf("%s attempt = %+v", version, store.attempts)
		}
	}

	store := &queryLogStoreFake{}
	reader := &queryLogReaderFake{configErr: domain.NewError(domain.ErrorCapability, "GET /control/querylog/config returned 404")}
	poller := queryLogPollerForTest(store, reader, now)
	poller.pollNode(context.Background(), queryLogNode())
	if store.attempts[0].Status != "unsupported" || store.attempts[0].ErrorCode != "QUERY_LOG_UNSUPPORTED" {
		t.Fatalf("missing endpoint attempt = %+v", store.attempts[0])
	}
}

func TestQueryLogPollerDoesNotCallUnknownAPIGeneration(t *testing.T) {
	store := &queryLogStoreFake{}
	reader := &queryLogReaderFake{}
	poller := queryLogPollerForTest(store, reader, time.Now())
	record := queryLogNode()
	record.Node.Version = "v0.108.0"
	poller.pollNode(context.Background(), record)
	if store.attempts[0].Status != "failed" || store.attempts[0].ErrorCode != "QUERY_LOG_CAPABILITY_UNKNOWN" || len(reader.cursors) != 0 {
		t.Fatalf("unknown generation attempt = %+v cursors=%v", store.attempts[0], reader.cursors)
	}
}

func TestQueryLogPollerDetectsStalledSourceCursor(t *testing.T) {
	now := time.Date(2026, 8, 9, 3, 0, 0, 0, time.UTC)
	events := make([]querylog.SourceEvent, queryLogPageSize)
	for index := range events {
		events[index] = querylog.SourceEvent{Timestamp: now.Add(-time.Duration(index) * time.Second), QueryName: "example.org", QueryType: "A", ResponseStatus: querylog.StatusAllowed, ElapsedMS: 1}
	}
	reader := &queryLogReaderFake{config: querylog.SourceConfig{Enabled: true}, pages: []querylog.SourcePage{
		{Events: events, Oldest: now.Format(time.RFC3339Nano)},
		{Events: events, Oldest: now.Format(time.RFC3339Nano)},
	}}
	store := &queryLogStoreFake{}
	poller := queryLogPollerForTest(store, reader, now)
	poller.pollNode(context.Background(), queryLogNode())
	if !store.attempts[0].GapDetected || store.attempts[0].GapReason != "QUERY_LOG_CURSOR_STALLED" || len(store.events) != queryLogPageSize {
		t.Fatalf("attempt = %+v events=%d", store.attempts[0], len(store.events))
	}
}

func TestQueryLogPollerDoesNotCallFullPageWithoutCursorComplete(t *testing.T) {
	now := time.Date(2026, 8, 9, 3, 0, 0, 0, time.UTC)
	events := make([]querylog.SourceEvent, queryLogPageSize)
	for index := range events {
		events[index] = querylog.SourceEvent{Timestamp: now.Add(-time.Duration(index) * time.Second), QueryName: "example.org", QueryType: "A", ResponseStatus: querylog.StatusAllowed, ElapsedMS: 1}
	}
	reader := &queryLogReaderFake{config: querylog.SourceConfig{Enabled: true}, pages: []querylog.SourcePage{{Events: events}}}
	store := &queryLogStoreFake{}
	poller := queryLogPollerForTest(store, reader, now)
	poller.pollNode(context.Background(), queryLogNode())
	if !store.attempts[0].GapDetected || store.attempts[0].GapReason != "QUERY_LOG_CURSOR_MISSING" {
		t.Fatalf("attempt = %+v", store.attempts[0])
	}
}

func queryLogPollerForTest(store *queryLogStoreFake, reader *queryLogReaderFake, now time.Time) *QueryLogPoller {
	poller := NewQueryLogPoller(store, decrypterFake{}, reader, time.Minute, time.Second, 7*24*time.Hour, slog.New(slog.NewTextHandler(io.Discard, nil)))
	poller.now = func() time.Time { return now }
	return poller
}

func queryLogNode() domain.NodeRecord {
	return domain.NodeRecord{Node: domain.Node{ID: "22222222-2222-4222-8222-222222222222", ClusterID: "11111111-1111-4111-8111-111111111111", Name: "dns-a", Enabled: true, Version: "v0.107.78"}}
}
