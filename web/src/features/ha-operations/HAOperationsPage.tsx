import { useCallback, useEffect, useState } from "react";
import { HealthSummaryCard, Pagination } from "../../components/DataDisplay";
import {
  Banner,
  EmptyState,
  ErrorState,
  Loading,
} from "../../components/Feedback";
import { PageHeader } from "../../components/Page";
import { StatusBadge } from "../../components/StatusBadge";
import { api } from "../../lib/api";
import type {
  CertificateHealth,
  Cluster,
  HAHistoryItem,
  HASummary,
  Node,
  UpgradeOperation,
  VersionHealth,
} from "../../lib/types";
import { nodeDetailPath } from "../../routing/routes";

type HASource =
  | "summary"
  | "nodes"
  | "certificates"
  | "versions"
  | "history"
  | "upgrades";

export function HAOperationsPage({ cluster }: { cluster: Cluster }) {
  const [summary, setSummary] = useState<HASummary>();
  const [nodes, setNodes] = useState<Node[]>([]);
  const [certificates, setCertificates] = useState<CertificateHealth[]>([]);
  const [versions, setVersions] = useState<VersionHealth[]>([]);
  const [events, setEvents] = useState<HAHistoryItem[]>([]);
  const [historyCursorStack, setHistoryCursorStack] = useState([""]);
  const [historyNextCursor, setHistoryNextCursor] = useState("");
  const [historyLoading, setHistoryLoading] = useState(false);
  const [upgrades, setUpgrades] = useState<UpgradeOperation[]>([]);
  const [loading, setLoading] = useState(true);
  const [loadedSources, setLoadedSources] = useState<
    Partial<Record<HASource, true>>
  >({});
  const [sourceErrors, setSourceErrors] = useState<
    Partial<Record<HASource, unknown>>
  >({});

  const historyCursor = historyCursorStack.at(-1) ?? "";
  const load = useCallback(async () => {
    setHistoryLoading(true);
    const results = await Promise.allSettled([
      api.haStatus(cluster.id),
      api.nodes(cluster.id),
      api.certificates(cluster.id),
      api.versions(cluster.id),
      api.haHistory(cluster.id, { cursor: historyCursor, limit: 50 }),
      api.upgrades(cluster.id),
    ] as const);
    const errors: Partial<Record<HASource, unknown>> = {};
    const loaded: Partial<Record<HASource, true>> = {};
    const [
      ha,
      nodeResult,
      certificateResult,
      versionResult,
      history,
      upgradeResult,
    ] = results;

    if (ha.status === "fulfilled") {
      setSummary(ha.value);
      loaded.summary = true;
    } else errors.summary = ha.reason;
    if (nodeResult.status === "fulfilled") {
      setNodes(nodeResult.value.items);
      loaded.nodes = true;
    } else errors.nodes = nodeResult.reason;
    if (certificateResult.status === "fulfilled") {
      setCertificates(certificateResult.value.items);
      loaded.certificates = true;
    } else errors.certificates = certificateResult.reason;
    if (versionResult.status === "fulfilled") {
      setVersions(versionResult.value.items);
      loaded.versions = true;
    } else errors.versions = versionResult.reason;
    if (history.status === "fulfilled") {
      setEvents(history.value.items);
      setHistoryNextCursor(history.value.nextCursor ?? "");
      loaded.history = true;
    } else errors.history = history.reason;
    if (upgradeResult.status === "fulfilled") {
      setUpgrades(upgradeResult.value.items);
      loaded.upgrades = true;
    } else errors.upgrades = upgradeResult.reason;

    setLoadedSources((current) => ({ ...current, ...loaded }));
    setSourceErrors(errors);
    setLoading(false);
    setHistoryLoading(false);
  }, [cluster.id, historyCursor]);

  useEffect(() => {
    void cluster.id;
    setHistoryCursorStack([""]);
  }, [cluster.id]);

  useEffect(() => {
    void load();
    const timer = window.setInterval(() => void load(), 30_000);
    return () => window.clearInterval(timer);
  }, [load]);

  const hasLoadedSource = Object.keys(loadedSources).length > 0;
  if (loading && !hasLoadedSource)
    return <Loading label="Loading HA operations…" />;
  if (!hasLoadedSource)
    return (
      <ErrorState
        error={Object.values(sourceErrors)[0]}
        retry={() => void load()}
      />
    );
  const nodeName = new Map(nodes.map((node) => [node.id, node.name]));

  return (
    <>
      <PageHeader
        eyebrow="HA Controller"
        title="HA Operations"
        description="DNS redundancy, lifecycle work, certificates, software, and operational history."
        primaryAction={
          summary && (
            <StatusBadge
              status={summary.state === "at_risk" ? "failed" : summary.state}
              label={summary.state === "at_risk" ? "At Risk" : undefined}
            />
          )
        }
      />
      <SourceWarning
        label="HA summary"
        error={sourceErrors.summary}
        stale={loadedSources.summary === true}
        retry={load}
      />
      {summary?.state !== undefined && summary.state !== "healthy" && (
        <Banner tone="warning" title="HA capacity needs attention">
          {summary.message}
        </Banner>
      )}

      {summary && (
        <section
          className="health-summary-grid health-summary-grid--four"
          aria-label="HA redundancy summary"
        >
          <HealthSummaryCard
            icon="dns"
            label="DNS serving"
            value={`${summary.servingDnsNodes} / ${summary.totalNodes}`}
            status={coverageStatus(summary.servingDnsNodes, summary.totalNodes)}
            detail="nodes serving DNS"
          />
          <HealthSummaryCard
            icon="nodes"
            label="API reachable"
            value={`${summary.apiReachableNodes} / ${summary.totalNodes}`}
            status={coverageStatus(
              summary.apiReachableNodes,
              summary.totalNodes,
            )}
            detail="management APIs responding"
          />
          <HealthSummaryCard
            icon="revisions"
            label="Converged"
            value={`${summary.convergedNodes} / ${summary.totalNodes}`}
            status={coverageStatus(summary.convergedNodes, summary.totalNodes)}
            detail="nodes on desired revision"
          />
          <HealthSummaryCard
            icon="system"
            label="Maintenance"
            value={String(summary.maintenanceNodes)}
            status={summary.maintenanceNodes === 0 ? "healthy" : "maintenance"}
            detail="nodes out of service"
          />
        </section>
      )}

      <section className="section-block">
        <div className="section-heading">
          <div>
            <h2>Node lifecycle</h2>
            <small>
              Open a node for maintenance, DNS probe, TLS, and guided upgrade
              workflows.
            </small>
          </div>
        </div>
        <SourceWarning
          label="Node inventory"
          error={sourceErrors.nodes}
          stale={loadedSources.nodes === true}
          retry={load}
        />
        <SourceWarning
          label="Version status"
          error={sourceErrors.versions}
          stale={loadedSources.versions === true}
          retry={load}
        />
        {loadedSources.nodes === true && nodes.length === 0 ? (
          <EmptyState title="No managed nodes" />
        ) : loadedSources.nodes === true ? (
          <div className="table-wrap">
            <table>
              <thead>
                <tr>
                  <th>Node</th>
                  <th>API</th>
                  <th>DNS</th>
                  <th>Maintenance</th>
                  <th>Version</th>
                  <th>Update</th>
                </tr>
              </thead>
              <tbody>
                {nodes.map((node) => {
                  const version = versions.find(
                    (item) => item.nodeId === node.id,
                  );
                  const dns = summary?.nodes.find(
                    (item) => item.nodeId === node.id,
                  );
                  return (
                    <tr key={node.id}>
                      <td>
                        <a href={nodeDetailPath(node.id)}>
                          <strong>{node.name}</strong>
                        </a>
                      </td>
                      <td>
                        <StatusBadge status={node.healthStatus} />
                      </td>
                      <td>
                        <a href={nodeDetailPath(node.id)}>
                          <StatusBadge status={dns?.dnsStatus ?? "unknown"} />
                        </a>
                      </td>
                      <td>
                        {node.maintenanceMode ? (
                          <StatusBadge status="maintenance" />
                        ) : (
                          "In service"
                        )}
                      </td>
                      <td>{node.version ?? "Unknown"}</td>
                      <td>
                        {loadedSources.versions !== true ? (
                          "Unavailable"
                        ) : version?.updateAvailable ? (
                          <StatusBadge status="warning" label="Available" />
                        ) : version?.releaseCheckStale ? (
                          "Check unavailable"
                        ) : (
                          "Current"
                        )}
                      </td>
                    </tr>
                  );
                })}
              </tbody>
            </table>
          </div>
        ) : null}
      </section>

      <section className="section-block">
        <div className="section-heading">
          <h2>Certificate expiry</h2>
          {summary && <span>{summary.certificateWarnings} warnings</span>}
        </div>
        <SourceWarning
          label="Certificate status"
          error={sourceErrors.certificates}
          stale={loadedSources.certificates === true}
          retry={load}
        />
        {loadedSources.certificates === true && (
          <div className="table-wrap">
            <table>
              <thead>
                <tr>
                  <th>Node</th>
                  <th>Certificate</th>
                  <th>Expiry</th>
                  <th>Remaining</th>
                  <th>State</th>
                </tr>
              </thead>
              <tbody>
                {certificates.map((certificate) => (
                  <tr key={certificate.nodeId}>
                    <td>{certificate.nodeName}</td>
                    <td>
                      {certificate.state === "not_applicable"
                        ? "Not configured"
                        : certificate.subject || "Not reported"}
                    </td>
                    <td>
                      {certificate.state === "not_applicable"
                        ? "Not applicable"
                        : formatTime(certificate.notAfter)}
                    </td>
                    <td>
                      {certificate.daysRemaining === undefined
                        ? "—"
                        : `${certificate.daysRemaining} days`}
                    </td>
                    <td>
                      <StatusBadge
                        status={
                          certificate.state === "critical" ||
                          certificate.state === "expired"
                            ? "failed"
                            : certificate.state
                        }
                        label={certificate.state.replaceAll("_", " ")}
                      />
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        )}
      </section>

      <section className="section-block">
        <div className="section-heading">
          <h2>Guided upgrades</h2>
          <small>
            Atlas DNS Controller coordinates validation; it never opens a remote
            shell.
          </small>
        </div>
        <SourceWarning
          label="Guided upgrade history"
          error={sourceErrors.upgrades}
          stale={loadedSources.upgrades === true}
          retry={load}
        />
        {loadedSources.upgrades !== true ? null : upgrades.length === 0 ? (
          <EmptyState title="No upgrade history">
            <p>Start a supported guided upgrade from Node Detail.</p>
          </EmptyState>
        ) : (
          <div className="table-wrap">
            <table>
              <thead>
                <tr>
                  <th>Node</th>
                  <th>Versions</th>
                  <th>Mode</th>
                  <th>Status</th>
                  <th>Started</th>
                </tr>
              </thead>
              <tbody>
                {upgrades.map((upgrade) => (
                  <tr key={upgrade.id}>
                    <td>{nodeName.get(upgrade.nodeId) ?? upgrade.nodeId}</td>
                    <td>
                      {upgrade.fromVersion} → {upgrade.targetVersion}
                    </td>
                    <td>{upgrade.mode}</td>
                    <td>
                      <StatusBadge
                        status={
                          upgrade.status === "succeeded"
                            ? "success"
                            : upgrade.status === "failed"
                              ? "failed"
                              : "pending"
                        }
                        label={upgrade.status.replaceAll("_", " ")}
                      />
                    </td>
                    <td>{formatTime(upgrade.startedAt)}</td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        )}
      </section>
      <Banner
        tone="info"
        title="Notification management"
        actions={
          <a className="button button--secondary" href="/ha/notifications">
            Manage Notifications
          </a>
        }
      >
        Notification channels and event policy are managed on Notifications.
        Delivery and webhook-test evidence remains in Operational History.
      </Banner>

      <section className="section-block">
        <div className="section-heading">
          <h2>Operational history</h2>
          <small>State transitions, not every successful probe.</small>
        </div>
        <SourceWarning
          label="Operational History"
          error={sourceErrors.history}
          stale={loadedSources.history === true}
          retry={load}
        />
        {loadedSources.history !== true ? null : events.length === 0 ? (
          <EmptyState title="No HA transitions recorded" />
        ) : (
          <div className="table-wrap">
            <table>
              <thead>
                <tr>
                  <th>Time</th>
                  <th>Node</th>
                  <th>Event</th>
                  <th>Severity</th>
                  <th>Notification</th>
                  <th>Summary</th>
                </tr>
              </thead>
              <tbody>
                {events.map((event) => (
                  <tr key={event.id}>
                    <td>{formatTime(event.occurredAt)}</td>
                    <td>
                      {event.nodeId
                        ? (nodeName.get(event.nodeId) ?? event.nodeId)
                        : "Cluster"}
                    </td>
                    <td>
                      {event.notification?.test
                        ? "Webhook test"
                        : event.kind === "notification"
                          ? `Webhook · ${event.eventType}`
                          : event.eventType}
                    </td>
                    <td>
                      <StatusBadge
                        status={
                          event.severity === "critical"
                            ? "failed"
                            : event.severity
                        }
                      />
                    </td>
                    <td>
                      {event.notification ? (
                        <StatusBadge
                          status={notificationTone(event.notification.status)}
                          label={notificationLabel(event.notification.status)}
                        />
                      ) : (
                        "—"
                      )}
                    </td>
                    <td>
                      {event.summary}
                      {event.notification && (
                        <span className="table-subtitle">
                          {event.notification.channelName}
                          {` · ${event.notification.attemptCount} ${event.notification.attemptCount === 1 ? "attempt" : "attempts"}`}
                          {event.notification.errorSummary
                            ? ` · ${event.notification.errorSummary}`
                            : ""}
                        </span>
                      )}
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        )}
        <Pagination
          page={historyCursorStack.length}
          hasPrevious={historyCursorStack.length > 1}
          hasNext={historyNextCursor !== ""}
          disabled={historyLoading}
          onPrevious={() =>
            setHistoryCursorStack((current) => current.slice(0, -1))
          }
          onNext={() =>
            historyNextCursor !== "" &&
            setHistoryCursorStack((current) => [...current, historyNextCursor])
          }
          label="Operational History pagination"
        />
      </section>
    </>
  );
}

function coverageStatus(current: number, expected: number) {
  if (expected === 0) return "unknown" as const;
  if (current >= expected) return "healthy" as const;
  return current === 0 ? ("failed" as const) : ("degraded" as const);
}

function SourceWarning({
  label,
  error,
  stale,
  retry,
}: {
  label: string;
  error?: unknown;
  stale: boolean;
  retry: () => Promise<void>;
}) {
  if (error === undefined) return null;
  return (
    <Banner
      tone="warning"
      title={`${label} ${stale ? "refresh failed" : "unavailable"}`}
      actions={
        <button
          className="button button--secondary"
          type="button"
          onClick={() => void retry()}
        >
          Try again
        </button>
      }
    >
      {stale
        ? `The last successful ${label.toLowerCase()} data remains visible and may be stale.`
        : error instanceof Error
          ? error.message
          : `Atlas could not load ${label.toLowerCase()}.`}
    </Banner>
  );
}

function notificationTone(status: string) {
  if (status === "delivered") return "success" as const;
  if (status === "failed") return "failed" as const;
  if (status === "suppressed") return "maintenance" as const;
  return "pending" as const;
}

function notificationLabel(status: string) {
  return status.charAt(0).toUpperCase() + status.slice(1);
}

function formatTime(value?: string) {
  return value ? new Date(value).toLocaleString() : "—";
}
