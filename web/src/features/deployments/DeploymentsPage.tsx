import { useCallback, useEffect, useMemo, useState } from "react";
import { DataTable, type DataTableColumn } from "../../components/DataDisplay";
import { Banner, ErrorState, Loading } from "../../components/Feedback";
import { PageHeader } from "../../components/Page";
import { StatusBadge, type StatusKind } from "../../components/StatusBadge";
import { api } from "../../lib/api";
import type {
  Cluster,
  ConfigurationRevision,
  Deployment,
  Node,
} from "../../lib/types";
import { useQuerySelection } from "../../lib/useQuerySelection";

const activeStates = new Set(["queued", "validating", "running"]);

export function DeploymentsPage({ cluster }: { cluster: Cluster }) {
  const [deployments, setDeployments] = useState<Deployment[]>();
  const [nodes, setNodes] = useState<Node[]>([]);
  const [revisions, setRevisions] = useState<ConfigurationRevision[]>([]);
  const [detailErrors, setDetailErrors] = useState<
    ReadonlyMap<string, unknown>
  >(new Map());
  const { selectedID, select, toggle, scrollIntoViewOnce } =
    useQuerySelection("deploymentId");
  const [busy, setBusy] = useState("");
  const [error, setError] = useState<unknown>();
  const [includeArchived, setIncludeArchived] = useState(false);

  const load = useCallback(async () => {
    try {
      const [deploymentResult, nodeResult, revisionResult] = await Promise.all([
        api.deployments(cluster.id, includeArchived),
        api.nodes(cluster.id),
        api.configurationRevisions(cluster.id, true),
      ]);
      const detailResults = await Promise.allSettled(
        deploymentResult.items
          .slice(0, 20)
          .map((item) => api.deployment(item.id)),
      );
      const failedDetails = new Map<string, unknown>();
      const detailed = deploymentResult.items
        .slice(0, 20)
        .map((item, index) => {
          const result = detailResults[index];
          if (result?.status === "fulfilled") return result.value;
          failedDetails.set(
            item.id,
            result?.reason ?? new Error("Deployment detail is unavailable."),
          );
          return item;
        });
      setDeployments(detailed);
      setDetailErrors(failedDetails);
      setNodes(nodeResult.items);
      setRevisions(revisionResult.items);
      setError(undefined);
    } catch (caught) {
      setError(caught);
    }
  }, [cluster.id, includeArchived]);

  useEffect(() => {
    void load();
    const timer = window.setInterval(() => void load(), 3000);
    return () => window.clearInterval(timer);
  }, [load]);

  const nodeNames = useMemo(
    () => new Map(nodes.map((node) => [node.id, node.name])),
    [nodes],
  );
  const revisionByID = useMemo(
    () => new Map(revisions.map((revision) => [revision.id, revision])),
    [revisions],
  );
  const active = deployments?.find((deployment) =>
    activeStates.has(deployment.status),
  );
  const selected = deployments?.find(
    (deployment) => deployment.id === selectedID,
  );
  const invalidSelection =
    deployments !== undefined && selectedID !== "" && selected === undefined;

  useEffect(() => {
    if (deployments !== undefined && selectedID === "" && active !== undefined)
      select(active.id, { replace: true });
  }, [active, deployments, select, selectedID]);

  useEffect(() => {
    if (selected)
      scrollIntoViewOnce(selected.id, deploymentSummaryID(selected.id));
  }, [scrollIntoViewOnce, selected]);

  async function cancel(deployment: Deployment) {
    if (
      !window.confirm(
        "Request cancellation at the next safe node boundary? The current node operation may finish first.",
      )
    )
      return;
    setBusy(deployment.id);
    try {
      await api.cancelDeployment(deployment.id);
      await load();
    } catch (caught) {
      setError(caught);
    } finally {
      setBusy("");
    }
  }

  async function archiveDeployment(deployment: Deployment) {
    if (
      !window.confirm(
        "Archive this completed deployment? Its revision, per-node results, verification evidence, and audit history remain intact.",
      )
    )
      return;
    setBusy(`lifecycle-${deployment.id}`);
    try {
      await api.archiveDeployment(deployment.id);
      await load();
    } catch (caught) {
      setError(caught);
    } finally {
      setBusy("");
    }
  }

  async function restoreDeployment(deployment: Deployment) {
    setBusy(`lifecycle-${deployment.id}`);
    try {
      await api.restoreDeployment(deployment.id);
      await load();
    } catch (caught) {
      setError(caught);
    } finally {
      setBusy("");
    }
  }

  async function deleteDeployment(deployment: Deployment) {
    const phrase = `DELETE DEPLOYMENT ${deployment.id}`;
    const confirmation = window.prompt(
      `This queued deployment has not started. Type ${phrase} to permanently delete it.`,
    );
    if (confirmation === null) return;
    setBusy(`lifecycle-${deployment.id}`);
    try {
      await api.deleteDeployment(deployment.id, confirmation);
      await load();
    } catch (caught) {
      setError(caught);
    } finally {
      setBusy("");
    }
  }

  if (deployments === undefined && error === undefined)
    return <Loading label="Loading deployments…" />;
  if (deployments === undefined)
    return <ErrorState error={error} retry={() => void load()} />;

  const columns: DataTableColumn<Deployment>[] = [
    {
      id: "deployment",
      header: "Deployment",
      render: (deployment) => (
        <code id={deploymentSummaryID(deployment.id)}>
          {shortID(deployment.id)}
        </code>
      ),
    },
    {
      id: "revision",
      header: "Revision",
      render: (deployment) => {
        const revision = revisionByID.get(deployment.revisionId);
        return revision
          ? `#${revision.revisionNumber}`
          : shortID(deployment.revisionId);
      },
    },
    {
      id: "requested",
      header: "Requested",
      render: (deployment) => formatTime(deployment.requestedAt),
    },
    {
      id: "progress",
      header: "Progress",
      render: (deployment) => {
        const complete = completedTasks(deployment);
        return `${complete} of ${deployment.nodes.length}`;
      },
    },
    {
      id: "status",
      header: "Result",
      render: (deployment) => (
        <StatusBadge
          status={deploymentStatus(deployment.status)}
          label={deployment.status.replaceAll("_", " ")}
        />
      ),
    },
    {
      id: "actions",
      header: <span className="visually-hidden">Details</span>,
      align: "right",
      render: (deployment) => {
        const expanded = deployment.id === selectedID;
        return (
          <button
            className="table-disclosure"
            type="button"
            aria-expanded={expanded}
            aria-controls={deploymentDetailID(deployment.id)}
            aria-label={`${expanded ? "Hide" : "View"} deployment ${shortID(deployment.id)} details`}
            onClick={() => toggle(deployment.id)}
          >
            <span aria-hidden="true">{expanded ? "−" : "+"}</span>
          </button>
        );
      },
    },
  ];

  return (
    <>
      <PageHeader
        eyebrow="Execution events"
        title="Deployments"
        description="Track sequential node mutation and read-back verification in one operational list."
        secondaryActions={
          <button
            className="button button--secondary"
            type="button"
            aria-pressed={includeArchived}
            onClick={() => setIncludeArchived((value) => !value)}
          >
            {includeArchived ? "Hide archived" : "Show archived"}
          </button>
        }
      />
      {error !== undefined && (
        <ErrorState error={error} retry={() => void load()} />
      )}
      {invalidSelection && (
        <Banner tone="warning" title="Deployment unavailable">
          The requested deployment is not in the current operational list.
        </Banner>
      )}
      {active !== undefined && (
        <Banner
          tone="info"
          title={`Active deployment ${shortID(active.id)}`}
          actions={
            <button
              className="button button--secondary"
              type="button"
              onClick={() => select(active.id)}
            >
              View active deployment
            </button>
          }
        >
          <StatusBadge
            status={deploymentStatus(active.status)}
            label={active.status.replaceAll("_", " ")}
          />{" "}
          {completedTasks(active)} of {active.nodes.length} node tasks complete.
          Details continue updating every 3 seconds.
        </Banner>
      )}
      <section className="section-block">
        <div className="section-heading">
          <h2>All deployments</h2>
          <small>Active and historical · refreshing every 3 seconds</small>
        </div>
        <DataTable
          caption="Active and historical deployments"
          columns={columns}
          rows={deployments}
          rowKey={(deployment) => deployment.id}
          expandedRowKey={selected?.id}
          expandedRowId={(deployment) => deploymentDetailID(deployment.id)}
          renderExpandedRow={(deployment) => (
            <DeploymentDetail
              deployment={deployment}
              revision={revisionByID.get(deployment.revisionId)}
              nodeNames={nodeNames}
              error={detailErrors.get(deployment.id)}
              busy={busy}
              onCancel={cancel}
              onArchive={archiveDeployment}
              onRestore={restoreDeployment}
              onDelete={deleteDeployment}
              onRetry={load}
            />
          )}
          emptyTitle="No deployments"
          emptyDescription={
            <p>
              Publish a revision in Configuration Control, then review and
              deploy it from Revisions.
            </p>
          }
        />
      </section>
    </>
  );
}

