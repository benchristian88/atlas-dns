package jobs

import (
	"context"
	"errors"
	"log/slog"
	"sync"
	"time"

	"github.com/benchristian88/atlas-dns/internal/domain"
	"github.com/benchristian88/atlas-dns/internal/operationalhealth"
	"github.com/benchristian88/atlas-dns/internal/querylog"
)

const (
	queryLogPageSize = 500
	queryLogMaxPages = 20
)

type QueryLogStore interface {
	PollableNodes(context.Context) ([]domain.NodeRecord, error)
	QueryLogCheckpoint(context.Context, string) (querylog.Checkpoint, bool, error)
	RecordQueryLogPoll(context.Context, querylog.Attempt, querylog.Checkpoint, []querylog.Event) (int, error)
	CleanupQueryLog(context.Context, time.Time, time.Duration, int) (int64, error)
}

type QueryLogReader interface {
	ReadQueryLogConfig(context.Context, domain.NodeProbeRequest, string) (querylog.SourceConfig, error)
	ReadQueryLog(context.Context, domain.NodeProbeRequest, string, int) (querylog.SourcePage, error)
}

type QueryLogPoller struct {
	store       QueryLogStore
	decrypter   CredentialDecrypter
	reader      QueryLogReader
	interval    time.Duration
	timeout     time.Duration
	retention   time.Duration
	overlap     time.Duration
	concurrency int
	logger      *slog.Logger
	now         func() time.Time
	health      *operationalhealth.Tracker
}

func NewQueryLogPoller(store QueryLogStore, decrypter CredentialDecrypter, reader QueryLogReader, interval, timeout, retention time.Duration, logger *slog.Logger, trackers ...*operationalhealth.Tracker) *QueryLogPoller {
	overlap := 2 * interval
	if overlap < 2*time.Minute {
		overlap = 2 * time.Minute
	}
	poller := &QueryLogPoller{store: store, decrypter: decrypter, reader: reader, interval: interval,
		timeout: timeout, retention: retention, overlap: overlap, concurrency: 4, logger: logger, now: time.Now}
	if len(trackers) > 0 {
		poller.health = trackers[0]
	}
	return poller
}

func (p *QueryLogPoller) Run(ctx context.Context) {
	p.poll(ctx)
	ticker := time.NewTicker(p.interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			p.poll(ctx)
		}
	}
}

func (p *QueryLogPoller) poll(ctx context.Context) {
	if p.health != nil {
		p.health.Start("query_log_collection", p.now().UTC().Add(p.interval))
	}
	records, err := p.store.PollableNodes(ctx)
	if err != nil {
		p.logger.Error("query-log polling could not load nodes", "subsystem", "query_log_collection", "error", err, "retry_in", p.interval)
		if p.health != nil {
			p.health.Failure("query_log_collection", "QUERY_LOG_NODE_LIST_FAILED", p.now().UTC().Add(p.interval))
		}
		return
	}
	semaphore := make(chan struct{}, p.concurrency)
	var group sync.WaitGroup
	for _, record := range records {
		record := record
		group.Add(1)
		go func() {
			defer group.Done()
			select {
			case semaphore <- struct{}{}:
				defer func() { <-semaphore }()
			case <-ctx.Done():
				return
			}
			p.pollNode(ctx, record)
		}()
	}
	group.Wait()
	if p.health != nil {
		p.health.Success("query_log_collection", p.now().UTC().Add(p.interval))
	}
	if p.health != nil {
		p.health.Start("query_log_retention", p.now().UTC().Add(p.interval))
	}
	if _, err := p.store.CleanupQueryLog(ctx, p.now().UTC(), p.retention, 10_000); err != nil {
		p.logger.Error("query-log retention cleanup failed", "subsystem", "query_log_retention", "error", err, "retry_in", p.interval)
		if p.health != nil {
			p.health.Failure("query_log_retention", "QUERY_LOG_RETENTION_FAILED", p.now().UTC().Add(p.interval))
		}
		return
	}
	if p.health != nil {
		p.health.Success("query_log_retention", p.now().UTC().Add(p.interval))
	}
}

