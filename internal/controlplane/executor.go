package controlplane

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/benchristian88/atlas-dns/internal/configuration"
	"github.com/benchristian88/atlas-dns/internal/domain"
	"github.com/benchristian88/atlas-dns/internal/inventory"
)

type ExecutionRepository interface {
	ClaimDeployment(context.Context, time.Time) (string, error)
	DeploymentByID(context.Context, string) (Deployment, error)
	RevisionByID(context.Context, string) (Revision, error)
	NodeRecordByID(context.Context, string) (domain.NodeRecord, error)
	SetDeploymentRunning(context.Context, string) error
	UpdateDeploymentNode(context.Context, DeploymentNode) error
	MarkNodeApplied(context.Context, string, string, string, string, time.Time) error
	UpdateNodeConvergence(context.Context, string, string, time.Time) error
	FinishDeployment(context.Context, Deployment, bool, domain.AuditEvent) error
	InterruptDeployments(context.Context, time.Time) error
}

type NodeCredentialProtector interface {
	Decrypt(string, domain.EncryptedCredentials) (domain.NodeCredentials, error)
}

type ConfigurationWriter interface {
	ApplyConfiguration(context.Context, domain.NodeProbeRequest, configuration.Document) error
}

type Observer interface {
	Observe(context.Context, string) (inventory.Snapshot, error)
}

type Executor struct {
	repository  ExecutionRepository
	credentials NodeCredentialProtector
	writer      ConfigurationWriter
	observer    Observer
	now         func() time.Time
}

func NewExecutor(repository ExecutionRepository, credentials NodeCredentialProtector, writer ConfigurationWriter, observer Observer) *Executor {
	return &Executor{repository: repository, credentials: credentials, writer: writer, observer: observer, now: time.Now}
}

func (e *Executor) RecoverInterrupted(ctx context.Context) error {
	return e.repository.InterruptDeployments(ctx, e.now().UTC())
}

func (e *Executor) RunOnce(ctx context.Context) (bool, error) {
	id, err := e.repository.ClaimDeployment(ctx, e.now().UTC())
	if err != nil || id == "" {
		return false, err
	}
	return true, e.execute(ctx, id)
}

type preparedNode struct {
	task      DeploymentNode
	record    domain.NodeRecord
	effective configuration.Document
}

