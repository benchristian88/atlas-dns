package jobs

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"testing"
	"time"

	"github.com/benchristian88/atlas-dns/internal/domain"
)

type healthStoreFake struct {
	records       []domain.NodeRecord
	health        domain.NodeHealth
	compatibility domain.Compatibility
	version       string
	errorCode     string
	seen          bool
}

func (s *healthStoreFake) PollableNodes(context.Context) ([]domain.NodeRecord, error) {
	return s.records, nil
}

func (s *healthStoreFake) UpdateNodeHealth(_ context.Context, _ string, health domain.NodeHealth, compatibility domain.Compatibility, version string, _ *int, errorCode string, _ time.Time, seen bool) error {
	s.health = health
	s.compatibility = compatibility
	s.version = version
	s.errorCode = errorCode
	s.seen = seen
	return nil
}

type decrypterFake struct{ err error }

func (d decrypterFake) Decrypt(string, domain.EncryptedCredentials) (domain.NodeCredentials, error) {
	return domain.NodeCredentials{Username: "admin", Password: "secret"}, d.err
}

type probeFake struct {
	result domain.NodeProbeResult
	err    error
}

func (p probeFake) Status(context.Context, domain.NodeProbeRequest) (domain.NodeProbeResult, error) {
	return p.result, p.err
}

func TestHealthPollerRecordsSupportedNode(t *testing.T) {
	t.Parallel()
	store := &healthStoreFake{records: []domain.NodeRecord{{Node: domain.Node{ID: "node"}}}}
	poller := NewHealthPoller(store, decrypterFake{}, probeFake{result: domain.NodeProbeResult{
		Version: "v0.107.78", Compatibility: domain.CompatibilitySupported, Running: true,
	}}, time.Minute, slog.New(slog.NewTextHandler(io.Discard, nil)))
	if err := poller.PollNow(context.Background(), "node"); err != nil {
		t.Fatalf("PollNow() error = %v", err)
	}
	if store.health != domain.NodeHealthy || !store.seen || store.version != "v0.107.78" {
		t.Fatalf("recorded health = %q, seen = %v, version = %q", store.health, store.seen, store.version)
	}
}

func TestHealthPollerDoesNotExposeProbeFailureAsHealthy(t *testing.T) {
	t.Parallel()
	store := &healthStoreFake{records: []domain.NodeRecord{{Node: domain.Node{ID: "node"}}}}
	poller := NewHealthPoller(store, decrypterFake{}, probeFake{err: &domain.Error{
		Kind: domain.ErrorNodeAuth, Message: "the node rejected credentials", Cause: errors.New("rejected"),
	}}, time.Minute, slog.New(slog.NewTextHandler(io.Discard, nil)))
	if err := poller.PollNow(context.Background(), "node"); err != nil {
		t.Fatalf("PollNow() error = %v", err)
	}
	if store.health != domain.NodeUnreachable || store.errorCode != string(domain.ErrorNodeAuth) || store.seen {
		t.Fatalf("recorded health = %q, code = %q, seen = %v", store.health, store.errorCode, store.seen)
	}
}
