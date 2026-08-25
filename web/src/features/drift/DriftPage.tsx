import { useCallback, useEffect, useMemo, useState } from "react";
import {
  DataTable,
  type DataTableColumn,
  StructuredDiff,
} from "../../components/DataDisplay";
import { Banner, ErrorState, Loading } from "../../components/Feedback";
import { PageHeader } from "../../components/Page";
import { StatusBadge } from "../../components/StatusBadge";
import { api } from "../../lib/api";
import { navigateTo } from "../../lib/browserNavigation";
import type {
  Cluster,
  ConfigurationDifference,
  DriftEvent,
  Node,
} from "../../lib/types";
import { useQuerySelection } from "../../lib/useQuerySelection";
import { nodeDetailPath } from "../../routing/routes";

export function DriftPage({ cluster }: { cluster: Cluster }) {
  const [drift, setDrift] = useState<DriftEvent[]>();
  const [nodes, setNodes] = useState<Node[]>([]);
  const [draftVersion, setDraftVersion] = useState(0);
  const [policy, setPolicy] = useState(cluster.reconciliationPolicy);
  const [clusterVersion, setClusterVersion] = useState(cluster.version);
  const { selectedID, toggle, scrollIntoViewOnce } =
    useQuerySelection("driftId");
  const [busy, setBusy] = useState("");
  const [error, setError] = useState<unknown>();

  const load = useCallback(async () => {
    try {
      const [driftResult, nodeResult, inventory] = await Promise.all([
        api.driftEvents(cluster.id),
        api.nodes(cluster.id),
        api.configurationInventory(cluster.id),
      ]);
      setDrift(driftResult.items);
      setNodes(nodeResult.items);
      setDraftVersion(inventory.draft?.version ?? 0);
      setError(undefined);
    } catch (caught) {
      setError(caught);
    }
  }, [cluster.id]);

  useEffect(() => {
    void load();
    const timer = window.setInterval(() => void load(), 10000);
    return () => window.clearInterval(timer);
  }, [load]);

  const nodeByID = useMemo(
    () => new Map(nodes.map((node) => [node.id, node])),
    [nodes],
  );
  const counts = useMemo(
    () => ({
      converged: nodes.filter(
        (node) =>
          node.convergenceStatus === "converged" && !node.maintenanceMode,
      ).length,
      drifted: nodes.filter(
        (node) => node.convergenceStatus === "drifted" && !node.maintenanceMode,
      ).length,
      unavailable: nodes.filter((node) => node.healthStatus === "unreachable")
        .length,
      maintenance: nodes.filter((node) => node.maintenanceMode).length,
      unknown: nodes.filter(
        (node) =>
          ["pending", "observation_failed", "unsupported"].includes(
            node.convergenceStatus,
          ) && !node.maintenanceMode,
      ).length,
    }),
    [nodes],
  );
  const selected = drift?.find((item) => item.id === selectedID);
  const invalidSelection =
    drift !== undefined && selectedID !== "" && selected === undefined;

  useEffect(() => {
    if (selected) scrollIntoViewOnce(selected.id, driftSummaryID(selected.id));
  }, [scrollIntoViewOnce, selected]);

  async function updatePolicy(value: Cluster["reconciliationPolicy"]) {
    setBusy("policy");
    try {
      const updated = await api.updateCluster({
        ...cluster,
        version: clusterVersion,
        reconciliationPolicy: value,
      });
      setPolicy(updated.reconciliationPolicy);
      setClusterVersion(updated.version);
      setError(undefined);
    } catch (caught) {
      setError(caught);
    } finally {
      setBusy("");
    }
  }

  async function restore(item: DriftEvent) {
    if (
      !window.confirm(
        "Restore desired state through a new durable, sequential, verified deployment? This deploys the controller-managed configuration.",
      )
    )
      return;
    setBusy(item.id);
    try {
      const deployment = await api.restoreDrift(item.id);
      navigateTo(
        `/ha/deployments?deploymentId=${encodeURIComponent(deployment.id)}`,
      );
    } catch (caught) {
      setError(caught);
      setBusy("");
    }
  }

  async function adopt(item: DriftEvent) {
    if (
      !window.confirm(
        "Adopt this observed configuration into the mutable draft only? You must still review, validate, publish, and deploy it.",
      )
    )
      return;
    setBusy(item.id);
    try {
      await api.adoptDrift(item.id, draftVersion);
      navigateTo("/ha/configuration#advanced-adoption");
    } catch (caught) {
      setError(caught);
      setBusy("");
    }
  }

  if (drift === undefined && error === undefined)
    return <Loading label="Loading convergence state…" />;
  if (drift === undefined)
    return <ErrorState error={error} retry={() => void load()} />;
  const open = drift.filter((item) => item.status === "open");
  const columns: DataTableColumn<DriftEvent>[] = [
    {
      id: "node",
      header: "Node",
      render: (item) => (
        <span id={driftSummaryID(item.id)}>
          {nodeByID.get(item.nodeId)?.name ?? item.nodeId}
        </span>
      ),
    },
    {
      id: "state",
      header: "State",
      render: (item) => (
        <StatusBadge
          status={item.status === "open" ? "drifted" : "converged"}
          label={`${item.status} · ${item.policy}`}
        />
      ),
    },
    {
      id: "differences",
      header: "Differences",
      render: (item) => item.differences.length,
    },
    {
      id: "revision",
      header: "Desired revision",
      render: (item) => (
        <a
          href={`/ha/revisions?revisionId=${encodeURIComponent(item.desiredRevisionId)}`}
        >
          <code>{shortID(item.desiredRevisionId)}</code>
        </a>
      ),
    },
    {
      id: "detected",
      header: "First detected",
      render: (item) => formatTime(item.detectedAt),
    },
    {
      id: "observed",
      header: "Last observed",
      render: (item) => formatTime(item.lastSeenAt),
    },
    {
      id: "actions",
      header: <span className="visually-hidden">Details</span>,
      align: "right",
      render: (item) => {
        const expanded = item.id === selectedID;
        return (
          <button
            className="table-disclosure"
            type="button"
            aria-expanded={expanded}
            aria-controls={driftDetailID(item.id)}
            aria-label={`${expanded ? "Hide" : "View"} drift incident details for ${nodeByID.get(item.nodeId)?.name ?? item.nodeId}`}
            onClick={() => toggle(item.id)}
          >
            <span aria-hidden="true">{expanded ? "⌃" : "⌄"}</span>
          </button>
        );
      },
    },
  ];

  return (
    <>
      <PageHeader
        eyebrow="HA Controller"
        title="Drift"
        description="Investigate desired-versus-observed differences and choose a deliberate restore or adoption response; node operations remain on Node Detail."
      />
      {error !== undefined && (
        <ErrorState error={error} retry={() => void load()} />
      )}
      {invalidSelection && (
        <Banner tone="warning" title="Drift incident unavailable">
          The requested incident is not in the current incident list.
        </Banner>
      )}
      <section
        className="section-block convergence-summary"
        aria-label="Cluster convergence summary"
      >
        <div>
          <StatusBadge status={open.length > 0 ? "drifted" : "converged"} />
          <strong>
            {open.length > 0
              ? `${open.length} open drift incidents`
              : "Cluster has no open drift incidents"}
          </strong>
        </div>
        <dl>
          <div>
            <dt>Converged nodes</dt>
            <dd>{counts.converged}</dd>
          </div>
          <div>
            <dt>Drifted nodes</dt>
            <dd>{counts.drifted}</dd>
          </div>
          <div>
            <dt>Unavailable nodes</dt>
            <dd>{counts.unavailable}</dd>
          </div>
          <div>
            <dt>Maintenance nodes</dt>
            <dd>{counts.maintenance}</dd>
          </div>
          <div>
            <dt>Unknown nodes</dt>
            <dd>{counts.unknown}</dd>
          </div>
        </dl>
      </section>
      <section className="section-block">
        <div className="section-heading">
          <h2>Drift incidents</h2>
          <small>
            {open.length} open · {drift.length - open.length} resolved
          </small>
        </div>
        <DataTable
          caption="Desired-versus-observed drift incidents"
          columns={columns}
          rows={drift}
          rowKey={(item) => item.id}
          expandedRowKey={selected?.id}
          expandedRowId={(item) => driftDetailID(item.id)}
          renderExpandedRow={(item) => (
            <DriftIncidentDetail
              item={item}
              node={nodeByID.get(item.nodeId)}
              busy={busy}
              draftVersion={draftVersion}
              onRestore={restore}
              onAdopt={adopt}
            />
          )}
          emptyTitle="No drift detected"
          emptyDescription={
            <p>
              Drift evaluation begins after a revision has fully verified and
              become active.
            </p>
          }
        />
      </section>
      <section className="section-block">
        <details className="card">
          <summary>
            <strong>Drift policy</strong> · {policy}
          </summary>
          <div className="form-stack">
            <p className="muted">
              Policy is cluster-wide. Maintenance nodes are always excluded.
            </p>
            <label>
              Reconciliation policy
              <select
                value={policy}
                disabled={busy !== ""}
                onChange={(event) =>
                  void updatePolicy(
                    event.target.value as Cluster["reconciliationPolicy"],
                  )
                }
              >
                <option value="manual">Manual — operator chooses</option>
                <option value="alert">Alert — record without mutation</option>
                <option value="enforce">Enforce — automatically restore</option>
              </select>
            </label>
          </div>
        </details>
      </section>
    </>
  );
}

