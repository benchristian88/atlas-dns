import { useCallback, useEffect, useMemo, useState } from "react";
import {
  DataTable,
  type DataTableColumn,
  RevisionBadge,
  StructuredDiff,
} from "../../components/DataDisplay";
import { Banner, ErrorState, Loading } from "../../components/Feedback";
import { Dialog } from "../../components/Overlays";
import { PageHeader } from "../../components/Page";
import { StatusBadge } from "../../components/StatusBadge";
import { api } from "../../lib/api";
import { navigateTo } from "../../lib/browserNavigation";
import type {
  Cluster,
  ConfigurationDifference,
  ConfigurationRevision,
  Deployment,
  DeploymentPreview,
} from "../../lib/types";
import { useQuerySelection } from "../../lib/useQuerySelection";

interface DeploymentReview {
  revision: ConfigurationRevision;
  preview?: DeploymentPreview;
  error?: unknown;
}

export function RevisionsPage({ cluster }: { cluster: Cluster }) {
  const [revisions, setRevisions] = useState<ConfigurationRevision[]>();
  const [deployments, setDeployments] = useState<Deployment[]>([]);
  const { selectedID, toggle, scrollIntoViewOnce } =
    useQuerySelection("revisionId");
  const [leftID, setLeftID] = useState("");
  const [rightID, setRightID] = useState("");
  const [differences, setDifferences] = useState<ConfigurationDifference[]>();
  const [detailDifferences, setDetailDifferences] =
    useState<ConfigurationDifference[]>();
  const [detailError, setDetailError] = useState<unknown>();
  const [review, setReview] = useState<DeploymentReview>();
  const [busy, setBusy] = useState("");
  const [error, setError] = useState<unknown>();
  const [includeArchived, setIncludeArchived] = useState(false);

  const load = useCallback(async () => {
    try {
      const [revisionResult, deploymentResult] = await Promise.all([
        api.configurationRevisions(cluster.id, includeArchived),
        api.deployments(cluster.id, true),
      ]);
      setRevisions(revisionResult.items);
      setDeployments(deploymentResult.items);
      setError(undefined);
    } catch (caught) {
      setError(caught);
    }
  }, [cluster.id, includeArchived]);

  useEffect(() => void load(), [load]);

  const selected = revisions?.find((revision) => revision.id === selectedID);
  const invalidSelection =
    revisions !== undefined && selectedID !== "" && selected === undefined;
  const preceding = selected
    ? revisions?.find(
        (revision) => revision.revisionNumber === selected.revisionNumber - 1,
      )
    : undefined;
  const deploymentsByRevision = useMemo(() => {
    const result = new Map<string, Deployment[]>();
    for (const deployment of deployments) {
      const items = result.get(deployment.revisionId) ?? [];
      items.push(deployment);
      result.set(deployment.revisionId, items);
    }
    return result;
  }, [deployments]);

  useEffect(() => {
    if (!selected || !preceding) {
      setDetailDifferences(undefined);
      setDetailError(undefined);
      return;
    }
    let current = true;
    setDetailDifferences(undefined);
    setDetailError(undefined);
    void api
      .compareConfigurationRevisions(preceding.id, selected.id)
      .then((result) => {
        if (current) setDetailDifferences(result.differences);
      })
      .catch((caught) => {
        if (current) setDetailError(caught);
      });
    return () => {
      current = false;
    };
  }, [preceding, selected]);

  useEffect(() => {
    if (selected)
      scrollIntoViewOnce(selected.id, revisionSummaryID(selected.id));
  }, [scrollIntoViewOnce, selected]);

  async function compare() {
    if (!leftID || !rightID) return;
    setBusy("compare");
    try {
      const result = await api.compareConfigurationRevisions(leftID, rightID);
      setDifferences(result.differences);
      setError(undefined);
    } catch (caught) {
      setError(caught);
    } finally {
      setBusy("");
    }
  }

  async function prepareDeployment(revision: ConfigurationRevision) {
    setReview({ revision });
    setBusy(`preview-${revision.id}`);
    try {
      const preview = await api.deploymentPreview(cluster.id, revision.id);
      setReview({ revision, preview });
    } catch (caught) {
      setReview({ revision, error: caught });
    } finally {
      setBusy("");
    }
  }

  async function confirmDeployment() {
    if (!review?.preview?.valid) return;
    setBusy(`deploy-${review.revision.id}`);
    try {
      const isRollback =
        Boolean(cluster.activeRevisionId) && !review.revision.active;
      const deployment = isRollback
        ? await api.rollback(cluster.id, review.revision.id)
        : await api.startDeployment(cluster.id, review.revision.id);
      navigateTo(
        `/ha/deployments?deploymentId=${encodeURIComponent(deployment.id)}`,
      );
    } catch (caught) {
      setReview((current) =>
        current ? { ...current, error: caught } : current,
      );
      setBusy("");
    }
  }

  async function archiveRevision(revision: ConfigurationRevision) {
    if (
      !window.confirm(
        `Archive revision #${revision.revisionNumber}? It remains immutable and available in the Archived view.`,
      )
    )
      return;
    setBusy(`lifecycle-${revision.id}`);
    try {
      await api.archiveConfigurationRevision(revision.id);
      await load();
    } catch (caught) {
      setError(caught);
    } finally {
      setBusy("");
    }
  }

  async function restoreRevision(revision: ConfigurationRevision) {
    setBusy(`lifecycle-${revision.id}`);
    try {
      await api.restoreConfigurationRevision(revision.id);
      await load();
    } catch (caught) {
      setError(caught);
    } finally {
      setBusy("");
    }
  }

  async function deleteRevision(revision: ConfigurationRevision) {
    const phrase = `DELETE REVISION #${revision.revisionNumber}`;
    const confirmation = window.prompt(
      `This revision has no retained references. Type ${phrase} to permanently delete it.`,
    );
    if (confirmation === null) return;
    setBusy(`lifecycle-${revision.id}`);
    try {
      await api.deleteConfigurationRevision(revision.id, confirmation);
      await load();
    } catch (caught) {
      setError(caught);
    } finally {
      setBusy("");
    }
  }

  if (revisions === undefined && error === undefined)
    return <Loading label="Loading configuration revisions…" />;
  if (revisions === undefined)
    return <ErrorState error={error} retry={() => void load()} />;

  const columns: DataTableColumn<ConfigurationRevision>[] = [
    {
      id: "revision",
      header: "Revision",
      render: (revision) => (
        <span id={revisionSummaryID(revision.id)}>
          <RevisionBadge
            number={revision.revisionNumber}
            active={revision.active}
          />
        </span>
      ),
    },
    {
      id: "summary",
      header: "Summary",
      render: (revision) => revision.summary,
    },
    {
      id: "publisher",
      header: "Published by",
      render: (revision) => <code>{shortID(revision.createdBy)}</code>,
    },
    {
      id: "published",
      header: "Published",
      render: (revision) => formatTime(revision.createdAt),
    },
    {
      id: "deployment",
      header: "Deployment",
      render: (revision) => {
        const latest = deploymentsByRevision.get(revision.id)?.[0];
        return latest ? latest.status.replaceAll("_", " ") : "Not yet deployed";
      },
    },
    {
      id: "actions",
      header: <span className="visually-hidden">Details</span>,
      align: "right",
      render: (revision) => {
        const expanded = selectedID === revision.id;
        return (
          <button
            className="table-disclosure"
            type="button"
            aria-expanded={expanded}
            aria-controls={revisionDetailID(revision.id)}
            aria-label={`${expanded ? "Hide" : "View"} revision ${revision.revisionNumber} details`}
            onClick={() => toggle(revision.id)}
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
        eyebrow="Immutable configuration"
        title="Configuration Revisions"
        description="Review immutable published revisions, compare what changed, and explicitly preview a deployment without modifying revision history."
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
        <Banner tone="warning" title="Revision unavailable">
          The requested revision is not in this cluster. The revision list
          remains available.
        </Banner>
      )}
      <section className="section-block">
        <div className="section-heading">
          <h2>Published revisions</h2>
          <small>{revisions.length} published</small>
        </div>
        <DataTable
          caption="Published immutable configuration revisions"
          columns={columns}
          rows={revisions}
          rowKey={(revision) => revision.id}
          expandedRowKey={selected?.id}
          expandedRowId={(revision) => revisionDetailID(revision.id)}
          renderExpandedRow={(revision) => (
            <RevisionDetail
              revision={revision}
              preceding={preceding}
              deployments={deploymentsByRevision.get(revision.id) ?? []}
              differences={detailDifferences}
              differenceError={detailError}
              busy={busy}
              onDeploy={prepareDeployment}
              onArchive={archiveRevision}
              onRestore={restoreRevision}
              onDelete={deleteRevision}
            />
          )}
          emptyTitle="No published revisions"
          emptyDescription={
            <p>
              Validate and publish the current draft from Configuration Control.
            </p>
          }
        />
      </section>

      {revisions.length > 1 && (
        <section className="section-block">
          <div className="section-heading">
            <h2>Compare revisions</h2>
            <small>Structured semantic comparison</small>
          </div>
          <div className="card form-stack">
            <div className="form-grid">
              <RevisionSelect
                label="Earlier revision"
                value={leftID}
                revisions={revisions}
                onChange={setLeftID}
              />
              <RevisionSelect
                label="Later revision"
                value={rightID}
                revisions={revisions}
                onChange={setRightID}
              />
            </div>
            <button
              className="button button--secondary"
              type="button"
              disabled={busy !== "" || !leftID || !rightID}
              onClick={() => void compare()}
            >
              {busy === "compare" ? "Comparing…" : "Compare revisions"}
            </button>
            {differences !== undefined && (
              <StructuredDiff
                differences={toStructured(differences)}
                beforeLabel="Earlier"
                afterLabel="Later"
              />
            )}
          </div>
        </section>
      )}

      <DeploymentReviewDialog
        review={review}
        busy={busy.startsWith("deploy-")}
        onClose={() => busy === "" && setReview(undefined)}
        onConfirm={confirmDeployment}
      />
    </>
  );
}

function RevisionDetail({
  revision,
  preceding,
  deployments,
  differences,
  differenceError,
  busy,
  onDeploy,
  onArchive,
  onRestore,
  onDelete,
}: {
  revision: ConfigurationRevision;
  preceding?: ConfigurationRevision;
  deployments: Deployment[];
  differences?: ConfigurationDifference[];
  differenceError?: unknown;
  busy: string;
  onDeploy: (revision: ConfigurationRevision) => Promise<void>;
  onArchive: (revision: ConfigurationRevision) => Promise<void>;
  onRestore: (revision: ConfigurationRevision) => Promise<void>;
  onDelete: (revision: ConfigurationRevision) => Promise<void>;
}) {
  const previousRevision = Boolean(
    revision.active === false && deployments.length > 0,
  );
  return (
    <article
      className="inline-operational-detail"
      aria-labelledby={`revision-${revision.id}-heading`}
    >
      <div className="section-heading">
        <div>
          <h3 id={`revision-${revision.id}-heading`}>
            Revision #{revision.revisionNumber}
          </h3>
          <p className="muted">{revision.summary}</p>
        </div>
        <RevisionBadge
          number={revision.revisionNumber}
          active={revision.active}
        />
        {revision.archivedAt && (
          <StatusBadge status="disabled" label="Archived" />
        )}
      </div>
      <dl className="summary-grid">
        <div>
          <dt>Published by</dt>
          <dd>
            <code>{revision.createdBy}</code>
          </dd>
        </div>
        <div>
          <dt>Published at</dt>
          <dd>{formatTime(revision.createdAt)}</dd>
        </div>
        <div>
          <dt>Schema</dt>
          <dd>Version {revision.schemaVersion}</dd>
        </div>
        <div>
          <dt>Publication validation</dt>
          <dd>Passed whole-draft and capability validation</dd>
        </div>
        <div>
          <dt>Configuration hash</dt>
          <dd>
            <code>{revision.canonicalHash}</code>
          </dd>
        </div>
        <div>
          <dt>Deployments</dt>
          <dd>{deployments.length}</dd>
        </div>
      </dl>
      <section className="inline-detail-section">
        <h4>Deployment history</h4>
        {deployments.length === 0 ? (
          <p className="muted">This revision has not been deployed.</p>
        ) : (
          <ul>
            {deployments.map((deployment) => (
              <li key={deployment.id}>
                <a
                  href={`/ha/deployments?deploymentId=${encodeURIComponent(deployment.id)}`}
                >
                  {shortID(deployment.id)} ·{" "}
                  {deployment.status.replaceAll("_", " ")}
                </a>
              </li>
            ))}
          </ul>
        )}
      </section>
      {preceding !== undefined && (
        <section className="inline-detail-section">
          <h4>Changed since revision #{preceding.revisionNumber}</h4>
          {differenceError !== undefined ? (
            <ErrorState error={differenceError} />
          ) : differences === undefined ? (
            <Loading label="Loading revision differences…" />
          ) : (
            <StructuredDiff
              differences={toStructured(differences)}
              beforeLabel={`Revision #${preceding.revisionNumber}`}
              afterLabel={`Revision #${revision.revisionNumber}`}
            />
          )}
        </section>
      )}
      <section className="inline-detail-section">
        <h4>Next action</h4>
        <p className="muted">
          Deployment is sequential, stops on failure, and verifies each node
          before continuing. This immutable revision will not be modified.
        </p>
        <div className="row-actions row-actions--start">
          <button
            className="button"
            type="button"
            disabled={busy !== ""}
            onClick={() => void onDeploy(revision)}
          >
            {busy === `preview-${revision.id}`
              ? "Preparing preview…"
              : previousRevision
                ? "Deploy this previous revision"
                : "Deploy revision"}
          </button>
          <a className="button button--secondary" href="/ha/deployments">
            View deployments
          </a>
        </div>
      </section>
      <section className="inline-detail-section">
        <h4>Lifecycle</h4>
        <p className="muted">
          Archive hides retained history from the default list. Permanent
          deletion is available only when the server proves this revision is
          unused and unreferenced.
        </p>
        <div className="row-actions row-actions--start">
          {revision.lifecycle.canArchive && (
            <button
              className="button button--secondary"
              type="button"
              disabled={busy !== ""}
              onClick={() => void onArchive(revision)}
            >
              Archive Revision
            </button>
          )}
          {revision.lifecycle.canRestore && (
            <button
              className="button button--secondary"
              type="button"
              disabled={busy !== ""}
              onClick={() => void onRestore(revision)}
            >
              Restore from Archive
            </button>
          )}
          {revision.lifecycle.canDelete && (
            <button
              className="button button--danger"
              type="button"
              disabled={busy !== ""}
              onClick={() => void onDelete(revision)}
            >
              Delete Unused Revision
            </button>
          )}
        </div>
      </section>
      <details className="inline-detail-section">
        <summary>
          View full immutable configuration for revision #
          {revision.revisionNumber}
        </summary>
        <pre className="technical-output">
          <code>{JSON.stringify(revision.document, null, 2)}</code>
        </pre>
      </details>
    </article>
  );
}

function DeploymentReviewDialog({
  review,
  busy,
  onClose,
  onConfirm,
}: {
  review?: DeploymentReview;
  busy: boolean;
  onClose: () => void;
  onConfirm: () => Promise<void>;
}) {
  const preview = review?.preview;
  return (
    <Dialog
      open={review !== undefined}
      onClose={onClose}
      title={
        review
          ? `Review deployment of revision #${review.revision.revisionNumber}`
          : "Review deployment"
      }
      description="Publishing has not changed any managed node. Review this preview before starting a new durable deployment."
      size="large"
      dismissible={!busy}
      actions={
        <>
          <button
            className="button button--secondary"
            type="button"
            disabled={busy}
            onClick={onClose}
          >
            Cancel
          </button>
          <button
            className="button"
            type="button"
            disabled={busy || preview?.valid !== true}
            onClick={() => void onConfirm()}
          >
            {busy ? "Starting…" : "Confirm deployment"}
          </button>
        </>
      }
    >
      {review?.error !== undefined && <ErrorState error={review.error} />}
      {review !== undefined &&
        preview === undefined &&
        review.error === undefined && (
          <Loading label="Loading deployment preview…" />
        )}
      {preview !== undefined && (
        <div className="form-stack">
          <dl className="summary-grid">
            <div>
              <dt>Revision</dt>
              <dd>#{review?.revision.revisionNumber}</dd>
            </div>
            <div>
              <dt>Target nodes</dt>
              <dd>{preview.nodes.length}</dd>
            </div>
            <div>
              <dt>Strategy</dt>
              <dd>Sequential</dd>
            </div>
            <div>
              <dt>Failure policy</dt>
              <dd>Stop on failure</dd>
            </div>
            <div>
              <dt>Read-back verification</dt>
              <dd>Required before the next node</dd>
            </div>
            <div>
              <dt>Restart required</dt>
              <dd>{preview.restartRequired ? "Yes" : "No"}</dd>
            </div>
          </dl>
          {preview.issues.length > 0 && (
            <Banner tone="danger" title="Preview validation failed">
              <ul>
                {preview.issues.map((issue) => (
                  <li key={`${issue.field}-${issue.message}`}>
                    {formatDeploymentValidationIssue(issue)}
                  </li>
                ))}
              </ul>
            </Banner>
          )}
          <section>
            <h3>Ordered target nodes</h3>
            <ol className="progress-list">
              {preview.nodes.map((node) => (
                <li key={node.nodeId}>
                  <strong>
                    {node.position}. <code>{shortID(node.nodeId)}</code>
                  </strong>
                  {node.warning && <p>{node.warning}</p>}{" "}
                  {!node.valid && (
                    <StatusBadge status="failed" label="Validation failed" />
                  )}
                </li>
              ))}
            </ol>
          </section>
          <section>
            <h3>Configuration differences</h3>
            <StructuredDiff
              differences={toStructured(preview.differences)}
              beforeLabel="Current intended state"
              afterLabel={`Revision #${review?.revision.revisionNumber}`}
            />
          </section>
        </div>
      )}
    </Dialog>
  );
}

function formatDeploymentValidationIssue(issue: {
  field: string;
  message: string;
}): string {
  if (
    /^nodeOverrides\.[^.]+$/.test(issue.field) &&
    issue.message.includes("is not present in this immutable revision")
  ) {
    return issue.message;
  }
  return `${issue.field}: ${issue.message}`;
}

function RevisionSelect({
  label,
  value,
  revisions,
  onChange,
}: {
  label: string;
  value: string;
  revisions: ConfigurationRevision[];
  onChange: (value: string) => void;
}) {
  return (
    <label>
      {label}
      <select value={value} onChange={(event) => onChange(event.target.value)}>
        <option value="">Select…</option>
        {revisions.map((revision) => (
          <option key={revision.id} value={revision.id}>
            #{revision.revisionNumber} — {revision.summary}
          </option>
        ))}
      </select>
    </label>
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
function revisionSummaryID(id: string) {
  return `revision-summary-${id}`;
}
function revisionDetailID(id: string) {
  return `revision-detail-${id}`;
}
function shortID(value: string): string {
  return value.length > 12 ? `${value.slice(0, 8)}…` : value;
}
function formatTime(value?: string): string {
  return value ? new Date(value).toLocaleString() : "—";
}

// Compatibility for low-risk downstream imports while the route and UI use Revisions.
export const HistoryPage = RevisionsPage;