func (e *Executor) execute(ctx context.Context, id string) error {
	deployment, err := e.repository.DeploymentByID(ctx, id)
	if err != nil {
		return err
	}
	if deployment.CancelRequested || deployment.Status == "cancelling" {
		return e.cancel(ctx, &deployment, 0)
	}
	revision, err := e.repository.RevisionByID(ctx, deployment.RevisionID)
	if err != nil {
		return e.failBeforeMutation(ctx, &deployment, "REVISION_UNAVAILABLE", err)
	}
	prepared := make([]preparedNode, 0, len(deployment.Nodes))
	// Revalidate every target immediately before the first mutation.
	for index := range deployment.Nodes {
		task := deployment.Nodes[index]
		record, err := e.repository.NodeRecordByID(ctx, task.NodeID)
		if err != nil || !record.Node.Enabled || record.Node.MaintenanceMode || record.Node.CompatibilityStatus != domain.CompatibilitySupported {
			return e.failBeforeMutation(ctx, &deployment, "TARGET_NOT_MUTABLE", errors.New("a target is disabled, unavailable, or in maintenance"))
		}
		if configuration.IsLegacyConverted(revision.Document.Unsupported) {
			return e.failBeforeMutation(ctx, &deployment, "LEGACY_SCHEMA_NOT_DEPLOYABLE", errors.New("legacy schema-1 records must be imported and published as schema 2 before deployment"))
		}
		effective, err := configuration.Effective(revision.Document, task.NodeID)
		if err != nil {
			return e.failBeforeMutation(ctx, &deployment, "EFFECTIVE_CONFIGURATION_INVALID", err)
		}
		snapshot, err := e.observer.Observe(ctx, task.NodeID)
		if err != nil || snapshot.Document == nil {
			return e.failBeforeMutation(ctx, &deployment, "TARGET_OBSERVATION_FAILED", err)
		}
		projected := configuration.ProjectDocument(*snapshot.Document, revision.SchemaVersion)
		if len(immutableNodeDifferences(effective, projected)) != 0 {
			return e.failBeforeMutation(ctx, &deployment, "LISTENER_MUTATION_UNSUPPORTED", errors.New("node listener identity changed after preview"))
		}
		prepared = append(prepared, preparedNode{task: task, record: record, effective: effective})
	}
	if err := e.repository.SetDeploymentRunning(ctx, deployment.ID); err != nil {
		return err
	}
	succeeded := 0
	for index := range prepared {
		fresh, err := e.repository.DeploymentByID(ctx, deployment.ID)
		if err != nil {
			return err
		}
		if fresh.CancelRequested {
			deployment = fresh
			return e.cancel(ctx, &deployment, index)
		}
		item := &prepared[index]
		now := e.now().UTC()
		item.task.Status, item.task.AttemptCount, item.task.StartedAt = "applying", item.task.AttemptCount+1, &now
		if err := e.repository.UpdateDeploymentNode(ctx, item.task); err != nil {
			return err
		}
		if err := e.repository.UpdateNodeConvergence(ctx, item.task.NodeID, "applying", now); err != nil {
			return err
		}
		credentials, err := e.credentials.Decrypt(item.task.NodeID, item.record.Secrets.Credentials)
		if err != nil {
			return e.failNode(ctx, &deployment, prepared, index, succeeded, "CREDENTIAL_DECRYPTION_FAILED", errors.New("stored node credentials could not be decrypted"))
		}
		request := domain.NodeProbeRequest{BaseURL: item.record.Node.BaseURL, CertificatePolicy: item.record.Node.CertificatePolicy, CustomCAPEM: item.record.Secrets.CustomCAPEM, Credentials: credentials}
		if err := e.writer.ApplyConfiguration(ctx, request, item.effective); err != nil {
			return e.failNode(ctx, &deployment, prepared, index, succeeded, errorCode(err), err)
		}
		item.task.Status = "verifying"
		if err := e.repository.UpdateDeploymentNode(ctx, item.task); err != nil {
			return err
		}
		if err := e.repository.UpdateNodeConvergence(ctx, item.task.NodeID, "verifying", e.now().UTC()); err != nil {
			return err
		}
		snapshot, err := e.observer.Observe(ctx, item.task.NodeID)
		if err != nil || snapshot.Document == nil {
			return e.failNode(ctx, &deployment, prepared, index, succeeded, "VERIFICATION_OBSERVATION_FAILED", err)
		}
		projected := configuration.ProjectDocument(*snapshot.Document, revision.SchemaVersion)
		if differences := managedDifferences(item.effective, projected); len(differences) != 0 {
			return e.failNode(ctx, &deployment, prepared, index, succeeded, string(domain.ErrorVerification), fmt.Errorf("read-back produced %d semantic differences", len(differences)))
		}
		completed := e.now().UTC()
		item.task.Status, item.task.CompletedAt, item.task.VerificationSnapshotID = "succeeded", &completed, &snapshot.ID
		if err := e.repository.UpdateDeploymentNode(ctx, item.task); err != nil {
			return err
		}
		if err := e.repository.MarkNodeApplied(ctx, item.task.NodeID, revision.ID, item.task.EffectiveHash, snapshot.ID, completed); err != nil {
			return err
		}
		succeeded++
	}
	completed := e.now().UTC()
	deployment.Status, deployment.CompletedAt = "succeeded", &completed
	event, err := systemAudit("deployment.succeeded", "deployment", deployment.ID, deployment.RequestID, map[string]any{"clusterId": deployment.ClusterID, "revisionId": deployment.RevisionID}, completed)
	if err != nil {
		return err
	}
	return e.repository.FinishDeployment(ctx, deployment, deployment.Origin != "reconciliation", event)
}

