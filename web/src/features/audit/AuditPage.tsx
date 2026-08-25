import { useCallback, useEffect, useMemo, useState } from "react";
import {
  DataTable,
  type DataTableColumn,
  Pagination,
} from "../../components/DataDisplay";
import { Banner } from "../../components/Feedback";
import { PageContainer, PageHeader } from "../../components/Page";
import { api } from "../../lib/api";
import type { AuditEvent, AuditEventPage } from "../../lib/types";
import { useQuerySelection } from "../../lib/useQuerySelection";
import {
  auditActionLabel,
  auditActorLabel,
  auditResourceHref,
  auditResourceLabel,
  presentAuditChange,
} from "./auditPresentation";

const PAGE_SIZE = 50;

export function AuditPage() {
  const [page, setPage] = useState<AuditEventPage>();
  const [cursorStack, setCursorStack] = useState([""]);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState<unknown>();
  const [selectedOutsidePage, setSelectedOutsidePage] = useState<AuditEvent>();
  const [selectionError, setSelectionError] = useState<unknown>();
  const { selectedID, toggle, scrollIntoViewOnce } =
    useQuerySelection("auditEventId");
  const cursor = cursorStack.at(-1) ?? "";

  const load = useCallback(async () => {
    setLoading(true);
    try {
      setPage(await api.auditEvents({ cursor, limit: PAGE_SIZE }));
      setError(undefined);
    } catch (caught) {
      setError(caught);
    } finally {
      setLoading(false);
    }
  }, [cursor]);

  useEffect(() => {
    void load();
  }, [load]);

  useEffect(() => {
    if (!selectedID || page === undefined) {
      setSelectedOutsidePage(undefined);
      setSelectionError(undefined);
      return;
    }
    if (page.items.some((event) => event.id === selectedID)) {
      setSelectedOutsidePage(undefined);
      setSelectionError(undefined);
      return;
    }
    let active = true;
    void api
      .auditEvent(selectedID)
      .then((event) => {
        if (!active) return;
        setSelectedOutsidePage(event);
        setSelectionError(undefined);
      })
      .catch((caught) => {
        if (!active) return;
        setSelectedOutsidePage(undefined);
        setSelectionError(caught);
      });
    return () => {
      active = false;
    };
  }, [page, selectedID]);

  const rows = useMemo(() => {
    if (page === undefined) return [];
    if (selectedOutsidePage === undefined) return page.items;
    return [selectedOutsidePage, ...page.items];
  }, [page, selectedOutsidePage]);
  const selected = rows.find((event) => event.id === selectedID);

  useEffect(() => {
    if (selected) scrollIntoViewOnce(selected.id, auditSummaryID(selected.id));
  }, [scrollIntoViewOnce, selected]);

  const columns: readonly DataTableColumn<AuditEvent>[] = [
    {
      id: "time",
      header: "Time",
      render: (event) => (
        <time dateTime={event.createdAt}>{formatTime(event.createdAt)}</time>
      ),
    },
    {
      id: "action",
      header: "Action",
      render: (event) => (
        <strong id={auditSummaryID(event.id)}>
          {auditActionLabel(event.action)}
        </strong>
      ),
    },
    {
      id: "resource",
      header: "Resource",
      render: (event) => (
        <>
          {auditResourceLabel(event.resourceType)}
          {event.resourceId && (
            <span className="table-subtitle monospace">
              {shortID(event.resourceId)}
            </span>
          )}
        </>
      ),
    },
    {
      id: "actor",
      header: "Actor",
      render: (event) => auditActorLabel(event),
    },
    {
      id: "details",
      header: <span className="visually-hidden">Details</span>,
      align: "right",
      render: (event) => {
        const expanded = selectedID === event.id;
        return (
          <button
            type="button"
            className="table-disclosure"
            aria-expanded={expanded}
            aria-controls={auditDetailID(event.id)}
            aria-label={`${expanded ? "Hide" : "View"} audit evidence for ${auditActionLabel(event.action)}`}
            onClick={() => toggle(event.id)}
          >
            <span aria-hidden="true">{expanded ? "−" : "+"}</span>
          </button>
        );
      },
    },
  ];

  return (
    <PageContainer size="wide" className="audit-page">
      <PageHeader
        eyebrow="System"
        title="Audit log"
        description="Who changed controller or managed-domain state, with durable request and resource evidence. Actor names are current labels, not historical snapshots."
        primaryAction={
          <button
            className="button button--secondary"
            type="button"
            disabled={loading}
            onClick={() => void load()}
          >
            Refresh
          </button>
        }
      />
      {cursor !== "" && (
        <Banner
          tone="info"
          title="Inspecting older audit evidence"
          actions={
            <button
              className="button button--secondary"
              type="button"
              disabled={loading}
              onClick={() => setCursorStack([""])}
            >
              Show newest
            </button>
          }
        >
          This page remains stable while new audit events are recorded.
        </Banner>
      )}
      {selectedOutsidePage !== undefined && (
        <Banner tone="info" title="Selected audit event loaded directly">
          The deep-linked event is outside page {cursorStack.length}; it is
          pinned above this page so its exact evidence remains available.
        </Banner>
      )}
      {selectionError !== undefined && (
        <Banner tone="warning" title="Audit event unavailable">
          The selected audit event does not exist or cannot be loaded. The
          current audit page remains available.
        </Banner>
      )}
      <DataTable
        columns={columns}
        rows={rows}
        rowKey={(event) => event.id}
        caption="Audit events, newest first"
        loading={page === undefined && loading}
        loadingLabel="Loading audit events…"
        error={page === undefined ? error : undefined}
        retry={() => void load()}
        emptyTitle="No audit events"
        emptyDescription={<p>Material actions will appear here.</p>}
        expandedRowKey={selected?.id}
        expandedRowId={(event) => auditDetailID(event.id)}
        renderExpandedRow={(event) => <AuditEventDetail event={event} />}
        pagination={
          <Pagination
            page={cursorStack.length}
            hasPrevious={cursorStack.length > 1}
            hasNext={page?.nextCursor !== undefined}
            disabled={loading}
            onPrevious={() => setCursorStack((current) => current.slice(0, -1))}
            onNext={() =>
              page?.nextCursor &&
              setCursorStack((current) => [...current, page.nextCursor ?? ""])
            }
            label="Audit Log pagination"
          />
        }
      />
      {error !== undefined && page !== undefined && (
        <Banner tone="warning" title="Refresh failed">
          The last available audit page remains visible.
        </Banner>
      )}
    </PageContainer>
  );
}