function DeploymentDetail({
  deployment,
  revision,
  nodeNames,
  error,
  busy,
  onCancel,
  onArchive,
  onRestore,
  onDelete,
  onRetry,
}: {
  deployment: Deployment;
  revision?: ConfigurationRevision;
  nodeNames: ReadonlyMap<string, string>;
  error?: unknown;
  busy: string;
  onCancel: (deployment: Deployment) => Promise<void>;
  onArchive: (deployment: Deployment) => Promise<void>;
  onRestore: (deployment: Deployment) => Promise<void>;
  onDelete: (deployment: Deployment) => Promise<void>;
  onRetry: () => Promise<void>;
}) {
  const complete = completedTasks(deployment);
  const current = deployment.nodes.find(
    (node) =>
      !["succeeded", "failed", "skipped", "cancelled"].includes(node.status),
  );
  return (
    <article
      className="inline-operational-detail"
      aria-labelledby={`deployment-${deployment.id}-heading`}
    >
      <div className="section-heading">
        <div>
          <h3 id={`deployment-${deployment.id}-heading`}>
            {revision
              ? `Revision #${revision.revisionNumber}`
              : `Revision ${shortID(deployment.revisionId)}`}
          </h3>
          <p className="muted">{revision?.summary ?? deployment.origin}</p>
        </div>
        <StatusBadge
          status={deploymentStatus(deployment.status)}
          label={deployment.status.replaceAll("_", " ")}
        />
        {deployment.archivedAt && (
          <StatusBadge status="disabled" label="Archived" />
        )}
      </div>
      {error !== undefined && (
        <ErrorState
          error={error}
          title="Unable to refresh this deployment detail"
          retry={() => void onRetry()}
        />
      )}
      <p>
        <strong>
          {complete} of {deployment.nodes.length} node tasks complete
        </strong>
      </p>
      <progress max={Math.max(deployment.nodes.length, 1)} value={complete}>
        {complete} of {deployment.nodes.length}
      </progress>
      <dl className="summary-grid">
        <div>
          <dt>Deployment ID</dt>
          <dd>
            <code>{deployment.id}</code>
          </dd>
        </div>
        <div>
          <dt>Requested by</dt>
          <dd>
            <code>{deployment.requestedBy ?? "system"}</code>
          </dd>
        </div>
        <div>
          <dt>Requested</dt>
          <dd>{formatTime(deployment.requestedAt)}</dd>
        </div>
        <div>
          <dt>Started</dt>
          <dd>{formatTime(deployment.startedAt)}</dd>
        </div>
        <div>
          <dt>Completed</dt>
          <dd>{formatTime(deployment.completedAt)}</dd>
        </div>
        <div>
          <dt>Current operation</dt>
          <dd>
            {current
              ? `${nodeNames.get(current.nodeId) ?? current.nodeId}: ${current.status.replaceAll("_", " ")}`
              : "No operation running"}
          </dd>
        </div>
        <div>
          <dt>Strategy</dt>
          <dd>Sequential · stop on failure</dd>
        </div>
        <div>
          <dt>Read-back verification</dt>
          <dd>Required per changed node</dd>
        </div>
        <div>
          <dt>Cancellation</dt>
          <dd>{deployment.cancelRequested ? "Requested" : "Not requested"}</dd>
        </div>
        <div>
          <dt>Request ID</dt>
          <dd>
            <code>{deployment.requestId}</code>
          </dd>
        </div>
      </dl>
      {deployment.errorCode && (
        <Banner tone="warning" title={deployment.errorCode}>
          The deployment did not complete normally. Review the node results
          below.
        </Banner>
      )}
      <section className="inline-detail-section">
        <h4>Ordered node results</h4>
        <ol className="progress-list">
          {deployment.nodes.map((task) => (
            <li key={task.id}>
              <div className="section-heading">
                <strong>
                  {task.position}. {nodeNames.get(task.nodeId) ?? task.nodeId}
                </strong>
                <StatusBadge
                  status={taskStatus(task.status)}
                  label={task.status.replaceAll("_", " ")}
                />
              </div>
              <p className="muted">
                Attempts: {task.attemptCount} · Verification:{" "}
                {task.verificationSnapshotId ? "recorded" : "not recorded"}
              </p>
              {task.errorCode && (
                <p>
                  <strong>{task.errorCode}</strong>
                  {task.errorMessage ? ` — ${task.errorMessage}` : ""}
                </p>
              )}
            </li>
          ))}
        </ol>
      </section>
      <div className="row-actions row-actions--start">
        {activeStates.has(deployment.status) && (
          <button
            className="button button--danger"
            type="button"
            disabled={busy !== ""}
            onClick={() => void onCancel(deployment)}
          >
            {busy === deployment.id ? "Requesting…" : "Cancel at safe boundary"}
          </button>
        )}
        <a
          className="button button--secondary"
          href={`/ha/revisions?revisionId=${encodeURIComponent(deployment.revisionId)}`}
        >
          View revision
        </a>
        {deployment.status === "failed" && (
          <a className="button button--secondary" href="/ha/drift">
            View resulting drift
          </a>
        )}
      </div>
      <section className="inline-detail-section">
        <h4>Lifecycle</h4>
        <p className="muted">
          Archive hides terminal operational history from the default list.
          Permanent deletion is limited to a server-verified deployment that
          never started.
        </p>
        <div className="row-actions row-actions--start">
          {deployment.lifecycle.canArchive && (
            <button
              className="button button--secondary"
              type="button"
              disabled={busy !== ""}
              onClick={() => void onArchive(deployment)}
            >
              Archive Deployment
            </button>
          )}
          {deployment.lifecycle.canRestore && (
            <button
              className="button button--secondary"
              type="button"
              disabled={busy !== ""}
              onClick={() => void onRestore(deployment)}
            >
              Restore from Archive
            </button>
          )}
          {deployment.lifecycle.canDelete && (
            <button
              className="button button--danger"
              type="button"
              disabled={busy !== ""}
              onClick={() => void onDelete(deployment)}
            >
              Delete Unstarted Deployment
            </button>
          )}
        </div>
      </section>
    </article>
  );
}

function completedTasks(deployment: Deployment) {
  return deployment.nodes.filter((node) =>
    ["succeeded", "failed", "skipped", "cancelled"].includes(node.status),
  ).length;
}
function deploymentStatus(status: string): StatusKind {
  if (status === "succeeded") return "success";
  if (status.includes("failed")) return "failed";
  if (status.includes("cancel")) return "warning";
  return "applying";
}
function taskStatus(status: string): StatusKind {
  if (status === "succeeded") return "success";
  if (status === "failed") return "failed";
  if (status === "verifying") return "verifying";
  if (status === "running") return "applying";
  return "pending";
}
function deploymentSummaryID(id: string) {
  return `deployment-summary-${id}`;
}
function deploymentDetailID(id: string) {
  return `deployment-detail-${id}`;
}
function shortID(value: string): string {
  return value.length > 12 ? `${value.slice(0, 8)}…` : value;
}
function formatTime(value?: string): string {
  return value ? new Date(value).toLocaleString() : "—";
}
