import { useCallback, useEffect, useMemo, useState } from "react";
import {
  DataTable,
  type DataTableColumn,
  NodeBadge,
  Pagination,
} from "../../components/DataDisplay";
import { Banner } from "../../components/Feedback";
import { PageContainer, PageHeader } from "../../components/Page";
import { StatusBadge, type StatusKind } from "../../components/StatusBadge";
import { api } from "../../lib/api";
import type {
  Cluster,
  QueryEvent,
  QueryEventPage,
  QueryEventStatus,
} from "../../lib/types";
import { nodeDetailPath } from "../../routing/routes";

const PAGE_SIZE = 50;
const REFRESH_INTERVAL_MS = 30_000;

export function QueryLogPage({ cluster }: { cluster: Cluster }) {
  const [searchInput, setSearchInput] = useState("");
  const [search, setSearch] = useState("");
  const [status, setStatus] = useState("");
  const [queryType, setQueryType] = useState("");
  const [client, setClient] = useState("");
  const [cursorState, setCursorState] = useState<{
    key: string;
    stack: string[];
  }>({ key: "", stack: [""] });
  const [report, setReport] = useState<QueryEventPage>();
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<unknown>();
  const [expanded, setExpanded] = useState("");
  const [newerCount, setNewerCount] = useState(0);

  const filterKey = `${search}\u0000${status}\u0000${queryType}\u0000${client}`;
  const cursorStack = cursorState.key === filterKey ? cursorState.stack : [""];
  const cursor = cursorStack.at(-1) ?? "";
  const pageNumber = cursorStack.length;

  useEffect(() => {
    const timer = window.setTimeout(() => setSearch(searchInput.trim()), 350);
    return () => window.clearTimeout(timer);
  }, [searchInput]);

  useEffect(() => {
    void filterKey;
    setExpanded("");
    setNewerCount(0);
  }, [filterKey]);

  const request = useMemo(
    () => ({
      nodeId: "",
      cursor,
      limit: PAGE_SIZE,
      search,
      status,
      queryType,
      client: client.trim(),
    }),
    [client, cursor, queryType, search, status],
  );

  const load = useCallback(
    async (quiet = false) => {
      if (!quiet) setLoading(true);
      try {
        const result = await api.queryEvents(cluster.id, request);
        setReport(result);
        setError(undefined);
        setNewerCount(0);
      } catch (caught) {
        setError(caught);
      } finally {
        if (!quiet) setLoading(false);
      }
    },
    [cluster.id, request],
  );

  useEffect(() => void load(), [load]);

  useEffect(() => {
    const timer = window.setInterval(() => {
      if (cursor === "" && expanded === "") {
        void load(true);
        return;
      }
      void api
        .queryEvents(cluster.id, { ...request, cursor: "", limit: PAGE_SIZE })
        .then((latest) => {
          const visible = new Set(report?.items.map((event) => event.id));
          setNewerCount(
            latest.items.filter((event) => !visible.has(event.id)).length,
          );
        })
        .catch(() => undefined);
    }, REFRESH_INTERVAL_MS);
    return () => window.clearInterval(timer);
  }, [cluster.id, cursor, expanded, load, report?.items, request]);

  const columns: readonly DataTableColumn<QueryEvent>[] = [
    {
      id: "time",
      header: "Time",
      render: (event) => (
        <time dateTime={event.timestamp}>{formatTime(event.timestamp)}</time>
      ),
    },
    {
      id: "node",
      header: "Node",
      render: (event) => <NodeBadge name={event.nodeName} />,
    },
    {
      id: "client",
      header: "Client",
      render: (event) => (
        <span title={event.clientIdentifier}>
          {event.clientDisplayName || event.clientIdentifier || "Unknown"}
        </span>
      ),
    },
    {
      id: "query",
      header: "Query",
      className: "query-log__domain",
      render: (event) => <strong>{event.query}</strong>,
    },
    {
      id: "type",
      header: "Type",
      render: (event) => <code>{event.queryType}</code>,
    },
    {
      id: "status",
      header: "Status",
      render: (event) => (
        <StatusBadge
          status={statusTone(event.status)}
          label={statusLabel(event.status)}
        />
      ),
    },
    {
      id: "elapsed",
      header: "Processing",
      align: "right",
      render: (event) => `${event.processingTimeMs.toFixed(2)} ms`,
    },
    {
      id: "details",
      header: <span className="visually-hidden">Details</span>,
      align: "right",
      render: (event) => (
        <button
          type="button"
          className="table-disclosure"
          aria-expanded={expanded === event.id}
          aria-controls={`query-detail-${event.id}`}
          aria-label={`${expanded === event.id ? "Close" : "View"} details for ${event.query}`}
          onClick={() => setExpanded(expanded === event.id ? "" : event.id)}
        >
          <span aria-hidden="true">{expanded === event.id ? "−" : "+"}</span>
        </button>
      ),
    },
  ];

  return (
    <PageContainer size="wide" className="query-log-page">
      <PageHeader
        eyebrow="Monitoring"
        title="Query Log"
        description="Controller-collected, node-attributed DNS query events. Central retention is independent of each node's query-log policy."
        primaryAction={
          <button
            className="button"
            type="button"
            disabled={loading}
            onClick={() => void load()}
          >
            {loading ? "Refreshing…" : "Refresh"}
          </button>
        }
        secondaryActions={
          <a className="button button--secondary" href="/settings/general">
            Node policy and clear
          </a>
        }
      />

      <form
        className="query-log-toolbar"
        aria-label="Query Log filters"
        onSubmit={(event) => event.preventDefault()}
      >
        <label>
          Search queries or clients
          <input
            type="search"
            maxLength={256}
            value={searchInput}
            placeholder="example.org or 192.0.2.10"
            onChange={(event) => setSearchInput(event.target.value)}
          />
        </label>
        <label>
          Response status
          <select
            value={status}
            onChange={(event) => setStatus(event.target.value)}
          >
            <option value="">All statuses</option>
            {(report?.filters.statuses ?? []).map((value) => (
              <option key={value} value={value}>
                {statusLabel(value)}
              </option>
            ))}
          </select>
        </label>
        <label>
          Query type
          <select
            value={queryType}
            onChange={(event) => setQueryType(event.target.value)}
          >
            <option value="">All types</option>
            {(report?.filters.queryTypes ?? []).map((value) => (
              <option key={value} value={value}>
                {value}
              </option>
            ))}
          </select>
        </label>
        <label>
          Client
          <input
            value={client}
            maxLength={512}
            placeholder="Exact client"
            onChange={(event) => setClient(event.target.value)}
          />
        </label>
      </form>

      {report && (
        <p className="query-log-retention muted">
          Central retention: {formatRetention(report.coverage.retentionSeconds)}
          . Node-local enablement, anonymisation, and retention remain managed
          separately in General Settings.
        </p>
      )}

      {newerCount > 0 && (
        <Banner
          tone="info"
          title={`${newerCount} newer ${newerCount === 1 ? "query" : "queries"} available`}
          actions={
            <button
              className="button button--secondary"
              type="button"
              onClick={() => {
                setExpanded("");
                if (cursor === "") void load();
                else setCursorState({ key: filterKey, stack: [""] });
              }}
            >
              Show newest
            </button>
          }
        >
          The current table was kept stable while you examined older results.
        </Banner>
      )}
      {report && <CoverageNotice report={report} />}

      <DataTable
        columns={columns}
        rows={report?.items ?? []}
        rowKey={(event) => event.id}
        caption="Query events for the entire cluster; every row identifies its source node"
        loading={loading && report === undefined}
        error={report === undefined ? error : undefined}
        retry={() => void load()}
        stale={(report?.coverage.staleNodes ?? 0) > 0}
        emptyTitle={
          filtered(request)
            ? "No queries match these filters"
            : "No query events collected yet"
        }
        filteredEmpty={filtered(request)}
        emptyDescription={emptyDescription(report)}
        expandedRowKey={expanded}
        expandedRowId={(event) => `query-detail-${event.id}`}
        renderExpandedRow={(event) => (
          <QueryDetail cluster={cluster} event={event} />
        )}
        pagination={
          <Pagination
            page={pageNumber}
            hasPrevious={pageNumber > 1}
            hasNext={report?.nextCursor !== undefined}
            disabled={loading}
            onPrevious={() =>
              setCursorState({
                key: filterKey,
                stack: cursorStack.slice(0, -1),
              })
            }
            onNext={() =>
              report?.nextCursor &&
              setCursorState({
                key: filterKey,
                stack: [...cursorStack, report.nextCursor],
              })
            }
            label="Query Log pagination"
          />
        }
      />
      {error !== undefined && report !== undefined && (
        <Banner tone="warning" title="Refresh failed">
          The last available results remain visible.
        </Banner>
      )}
    </PageContainer>
  );
}

