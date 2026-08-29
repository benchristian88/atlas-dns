package controlplane

import (
	"context"
	"log/slog"
	"time"

	"github.com/benchristian88/atlas-dns/internal/configuration"
	"github.com/benchristian88/atlas-dns/internal/domain"
	"github.com/benchristian88/atlas-dns/internal/inventory"
)

type ReconciliationObserver interface {
	Observe(context.Context, string) (inventory.Snapshot, error)
}

type Reconciler struct {
	repository Repository
	service    *Service
	observer   ReconciliationObserver
	logger     *slog.Logger
	now        func() time.Time
}

func NewReconciler(repository Repository, service *Service, observer ReconciliationObserver, logger *slog.Logger) *Reconciler {
	return &Reconciler{repository: repository, service: service, observer: observer, logger: logger, now: time.Now}
}

func (r *Reconciler) RunOnce(ctx context.Context) error {
	clusters, err := r.repository.ListClusters(ctx)
	if err != nil {
		return err
	}
	for _, cluster := range clusters {
		if cluster.ActiveRevisionID == nil {
			continue
		}
		activeDeployment, err := r.repository.ClusterHasActiveDeployment(ctx, cluster.ID)
		if err != nil {
			return err
		}
		if activeDeployment {
			continue
		}
		revision, err := r.repository.RevisionByID(ctx, *cluster.ActiveRevisionID)
		if err != nil {
			r.logger.Error("active revision could not be loaded", "cluster_id", cluster.ID, "error", err)
			continue
		}
		nodes, err := r.repository.ListNodes(ctx, cluster.ID)
		if err != nil {
			return err
		}
		for _, node := range nodes {
			if !node.Enabled {
				continue
			}
			if node.MaintenanceMode {
				_ = r.repository.UpdateNodeConvergence(ctx, node.ID, "maintenance", r.now().UTC())
				continue
			}
			if node.CompatibilityStatus != domain.CompatibilitySupported || configuration.IsLegacyConverted(revision.Document.Unsupported) {
				_ = r.repository.UpdateNodeConvergence(ctx, node.ID, "unsupported", r.now().UTC())
				continue
			}
			if err := r.evaluateNode(ctx, cluster, revision, node); err != nil {
				r.logger.Error("node reconciliation failed", "cluster_id", cluster.ID, "node_id", node.ID, "error", err)
			}
		}
	}
	return nil
}

func (r *Reconciler) evaluateNode(ctx context.Context, cluster domain.Cluster, revision Revision, node domain.Node) error {
	effective, err := configuration.Effective(revision.Document, node.ID)
	if err != nil {
		return err
	}
	_, desiredHash, err := configuration.Marshal(effective)
	if err != nil {
		return err
	}
	snapshot, err := r.observer.Observe(ctx, node.ID)
	if err != nil || snapshot.Document == nil {
		_ = r.repository.UpdateNodeConvergence(ctx, node.ID, "observation_failed", r.now().UTC())
		return err
	}
	projected := configuration.ProjectDocument(*snapshot.Document, revision.SchemaVersion)
	differences := managedDifferences(effective, projected)
	requestID, err := domain.NewID()
	if err != nil {
		return err
	}
	now := r.now().UTC()
	if len(differences) == 0 {
		event, err := systemAudit("drift.resolved", "node", node.ID, requestID, map[string]any{"clusterId": cluster.ID, "revisionId": revision.ID, "resolution": "observed_converged"}, now)
		if err != nil {
			return err
		}
		if _, err := r.repository.ResolveNodeDrift(ctx, node.ID, "observed_converged", now, event); err != nil {
			return err
		}
		return r.repository.UpdateNodeConvergence(ctx, node.ID, "converged", now)
	}
	if err := r.repository.UpdateNodeConvergence(ctx, node.ID, "drifted", now); err != nil {
		return err
	}
	id, err := domain.NewID()
	if err != nil {
		return err
	}
	reconciliationStatus := "pending"
	if cluster.ReconciliationPolicy == domain.ReconciliationAlert {
		reconciliationStatus = "alerted"
	}
	item := DriftEvent{
		ID: id, ClusterID: cluster.ID, NodeID: node.ID, DesiredRevisionID: revision.ID,
		DesiredHash: desiredHash, ObservedSnapshotID: snapshot.ID, ObservedHash: snapshot.CanonicalHash,
		Fingerprint: DriftFingerprint(revision.ID, desiredHash, snapshot.CanonicalHash), Status: "open",
		Policy: string(cluster.ReconciliationPolicy), ReconciliationStatus: reconciliationStatus, Differences: differences,
		DetectedAt: now, LastSeenAt: now,
	}
	event, err := systemAudit("drift.detected", "drift_event", id, requestID, map[string]any{"clusterId": cluster.ID, "nodeId": node.ID, "revisionId": revision.ID, "policy": cluster.ReconciliationPolicy, "differenceCount": len(differences)}, now)
	if err != nil {
		return err
	}
	stored, _, err := r.repository.UpsertDriftEvent(ctx, item, event)
	if err != nil {
		return err
	}
	if cluster.ReconciliationPolicy != domain.ReconciliationEnforce || stored.ReconciliationStatus != "pending" {
		return nil
	}
	deployment, err := r.service.StartReconciliation(ctx, cluster.ID, revision.ID, node.ID)
	if err != nil {
		_ = r.repository.UpdateDriftReconciliation(ctx, stored.ID, "failed", nil)
		return err
	}
	return r.repository.UpdateDriftReconciliation(ctx, stored.ID, "enforcing", &deployment.ID)
}
