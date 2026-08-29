import { type ReactNode, useCallback, useEffect, useState } from "react";
import {
  DataTable,
  type DataTableColumn,
  HealthSummaryCard,
  SummaryTileGrid,
} from "../../components/DataDisplay";
import { Banner, ErrorState, Loading } from "../../components/Feedback";
import { PageHeader } from "../../components/Page";
import { StatusBadge, type StatusKind } from "../../components/StatusBadge";
import { api } from "../../lib/api";
import type {
  Cluster,
  OperationalCollectionHealth,
  OperationalNodeHealth,
  OperationalStatus,
} from "../../lib/types";

export function OperationalStatusPage({ cluster }: { cluster: Cluster }) {
  const [status, setStatus] = useState<OperationalStatus>();
  const [error, setError] = useState<unknown>();

  const load = useCallback(async () => {
    try {
      setStatus(await api.operationalStatus(cluster.id));
      setError(undefined);
    } catch (caught) {
      setError(caught);
    }
  }, [cluster.id]);

  useEffect(() => {
    void load();
    const timer = window.setInterval(() => void load(), 30_000);
    return () => window.clearInterval(timer);
  }, [load]);

  if (status === undefined && error === undefined)
    return <Loading label="Loading operational status…" />;
  if (status === undefined)
    return <ErrorState error={error} retry={() => void load()} />;

  return (
    <div className="operational-status-page">
      <PageHeader
        eyebrow="Monitoring"
        title="Operational Status"
        description="Health of the controller, collectors, storage, and background work."
        primaryAction={<StatusBadge status={badge(status.summary.state)} />}
      />
      {error !== undefined && (
        <Banner tone="warning" title="Status refresh failed">
          The last successful operational snapshot remains visible.
        </Banner>
      )}
      {status.summary.state !== "healthy" && (
        <Banner tone="warning" title="Operator attention recommended">
          {status.summary.message}
        </Banner>
      )}

      <section
        className="health-summary-grid"
        aria-label="Overall controller health"
      >
        <HealthSummaryCard
          icon="system"
          label="Controller"
          value={status.summary.state.replaceAll("_", " ")}
          status={badge(status.summary.state)}
          detail={status.summary.message}
          valueClassName="operational-value"
        />
        <HealthSummaryCard
          icon="ha"
          label="HA redundancy"
          value={`${status.ha.servingDnsNodes} / ${status.ha.totalNodes}`}
          status={haBadge(status.ha.state)}
          detail="nodes serving DNS"
        />
        <HealthSummaryCard
          icon="dns"
          label="DNS service"
          value={`${status.dnsService.currentNodes} / ${status.dnsService.expectedNodes}`}
          status={badge(status.dnsService.state)}
          detail="nodes current"
        />
        <HealthSummaryCard
          icon="nodes"
          label="Nodes"
          value={`${status.summary.healthyNodes} / ${status.summary.expectedNodes}`}
          status={coverageBadge(
            status.summary.healthyNodes,
            status.summary.expectedNodes,
          )}
          detail="healthy APIs"
        />
        <HealthSummaryCard
          icon="statistics"
          label="Statistics"
          value={`${status.statistics.currentNodes} / ${status.statistics.expectedNodes}`}
          status={badge(status.statistics.state)}
          detail="collectors current"
        />
        <HealthSummaryCard
          icon="activity"
          label="Query Log"
          value={`${status.queryLog.currentNodes} / ${status.queryLog.expectedNodes}`}
          status={badge(status.queryLog.state)}
          detail="collectors current"
        />
      </section>

      <section className="section-block operational-section">
        <OperationalSectionHeading
          title="Core Services"
          description="Controller API and PostgreSQL readiness."
        />
        <div className="card operational-core-panel">
          <SummaryTileGrid
            label="Core service status"
            items={[
              {
                id: "api",
                label: "API",
                value: <StatusBadge status={badge(status.api)} />,
              },
              {
                id: "postgresql",
                label: "PostgreSQL",
                value: <StatusBadge status={badge(status.database.state)} />,
                detail: `${status.database.pingLatencyMs} ms ping`,
              },
              {
                id: "schema-migration",
                label: "Schema migration",
                value: `Version ${status.database.schemaVersion}`,
              },
              {
                id: "connection-pool",
                label: "Connection pool",
                value: `${status.database.poolAcquired} acquired / ${status.database.poolMax} maximum`,
              },
            ]}
          />
        </div>
      </section>

      <CollectionSection
        title="DNS service health"
        collection={status.dnsService}
      />
      <CollectionSection
        title="Node observation"
        collection={status.observation}
      />
      <CollectionSection
        title="Statistics collection"
        collection={status.statistics}
      />
      <CollectionSection
        title="Query Log ingestion"
        collection={status.queryLog}
      />

      <section className="section-block">
        <OperationalSectionHeading title="Background workers" />
        <DataTable
          caption="Background worker health"
          rows={status.workers}
          rowKey={(row) => row.name}
          columns={workerColumns}
        />
      </section>

      <section className="section-block">
        <OperationalSectionHeading
          title="Storage and retention"
          aside="PostgreSQL estimates"
        />
        <div className="storage-grid">
          {status.database.datasets.map((dataset) => (
            <article className="card" key={dataset.name}>
              <h3>
                {dataset.name === "query_log" ? "Query Log" : "Statistics"}
              </h3>
              <dl className="detail-list">
                <div>
                  <dt>Estimated rows</dt>
                  <dd>{formatNumber(dataset.estimatedRows)}</dd>
                </div>
                <div>
                  <dt>Approximate storage</dt>
                  <dd>{formatBytes(dataset.approximateBytes)}</dd>
                </div>
                <div>
                  <dt>Retention</dt>
                  <dd>{formatDuration(dataset.retentionSeconds)}</dd>
                </div>
                <div>
                  <dt>Oldest retained</dt>
                  <dd>{formatTime(dataset.oldestRetainedAt)}</dd>
                </div>
                <div>
                  <dt>Newest retained</dt>
                  <dd>{formatTime(dataset.newestRetainedAt)}</dd>
                </div>
              </dl>
            </article>
          ))}
        </div>
      </section>
      <p className="muted">
        Generated {formatTime(status.generatedAt)}. Error codes are safe
        summaries; detailed diagnostics remain in controller logs.
      </p>
    </div>
  );
}

