import { useCallback, useEffect, useMemo, useState } from "react";
import { EmptyState, ErrorState, Loading } from "../../components/Feedback";
import { Icon, type IconName } from "../../components/Icon";
import { PageHeader } from "../../components/Page";
import { StatusBadge, type StatusKind } from "../../components/StatusBadge";
import { api } from "../../lib/api";
import { clusterHealth, isStale } from "../../lib/freshness";
import type {
  AuditEvent,
  Cluster,
  ConfigurationRevision,
  Deployment,
  DriftEvent,
  HASummary,
  Node,
  OperationalHealthState,
  OperationalStatus,
  StatisticsRanking,
  StatisticsReport,
  VersionHealth,
} from "../../lib/types";
import {
  auditActionLabel,
  auditActorLabel,
  auditEventHref,
  auditResourceLabel,
} from "../audit/auditPresentation";

type DashboardSource =
  | "statistics"
  | "operational"
  | "ha"
  | "versions"
  | "revisions"
  | "deployments"
  | "drift"
  | "audit";

interface RecentChange {
  id: string;
  label: string;
  detail: string;
  at: string;
  href: string;
  icon: IconName;
}

export function DashboardPage({ cluster }: { cluster: Cluster }) {
  const [nodes, setNodes] = useState<Node[]>();
  const [refreshedAt, setRefreshedAt] = useState<string>();
  const [staleAfterMs, setStaleAfterMs] = useState(90_000);
  const [error, setError] = useState<unknown>();
  const [statistics, setStatistics] = useState<StatisticsReport>();
  const [operational, setOperational] = useState<OperationalStatus>();
  const [ha, setHA] = useState<HASummary>();
  const [versions, setVersions] = useState<VersionHealth[]>([]);
  const [revisions, setRevisions] = useState<ConfigurationRevision[]>([]);
  const [deployments, setDeployments] = useState<Deployment[]>([]);
  const [drift, setDrift] = useState<DriftEvent[]>([]);
  const [audit, setAudit] = useState<AuditEvent[]>([]);
  const [supplementaryLoading, setSupplementaryLoading] = useState(true);
  const [sourceErrors, setSourceErrors] = useState<
    ReadonlySet<DashboardSource>
  >(new Set());

  const load = useCallback(async () => {
    setSupplementaryLoading(true);
    setSourceErrors(new Set());
    const supplementary: [DashboardSource, Promise<unknown>][] = [
      ["statistics", api.statistics(cluster.id, "24h", "").then(setStatistics)],
      ["operational", api.operationalStatus(cluster.id).then(setOperational)],
      ["ha", api.haStatus(cluster.id).then(setHA)],
      [
        "versions",
        api.versions(cluster.id).then((result) => setVersions(result.items)),
      ],
      [
        "revisions",
        api
          .configurationRevisions(cluster.id)
          .then((result) => setRevisions(result.items)),
      ],
      [
        "deployments",
        api
          .deployments(cluster.id)
          .then((result) => setDeployments(result.items)),
      ],
      [
        "drift",
        api.driftEvents(cluster.id).then((result) => setDrift(result.items)),
      ],
      [
        "audit",
        api
          .auditEvents({
            clusterId: cluster.id,
            includeController: true,
            limit: 50,
          })
          .then((result) => setAudit(result.items)),
      ],
    ];
    void Promise.allSettled(supplementary.map(([, promise]) => promise)).then(
      (results) => {
        setSourceErrors(
          new Set(
            results.flatMap((result, index) =>
              result.status === "rejected"
                ? [supplementary[index]?.[0] as DashboardSource]
                : [],
            ),
          ),
        );
        setSupplementaryLoading(false);
      },
    );

    try {
      const result = await api.nodes(cluster.id);
      setNodes(result.items);
      setRefreshedAt(result.refreshedAt);
      setStaleAfterMs(result.staleAfterSeconds * 1000);
      setError(undefined);
    } catch (caught) {
      setError(caught);
    }
  }, [cluster.id]);

  useEffect(() => {
    void load();
    const interval = window.setInterval(() => void load(), 30_000);
    return () => window.clearInterval(interval);
  }, [load]);

  const currentNodes = nodes ?? [];
  const attention = useMemo(
    () =>
      attentionItems(
        operational,
        ha,
        drift.filter((item) => item.clusterId === cluster.id),
        deployments.filter((item) => item.clusterId === cluster.id),
      ),
    [cluster.id, operational, ha, drift, deployments],
  );
  const recent = useMemo(
    () => recentChanges(cluster.id, revisions, deployments, audit),
    [cluster.id, revisions, deployments, audit],
  );

  if (nodes === undefined && error === undefined)
    return <Loading label="Loading cluster health…" />;
  if (nodes === undefined)
    return <ErrorState error={error} retry={() => void load()} />;

  const apiReachable = currentNodes.filter(
    (node) => node.healthStatus === "healthy",
  ).length;
  const collectionCurrent =
    (operational?.statistics.currentNodes ?? 0) +
    (operational?.queryLog.currentNodes ?? 0);
  const collectionExpected =
    (operational?.statistics.expectedNodes ?? 0) +
    (operational?.queryLog.expectedNodes ?? 0);

  return (
    <div className="dashboard-page">
      <PageHeader
        eyebrow="Cluster overview"
        title={cluster.name}
        description={
          <span>
            Cluster: {cluster.name} · Active revision{" "}
            {activeRevisionLabel(cluster, revisions)} · Last refreshed{" "}
            {formatDate(refreshedAt)}
          </span>
        }
        primaryAction={<StatusBadge status={clusterHealth(currentNodes)} />}
      />
      {error !== undefined && (
        <div className="notice notice--warning">
          Node health refresh failed. Showing the last available data.
        </div>
      )}
      {sourceErrors.size > 0 && !supplementaryLoading && (
        <div className="notice notice--warning">
          Some dashboard sources are unavailable. Available operational data
          remains visible.
        </div>
      )}

      <section className="dashboard-health-grid" aria-label="Primary health">
        <HealthCard
          icon="dns"
          label="DNS Serving"
          value={
            ha
              ? `${ha.servingDnsNodes} / ${ha.totalNodes}`
              : supplementaryLoading
                ? "Loading…"
                : "—"
          }
          status={haStatus(ha)}
          detail={
            ha
              ? `${percentage(ha.servingDnsNodes, ha.totalNodes)} serving`
              : "HA evidence unavailable"
          }
          href="/ha/operations"
        />
        <HealthCard
          icon="nodes"
          label="API Reachable"
          value={`${apiReachable} / ${currentNodes.length}`}
          status={
            currentNodes.length === 0
              ? "unknown"
              : apiReachable === currentNodes.length
                ? "healthy"
                : "degraded"
          }
          detail={`${percentage(apiReachable, currentNodes.length)} reachable`}
          href="/system/operational-status"
        />
        <HealthCard
          icon="ha"
          label="HA Status"
          value={
            ha
              ? titleCase(ha.state === "at_risk" ? "at risk" : ha.state)
              : supplementaryLoading
                ? "Loading…"
                : "—"
          }
          status={haStatus(ha)}
          detail={ha?.message ?? "HA evidence unavailable"}
          href="/ha/operations"
        />
        <HealthCard
          icon="statistics"
          label="Collection"
          value={
            operational
              ? `${collectionCurrent} / ${collectionExpected}`
              : supplementaryLoading
                ? "Loading…"
                : "—"
          }
          status={collectionStatus(operational)}
          detail={collectionDetail(operational)}
          href="/system/operational-status"
        />
        <HealthCard
          icon="attention"
          label="Attention"
          value={
            supplementaryLoading
              ? "Loading…"
              : sourceErrors.size === 8
                ? "—"
                : String(attention.length)
          }
          status={
            supplementaryLoading || sourceErrors.size === 8
              ? "unknown"
              : attention.length === 0
                ? "healthy"
                : "warning"
          }
          detail={
            supplementaryLoading
              ? "Checking operational evidence"
              : sourceErrors.size === 8
                ? "Attention evidence unavailable"
                : attention.length === 0
                  ? "No current warnings"
                  : `${attention.length} actionable ${attention.length === 1 ? "condition" : "conditions"}`
          }
          href="#dashboard-attention"
        />
      </section>

      <section className="dashboard-main-grid">
        <article className="card dashboard-activity-card">
          <DashboardPanelHeader
            title="DNS activity"
            metadata="Last 24 hours · Entire cluster"
            action={{ label: "View statistics", href: "/statistics" }}
          />
          {supplementaryLoading && statistics === undefined ? (
            <Loading label="Loading DNS activity…" />
          ) : statistics === undefined || statistics.state === "unavailable" ? (
            <UnavailablePanel
              message="No usable cluster-wide 24-hour Statistics snapshot is available."
              href="/system/operational-status"
            />
          ) : (
            <>
              <ActivityChart report={statistics} />
              <dl className="dashboard-kpis">
                <KPI
                  label="Queries"
                  value={formatCount(statistics.totals.dnsQueries)}
                />
                <KPI
                  label="Blocked"
                  value={`${formatNumber(statistics.totals.blockedPercentage)}%`}
                  detail={`${formatCount(statistics.totals.blockedFiltering)} queries`}
                />
                <KPI
                  label="Safety interventions"
                  value={formatCount(statistics.totals.safetyInterventions)}
                />
                <KPI
                  label="Average processing"
                  value={`${formatNumber(statistics.totals.averageProcessingMs)} ms`}
                />
              </dl>
            </>
          )}
        </article>

        <article className="card dashboard-list-card" id="dashboard-attention">
          <DashboardPanelHeader
            title="Attention"
            action={{
              label: "Operational status",
              href: "/system/operational-status",
            }}
          />
          {supplementaryLoading &&
          operational === undefined &&
          ha === undefined ? (
            <Loading label="Checking attention items…" />
          ) : attention.length === 0 ? (
            <CompactEmpty
              icon="healthy"
              title="Nothing needs attention"
              detail="Current operational evidence reports no actionable conditions."
            />
          ) : (
            <ul className="dashboard-event-list">
              {attention.slice(0, 5).map((item) => (
                <li key={item.id}>
                  <span
                    className={`dashboard-event-dot dashboard-event-dot--${item.tone}`}
                    aria-hidden="true"
                  />
                  <a href={item.href}>
                    <strong>{item.title}</strong>
                    <small>{item.detail}</small>
                  </a>
                  <Icon name="chevron" />
                </li>
              ))}
            </ul>
          )}
        </article>

        <article className="card dashboard-list-card">
          <DashboardPanelHeader
            title="Recent changes"
            action={{ label: "Audit log", href: "/system/audit" }}
          />
          {(sourceErrors.has("revisions") ||
            sourceErrors.has("deployments") ||
            sourceErrors.has("audit")) && (
            <p className="dashboard-partial-warning" role="status">
              Recent changes is partial: {recentFailureLabels(sourceErrors)}.
              Available sources remain visible.
            </p>
          )}
          {supplementaryLoading && recent.length === 0 ? (
            <Loading label="Loading recent changes…" />
          ) : recent.length === 0 ? (
            <CompactEmpty
              icon="info"
              title="No recent changes"
              detail="Published revisions, deployments, and audited actions will appear here."
            />
          ) : (
            <ul className="dashboard-change-list">
              {recent.slice(0, 5).map((change) => (
                <li key={change.id}>
                  <span className="dashboard-change-icon">
                    <Icon name={change.icon} />
                  </span>
                  <a href={change.href}>
                    <strong>{change.label}</strong>
                    <small>{change.detail}</small>
                  </a>
                  <time dateTime={change.at}>{formatRelative(change.at)}</time>
                </li>
              ))}
            </ul>
          )}
        </article>
      </section>

      <section className="card dashboard-nodes-card">
        <DashboardPanelHeader
          title={`Nodes (${currentNodes.length})`}
          action={{ label: "View all nodes", href: "/ha/nodes" }}
        />
        {currentNodes.length === 0 ? (
          <EmptyState title="Add your first AdGuard Home node">
            <p>
              Atlas will inspect node status without entering the live DNS
              request path.
            </p>
            <a className="button" href="/ha/nodes">
              Add a node
            </a>
          </EmptyState>
        ) : (
          <NodeSummaryTable
            nodes={currentNodes}
            ha={ha}
            versions={versions}
            staleAfterMs={staleAfterMs}
          />
        )}
      </section>

      <section className="dashboard-ranking-grid" aria-label="Top domains">
        <RankingPanel
          title="Top queried domains"
          metadata="Entire cluster"
          values={
            statistics?.state === "unavailable"
              ? []
              : (statistics?.rankings?.queriedDomains ?? [])
          }
          loading={supplementaryLoading && statistics === undefined}
        />
        <RankingPanel
          title="Top blocked domains"
          metadata="Entire cluster"
          values={
            statistics?.state === "unavailable"
              ? []
              : (statistics?.rankings?.blockedDomains ?? [])
          }
          loading={supplementaryLoading && statistics === undefined}
        />
      </section>

      <footer className="dashboard-boundary">
        <Icon name="dns" />
        <span>
          <strong>DNS stays on the nodes.</strong> Controller downtime pauses
          management and collection, not DNS service.
        </span>
        <small>API view refreshed {formatDate(refreshedAt)}</small>
      </footer>
    </div>
  );
}