function DriftIncidentDetail({
  item,
  node,
  busy,
  draftVersion,
  onRestore,
  onAdopt,
}: {
  item: DriftEvent;
  node?: Node;
  busy: string;
  draftVersion: number;
  onRestore: (item: DriftEvent) => Promise<void>;
  onAdopt: (item: DriftEvent) => Promise<void>;
}) {
  return (
    <article
      className="inline-operational-detail"
      aria-labelledby={`drift-${item.id}-heading`}
    >
      <div className="section-heading">
        <div>
          <h3 id={`drift-${item.id}-heading`}>{node?.name ?? item.nodeId}</h3>
          <p className="muted">
            First detected {formatTime(item.detectedAt)} · last observed{" "}
            {formatTime(item.lastSeenAt)}
          </p>
        </div>
        <StatusBadge
          status={item.status === "open" ? "drifted" : "converged"}
          label={`${item.status} · ${item.policy}`}
        />
      </div>
      {node?.maintenanceMode && (
        <Banner tone="warning" title="Node in maintenance">
          Deployments and reconciliation exclude this node. Maintenance does not
          resolve the configuration difference.
        </Banner>
      )}
      <section className="inline-detail-section">
        <h4>Structured differences</h4>
        <StructuredDiff
          differences={toStructured(item.differences)}
          beforeLabel="Desired"
          afterLabel="Observed"
        />
      </section>
      <dl className="summary-grid">
        <div>
          <dt>Reconciliation</dt>
          <dd>{item.reconciliationStatus.replaceAll("_", " ")}</dd>
        </div>
        <div>
          <dt>Desired revision</dt>
          <dd>
            <code>{item.desiredRevisionId}</code>
          </dd>
        </div>
        <div>
          <dt>Desired hash</dt>
          <dd>
            <code>{item.desiredHash}</code>
          </dd>
        </div>
        <div>
          <dt>Observed hash</dt>
          <dd>
            <code>{item.observedHash}</code>
          </dd>
        </div>
        {item.resolvedAt && (
          <div>
            <dt>Resolved</dt>
            <dd>
              {formatTime(item.resolvedAt)} · {item.resolution ?? "resolved"}
            </dd>
          </div>
        )}
      </dl>
      <section className="inline-detail-section">
        <h4>Reconciliation actions</h4>
        <p>
          <strong>Restore desired state</strong> deploys the controller-managed
          configuration. <strong>Adopt into draft</strong> copies observed
          changes into editable controller state; it does not publish or deploy.
        </p>
        <div className="row-actions row-actions--start">
          {item.status === "open" && (
            <>
              <button
                className="button"
                type="button"
                disabled={busy !== "" || node?.maintenanceMode}
                onClick={() => void onRestore(item)}
              >
                Restore desired state
              </button>
              <button
                className="button button--secondary"
                type="button"
                disabled={busy !== "" || draftVersion === 0}
                onClick={() => void onAdopt(item)}
              >
                Adopt into draft
              </button>
            </>
          )}
          <a
            className="button button--secondary"
            href={nodeDetailPath(item.nodeId)}
          >
            View node
          </a>
          <a
            className="button button--quiet"
            href={`/ha/revisions?revisionId=${encodeURIComponent(item.desiredRevisionId)}`}
          >
            View desired revision
          </a>
          {item.relatedDeploymentId && (
            <a
              className="button button--quiet"
              href={`/ha/deployments?deploymentId=${encodeURIComponent(item.relatedDeploymentId)}`}
            >
              View deployment {shortID(item.relatedDeploymentId)}
            </a>
          )}
        </div>
      </section>
    </article>
  );
}

function toStructured(differences: ConfigurationDifference[]) {
  return differences.map((difference, index) => ({
    id: `${difference.section}-${difference.field}-${index}`,
    section: difference.section,
    field: difference.field,
    before: difference.left,
    after: difference.right,
    summary: difference.summary,
  }));
}
function driftSummaryID(id: string) {
  return `drift-summary-${id}`;
}
function driftDetailID(id: string) {
  return `drift-detail-${id}`;
}
function shortID(value: string): string {
  return value.length > 12 ? `${value.slice(0, 8)}…` : value;
}
function formatTime(value?: string): string {
  return value ? new Date(value).toLocaleString() : "—";
}