function CoverageNotice({ report }: { report: QueryEventPage }) {
  const coverage = report.coverage;
  if (!coverage.collectionEnabled) {
    return (
      <Banner tone="warning" title="Central collection is disabled">
        Existing retained events remain searchable. Enable central Query Log
        collection in System Settings to resume ingestion.
      </Banner>
    );
  }
  if (coverage.status === "complete") return null;
  return (
    <Banner tone="warning" title={`Query Log coverage is ${coverage.status}`}>
      {coverage.includedNodes} of {coverage.expectedNodes} enabled nodes have
      contributed data.
      {coverage.staleNodes > 0 && ` ${coverage.staleNodes} stale.`}
      {coverage.unsupportedNodes > 0 &&
        ` ${coverage.unsupportedNodes} unsupported.`}
      {coverage.disabledNodes > 0 &&
        ` ${coverage.disabledNodes} have query logging disabled.`}
      {coverage.errorNodes > 0 && ` ${coverage.errorNodes} currently failing.`}
      {coverage.gapNodes > 0 &&
        ` ${coverage.gapNodes} have a known ingestion gap.`}{" "}
      <a href="/system/operational-status">View ingestion health</a>
    </Banner>
  );
}

function QueryDetail({
  cluster,
  event,
}: {
  cluster: Cluster;
  event: QueryEvent;
}) {
  const domain = encodeURIComponent(event.query);
  const client = encodeURIComponent(event.clientIdentifier);
  return (
    <div className="query-log-detail">
      <dl className="summary-grid">
        <Detail
          label="Timestamp"
          value={new Date(event.timestamp).toLocaleString()}
        />
        <Detail label="Node" value={event.nodeName} />
        <Detail label="Client" value={event.clientDisplayName || "—"} />
        <Detail
          label="Client identifier"
          value={event.clientIdentifier || "—"}
        />
        <Detail label="Query" value={event.query} />
        <Detail label="Query type" value={event.queryType} />
        <Detail
          label="Response"
          value={`${statusLabel(event.status)}${event.responseCode ? ` · ${event.responseCode}` : ""}`}
        />
        <Detail
          label="Processing time"
          value={`${event.processingTimeMs.toFixed(3)} ms`}
        />
        <Detail label="Upstream" value={event.upstream || "—"} />
        <Detail label="Filtering reason" value={event.filteringReason || "—"} />
        <Detail label="Service" value={event.serviceName || "—"} />
        <Detail
          label="Flags"
          value={
            [event.cached && "Cached", event.answerDnssec && "DNSSEC"]
              .filter(Boolean)
              .join(", ") || "—"
          }
        />
      </dl>
      {event.rules.length > 0 && (
        <DetailList
          title="Matched rules"
          items={event.rules.map((rule) =>
            rule.filterListId
              ? `${rule.text} (filter ${rule.filterListId})`
              : rule.text,
          )}
        />
      )}
      {event.answers.length > 0 && (
        <DetailList
          title="Answers"
          items={event.answers.map(
            (answer) =>
              `${answer.type} ${answer.value}${answer.ttl === undefined ? "" : ` · TTL ${answer.ttl}`}`,
          )}
        />
      )}
      <nav
        className="query-log-actions"
        aria-label={`Actions for ${event.query}`}
      >
        <a
          className="button button--secondary"
          href={`/filters/custom-rules?action=allow&domain=${domain}`}
        >
          Allow domain
        </a>
        <a
          className="button button--secondary"
          href={`/filters/custom-rules?action=block&domain=${domain}`}
        >
          Block domain
        </a>
        <a
          className="button button--secondary"
          href={`/filters/rewrites?action=create&domain=${domain}`}
        >
          Create rewrite
        </a>
        {event.clientIdentifier && (
          <a
            className="button button--quiet"
            href={`/settings/clients?client=${client}`}
          >
            Find managed client
          </a>
        )}
        <a className="button button--quiet" href={nodeDetailPath(event.nodeId)}>
          View node
        </a>
        <a
          className="button button--quiet"
          href={`/ha/configuration?clusterId=${encodeURIComponent(cluster.id)}`}
        >
          Configuration Control
        </a>
      </nav>
      <p className="muted">
        Configuration actions open the mutable draft workflow. They never
        publish, deploy, or directly change a node.
      </p>
    </div>
  );
}

