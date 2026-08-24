package jobs

import (
	"context"
	"errors"
	"log/slog"
	"sync"
	"time"

	"github.com/benchristian88/atlas-dns/internal/domain"
	"github.com/benchristian88/atlas-dns/internal/operationalhealth"
)

type HealthStore interface {
	PollableNodes(context.Context) ([]domain.NodeRecord, error)
	UpdateNodeHealth(context.Context, string, domain.NodeHealth, domain.Compatibility, string, *int, string, time.Time, bool) error
}

type CredentialDecrypter interface {
	Decrypt(string, domain.EncryptedCredentials) (domain.NodeCredentials, error)
}

type StatusProbe interface {
	Status(context.Context, domain.NodeProbeRequest) (domain.NodeProbeResult, error)
}

type HealthPoller struct {
	store       HealthStore
	decrypter   CredentialDecrypter
	probe       StatusProbe
	interval    time.Duration
	concurrency int
	logger      *slog.Logger
	now         func() time.Time
	health      *operationalhealth.Tracker
	settings    RuntimeSettingsProvider
}

func (p *HealthPoller) SetRuntimeSettings(provider RuntimeSettingsProvider) { p.settings = provider }

func (p *HealthPoller) currentInterval() time.Duration {
	if p.settings != nil {
		return p.settings.RuntimeSettings().NodeHealthInterval
	}
	return p.interval
}

func NewHealthPoller(store HealthStore, decrypter CredentialDecrypter, probe StatusProbe, interval time.Duration, logger *slog.Logger, trackers ...*operationalhealth.Tracker) *HealthPoller {
	poller := &HealthPoller{
		store: store, decrypter: decrypter, probe: probe, interval: interval,
		concurrency: 4, logger: logger, now: time.Now,
	}
	if len(trackers) > 0 {
		poller.health = trackers[0]
	}
	return poller
}

func (p *HealthPoller) Run(ctx context.Context) {
	p.poll(ctx)
	for {
		timer := time.NewTimer(p.currentInterval())
		select {
		case <-ctx.Done():
			timer.Stop()
			return
		case <-timer.C:
			p.poll(ctx)
		}
	}
}

func (p *HealthPoller) PollNow(ctx context.Context, nodeID string) error {
	records, err := p.store.PollableNodes(ctx)
	if err != nil {
		return err
	}
	for _, record := range records {
		if record.Node.ID == nodeID {
			p.pollNode(ctx, record)
			return nil
		}
	}
	return domain.NewError(domain.ErrorNotFound, "node was not found or is disabled")
}

// PollClusterNow refreshes every enabled node in one cluster using the same
// durable health path as the scheduled worker.
func (p *HealthPoller) PollClusterNow(ctx context.Context, clusterID string) error {
	return p.pollCluster(ctx, clusterID)
}

func (p *HealthPoller) poll(ctx context.Context) {
	_ = p.pollCluster(ctx, "")
}

func (p *HealthPoller) pollCluster(ctx context.Context, clusterID string) error {
	interval := p.currentInterval()
	if p.health != nil {
		p.health.Start("node_connectivity", p.now().UTC().Add(interval))
	}
	records, err := p.store.PollableNodes(ctx)
	if err != nil {
		p.logger.Error("node health polling could not load nodes", "subsystem", "node_connectivity", "error", err, "retry_in", interval)
		if p.health != nil {
			p.health.Failure("node_connectivity", "NODE_LIST_FAILED", p.now().UTC().Add(interval))
		}
		return err
	}
	semaphore := make(chan struct{}, p.concurrency)
	var group sync.WaitGroup
	for _, record := range records {
		if clusterID != "" && record.Node.ClusterID != clusterID {
			continue
		}
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
		p.health.Success("node_connectivity", p.now().UTC().Add(interval))
	}
	return nil
}

func (p *HealthPoller) pollNode(ctx context.Context, record domain.NodeRecord) {
	at := p.now().UTC()
	credentials, err := p.decrypter.Decrypt(record.Node.ID, record.Secrets.Credentials)
	if err != nil {
		p.logger.Error("node credentials could not be decrypted", "node_id", record.Node.ID, "error_code", "CREDENTIAL_DECRYPTION_FAILED")
		_ = p.store.UpdateNodeHealth(ctx, record.Node.ID, domain.NodeUnreachable, domain.CompatibilityUnknown, record.Node.Version, nil, "CREDENTIAL_DECRYPTION_FAILED", at, false)
		return
	}
	result, err := p.probe.Status(ctx, domain.NodeProbeRequest{
		BaseURL: record.Node.BaseURL, CertificatePolicy: record.Node.CertificatePolicy,
		CustomCAPEM: record.Secrets.CustomCAPEM, Credentials: credentials,
	})
	if err != nil {
		code := string(domain.ErrorNodeUnreachable)
		var domainError *domain.Error
		if errors.As(err, &domainError) {
			code = string(domainError.Kind)
		}
		if updateErr := p.store.UpdateNodeHealth(ctx, record.Node.ID, domain.NodeUnreachable, record.Node.CompatibilityStatus, record.Node.Version, nil, code, at, false); updateErr != nil {
			p.logger.Error("node health failure could not be recorded", "node_id", record.Node.ID, "error", updateErr)
		}
		return
	}
	health := domain.NodeHealthy
	errorCode := ""
	if result.Compatibility == domain.CompatibilityUnsupported || result.Compatibility == domain.CompatibilityUnknown {
		health = domain.NodeIncompatible
	}
	if !result.Running {
		health = domain.NodeUnreachable
		errorCode = "NODE_DNS_NOT_RUNNING"
	}
	latency := result.LatencyMS
	if err := p.store.UpdateNodeHealth(ctx, record.Node.ID, health, result.Compatibility, result.Version, &latency, errorCode, at, true); err != nil {
		p.logger.Error("node health result could not be recorded", "node_id", record.Node.ID, "error", err)
	}
}