function HealthCard({
  icon,
  label,
  value,
  status,
  detail,
  href,
}: {
  icon: IconName;
  label: string;
  value: string;
  status: StatusKind;
  detail: string;
  href: string;
}) {
  return (
    <a className="dashboard-health-card" href={href}>
      <span className={`dashboard-health-card__icon status-tone--${status}`}>
        <Icon name={icon} />
      </span>
      <span className="dashboard-health-card__body">
        <small>{label}</small>
        <strong>{value}</strong>
        <span>
          <StatusBadge status={status} />
          <em>{detail}</em>
        </span>
      </span>
    </a>
  );
}

function DashboardPanelHeader({
  title,
  metadata,
  action,
}: {
  title: string;
  metadata?: string;
  action?: { label: string; href: string };
}) {
  return (
    <header className="dashboard-panel-header">
      <div>
        <h2>{title}</h2>
        {metadata && <p className="dashboard-panel-metadata">{metadata}</p>}
      </div>
      {action && (
        <a href={action.href}>
          {action.label} <span aria-hidden="true">→</span>
        </a>
      )}
    </header>
  );
}

function ActivityChart({ report }: { report: StatisticsReport }) {
  if (!report.series || report.series.length === 0)
    return (
      <p className="muted dashboard-chart-empty">
        No time-series buckets were returned.
      </p>
    );
  const width = 800;
  const height = 180;
  const maximum = Math.max(
    1,
    ...report.series.flatMap((point) => [
      point.dnsQueries,
      point.blockedFiltering,
    ]),
  );
  const path = (key: "dnsQueries" | "blockedFiltering") =>
    report.series
      .map((point, index) => {
        const x =
          report.series.length <= 1
            ? 0
            : (index / (report.series.length - 1)) * width;
        const y = height - (point[key] / maximum) * (height - 12) - 6;
        return `${index === 0 ? "M" : "L"}${x.toFixed(2)},${y.toFixed(2)}`;
      })
      .join(" ");
  return (
    <figure className="dashboard-chart">
      <svg
        viewBox={`0 0 ${width} ${height}`}
        role="img"
        aria-label="DNS queries and blocked activity over the last 24 hours"
      >
        <title>DNS queries and blocked activity over the last 24 hours</title>
        <desc>
          {report.series.length} chronological points.{" "}
          {report.totals.dnsQueries} queries and{" "}
          {report.totals.blockedFiltering} blocked queries.
        </desc>
        <path className="dashboard-chart__queries" d={path("dnsQueries")} />
        <path
          className="dashboard-chart__blocked"
          d={path("blockedFiltering")}
        />
      </svg>
      <figcaption>
        <span className="legend-query">Queries</span>
        <span className="legend-blocked">Blocked</span>
        <span>
          {formatChartDate(report.series[0]?.at)} –{" "}
          {formatChartDate(report.series.at(-1)?.at)}
        </span>
      </figcaption>
    </figure>
  );
}

