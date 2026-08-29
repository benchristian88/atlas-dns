import { type FormEvent, useCallback, useEffect, useState } from "react";
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
  Cluster,
  NotificationChannel,
  NotificationPolicy,
} from "../../lib/types";

const notificationCategories = [
  ["dns", "DNS service"],
  ["redundancy", "Redundancy"],
  ["certificates", "Certificates"],
  ["versions", "Versions"],
  ["maintenance", "Maintenance"],
  ["upgrades", "Upgrades"],
] as const;

export function NotificationsPage({ cluster }: { cluster: Cluster }) {
  const [channels, setChannels] = useState<NotificationChannel[]>();
  const [policy, setPolicy] = useState<NotificationPolicy>();
  const [refreshError, setRefreshError] = useState<unknown>();
  const [showEditor, setShowEditor] = useState(false);
  const [editing, setEditing] = useState<NotificationChannel>();
  const [name, setName] = useState("");
  const [destination, setDestination] = useState("");
  const [enabled, setEnabled] = useState(true);
  const [replaceDestination, setReplaceDestination] = useState(false);
  const [categories, setCategories] = useState<string[]>(
    notificationCategories.map(([value]) => value),
  );
  const [busy, setBusy] = useState("");
  const [feedback, setFeedback] = useState<{
    tone: "success" | "warning";
    title: string;
    message: string;
  }>();

  const load = useCallback(async () => {
    try {
      const [loadedChannels, loadedPolicy] = await Promise.all([
        api.notificationChannels(cluster.id),
        api.notificationPolicy(),
      ]);
      setChannels(loadedChannels.items);
      setPolicy(loadedPolicy);
      setRefreshError(undefined);
    } catch (caught) {
      setRefreshError(caught);
    }
  }, [cluster.id]);

  useEffect(() => {
    void load();
  }, [load]);

  if (
    (channels === undefined || policy === undefined) &&
    refreshError === undefined
  )
    return <Loading label="Loading notifications…" />;
  if (channels === undefined || policy === undefined)
    return <ErrorState error={refreshError} retry={() => void load()} />;

  return (
    <>
      <PageHeader
        eyebrow="HA Controller"
        title="Notifications"
        description="Configure Atlas notification delivery for meaningful HA transitions and recoveries."
        primaryAction={
          <button
            className="button"
            type="button"
            onClick={() =>
              showEditor && editing === undefined ? closeEditor() : openEditor()
            }
          >
            {showEditor && editing === undefined ? "Cancel" : "Add webhook"}
          </button>
        }
      />
      <Banner tone="info" title="Controller notifications">
        These webhooks report Atlas HA lifecycle events. They do not change
        AdGuard Home configuration, and expected DNS failures during maintenance
        are suppressed.
      </Banner>
      {refreshError !== undefined && (
        <Banner
          tone="warning"
          title="Notifications refresh failed"
          actions={
            <button
              className="button button--secondary"
              type="button"
              onClick={() => void load()}
            >
              Try again
            </button>
          }
        >
          The last successful notification policy and channel list remain
          visible and may be stale.
        </Banner>
      )}
      {feedback && (
        <Banner tone={feedback.tone} title={feedback.title}>
          {feedback.message}
        </Banner>
      )}
      <SettingsGroup
        title="Notification policy"
        description="Choose the exact operational events delivered to every eligible channel. Recommended defaults focus on failures, recovery, and redundancy risk."
        bodySpacing="padded"
      >
        {policy.groups.map((group) => (
          <fieldset className="notification-categories" key={group.id}>
            <legend>{group.label}</legend>
            {group.events.map((item) => (
              <label className="checkbox" key={item.eventType}>
                <input
                  type="checkbox"
                  checked={policy.enabledEventTypes.includes(item.eventType)}
                  onChange={(event) =>
                    setPolicy({
                      ...policy,
                      enabledEventTypes: event.target.checked
                        ? [...policy.enabledEventTypes, item.eventType]
                        : policy.enabledEventTypes.filter(
                            (value) => value !== item.eventType,
                          ),
                    })
                  }
                />{" "}
                {item.label}
              </label>
            ))}
          </fieldset>
        ))}
        <div className="row-actions row-actions--start">
          <button
            className="button"
            type="button"
            disabled={busy !== ""}
            onClick={() => void savePolicy()}
          >
            {busy === "policy" ? "Saving…" : "Save notification policy"}
          </button>
          <button
            className="button button--secondary"
            type="button"
            disabled={busy !== ""}
            onClick={() =>
              setPolicy({
                ...policy,
                enabledEventTypes: policy.groups.flatMap((group) =>
                  group.events
                    .filter((item) => item.defaultEnabled)
                    .map((item) => item.eventType),
                ),
              })
            }
          >
            Use recommended defaults
          </button>
        </div>
      </SettingsGroup>
      <SettingsGroup
        title="Webhook channels"
        description="Destinations are encrypted and write-only. Atlas never returns the full URL."
        bodySpacing="padded"
      >
        {showEditor && (
          <form
            className="card form-stack panel-form"
            aria-label={editing ? "Edit webhook" : "Add webhook"}
            onSubmit={(event) => void save(event)}
          >
            <h3>{editing ? `Edit ${editing.name}` : "Add webhook"}</h3>
            <Field label="Channel name" htmlFor="notification-name" required>
              <input
                id="notification-name"
                value={name}
                onChange={(event) => setName(event.target.value)}
                required
                maxLength={120}
              />
            </Field>
            {editing && (
              <label className="checkbox">
                <input
                  type="checkbox"
                  checked={replaceDestination}
                  onChange={(event) => {
                    setReplaceDestination(event.target.checked);
                    if (!event.target.checked) setDestination("");
                  }}
                />
                Replace destination secret
              </label>
            )}
            {(!editing || replaceDestination) && (
              <Field
                label="HTTPS webhook URL"
                htmlFor="notification-url"
                required
                help="The full URL is encrypted and is never returned by the API."
              >
                <input
                  id="notification-url"
                  type="url"
                  value={destination}
                  onChange={(event) => setDestination(event.target.value)}
                  required
                  autoComplete="off"
                />
              </Field>
            )}
            <label className="checkbox">
              <input
                type="checkbox"
                checked={enabled}
                onChange={(event) => setEnabled(event.target.checked)}
              />{" "}
              Enabled
            </label>
            <fieldset className="notification-categories">
              <legend>Notification events</legend>
              {notificationCategories.map(([value, label]) => (
                <label className="checkbox" key={value}>
                  <input
                    type="checkbox"
                    checked={categories.includes(value)}
                    onChange={(event) =>
                      setCategories((current) =>
                        event.target.checked
                          ? [...current, value]
                          : current.filter((item) => item !== value),
                      )
                    }
                  />{" "}
                  {label}
                </label>
              ))}
            </fieldset>
            <div className="row-actions row-actions--start">
              <button
                className="button"
                type="submit"
                disabled={busy !== "" || categories.length === 0}
              >
                {busy === "save"
                  ? "Saving…"
                  : editing
                    ? "Save webhook"
                    : "Add encrypted webhook"}
              </button>
              {editing && (
                <button
                  className="button button--secondary"
                  type="button"
                  disabled={busy !== ""}
                  onClick={closeEditor}
                >
                  Cancel
                </button>
              )}
            </div>
          </form>
        )}
        {channels.length === 0 && !showEditor ? (
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
                    <td>{channel.subscribedCategories.join(", ")}</td>
                    <td>{formatTime(channel.updatedAt)}</td>
                    <td>
                      <div className="row-actions">
                        <button
                          className="button button--quiet"
                          type="button"
                          disabled={busy !== ""}
                          onClick={() => edit(channel)}
                        >
                          Edit
                        </button>
                        <button
                          className="button button--quiet"
                          type="button"
                          disabled={busy !== ""}
                          onClick={() => void toggle(channel)}
                        >
                          {channel.enabled ? "Disable" : "Enable"}
                        </button>
                        <button
                          className="button button--quiet"
                          type="button"
                          disabled={busy !== ""}
                          onClick={() => void test(channel)}
                        >
                          {busy === `test-${channel.id}` ? "Testing…" : "Test"}
                        </button>
                        <button
                          className="button button--danger"
                          type="button"
                          disabled={busy !== ""}
                          onClick={() => void remove(channel)}
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
      <p className="muted notification-history-link">
        Delivery outcomes and retained HA transitions remain available in{" "}
        <a href="/ha/operations">HA Operations history</a>.
      </p>
    </>
  );

  async function save(event: FormEvent) {
    event.preventDefault();
    setBusy("save");
    try {
      if (!editing)
        await api.createNotificationChannel(cluster.id, {
          name,
          destination,
          enabled,
          subscribedCategories: categories,
        });
      else
        await api.updateNotificationChannel(editing.id, {
          name,
          enabled,
          recordVersion: editing.recordVersion,
          subscribedCategories: categories,
          ...(replaceDestination
            ? { destination, replaceDestination: true }
            : {}),
        });
      setFeedback({
        tone: "success",
        title: "Webhook saved",
        message: "The encrypted notification channel is ready.",
      });
      closeEditor();
      await load();
    } catch (caught) {
      failure(caught);
    } finally {
      setBusy("");
    }
  }

  function openEditor() {
    setEditing(undefined);
    setName("");
    setDestination("");
    setEnabled(true);
    setReplaceDestination(false);
    setCategories(notificationCategories.map(([value]) => value));
    setShowEditor(true);
  }
  function edit(channel: NotificationChannel) {
    setEditing(channel);
    setName(channel.name);
    setDestination("");
    setEnabled(channel.enabled);
    setReplaceDestination(false);
    setCategories(channel.subscribedCategories);
    setShowEditor(true);
  }
  function closeEditor() {
    setShowEditor(false);
    setEditing(undefined);
    setName("");
    setDestination("");
    setReplaceDestination(false);
  }

  async function toggle(channel: NotificationChannel) {
    setBusy(`toggle-${channel.id}`);
    try {
      await api.updateNotificationChannel(channel.id, {
        name: channel.name,
        enabled: !channel.enabled,
        recordVersion: channel.recordVersion,
        subscribedCategories: channel.subscribedCategories,
      });
      setFeedback({
        tone: "success",
        title: channel.enabled ? "Webhook disabled" : "Webhook enabled",
        message: channel.enabled
          ? "New HA notifications will not be queued for this channel. Existing configuration and history are retained."
          : "New HA notifications will be queued for this channel.",
      });
      await load();
    } catch (caught) {
      failure(caught);
    } finally {
      setBusy("");
    }
  }

  async function savePolicy() {
    if (!policy) return;
    setBusy("policy");
    try {
      setPolicy(
        await api.updateNotificationPolicy(
          policy.enabledEventTypes,
          policy.recordVersion,
        ),
      );
      setFeedback({
        tone: "success",
        title: "Notification policy saved",
        message: "New operational events will use the updated policy.",
      });
      setRefreshError(undefined);
    } catch (caught) {
      setFeedback({
        tone: "warning",
        title: "Notification policy save failed",
        message:
          caught instanceof Error
            ? caught.message
            : "The notification policy could not be saved.",
      });
    } finally {
      setBusy("");
    }
  }

  async function test(channel: NotificationChannel) {
    setBusy(`test-${channel.id}`);
    try {
      const result = await api.testNotificationChannel(channel.id);
      setFeedback(
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
    } catch (caught) {
      failure(caught);
    } finally {
      setBusy("");
    }
  }

  async function remove(channel: NotificationChannel) {
    const confirmation = window.prompt(
      `Type ${channel.name} to delete this webhook. Historical HA events and delivery evidence will be retained.`,
    );
    if (confirmation === null) return;
    setBusy(`delete-${channel.id}`);
    try {
      await api.deleteNotificationChannel(
        channel.id,
        channel.recordVersion,
        confirmation,
      );
      setFeedback({
        tone: "success",
        title: "Webhook deleted",
        message:
          "The encrypted destination was destroyed. Historical operational evidence remains available.",
      });
      if (editing?.id === channel.id) closeEditor();
      await load();
    } catch (caught) {
      failure(caught);
    } finally {
      setBusy("");
    }
  }

  function failure(caught: unknown) {
    setFeedback({
      tone: "warning",
      title: "Webhook action failed",
      message:
        caught instanceof Error
          ? caught.message
          : "The webhook action could not be completed.",
    });
  }
}

function formatTime(value?: string) {
  return value ? new Date(value).toLocaleString() : "—";
}
