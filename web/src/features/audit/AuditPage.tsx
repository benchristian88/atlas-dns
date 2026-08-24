import { useCallback, useEffect, useState } from "react";
import { DataTable, type DataTableColumn } from "../../components/DataDisplay";
import { PageContainer, PageHeader } from "../../components/Page";
import { api } from "../../lib/api";
import type { AuditEvent } from "../../lib/types";

const columns: readonly DataTableColumn<AuditEvent>[] = [
  {
    id: "time",
    header: "Time",
    render: (event) => new Date(event.createdAt).toLocaleString(),
  },
  {
    id: "action",
    header: "Action",
    render: (event) => <strong>{event.action}</strong>,
  },
  {
    id: "resource",
    header: "Resource",
    render: (event) => (
      <>
        {event.resourceType}
        <span className="table-subtitle monospace">
          {event.resourceId ?? "—"}
        </span>
      </>
    ),
  },
  { id: "actor", header: "Actor", render: (event) => event.actorType },
  {
    id: "request",
    header: "Request ID",
    className: "monospace",
    render: (event) => event.requestId,
  },
];

export function AuditPage() {
  const [events, setEvents] = useState<AuditEvent[]>();
  const [error, setError] = useState<unknown>();
  const load = useCallback(async () => {
    try {
      setEvents((await api.auditEvents()).items);
      setError(undefined);
    } catch (caught) {
      setError(caught);
    }
  }, []);
  useEffect(() => {
    void load();
  }, [load]);

  return (
    <PageContainer size="wide">
      <PageHeader
        eyebrow="System"
        title="Audit log"
        description="Durable security and node-management activity."
      />
      <DataTable
        columns={columns}
        rows={events ?? []}
        rowKey={(event) => event.id}
        caption="Audit events"
        loading={events === undefined && error === undefined}
        loadingLabel="Loading audit events…"
        error={events === undefined ? error : undefined}
        retry={() => void load()}
        emptyTitle="No audit events"
        emptyDescription={<p>Material actions will appear here.</p>}
      />
    </PageContainer>
  );
}