function KPI({
  label,
  value,
  detail,
}: {
  label: string;
  value: string;
  detail?: string;
}) {
  return (
    <div>
      <dt>{label}</dt>
      <dd>
        <span>{value}</span>
        {detail && <small>{detail}</small>}
      </dd>
    </div>
  );
}

function NodeSummaryTable({
  nodes,
  ha,
  versions,
  staleAfterMs,
}: {
  nodes: Node[];
  ha?: HASummary;
  versions: VersionHealth[];
  staleAfterMs: number;
}) {
  const versionByNode = new Map(
    versions.map((version) => [version.nodeId, version]),
  );
  const dnsByNode = new Map(ha?.nodes.map((node) => [node.nodeId, node]) ?? []);
  return (
    <div className="table-wrap">
      <table className="dashboard-node-table">
        <thead>
          <tr>
            <th>Node</th>
            <th>DNS</th>
            <th>API</th>
            <th>Version</th>
            <th>Last seen</th>
            <th>Update state</th>
            <th>
              <span className="visually-hidden">Actions</span>
            </th>
          </tr>
        </thead>
        <tbody>
          {nodes.map((node) => {
            const version = versionByNode.get(node.id);
            const stale = isStale(node.lastPolledAt, Date.now(), staleAfterMs);
            return (
              <tr key={node.id}>
                <th scope="row">
                  <a href={`/ha/nodes/${node.id}`}>{node.name}</a>
                  <span className="table-subtitle monospace">
                    {node.baseUrl}
                  </span>
                </th>
                <td>
                  <StatusBadge
                    status={dnsStatus(dnsByNode.get(node.id)?.dnsStatus)}
                  />
                </td>
                <td>
                  <StatusBadge status={stale ? "stale" : node.healthStatus} />
                </td>
                <td>{node.version ?? "Unknown"}</td>
                <td>{formatRelative(node.lastSeenAt)}</td>
                <td>
                  {version === undefined ? (
                    <StatusBadge status="unknown" label="Not checked" />
                  ) : version.updateAvailable ? (
                    <StatusBadge status="warning" label="Available" />
                  ) : version.releaseCheckStale ? (
                    <StatusBadge status="stale" label="Check stale" />
                  ) : (
                    <StatusBadge status="success" label="Current" />
                  )}
                </td>
                <td>
                  <a
                    className="button button--quiet"
                    href={`/ha/nodes/${node.id}`}
                  >
                    Manage
                  </a>
                </td>
              </tr>
            );
          })}
        </tbody>
      </table>
    </div>
  );
}

