import { useCallback, useEffect, useState } from "react";
import { ErrorState, Loading } from "../../components/Feedback";
import { PageContainer, PageHeader } from "../../components/Page";
import { SettingRow, SettingsGroup } from "../../components/Settings";
import { api } from "../../lib/api";
import type { VersionInfo } from "../../lib/types";

export function AboutPage() {
  const [info, setInfo] = useState<VersionInfo>();
  const [error, setError] = useState<unknown>();
  const load = useCallback(async () => {
    try {
      setInfo(await api.versionInfo());
      setError(undefined);
    } catch (caught) {
      setError(caught);
    }
  }, []);
  useEffect(() => {
    void load();
  }, [load]);
  return (
    <PageContainer size="standard">
      <PageHeader
        eyebrow="System"
        title="About Atlas DNS Controller"
        description="Build, compatibility, attribution, and project information."
      />
      {error !== undefined && (
        <ErrorState error={error} retry={() => void load()} />
      )}
      {!info && !error && <Loading label="Loading build information…" />}
      {info && (
        <SettingsGroup title="Build">
          <SettingRow title="Product" control="Atlas DNS Controller" />
          <SettingRow title="Version" control={<code>{info.version}</code>} />
          <SettingRow
            title="Build / commit"
            control={<code>{info.commit}</code>}
          />
          <SettingRow
            title="Build date"
            control={<span>{info.builtAt}</span>}
          />
          <SettingRow
            title="Database schema"
            control={<code>{info.databaseSchemaVersion}</code>}
          />
        </SettingsGroup>
      )}
      <SettingsGroup title="Project">
        <div className="settings-group-content about-project">
          <p>
            Atlas DNS Controller is an independent project. It is not AdGuard
            Home and is not an official AdGuard product.
          </p>
          <p>
            Supported managed configuration: AdGuard Home v0.107.52 on schema v1
            and v0.107.53+ patches in the v0.107 API generation on schema v2.
            v0.107.78 and v0.107.79 are explicitly tested; newer v0.107 patches
            are provisionally compatible after API validation. PostgreSQL 17 and
            Debian 13/systemd or Docker Compose v2 are the current reference
            platforms.
          </p>
          <p>
            <a
              href="https://github.com/benchristian88/atlas-dns"
              target="_blank"
              rel="noreferrer"
            >
              Repository
            </a>{" "}
            ·{" "}
            <a
              href="https://github.com/benchristian88/atlas-dns/tree/HEAD/docs"
              target="_blank"
              rel="noreferrer"
            >
              Documentation
            </a>
          </p>
          <p>
            <strong>Licence:</strong> Business Source License 1.1 (BUSL-1.1).
            Non-commercial personal and homelab use is permitted; commercial
            hosting or resale is prohibited. The Change License is Apache
            License 2.0, effective no later than 12 August 2032 for this
            release.
          </p>
        </div>
      </SettingsGroup>
    </PageContainer>
  );
}
