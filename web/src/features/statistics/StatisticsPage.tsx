import { useCallback, useEffect, useState } from "react";
import { MetricCard } from "../../components/DataDisplay";
import { EmptyState, ErrorState, Loading } from "../../components/Feedback";
import { PageHeader } from "../../components/Page";
import { StatusBadge, type StatusKind } from "../../components/StatusBadge";
import { api } from "../../lib/api";
import type {
  Cluster,
  Node,
  StatisticsRange,
  StatisticsRanking,
  StatisticsReport,
} from "../../lib/types";

const ranges: { value: StatisticsRange; label: string }[] = [
  { value: "24h", label: "24 hours" },
  { value: "7d", label: "7 days" },
  { value: "30d", label: "30 days" },
];

export function StatisticsPage({ cluster }: { cluster: Cluster }) {
  const [nodeId, setNodeId] = useState("");
  const [nodes, setNodes] = useState<Node[]>([]);
  const [nodeError, setNodeError] = useState<unknown>();
  const [range, setRange] = useState<StatisticsRange>("24h");
  const [report, setReport] = useState<StatisticsReport>();
  const [reportKey, setReportKey] = useState("");
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<unknown>();

  const loadNodes = useCallback(async () => {
    try {
      setNodes((await api.nodes(cluster.id)).items);
      setNodeError(undefined);
    } catch (caught) {
      setNodeError(caught);
    }
  }, [cluster.id]);

  useEffect(() => {
    setNodeId("");
    void loadNodes();
  }, [loadNodes]);

  const load = useCallback(async () => {
    setLoading(true);
    try {
      setReport(await api.statistics(cluster.id, range, nodeId));
      setReportKey(`${cluster.id}:${range}:${nodeId}`);
      setError(undefined);
    } catch (caught) {
      setError(caught);
    } finally {
      setLoading(false);
    }
  }, [cluster.id, nodeId, range]);

  useEffect(() => {
    void load();
  }, [load]);

  const scopeName =
    nodes.find((node) => node.id === nodeId)?.name ?? "Entire Cluster";
  const requestedKey = `${cluster.id}:${range}:${nodeId}`;
  if (reportKey !== requestedKey && loading)
    return <Loading label="Loading aggregated statistics…" />;
  if (reportKey !== requestedKey && error !== undefined)
    return <ErrorState error={error} retry={() => void load()} />;
  if (loading && report === undefined)
    return <Loading label="Loading aggregated statistics…" />;
  if (report === undefined && error !== undefined)
    return <ErrorState error={error} retry={() => void load()} />;
  if (report === undefined) return null;

  return (
    <>
      <PageHeader
        eyebrow={`Observability · ${scopeName}`}
        title="Statistics"
        description={`Cluster: ${cluster.name} · Last refreshed ${formatDate(report.generatedAt)}. Controller-collected DNS activity; no Query Log records are used.`}
      />

      <section
        className="card statistics-toolbar"
        aria-labelledby="statistics-scope-heading"
      >
        <div>
          <h2 id="statistics-scope-heading">Traffic scope</h2>
          <p>
            Choose the entire cluster or one node, then select the reporting
            range.
          </p>
        </div>
        <div className="statistics-controls">
          <label className="statistics-scope">
            <span>Node</span>
            <select
              value={nodeId}
              onChange={(event) => setNodeId(event.target.value)}
            >
              <option value="">Entire Cluster</option>
              {nodes.map((node) => (
                <option key={node.id} value={node.id}>
                  {node.name}
                </option>
              ))}
            </select>
          </label>
          <div className="statistics-range-control">
            <span>Time range</span>
            <fieldset className="statistics-range">
              <legend className="visually-hidden">Statistics range</legend>
              {ranges.map((item) => (
                <button
                  className={
                    item.value === range ? "button" : "button button--secondary"
                  }
                  type="button"
                  key={item.value}
                  aria-pressed={item.value === range}
                  onClick={() => setRange(item.value)}
                >
                  {item.label}
                </button>
              ))}
            </fieldset>
          </div>
        </div>
      </section>

      {nodeError !== undefined && (
        <div className="notice notice--warning">
          Node choices are unavailable. Statistics remain scoped to the entire
          cluster.{" "}
          <button
            className="link-button"
            type="button"
            onClick={() => void loadNodes()}
          >
            Retry
          </button>
        </div>
      )}
      {error !== undefined && (
        <div className="notice notice--warning">
          Refresh failed. Showing the last available statistics.{" "}
          <button
            className="link-button"
            type="button"
            onClick={() => void load()}
          >
            Retry
          </button>
        </div>
      )}
      {report.coverage.status !== "complete" && (
        <div className="notice notice--warning">
          Coverage is {report.coverage.status}: {report.coverage.includedNodes}{" "}
          of {report.coverage.expectedNodes} enabled nodes contributed.
          {report.coverage.unsupportedNodes > 0 &&
            ` ${report.coverage.unsupportedNodes} do not support exact range collection.`}
          {report.coverage.staleNodes > 0 &&
            ` ${report.coverage.staleNodes} have stale data.`}{" "}
          <a href="/system/operational-status">View collector health</a>
        </div>
      )}

      {report.state === "unavailable" ? (
        <EmptyState title="Statistics are not available yet">
          <p>
            The controller has not collected a usable exact-range snapshot for
            this scope. Check node coverage below or wait for the next poll.
          </p>
          <a href="/system/operational-status">View collector health</a>
        </EmptyState>
      ) : (
        <>
          <section className="metrics" aria-label="Aggregated DNS metrics">
            <MetricCard
              label="DNS queries"
              value={formatCount(report.totals.dnsQueries)}
            />
            <MetricCard
              label="Blocked by filters"
              value={formatCount(report.totals.blockedFiltering)}
              detail={`${formatPercent(report.totals.blockedPercentage)} of queries`}
            />
            <MetricCard
              label="Safety interventions"
              value={formatCount(report.totals.safetyInterventions)}
              detail={`${formatPercent(report.totals.safetyInterventionPercentage)} of queries`}
            />
            <MetricCard
              label="Average processing"
              value={`${formatNumber(report.totals.averageProcessingMs, 2)} ms`}
              detail="Query-weighted across nodes"
            />
          </section>
          <section className="card statistics-chart-card">
            <div className="section-heading">
              <div>
                <h2>Activity over time</h2>
                <p className="muted">
                  Queries and filter blocks by source time unit.
                </p>
              </div>
              <small>
                Latest collection {formatDate(report.freshness.newestAt)}
              </small>
            </div>
            <Sparkline report={report} />
          </section>
          <section className="statistics-panels" aria-label="Ranked statistics">
            <RankingPanel
              title="Top queried domains"
              values={report.rankings.queriedDomains}
            />
            <RankingPanel
              title="Top blocked domains"
              values={report.rankings.blockedDomains}
            />
            <RankingPanel
              title="Top clients"
              values={report.rankings.clients}
            />
            <RankingPanel
              title="Upstream responses"
              values={report.rankings.upstreamResponses}
            />
            <RankingPanel
              title="Upstream average latency"
              values={report.rankings.upstreamAverageLatencyMs}
              unit=" ms"
            />
          </section>
        </>
      )}

      <section className="section-block">
        <div className="section-heading">
          <h2>Node coverage</h2>
          <small>Collection and freshness remain node-attributed.</small>
        </div>
        <CoverageTable report={report} />
      </section>
    </>
  );
}