function RankingPanel({
  title,
  metadata,
  values,
  loading,
}: {
  title: string;
  metadata: string;
  values: StatisticsRanking[];
  loading: boolean;
}) {
  const maximum = values[0]?.value ?? 1;
  return (
    <article className="card dashboard-ranking">
      <DashboardPanelHeader
        title={title}
        metadata={metadata}
        action={{ label: "View statistics", href: "/statistics" }}
      />
      {loading ? (
        <Loading label={`Loading ${title.toLowerCase()}…`} />
      ) : values.length === 0 ? (
        <p className="muted">
          No domain rankings are available for this scope.
        </p>
      ) : (
        <ol>
          {values.slice(0, 5).map((item, index) => (
            <li key={item.key}>
              <span className="dashboard-ranking__number">{index + 1}</span>
              <div>
                <span title={item.key}>{item.key}</span>
                <span className="statistics-bar" aria-hidden="true">
                  <span style={{ width: `${(item.value / maximum) * 100}%` }} />
                </span>
              </div>
              <strong>{formatCount(item.value)}</strong>
            </li>
          ))}
        </ol>
      )}
    </article>
  );
}

function UnavailablePanel({
  message,
  href,
}: {
  message: string;
  href: string;
}) {
  return (
    <div className="dashboard-unavailable">
      <Icon name="statistics" />
      <p>{message}</p>
      <a href={href}>Check collection health</a>
    </div>
  );
}
function CompactEmpty({
  icon,
  title,
  detail,
}: {
  icon: "healthy" | "info";
  title: string;
  detail: string;
}) {
  return (
    <div className={`dashboard-compact-empty dashboard-compact-empty--${icon}`}>
      <span aria-hidden="true">{icon === "healthy" ? "✓" : "i"}</span>
      <strong>{title}</strong>
      <small>{detail}</small>
    </div>
  );
}

