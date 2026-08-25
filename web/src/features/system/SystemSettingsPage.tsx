import { useCallback, useEffect, useState } from "react";
import { ErrorState, Loading } from "../../components/Feedback";
import { PageContainer, PageHeader } from "../../components/Page";
import { SettingRow, SettingsGroup } from "../../components/Settings";
import { api } from "../../lib/api";
import type { Cluster, SystemSettings } from "../../lib/types";

export function SystemSettingsPage({ cluster }: { cluster?: Cluster }) {
  const [settings, setSettings] = useState<SystemSettings>();
  const [error, setError] = useState<unknown>();
  const [clearConfirmation, setClearConfirmation] = useState("");
  const [clearFeedback, setClearFeedback] = useState("");
  const load = useCallback(async () => {
    try {
      setSettings(await api.systemSettings());
      setError(undefined);
    } catch (caught) {
      setError(caught);
    }
  }, []);
  useEffect(() => {
    void load();
  }, [load]);
  async function toggleUpdates() {
    if (!settings) return;
    try {
      setSettings(
        await api.updateSystemSettings({
          ...settings,
          updateChecksEnabled: !settings.updateChecksEnabled,
        }),
      );
      setError(undefined);
    } catch (caught) {
      setError(caught);
    }
  }
  async function saveMonitoring() {
    if (!settings) return;
    try {
      const saved = await api.updateSystemSettings(settings);
      setSettings(saved);
      if (cluster) {
        const onboarding = await api.onboardingStatus(cluster.id);
        if (onboarding.state.monitoringReviewedAt === undefined) {
          await api.updateOnboardingProgress(
            cluster.id,
            onboarding.state.recordVersion,
            { monitoringReviewed: true },
          );
        }
      }
      setError(undefined);
    } catch (caught) {
      setError(caught);
    }
  }
  async function clearHistory() {
    try {
      const result = await api.clearOperationalHistory(clearConfirmation);
      setClearFeedback(
        `Cleared ${result.eventsDeleted} events and ${result.deliveriesDeleted} delivery records. Audit Log entries were retained.`,
      );
      setClearConfirmation("");
      setError(undefined);
    } catch (caught) {
      setError(caught);
    }
  }
  return (
    <PageContainer size="wide">
      <PageHeader
        eyebrow="System"
        title="System Settings"
        description="Controller-wide data, recovery, update, operations, and security boundaries."
      />
      {error !== undefined && (
        <ErrorState error={error} retry={() => void load()} />
      )}
      {!settings && error === undefined && (
        <Loading label="Loading system settings…" />
      )}
      <SettingsGroup title="General">
        <SettingRow title="Product name" control="Atlas DNS Controller" />
        <SettingRow
          title="Configuration source"
          description="Operational runtime policy is stored in PostgreSQL. Bootstrap, networking, and secrets remain external."
          control="Database + bootstrap environment"
        />
      </SettingsGroup>
      <SettingsGroup
        title="Runtime configuration"
        description="These controller-wide settings are saved together, audited, and applied without a restart where supported."
        className="runtime-settings-card"
        bodySpacing="padded"
        actions={
          <button
            className="button"
            type="button"
            disabled={!settings}
            onClick={() => void saveMonitoring()}
          >
            Save runtime settings
          </button>
        }
      >
        <div className="runtime-settings-card__groups">
          <SettingsGroup
            title="Session & Security"
            headingLevel={3}
            bodySpacing="padded"
          >
            <div className="form-grid">
              <SecondsSetting
                label="Session duration"
                min={900}
                max={2592000}
                value={settings?.sessionDurationSeconds}
                onChange={(value) =>
                  settings &&
                  setSettings({ ...settings, sessionDurationSeconds: value })
                }
              />
            </div>
          </SettingsGroup>
          <SettingsGroup
            title="Node Monitoring"
            headingLevel={3}
            bodySpacing="padded"
          >
            <p className="muted runtime-setting-note">
              Collector scheduling adopts changes without restarting Atlas.
            </p>
            <div className="form-grid">
              <SecondsSetting
                label="Node health interval"
                min={5}
                max={3600}
                value={settings?.nodeHealthIntervalSeconds}
                onChange={(value) =>
                  settings &&
                  setSettings({ ...settings, nodeHealthIntervalSeconds: value })
                }
              />
              <SecondsSetting
                label="Node request timeout"
                min={1}
                max={120}
                value={settings?.nodeRequestTimeoutSeconds}
                onChange={(value) =>
                  settings &&
                  setSettings({ ...settings, nodeRequestTimeoutSeconds: value })
                }
              />
            </div>
          </SettingsGroup>
          <SettingsGroup title="Statistics Collection" headingLevel={3}>
            <SettingRow
              title="Stored statistics"
              description="Snapshot and hourly data retain 32 days; daily rollups retain 400 days."
              control={
                <span className="runtime-setting-summary">
                  {settings?.statisticsRetention ?? "Managed retention"}
                </span>
              }
            />
            <div className="runtime-setting-fields">
              <div className="form-grid">
                <SecondsSetting
                  label="Statistics poll interval"
                  min={60}
                  max={86400}
                  value={settings?.statisticsPollIntervalSeconds}
                  onChange={(value) =>
                    settings &&
                    setSettings({
                      ...settings,
                      statisticsPollIntervalSeconds: value,
                    })
                  }
                />
              </div>
            </div>
          </SettingsGroup>
          <SettingsGroup title="Query Log Collection" headingLevel={3}>
            <SettingRow
              title="Current central retention"
              description="Stored centrally and independently from each node's Query Log policy."
              control={
                <span className="runtime-setting-summary">
                  {settings?.queryLogRetention ?? "1 hour–90 days"}
                </span>
              }
            />
            <div className="runtime-setting-fields">
              <div className="form-grid">
                <SecondsSetting
                  label="Query Log poll interval"
                  min={5}
                  max={3600}
                  value={settings?.queryLogPollIntervalSeconds}
                  onChange={(value) =>
                    settings &&
                    setSettings({
                      ...settings,
                      queryLogPollIntervalSeconds: value,
                    })
                  }
                />
                <SecondsSetting
                  label="Query Log retention"
                  min={3600}
                  max={7776000}
                  value={settings?.queryLogRetentionSeconds}
                  onChange={(value) =>
                    settings &&
                    setSettings({
                      ...settings,
                      queryLogRetentionSeconds: value,
                    })
                  }
                />
              </div>
              <label className="checkbox">
                <input
                  type="checkbox"
                  checked={settings?.queryLogCollectionEnabled ?? false}
                  disabled={!settings}
                  onChange={(event) =>
                    settings &&
                    setSettings({
                      ...settings,
                      queryLogCollectionEnabled: event.target.checked,
                    })
                  }
                />{" "}
                Central Query Log collection enabled
              </label>
            </div>
          </SettingsGroup>
          <SettingsGroup title="Logging" headingLevel={3} bodySpacing="padded">
            <label>
              Controller log level
              <select
                value={settings?.logLevel ?? "info"}
                disabled={!settings}
                onChange={(event) =>
                  settings &&
                  setSettings({
                    ...settings,
                    logLevel: event.target.value as SystemSettings["logLevel"],
                  })
                }
              >
                <option value="debug">Debug</option>
                <option value="info">Info</option>
                <option value="warn">Warning</option>
                <option value="error">Error</option>
              </select>
            </label>
          </SettingsGroup>
          <SettingsGroup
            title="Operational History retention"
            headingLevel={3}
            bodySpacing="padded"
          >
            <p className="muted runtime-setting-note">
              Applies to HA lifecycle events and notification delivery evidence.
              Audit Log and configuration records are separate.
            </p>
            <label>
              Retention
              <select
                value={settings?.operationalHistoryRetentionDays ?? 90}
                disabled={!settings}
                onChange={(event) =>
                  settings &&
                  setSettings({
                    ...settings,
                    operationalHistoryRetentionDays: Number(
                      event.target.value,
                    ) as SystemSettings["operationalHistoryRetentionDays"],
                  })
                }
              >
                {[7, 14, 30, 90, 180, 365].map((days) => (
                  <option key={days} value={days}>
                    {days} days
                  </option>
                ))}
              </select>
            </label>
            <div className="runtime-settings-danger-zone">
              <div>
                <h4>Clear Operational History</h4>
                <p className="muted runtime-setting-note">
                  This acts immediately and is not part of Save runtime
                  settings. It removes retained operational events and delivery
                  evidence only; Security Audit Log entries are retained.
                </p>
              </div>
              <label>
                Type CLEAR OPERATIONAL HISTORY to remove retained operational
                events
                <input
                  value={clearConfirmation}
                  onChange={(event) => setClearConfirmation(event.target.value)}
                />
              </label>
              <button
                className="button button--danger"
                type="button"
                disabled={clearConfirmation !== "CLEAR OPERATIONAL HISTORY"}
                onClick={() => void clearHistory()}
              >
                Clear Operational History
              </button>
              {clearFeedback && (
                <p className="muted" role="status">
                  {clearFeedback}
                </p>
              )}
            </div>
          </SettingsGroup>
        </div>
        <div className="runtime-settings-card__footer">
          <button
            className="button button--secondary"
            type="button"
            disabled={!settings}
            onClick={() =>
              settings &&
              setSettings({
                ...settings,
                sessionDurationSeconds: 43200,
                nodeHealthIntervalSeconds: 30,
                nodeRequestTimeoutSeconds: 10,
                statisticsPollIntervalSeconds: 3600,
                queryLogCollectionEnabled: true,
                queryLogPollIntervalSeconds: 30,
                queryLogRetentionSeconds: 604800,
                logLevel: "info",
                operationalHistoryRetentionDays: 90,
              })
            }
          >
            Use recommended defaults
          </button>
          <button
            className="button"
            type="button"
            disabled={!settings}
            onClick={() => void saveMonitoring()}
          >
            Save runtime settings
          </button>
        </div>
      </SettingsGroup>
      <SettingsGroup title="Backup & Restore">
        <p className="settings-group-content settings-group-action">
          <a className="button button--secondary" href="/system/backups">
            Open Backup & Restore
          </a>
        </p>
      </SettingsGroup>
      <SettingsGroup title="Updates">
        <SettingRow
          title="Stable release checks"
          description="Cached GitHub release awareness; no update is installed automatically."
          control={
            <label className="checkbox">
              <input
                type="checkbox"
                checked={settings?.updateChecksEnabled ?? false}
                disabled={!settings}
                onChange={() => void toggleUpdates()}
              />{" "}
              Enabled
            </label>
          }
        />
        <p className="settings-group-content settings-group-action">
          <a className="button button--secondary" href="/system/updates">
            Open Updates
          </a>
        </p>
      </SettingsGroup>
      <SettingsGroup title="Operations">
        <p className="settings-group-content settings-group-copy">
          Operational thresholds and worker evidence are available from{" "}
          <a href="/system/operational-status">Operational Status</a>. Node
          lifecycle settings remain node-specific.
        </p>
      </SettingsGroup>
      <SettingsGroup title="Security">
        <p className="settings-group-content settings-group-copy">
          Secure HTTP-only sessions, CSRF protection, Argon2id passwords,
          AES-256-GCM credential envelopes, passphrase-encrypted backups, and
          server-side administrator enforcement are active architecture
          boundaries.
        </p>
      </SettingsGroup>
    </PageContainer>
  );
}

function SecondsSetting({
  label,
  value,
  min,
  max,
  onChange,
}: {
  label: string;
  value?: number;
  min: number;
  max: number;
  onChange: (value: number) => void;
}) {
  return (
    <label>
      {label} (seconds)
      <input
        type="number"
        value={value ?? ""}
        min={min}
        max={max}
        disabled={value === undefined}
        onChange={(event) => onChange(Number(event.target.value))}
      />
    </label>
  );
}