function Sparkline({ report }: { report: StatisticsReport }) {
  const maximum = Math.max(
    1,
    ...report.series.flatMap((point) => [
      point.dnsQueries,
      point.blockedFiltering,
    ]),
  );
  const width = 800;
  const height = 220;
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
  if (report.series.length === 0)
    return <p className="muted">No time-series buckets were returned.</p>;
  return (
    <figure className="statistics-chart">
      <svg
        viewBox={`0 0 ${width} ${height}`}
        role="img"
        aria-labelledby="statistics-chart-title statistics-chart-description"
      >
        <title id="statistics-chart-title">
          DNS query and blocked query activity
        </title>
        <desc id="statistics-chart-description">
          {report.series.length} chronological points. Total queries{" "}
          {report.totals.dnsQueries}; blocked {report.totals.blockedFiltering}.
        </desc>
        <path className="statistics-chart__queries" d={path("dnsQueries")} />
        <path
          className="statistics-chart__blocked"
          d={path("blockedFiltering")}
        />
      </svg>
      <figcaption>
        <span className="legend-query">Queries</span>
        <span className="legend-blocked">Blocked</span>
        <span className="statistics-chart__range">
          {formatShortDate(report.series[0]?.at)} –{" "}
          {formatShortDate(report.series.at(-1)?.at)}
        </span>
      </figcaption>
    </figure>
  );
}

