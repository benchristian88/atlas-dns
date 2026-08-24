import { useCallback, useEffect, useState } from "react";
import { ErrorState, Loading } from "../../components/Feedback";
import { PageContainer, PageHeader } from "../../components/Page";
import { SettingRow, SettingsGroup } from "../../components/Settings";
import { api } from "../../lib/api";
import type { SystemSettings } from "../../lib/types";

export function SystemSettingsPage() {
  const [settings, setSettings] = useState<SystemSettings>();
  const [error, setError] = useState<unknown>();
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
      setSettings(await api.updateSystemSettings(settings));
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
          description="Runtime installation values remain in the protected environment file."
          control="Environment"
        />
      </SettingsGroup>
      <SettingsGroup title="Data">
        <SettingRow
          title="Statistics"
          description="Snapshots/hourly data retain 32 days; daily rollups retain 400 days."
          control={settings?.statisticsRetention ?? "Managed retention"}
        />
        <SettingRow
          title="Query Log"
          description="Central retention is persisted below and remains distinct from node policy; an upgrade initializes it once from QUERY_LOG_RETENTION."
          control={settings?.queryLogRetention ?? "1 hour–90 days"}
        />
      </SettingsGroup>
      <SettingsGroup title="Monitoring runtime" bodySpacing="padded">
        <p className="muted">
          Changes are persisted, audited, and adopted by collector scheduling
          without restarting Atlas.
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
            label="Statistics poll interval"
            min={60}
            max={86400}
            value={settings?.statisticsPollIntervalSeconds}
            onChange={(value) =>
              settings &&
              setSettings({ ...settings, statisticsPollIntervalSeconds: value })
            }
          />
          <SecondsSetting
            label="Query Log poll interval"
            min={5}
            max={3600}
            value={settings?.queryLogPollIntervalSeconds}
            onChange={(value) =>
              settings &&
              setSettings({ ...settings, queryLogPollIntervalSeconds: value })
            }
          />
          <SecondsSetting
            label="Query Log retention"
            min={3600}
            max={7776000}
            value={settings?.queryLogRetentionSeconds}
            onChange={(value) =>
              settings &&
              setSettings({ ...settings, queryLogRetentionSeconds: value })
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
        <div className="row-actions row-actions--start">
          <button
            className="button"
            type="button"
            disabled={!settings}
            onClick={() => void saveMonitoring()}
          >
            Save monitoring settings
          </button>
          <button
            className="button button--secondary"
            type="button"
            disabled={!settings}
            onClick={() =>
              settings &&
              setSettings({
                ...settings,
                nodeHealthIntervalSeconds: 30,
                statisticsPollIntervalSeconds: 3600,
                queryLogCollectionEnabled: true,
                queryLogPollIntervalSeconds: 30,
                queryLogRetentionSeconds: 604800,
              })
            }
          >
            Use recommended defaults
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
