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
        await api.updateSystemSettings(settings, !settings.updateChecksEnabled),
      );
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
          description="Central retention is configured by QUERY_LOG_RETENTION and remains distinct from node policy."
          control={settings?.queryLogRetention ?? "1 hour–90 days"}
        />
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