function attentionItems(
  operational: OperationalStatus | undefined,
  ha: HASummary | undefined,
  drift: DriftEvent[],
  deployments: Deployment[],
) {
  const items: {
    id: string;
    title: string;
    detail: string;
    href: string;
    tone: "warning" | "danger";
  }[] = [];
  if (operational?.summary.actionRequired)
    items.push({
      id: "operational",
      title: "Controller operations need attention",
      detail: operational.summary.message,
      href: "/system/operational-status",
      tone: operational.summary.state === "failed" ? "danger" : "warning",
    });
  if (ha && ha.state !== "healthy")
    items.push({
      id: "ha",
      title:
        ha.state === "at_risk"
          ? "DNS redundancy is at risk"
          : "HA health is degraded",
      detail: ha.message,
      href: "/ha/operations",
      tone: ha.state === "at_risk" ? "danger" : "warning",
    });
  const openDrift = drift.filter((item) => item.status === "open").length;
  if (openDrift > 0)
    items.push({
      id: "drift",
      title: `${openDrift} open drift ${openDrift === 1 ? "incident" : "incidents"}`,
      detail: "Desired and observed configuration differ.",
      href: "/ha/drift",
      tone: "warning",
    });
  const failed = deployments.filter((item) => item.status === "failed").length;
  if (failed > 0)
    items.push({
      id: "deployments",
      title: `${failed} failed ${failed === 1 ? "deployment" : "deployments"}`,
      detail: "Review durable per-node execution results.",
      href: "/ha/deployments",
      tone: "danger",
    });
  if (ha && ha.certificateWarnings > 0)
    items.push({
      id: "certificates",
      title: `${ha.certificateWarnings} certificate ${ha.certificateWarnings === 1 ? "warning" : "warnings"}`,
      detail: "Certificate expiry needs review.",
      href: "/ha/operations",
      tone: "warning",
    });
  if (ha && ha.updateAvailableNodes > 0)
    items.push({
      id: "updates",
      title: `${ha.updateAvailableNodes} node ${ha.updateAvailableNodes === 1 ? "update" : "updates"} available`,
      detail: "Review guided upgrade readiness.",
      href: "/ha/operations",
      tone: "warning",
    });
  return items;
}

