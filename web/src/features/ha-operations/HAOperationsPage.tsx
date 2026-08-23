import { useCallback, useEffect, useState } from "react";
import { MetricCard, Pagination } from "../../components/DataDisplay";
import {
  Banner,
  EmptyState,
  ErrorState,
  Loading,
} from "../../components/Feedback";
import { PageHeader } from "../../components/Page";
import { Field, SettingsGroup } from "../../components/Settings";
import { StatusBadge } from "../../components/StatusBadge";
import { api } from "../../lib/api";
import type {
  CertificateHealth,
  Cluster,
  HAHistoryItem,
  HASummary,
  Node,
  NotificationChannel,
  UpgradeOperation,
  VersionHealth,
} from "../../lib/types";

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
  const [channels, setChannels] = useState<NotificationChannel[]>([]);
  const [error, setError] = useState<unknown>();
  const [showWebhook, setShowWebhook] = useState(false);
  const [editingChannel, setEditingChannel] = useState<NotificationChannel>();
  const [webhookName, setWebhookName] = useState("");
  const [webhookURL, setWebhookURL] = useState("");
  const [webhookEnabled, setWebhookEnabled] = useState(true);
  const [replaceDestination, setReplaceDestination] = useState(false);
  const [webhookBusy, setWebhookBusy] = useState("");
  const [webhookFeedback, setWebhookFeedback] = useState<{
    tone: "success" | "warning";
    title: string;
    message: string;
  }>();

  const historyCursor = historyCursorStack.at(-1) ?? "";
  const load = useCallback(async () => {
    setHistoryLoading(true);
    try {
      const [
        ha,
        nodeResult,
        certificateResult,
        versionResult,
        history,
        upgradeResult,
        channelResult,
      ] = await Promise.all([
        api.haStatus(cluster.id),
        api.nodes(cluster.id),
        api.certificates(cluster.id),
        api.versions(cluster.id),
        api.haHistory(cluster.id, { cursor: historyCursor, limit: 50 }),
        api.upgrades(cluster.id),
        api.notificationChannels(cluster.id),
      ]);
      setSummary(ha);
      setNodes(nodeResult.items);
      setCertificates(certificateResult.items);
      setVersions(versionResult.items);
      setEvents(history.items);
      setHistoryNextCursor(history.nextCursor ?? "");
      setUpgrades(upgradeResult.items);
      setChannels(channelResult.items);
      setError(undefined);
    } catch (caught) {
      setError(caught);
    } finally {
      setHistoryLoading(false);
    }
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

  if (summary === undefined && error === undefined)
    return <Loading label="Loading HA operations…" />;
  if (summary === undefined)
    return <ErrorState error={error} retry={() => void load()} />;
  const nodeName = new Map(nodes.map((node) => [node.id, node.name]));

  return (
    <>
      <PageHeader
        eyebrow="HA Controller"
        title="HA Operations"
        description="DNS redundancy, lifecycle work, certificates, software, and operational history."
        primaryAction={
          <StatusBadge
            status={summary.state === "at_risk" ? "failed" : summary.state}
            label={summary.state === "at_risk" ? "At Risk" : undefined}
          />
        }
      />
      {error !== undefined && (
        <Banner tone="warning" title="Refresh failed">
          Showing the last complete HA snapshot.
        </Banner>
      )}
      {summary.state !== "healthy" && (
        <Banner tone="warning" title="HA capacity needs attention">
          {summary.message}
        </Banner>
      )}

      <section className="metrics" aria-label="HA redundancy summary">
        <MetricCard
          label="DNS serving"
          value={`${summary.servingDnsNodes} / ${summary.totalNodes}`}
        />
        <MetricCard
          label="API reachable"
          value={`${summary.apiReachableNodes} / ${summary.totalNodes}`}
        />
        <MetricCard
          label="Converged"
          value={`${summary.convergedNodes} / ${summary.totalNodes}`}
        />
        <MetricCard
          label="Maintenance"
          value={String(summary.maintenanceNodes)}
        />
      </section>

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
                const dns = summary.nodes.find(
                  (item) => item.nodeId === node.id,
                );
                return (
                  <tr key={node.id}>
                    <td>
                      <a href={`/ha/nodes/${node.id}`}>
                        <strong>{node.name}</strong>
                      </a>
                    </td>
                    <td>
                      <StatusBadge status={node.healthStatus} />
                    </td>
                    <td>
                      <a href={`/ha/nodes/${node.id}`}>
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
                      {version?.updateAvailable ? (
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
      </section>

      <section className="section-block">
        <div className="section-heading">
          <h2>Certificate expiry</h2>
          <span>{summary.certificateWarnings} warnings</span>
        </div>
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
                  <td>{certificate.subject || "Not reported"}</td>
                  <td>{formatTime(certificate.notAfter)}</td>
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
      </section>

      <section className="section-block">
        <div className="section-heading">
          <h2>Guided upgrades</h2>
          <small>
            Atlas DNS Controller coordinates validation; it never opens a remote
            shell.
          </small>
        </div>
        {upgrades.length === 0 ? (
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

      <SettingsGroup
        title="Notifications"
        description="Meaningful HA transitions and recoveries. Expected DNS failures during maintenance are suppressed."
        bodySpacing="padded"
        actions={
          <button
            type="button"
            className="button button--secondary"
            onClick={() => {
              if (showWebhook && editingChannel === undefined) closeWebhook();
              else openWebhook();
            }}
          >
            {showWebhook && editingChannel === undefined
              ? "Cancel"
              : "Add webhook"}
          </button>
        }
      >
        {webhookFeedback !== undefined && (
          <Banner tone={webhookFeedback.tone} title={webhookFeedback.title}>
            {webhookFeedback.message}
          </Banner>
        )}
        {showWebhook && (
          <form
            className="card form-stack panel-form"
            aria-label={editingChannel ? "Edit webhook" : "Add webhook"}
            onSubmit={(event) => void saveWebhook(event)}
          >
            <h3>
              {editingChannel ? `Edit ${editingChannel.name}` : "Add webhook"}
            </h3>
            <Field label="Channel name" htmlFor="webhook-name" required>
              <input
                id="webhook-name"
                value={webhookName}
                onChange={(event) => setWebhookName(event.target.value)}
                required
                maxLength={120}
              />
            </Field>
            {editingChannel !== undefined && (
              <label>
                <input
                  type="checkbox"
                  checked={replaceDestination}
                  onChange={(event) => {
                    setReplaceDestination(event.target.checked);
                    if (!event.target.checked) setWebhookURL("");
                  }}
                />{" "}
                Replace destination secret
              </label>
            )}
            {(editingChannel === undefined || replaceDestination) && (
              <Field
                label="HTTPS webhook URL"
                htmlFor="webhook-url"
                required
                help="The full URL is encrypted and is never returned by the API."
              >
                <input
                  id="webhook-url"
                  type="url"
                  value={webhookURL}
                  onChange={(event) => setWebhookURL(event.target.value)}
                  required
                  autoComplete="off"
                />
              </Field>
            )}
            <label>
              <input
                type="checkbox"
                checked={webhookEnabled}
                onChange={(event) => setWebhookEnabled(event.target.checked)}
              />{" "}
              Enabled
            </label>
            <div className="row-actions row-actions--start">
              <button
                className="button"
                type="submit"
                disabled={webhookBusy !== ""}
              >
                {webhookBusy === "save"
                  ? "Saving…"
                  : editingChannel
                    ? "Save webhook"
                    : "Add encrypted webhook"}
              </button>
              {editingChannel !== undefined && (
                <button
                  className="button button--secondary"
                  type="button"
                  disabled={webhookBusy !== ""}
                  onClick={closeWebhook}
                >
                  Cancel
                </button>
              )}
            </div>
          </form>
        )}
        {channels.length === 0 && !showWebhook ? (
          <EmptyState title="No notification webhooks">
            <p>Add an HTTPS destination for HA lifecycle transitions.</p>
          </EmptyState>
        ) : channels.length > 0 ? (
          <div className="table-wrap">
            <table>
              <thead>
                <tr>
                  <th>Webhook</th>
                  <th>Destination</th>
                  <th>State</th>
                  <th>Events</th>
                  <th>Updated</th>
                  <th>
                    <span className="visually-hidden">Actions</span>
                  </th>
                </tr>
              </thead>
              <tbody>
                {channels.map((channel) => (
                  <tr key={channel.id}>
                    <td>
                      <strong>{channel.name}</strong>
                      <span className="table-subtitle">
                        Created {formatTime(channel.createdAt)}
                      </span>
                    </td>
                    <td>
                      {channel.destinationSummary || "Encrypted destination"}
                    </td>
                    <td>
                      <StatusBadge
                        status={channel.enabled ? "success" : "disabled"}
                        label={channel.enabled ? "Enabled" : "Disabled"}
                      />
                    </td>
                    <td>All HA transitions</td>
                    <td>{formatTime(channel.updatedAt)}</td>
                    <td>
                      <div className="row-actions">
                        <button
                          className="button button--quiet"
                          type="button"
                          disabled={webhookBusy !== ""}
                          onClick={() => editWebhook(channel)}
                        >
                          Edit
                        </button>
                        <button
                          className="button button--quiet"
                          type="button"
                          disabled={webhookBusy !== ""}
                          onClick={() => void toggleWebhook(channel)}
                        >
                          {channel.enabled ? "Disable" : "Enable"}
                        </button>
                        <button
                          className="button button--quiet"
                          type="button"
                          disabled={webhookBusy !== ""}
                          onClick={() => void testWebhook(channel)}
                        >
                          {webhookBusy === `test-${channel.id}`
                            ? "Testing…"
                            : "Test"}
                        </button>
                        <button
                          className="button button--danger"
                          type="button"
                          disabled={webhookBusy !== ""}
                          onClick={() => void deleteWebhook(channel)}
                        >
                          Delete
                        </button>
                      </div>
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        ) : null}
      </SettingsGroup>

      <section className="section-block">
        <div className="section-heading">
          <h2>Operational history</h2>
          <small>State transitions, not every successful probe.</small>
        </div>
        {events.length === 0 ? (
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

  async function saveWebhook(event: React.FormEvent) {
    event.preventDefault();
    setWebhookBusy("save");
    try {
      if (editingChannel === undefined) {
        await api.createNotificationChannel(cluster.id, {
          name: webhookName,
          destination: webhookURL,
          enabled: webhookEnabled,
        });
      } else {
        await api.updateNotificationChannel(editingChannel.id, {
          name: webhookName,
          enabled: webhookEnabled,
          recordVersion: editingChannel.recordVersion,
          ...(replaceDestination
            ? { destination: webhookURL, replaceDestination: true }
            : {}),
        });
      }
      setWebhookFeedback({
        tone: "success",
        title: "Webhook saved",
        message: "The encrypted notification channel is ready.",
      });
      closeWebhook();
      await load();
    } catch (caught) {
      setWebhookFailure(caught);
    } finally {
      setWebhookBusy("");
    }
  }

  function openWebhook() {
    setEditingChannel(undefined);
    setWebhookName("");
    setWebhookURL("");
    setWebhookEnabled(true);
    setReplaceDestination(false);
    setShowWebhook(true);
  }

  function editWebhook(channel: NotificationChannel) {
    setEditingChannel(channel);
    setWebhookName(channel.name);
    setWebhookURL("");
    setWebhookEnabled(channel.enabled);
    setReplaceDestination(false);
    setShowWebhook(true);
  }

  function closeWebhook() {
    setShowWebhook(false);
    setEditingChannel(undefined);
    setWebhookName("");
    setWebhookURL("");
    setReplaceDestination(false);
  }

  async function toggleWebhook(channel: NotificationChannel) {
    setWebhookBusy(`toggle-${channel.id}`);
    try {
      await api.updateNotificationChannel(channel.id, {
        name: channel.name,
        enabled: !channel.enabled,
        recordVersion: channel.recordVersion,
      });
      setWebhookFeedback({
        tone: "success",
        title: channel.enabled ? "Webhook disabled" : "Webhook enabled",
        message: channel.enabled
          ? "New HA notifications will not be queued for this channel. Its configuration and history are retained."
          : "New HA notifications will be queued for this channel.",
      });
      await load();
    } catch (caught) {
      setWebhookFailure(caught);
    } finally {
      setWebhookBusy("");
    }
  }

  async function testWebhook(channel: NotificationChannel) {
    setWebhookBusy(`test-${channel.id}`);
    try {
      const result = await api.testNotificationChannel(channel.id);
      setWebhookFeedback(
        result.success
          ? {
              tone: "success",
              title: "Webhook test succeeded",
              message: `The endpoint accepted the bounded test at ${formatTime(result.testedAt)}.`,
            }
          : {
              tone: "warning",
              title: "Webhook test failed",
              message: `The endpoint did not accept the test (${result.errorCode ?? "NOTIFICATION_TEST_FAILED"}). No destination details were exposed.`,
            },
      );
      if (historyCursorStack.length > 1) setHistoryCursorStack([""]);
      else await load();
    } catch (caught) {
      setWebhookFailure(caught);
    } finally {
      setWebhookBusy("");
    }
  }

  async function deleteWebhook(channel: NotificationChannel) {
    const confirmation = window.prompt(
      `Type ${channel.name} to delete this webhook. Historical HA events and delivery evidence will be retained.`,
    );
    if (confirmation === null) return;
    setWebhookBusy(`delete-${channel.id}`);
    try {
      await api.deleteNotificationChannel(
        channel.id,
        channel.recordVersion,
        confirmation,
      );
      setWebhookFeedback({
        tone: "success",
        title: "Webhook deleted",
        message:
          "The encrypted destination was destroyed. Historical operational evidence remains available.",
      });
      if (editingChannel?.id === channel.id) closeWebhook();
      await load();
    } catch (caught) {
      setWebhookFailure(caught);
    } finally {
      setWebhookBusy("");
    }
  }

  function setWebhookFailure(caught: unknown) {
    setWebhookFeedback({
      tone: "warning",
      title: "Webhook action failed",
      message:
        caught instanceof Error
          ? caught.message
          : "The webhook action could not be completed.",
    });
  }
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