function RankingPanel({
  title,
  values,
  unit = "",
}: {
  title: string;
  values: StatisticsRanking[];
  unit?: string;
}) {
  const maximum = values[0]?.value ?? 1;
  return (
    <article className="card statistics-ranking">
      <h2>{title}</h2>
      {values.length === 0 ? (
        <p className="muted">No values reported.</p>
      ) : (
        <ol>
          {values.map((item) => (
            <li key={item.key}>
              <div>
                <span title={item.key}>{item.key}</span>
                <strong>
                  {formatNumber(item.value, unit === "" ? 0 : 2)}
                  {unit}
                </strong>
              </div>
              <span className="statistics-bar" aria-hidden="true">
                <span style={{ width: `${(item.value / maximum) * 100}%` }} />
              </span>
            </li>
          ))}
        </ol>
      )}
    </article>
  );
}

function CoverageTable({ report }: { report: StatisticsReport }) {
  return (
    <div className="table-wrap">
      <table>
        <thead>
          <tr>
            <th>Node</th>
            <th>Status</th>
            <th>Queries</th>
            <th>Collected</th>
            <th>Reason</th>
          </tr>
        </thead>
        <tbody>
          {report.nodes.map((node) => (
            <tr key={node.nodeId}>
              <th scope="row">{node.nodeName}</th>
              <td>
                <StatusBadge
                  status={coverageStatus(node.status)}
                  label={node.status}
                />
              </td>
              <td>
                {node.dnsQueries === undefined
                  ? "—"
                  : formatCount(node.dnsQueries)}
              </td>
              <td>{formatDate(node.collectedAt)}</td>
              <td className="monospace">{node.reasonCode ?? "—"}</td>
            </tr>
          ))}
        </tbody>
      </table>
    </div>
  );
}

function formatCount(value: number) {
  return new Intl.NumberFormat().format(value);
}
function formatNumber(value: number, digits: number) {
  return new Intl.NumberFormat(undefined, {
    maximumFractionDigits: digits,
  }).format(value);
}
function formatPercent(value: number) {
  return `${formatNumber(value, 2)}%`;
}
function formatDate(value?: string) {
  if (value === undefined) return "Never";
  const date = new Date(value);
  return Number.isNaN(date.valueOf()) ? "Unknown" : date.toLocaleString();
}
function formatShortDate(value?: string) {
  if (value === undefined) return "Unknown";
  const date = new Date(value);
  return Number.isNaN(date.valueOf())
    ? "Unknown"
    : date.toLocaleString(undefined, {
        month: "short",
        day: "numeric",
        hour: "numeric",
        minute: "2-digit",
      });
}
function coverageStatus(status: string): StatusKind {
  if (status === "included") return "success";
  if (status === "stale") return "stale";
  if (status === "unsupported") return "unsupported";
  if (status === "maintenance") return "maintenance";
  if (status === "excluded") return "disabled";
  return "warning";
}