export function recentChanges(
  clusterID: string,
  revisions: ConfigurationRevision[],
  deployments: Deployment[],
  audit: AuditEvent[],
): RecentChange[] {
  const scopedRevisions = revisions.filter(
    (revision) =>
      revision.clusterId === undefined || revision.clusterId === clusterID,
  );
  const scopedDeployments = deployments.filter(
    (deployment) =>
      deployment.clusterId === undefined || deployment.clusterId === clusterID,
  );
  const scopedAudit = audit.filter(
    (event) =>
      event.scope === "controller" ||
      event.clusterId === undefined ||
      event.clusterId === clusterID,
  );
  const usedAuditIDs = new Set<string>();
  const revisionItems = scopedRevisions.map((revision) => {
    const attribution = scopedAudit.find(
      (event) =>
        event.action === "configuration.revision_published" &&
        event.resourceType === "configuration_revision" &&
        event.resourceId === revision.id,
    );
    if (attribution) usedAuditIDs.add(attribution.id);
    return {
      id: `revision-${revision.id}`,
      label:
        revision.summary || `Revision #${revision.revisionNumber} published`,
      detail: `Configuration revision #${revision.revisionNumber}${attribution ? ` · Current actor: ${auditActorLabel(attribution)}` : ""}`,
      at: revision.createdAt,
      href: `/ha/revisions?revisionId=${encodeURIComponent(revision.id)}`,
      icon: "revisions" as const,
    };
  });
  const deploymentItems = scopedDeployments.map((deployment) => {
    const attribution = scopedAudit.find(
      (event) =>
        event.resourceType === "deployment" &&
        event.resourceId === deployment.id &&
        deploymentAuditMatches(deployment, event.action),
    );
    if (attribution) usedAuditIDs.add(attribution.id);
    return {
      id: `deployment-${deployment.id}`,
      label: `Deployment ${titleCase(deployment.status)}`,
      detail: `${titleCase(deployment.origin)} deployment · ${deployment.id.slice(0, 8)}${attribution ? ` · Current actor: ${auditActorLabel(attribution)}` : ""}`,
      at:
        deployment.completedAt ??
        deployment.startedAt ??
        deployment.requestedAt,
      href: `/ha/deployments?deploymentId=${encodeURIComponent(deployment.id)}`,
      icon: "deployments" as const,
    };
  });
  const auditItems = scopedAudit
    .filter((event) => !usedAuditIDs.has(event.id))
    .map((event) => ({
      id: `audit-${event.id}`,
      label:
        event.scope === "controller"
          ? `Controller · ${auditActionLabel(event.action)}`
          : auditActionLabel(event.action),
      detail: `${auditResourceLabel(event.resourceType)} · ${auditActorLabel(event)}`,
      at: event.createdAt,
      href: auditEventHref(event.id),
      icon: "audit" as const,
    }));
  return [...revisionItems, ...deploymentItems, ...auditItems]
    .filter((item) => Boolean(item.at))
    .sort(
      (left, right) =>
        new Date(right.at).valueOf() - new Date(left.at).valueOf() ||
        right.id.localeCompare(left.id),
    )
    .slice(0, 8);
}