func (p *QueryLogPoller) pollNode(ctx context.Context, record domain.NodeRecord) {
	started := p.now().UTC()
	attempt := querylog.Attempt{ClusterID: record.Node.ClusterID, NodeID: record.Node.ID, StartedAt: started}
	var err error
	attempt.ID, err = domain.NewID()
	if err != nil {
		p.logger.Error("query-log poll identifier could not be created", "node_id", record.Node.ID, "error", err)
		return
	}
	checkpoint, exists, err := p.store.QueryLogCheckpoint(ctx, record.Node.ID)
	if err != nil {
		p.logger.Error("query-log checkpoint could not be loaded", "node_id", record.Node.ID, "error", err)
		return
	}
	if !exists {
		checkpoint = querylog.Checkpoint{ClusterID: record.Node.ClusterID, NodeID: record.Node.ID}
	}
	checkpoint.ClusterID, checkpoint.NodeID, checkpoint.NodeVersion = record.Node.ClusterID, record.Node.ID, record.Node.Version
	recordAttempt := func(events []querylog.Event) {
		attempt.CompletedAt = p.now().UTC()
		checkpoint.LastAttemptAt, checkpoint.UpdatedAt = attempt.CompletedAt, attempt.CompletedAt
		checkpoint.LastStatus, checkpoint.ErrorCode = attempt.Status, attempt.ErrorCode
		checkpoint.GapDetected, checkpoint.GapReason = attempt.GapDetected, attempt.GapReason
		inserted, storeErr := p.store.RecordQueryLogPoll(ctx, attempt, checkpoint, events)
		if storeErr != nil {
			p.logger.Error("query-log poll result could not be recorded", "node_id", record.Node.ID, "error", storeErr)
			return
		}
		if inserted > 0 {
			p.logger.Debug("query-log events ingested", "node_id", record.Node.ID, "inserted", inserted)
		}
	}
	if record.Node.MaintenanceMode {
		attempt.Status, attempt.ErrorCode = "maintenance", "NODE_MAINTENANCE"
		recordAttempt(nil)
		return
	}
	if querylog.VersionBelowMinimum(record.Node.Version) {
		attempt.Status, attempt.ErrorCode = "unsupported", "QUERY_LOG_UNSUPPORTED"
		recordAttempt(nil)
		return
	}
	if !querylog.SupportsVersion(record.Node.Version) {
		attempt.Status, attempt.ErrorCode = "failed", "QUERY_LOG_CAPABILITY_UNKNOWN"
		recordAttempt(nil)
		return
	}
	credentials, err := p.decrypter.Decrypt(record.Node.ID, record.Secrets.Credentials)
	if err != nil {
		attempt.Status, attempt.ErrorCode = "failed", "CREDENTIAL_DECRYPTION_FAILED"
		recordAttempt(nil)
		return
	}
	request := domain.NodeProbeRequest{BaseURL: record.Node.BaseURL, CertificatePolicy: record.Node.CertificatePolicy,
		CustomCAPEM: record.Secrets.CustomCAPEM, Credentials: credentials}
	requestContext, cancel := context.WithTimeout(ctx, p.timeout)
	config, err := p.reader.ReadQueryLogConfig(requestContext, request, record.Node.Version)
	cancel()
	if err != nil {
		attempt.Status, attempt.ErrorCode = queryLogFailure(err)
		recordAttempt(nil)
		return
	}
	checkpoint.LoggingEnabled = &config.Enabled
	if !config.Enabled {
		attempt.Status, attempt.ErrorCode = "logging_disabled", "QUERY_LOG_DISABLED"
		recordAttempt(nil)
		return
	}

	events := make([]querylog.Event, 0, queryLogPageSize)
	occurrences := map[[32]byte]int{}
	var newest, oldest *time.Time
	exhausted, crossedOverlap, cursorStalled := false, false, false
	olderThan := ""
	for pageIndex := 0; pageIndex < queryLogMaxPages; pageIndex++ {
		requestContext, cancel = context.WithTimeout(ctx, p.timeout)
		page, readErr := p.reader.ReadQueryLog(requestContext, request, olderThan, queryLogPageSize)
		cancel()
		if readErr != nil {
			attempt.Status, attempt.ErrorCode = queryLogFailure(readErr)
			recordAttempt(nil)
			return
		}
		attempt.PageCount++
		attempt.FetchedRecords += len(page.Events) + page.InvalidRecords
		if olderThan != "" && page.Oldest == olderThan {
			attempt.GapDetected, attempt.GapReason = true, "QUERY_LOG_CURSOR_STALLED"
			cursorStalled = true
			break
		}
		if page.InvalidRecords > 0 {
			attempt.GapDetected, attempt.GapReason = true, "QUERY_LOG_MALFORMED_RECORD"
			p.logger.Warn("query-log page contained malformed records", "subsystem", "query_log_collection", "node_id", record.Node.ID, "invalid_records", page.InvalidRecords)
		}
		for _, source := range page.Events {
			fingerprint := source.Fingerprint()
			occurrences[fingerprint]++
			id, idErr := domain.NewID()
			if idErr != nil {
				attempt.Status, attempt.ErrorCode = "failed", "QUERY_LOG_ID_FAILED"
				recordAttempt(nil)
				return
			}
			event := querylog.Event{ID: id, ClusterID: record.Node.ClusterID, NodeID: record.Node.ID,
				SourceTimestamp: source.Timestamp, IngestedAt: p.now().UTC(), SourceFingerprint: fingerprint[:],
				SourceOccurrence: occurrences[fingerprint], SourceEvent: source}
			events = append(events, event)
			at := source.Timestamp
			if newest == nil || at.After(*newest) {
				newest = &at
			}
			if oldest == nil || at.Before(*oldest) {
				oldest = &at
			}
		}
		if len(page.Events)+page.InvalidRecords < queryLogPageSize {
			exhausted = true
			break
		}
		if page.Oldest == "" {
			attempt.GapDetected, attempt.GapReason = true, "QUERY_LOG_CURSOR_MISSING"
			cursorStalled = true
			break
		}
		olderThan = page.Oldest
		if checkpoint.HighWatermarkAt != nil && oldest != nil && oldest.Before(checkpoint.HighWatermarkAt.Add(-p.overlap)) {
			crossedOverlap = true
			break
		}
	}
	if !exhausted && !crossedOverlap && !cursorStalled {
		attempt.GapDetected, attempt.GapReason = true, "QUERY_LOG_SOURCE_WINDOW_TRUNCATED"
	}
	if checkpoint.HighWatermarkAt != nil {
		switch {
		case newest == nil:
			attempt.GapDetected, attempt.GapReason = true, "QUERY_LOG_SOURCE_EMPTY_AFTER_CHECKPOINT"
		case newest.Before(*checkpoint.HighWatermarkAt):
			attempt.GapDetected, attempt.GapReason = true, "QUERY_LOG_CLOCK_REGRESSION_OR_RESET"
		case exhausted && oldest != nil && oldest.After(checkpoint.HighWatermarkAt.Add(p.overlap)):
			attempt.GapDetected, attempt.GapReason = true, "QUERY_LOG_NODE_RETENTION_GAP"
		}
	}
	if newest != nil {
		checkpoint.SourceNewestAt = newest
		if checkpoint.HighWatermarkAt == nil || newest.After(*checkpoint.HighWatermarkAt) {
			checkpoint.HighWatermarkAt = newest
		}
	}
	if oldest != nil {
		checkpoint.SourceOldestAt = oldest
	}
	completed := p.now().UTC()
	checkpoint.LastSuccessAt = &completed
	attempt.Status = "succeeded"
	recordAttempt(events)
}

func queryLogFailure(err error) (status, code string) {
	var domainError *domain.Error
	if errors.As(err, &domainError) && domainError.Kind == domain.ErrorCapability {
		return "unsupported", "QUERY_LOG_UNSUPPORTED"
	}
	return "failed", queryLogErrorCode(err)
}

func queryLogErrorCode(err error) string {
	if errors.Is(err, context.DeadlineExceeded) {
		return "QUERY_LOG_TIMEOUT"
	}
	var domainError *domain.Error
	if errors.As(err, &domainError) {
		return string(domainError.Kind)
	}
	return "QUERY_LOG_COLLECTION_FAILED"
}