function Detail({ label, value }: { label: string; value: string }) {
  return (
    <div>
      <dt>{label}</dt>
      <dd>{value}</dd>
    </div>
  );
}

function DetailList({ title, items }: { title: string; items: string[] }) {
  return (
    <section className="inline-detail-section">
      <h3>{title}</h3>
      <ul className="compact-list">
        {items.map((item) => (
          <li key={item}>
            <code>{item}</code>
          </li>
        ))}
      </ul>
    </section>
  );
}

function filtered(request: {
  search: string;
  status: string;
  queryType: string;
  client: string;
}) {
  return (
    request.search !== "" ||
    request.status !== "" ||
    request.queryType !== "" ||
    request.client.trim() !== ""
  );
}

function emptyDescription(report?: QueryEventPage) {
  if (report && !report.coverage.collectionEnabled)
    return "Central collection is disabled; retained results may still appear when filters are cleared.";
  if (
    report &&
    report.coverage.disabledNodes === report.coverage.expectedNodes &&
    report.coverage.expectedNodes > 0
  )
    return "Query logging is disabled on every node in this cluster.";
  return "Wait for the next collection pass or clear the current filters.";
}

function statusLabel(status: QueryEventStatus) {
  return status
    .split("_")
    .map((part) => part[0]?.toUpperCase() + part.slice(1))
    .join(" ");
}

function statusTone(status: QueryEventStatus): StatusKind {
  if (status === "allowed") return "success";
  if (status === "blocked" || status === "error") return "failed";
  if (status === "rewritten") return "info";
  return "warning";
}

function formatTime(value: string) {
  return new Intl.DateTimeFormat(undefined, {
    hour: "2-digit",
    minute: "2-digit",
    second: "2-digit",
  }).format(new Date(value));
}

function formatRetention(seconds: number) {
  const hours = Math.round(seconds / 3600);
  if (hours % 24 === 0) {
    const days = hours / 24;
    return `${days} ${days === 1 ? "day" : "days"}`;
  }
  return `${hours} ${hours === 1 ? "hour" : "hours"}`;
}