function deploymentAuditMatches(deployment: Deployment, action: string) {
  const terminalAction = `deployment.${deployment.status}`;
  if (action === terminalAction) return true;
  if (
    !deployment.completedAt &&
    [
      "deployment.created",
      "deployment.manual_created",
      "deployment.reconciliation_created",
      "deployment.rollback_created",
    ].includes(action)
  )
    return true;
  return false;
}

function recentFailureLabels(errors: ReadonlySet<DashboardSource>) {
  const labels = [
    errors.has("revisions") ? "Revisions unavailable" : "",
    errors.has("deployments") ? "Deployments unavailable" : "",
    errors.has("audit") ? "Audit Log unavailable" : "",
  ].filter(Boolean);
  return labels.join(", ");
}

function haStatus(ha?: HASummary): StatusKind {
  if (!ha) return "unknown";
  return ha.state === "healthy"
    ? "healthy"
    : ha.state === "at_risk"
      ? "failed"
      : "degraded";
}
function collectionStatus(operational?: OperationalStatus): StatusKind {
  if (!operational) return "unknown";
  return worstState([operational.statistics.state, operational.queryLog.state]);
}
function worstState(states: OperationalHealthState[]): StatusKind {
  if (states.includes("failed")) return "failed";
  if (states.some((state) => ["degraded", "stale"].includes(state)))
    return "degraded";
  if (states.every((state) => state === "healthy")) return "healthy";
  return states[0] ?? "unknown";
}
function collectionDetail(operational?: OperationalStatus) {
  if (!operational) return "Collection evidence unavailable";
  const states = [operational.statistics.state, operational.queryLog.state];
  return states.every((state) => state === "healthy")
    ? "Statistics and Query Log healthy"
    : `Statistics ${operational.statistics.state}, Query Log ${operational.queryLog.state}`;
}
function dnsStatus(
  status?: HASummary["nodes"][number]["dnsStatus"],
): StatusKind {
  if (!status) return "unknown";
  if (status === "failed") return "failed";
  return status;
}
function activeRevisionLabel(
  cluster: Cluster,
  revisions: ConfigurationRevision[],
) {
  const active = revisions.find(
    (revision) =>
      revision.clusterId === cluster.id &&
      (revision.active || revision.id === cluster.activeRevisionId),
  );
  return active ? `#${active.revisionNumber}` : "none";
}
function percentage(value: number, total: number) {
  return total > 0 ? `${Math.round((value / total) * 100)}%` : "0%";
}
function formatCount(value: number) {
  return new Intl.NumberFormat().format(value);
}
function formatNumber(value: number) {
  return new Intl.NumberFormat(undefined, { maximumFractionDigits: 2 }).format(
    value,
  );
}
function formatDate(value?: string) {
  if (!value) return "not yet";
  const date = new Date(value);
  return Number.isNaN(date.valueOf()) ? "unknown" : date.toLocaleString();
}
function formatRelative(value?: string) {
  if (!value) return "Never";
  const date = new Date(value);
  if (Number.isNaN(date.valueOf())) return "Unknown";
  const seconds = Math.round((Date.now() - date.valueOf()) / 1000);
  if (seconds < 60) return "Just now";
  if (seconds < 3600) return `${Math.round(seconds / 60)}m ago`;
  if (seconds < 86400) return `${Math.round(seconds / 3600)}h ago`;
  return `${Math.round(seconds / 86400)}d ago`;
}
function formatChartDate(value?: string) {
  if (!value) return "Unknown";
  const date = new Date(value);
  return Number.isNaN(date.valueOf())
    ? "Unknown"
    : date.toLocaleString(undefined, { hour: "numeric", minute: "2-digit" });
}
function titleCase(value: string) {
  return value
    .split(/\s+/)
    .map((word) => (word ? word[0]?.toUpperCase() + word.slice(1) : word))
    .join(" ");
}