function AuditEventDetail({ event }: { event: AuditEvent }) {
  const change = presentAuditChange(event);
  const resourceHref = auditResourceHref(event);
  return (
    <article
      className="audit-event-detail"
      aria-labelledby={`audit-${event.id}-heading`}
    >
      <div>
        <h3 id={`audit-${event.id}-heading`}>{change.title}</h3>
        <p className="muted">{change.description}</p>
      </div>
      <dl className="summary-grid" aria-label="Audit identity evidence">
        <div>
          <dt>Raw action code</dt>
          <dd>
            <code>{event.action}</code>
          </dd>
        </div>
        <div>
          <dt>Audit event ID</dt>
          <dd>
            <code>{event.id}</code>
          </dd>
        </div>
        <div>
          <dt>Actor type</dt>
          <dd>{auditResourceLabel(event.actorType)}</dd>
        </div>
        <div>
          <dt>Current actor label</dt>
          <dd>{auditActorLabel(event)}</dd>
        </div>
        <div>
          <dt>Immutable actor user ID</dt>
          <dd>{event.actorUserId ? <code>{event.actorUserId}</code> : "—"}</dd>
        </div>
        <div>
          <dt>Resource type</dt>
          <dd>{auditResourceLabel(event.resourceType)}</dd>
        </div>
        <div>
          <dt>Resource ID</dt>
          <dd>{event.resourceId ? <code>{event.resourceId}</code> : "—"}</dd>
        </div>
        <div>
          <dt>Canonical resource</dt>
          <dd>
            {resourceHref ? <a href={resourceHref}>Open resource</a> : "—"}
          </dd>
        </div>
        <div>
          <dt>Request ID</dt>
          <dd>
            <code>{event.requestId}</code>
          </dd>
        </div>
        <div>
          <dt>Timestamp</dt>
          <dd>
            <time dateTime={event.createdAt}>
              {formatTime(event.createdAt)}
            </time>
          </dd>
        </div>
      </dl>
      <section className="inline-detail-section" aria-label="Recorded change">
        <h4>What changed</h4>
        {change.fields.length === 0 ? (
          <p className="muted">
            This action recorded no additional safe change fields.
          </p>
        ) : (
          <dl className="audit-change-list">
            {change.fields.map((item) => (
              <div key={`${item.label}-${item.value}`}>
                <dt>{humanizeKey(item.label)}</dt>
                <dd>{item.value}</dd>
              </div>
            ))}
          </dl>
        )}
      </section>
    </article>
  );
}

function auditSummaryID(id: string) {
  return `audit-summary-${id}`;
}
function auditDetailID(id: string) {
  return `audit-detail-${id}`;
}
function shortID(id: string) {
  return id.slice(0, 8);
}
function formatTime(value: string) {
  return new Date(value).toLocaleString();
}
function humanizeKey(value: string) {
  return value
    .replaceAll(".", " · ")
    .replace(/([a-z])([A-Z])/g, "$1 $2")
    .replace(/^./, (letter) => letter.toUpperCase());
}