function CollectionSection({
  title,
  collection,
}: {
  title: string;
  collection: OperationalCollectionHealth;
}) {
  return (
    <section className="section-block">
      <OperationalSectionHeading
        title={title}
        description={
          <>
            {collection.currentNodes} / {collection.expectedNodes} current ·{" "}
            {collection.coveragePercent.toLocaleString(undefined, {
              maximumFractionDigits: 1,
            })}
            % coverage
          </>
        }
        aside={<StatusBadge status={badge(collection.state)} />}
      />
      <DataTable
        caption={`${title} per node`}
        rows={collection.nodes}
        rowKey={(row) => row.nodeId}
        columns={nodeColumns}
      />
    </section>
  );
}

function OperationalSectionHeading({
  title,
  description,
  aside,
}: {
  title: string;
  description?: ReactNode;
  aside?: ReactNode;
}) {
  return (
    <header className="operational-section-heading">
      <div>
        <h2>{title}</h2>
        {description !== undefined && <p>{description}</p>}
      </div>
      {aside !== undefined && (
        <div className="operational-section-heading__aside">{aside}</div>
      )}
    </header>
  );
}

const nodeColumns: readonly DataTableColumn<OperationalNodeHealth>[] = [
  { id: "node", header: "Node", render: (row) => row.nodeName },
  {
    id: "state",
    header: "State",
    render: (row) => <StatusBadge status={badge(row.state)} />,
  },
  {
    id: "success",
    header: "Last success",
    render: (row) => formatTime(row.lastSuccessAt),
  },
  {
    id: "capability",
    header: "Capability",
    render: (row) =>
      row.capabilityState === undefined ? (
        "—"
      ) : (
        <StatusBadge status={badge(row.capabilityState)} />
      ),
  },
  {
    id: "lag",
    header: "Lag",
    render: (row) =>
      row.lagSeconds === undefined ? "—" : formatDuration(row.lagSeconds),
  },
  {
    id: "failures",
    header: "Failures",
    render: (row) => String(row.consecutiveFailures),
    align: "right",
  },
  {
    id: "issue",
    header: "Latest issue",
    render: (row) => row.gapReason ?? row.errorCode ?? "—",
  },
  {
    id: "next",
    header: "Next attempt",
    render: (row) => formatTime(row.nextScheduledAt),
  },
];

const workerColumns: readonly DataTableColumn<
  OperationalStatus["workers"][number]
>[] = [
  {
    id: "worker",
    header: "Worker",
    render: (row) => row.name.replaceAll("_", " "),
  },
  {
    id: "state",
    header: "State",
    render: (row) => (
      <StatusBadge
        status={badge(row.state)}
        label={row.running ? "Running" : undefined}
      />
    ),
  },
  {
    id: "success",
    header: "Last success",
    render: (row) => formatTime(row.lastSuccessAt),
  },
  {
    id: "failures",
    header: "Failures",
    render: (row) => String(row.consecutiveFailures),
    align: "right",
  },
  {
    id: "issue",
    header: "Latest issue",
    render: (row) => row.errorCode ?? "—",
  },
  {
    id: "next",
    header: "Next run",
    render: (row) => formatTime(row.nextScheduledAt),
  },
];

function badge(state: OperationalStatus["api"]): StatusKind {
  return state;
}
function haBadge(state: OperationalStatus["ha"]["state"]): StatusKind {
  return state === "at_risk" ? "warning" : state;
}
function coverageBadge(current: number, expected: number): StatusKind {
  if (expected === 0) return "unknown";
  if (current >= expected) return "healthy";
  return current === 0 ? "failed" : "degraded";
}
function formatNumber(value: number) {
  return new Intl.NumberFormat().format(value);
}
function formatBytes(value: number) {
  if (value < 1024) return `${value} B`;
  const units = ["KiB", "MiB", "GiB", "TiB"];
  let amount = value / 1024;
  let unit = units[0];
  for (let index = 1; amount >= 1024 && index < units.length; index++) {
    amount /= 1024;
    unit = units[index];
  }
  return `${amount.toLocaleString(undefined, { maximumFractionDigits: 1 })} ${unit}`;
}
function formatDuration(seconds: number) {
  if (seconds < 60) return `${Math.max(0, Math.round(seconds))} seconds`;
  if (seconds < 3600) return `${Math.round(seconds / 60)} minutes`;
  if (seconds < 86400) return `${Math.round(seconds / 3600)} hours`;
  return `${Math.round(seconds / 86400)} days`;
}
function formatTime(value?: string) {
  return value === undefined ? "Never" : new Date(value).toLocaleString();
}