func (e *Executor) failBeforeMutation(ctx context.Context, deployment *Deployment, code string, cause error) error {
	now := e.now().UTC()
	for index := range deployment.Nodes {
		task := deployment.Nodes[index]
		task.Status, task.CompletedAt, task.ErrorCode = "skipped", &now, code
		task.ErrorMessage = safeFailure(cause)
		if err := e.repository.UpdateDeploymentNode(ctx, task); err != nil {
			return err
		}
	}
	deployment.Status, deployment.ErrorCode, deployment.CompletedAt = "failed", code, &now
	event, err := systemAudit("deployment.failed", "deployment", deployment.ID, deployment.RequestID, map[string]any{"errorCode": code, "mutatedNodes": 0}, now)
	if err != nil {
		return err
	}
	return e.repository.FinishDeployment(ctx, *deployment, false, event)
}

func (e *Executor) failNode(ctx context.Context, deployment *Deployment, prepared []preparedNode, failedIndex, succeeded int, code string, cause error) error {
	now := e.now().UTC()
	failed := prepared[failedIndex].task
	failed.Status, failed.CompletedAt, failed.ErrorCode, failed.ErrorMessage = "failed", &now, code, safeFailure(cause)
	if err := e.repository.UpdateDeploymentNode(ctx, failed); err != nil {
		return err
	}
	if err := e.repository.UpdateNodeConvergence(ctx, failed.NodeID, "apply_failed", now); err != nil {
		return err
	}
	for index := failedIndex + 1; index < len(prepared); index++ {
		task := prepared[index].task
		task.Status, task.CompletedAt, task.ErrorCode = "skipped", &now, "STOPPED_AFTER_FAILURE"
		if err := e.repository.UpdateDeploymentNode(ctx, task); err != nil {
			return err
		}
	}
	deployment.Status = "failed"
	if succeeded > 0 {
		deployment.Status = "partially_succeeded"
	}
	deployment.ErrorCode, deployment.CompletedAt = code, &now
	event, err := systemAudit("deployment."+deployment.Status, "deployment", deployment.ID, deployment.RequestID, map[string]any{"errorCode": code, "mutatedNodes": succeeded}, now)
	if err != nil {
		return err
	}
	return e.repository.FinishDeployment(ctx, *deployment, false, event)
}

func (e *Executor) cancel(ctx context.Context, deployment *Deployment, start int) error {
	now := e.now().UTC()
	for index := start; index < len(deployment.Nodes); index++ {
		task := deployment.Nodes[index]
		if task.Status == "pending" {
			task.Status, task.CompletedAt, task.ErrorCode = "skipped", &now, "CANCELLED_AT_SAFE_BOUNDARY"
			if err := e.repository.UpdateDeploymentNode(ctx, task); err != nil {
				return err
			}
		}
	}
	deployment.Status, deployment.CompletedAt = "cancelled", &now
	event, err := systemAudit("deployment.cancelled", "deployment", deployment.ID, deployment.RequestID, map[string]any{"safeBoundary": true}, now)
	if err != nil {
		return err
	}
	return e.repository.FinishDeployment(ctx, *deployment, false, event)
}

func errorCode(err error) string {
	var domainError *domain.Error
	if errors.As(err, &domainError) {
		return string(domainError.Kind)
	}
	return string(domain.ErrorInternal)
}

func safeFailure(err error) string {
	if err == nil {
		return "Operation failed without a safe diagnostic."
	}
	var domainError *domain.Error
	if errors.As(err, &domainError) {
		return domainError.Message
	}
	return "The operation failed; inspect controller logs using the deployment request ID."
}

func systemAudit(action, resourceType, resourceID, requestID string, metadata map[string]any, at time.Time) (domain.AuditEvent, error) {
	id, err := domain.NewID()
	if err != nil {
		return domain.AuditEvent{}, err
	}
	return domain.AuditEvent{ID: id, ActorType: "system", Action: action, ResourceType: resourceType, ResourceID: &resourceID, RequestID: requestID, Metadata: metadata, CreatedAt: at}, nil
}
