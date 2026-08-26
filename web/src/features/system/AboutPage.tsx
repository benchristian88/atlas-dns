import { type ReactNode, useCallback, useEffect, useState } from "react";
import { ErrorState, Loading } from "../../components/Feedback";
import { PageHeader } from "../../components/Page";
import { SettingsGroup } from "../../components/Settings";
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
    <div className="about-page">
      <PageHeader
        eyebrow="Administration"
        title="About Atlas DNS Controller"
        description="Build, compatibility, attribution, and project information."
      />
      {error !== undefined && (
        <ErrorState error={error} retry={() => void load()} />
      )}
      {!info && !error && <Loading label="Loading build information…" />}
      <SettingsGroup title="About Atlas Project" bodySpacing="padded">
        <div className="about-project">
          <p>
            Atlas DNS Controller is an independent project. It is not AdGuard
            Home and is not an official AdGuard product.
          </p>
          <p>
            Atlas provides an authoritative management plane for resilient
            AdGuard Home nodes while remaining outside the live DNS request
            path. Configuration revisions, deployments, drift, and operational
            evidence remain attributable to their source cluster and node.
          </p>
          <nav className="about-links" aria-label="Atlas project links">
            <a
              href="https://github.com/benchristian88/atlas-dns"
              target="_blank"
              rel="noreferrer"
            >
              Repository
            </a>
            <a
              href="https://github.com/benchristian88/atlas-dns/tree/HEAD/docs"
              target="_blank"
              rel="noreferrer"
            >
              Documentation
            </a>
          </nav>
        </div>
      </SettingsGroup>
      {info && (
        <SettingsGroup
          title="Installation / Build Information"
          description="Technical details useful for compatibility checks and support."
          bodySpacing="padded"
        >
          <dl className="about-metadata">
            <Metadata label="Version" value={<code>{info.version}</code>} />
            <Metadata label="Commit" value={<code>{info.commit}</code>} />
            <Metadata label="Build date" value={info.builtAt} />
            <Metadata
              label="Database schema"
              value={<code>{info.databaseSchemaVersion}</code>}
            />
            <Metadata
              label="Supported AdGuard Home"
              value="v0.107.78+ patches in the v0.107 API generation; v0.107.78 and v0.107.79 explicitly tested"
            />
            <Metadata
              label="Reference platforms"
              value="PostgreSQL 17 · Debian 13/systemd · Docker Compose v2"
            />
          </dl>
        </SettingsGroup>
      )}
      <SettingsGroup title="Licensing / Attribution" bodySpacing="padded">
        <div className="about-project">
          <p>
            Atlas DNS Controller is licensed under the Business Source License
            1.1 (BUSL-1.1). Non-commercial personal and homelab use is
            permitted; commercial hosting or resale is prohibited.
          </p>
          <p>
            The Change License is Apache License 2.0, effective no later than 12
            August 2032 for this release. AdGuard Home is a separate project;
            Atlas does not copy its source code or proprietary assets.
          </p>
        </div>
      </SettingsGroup>
    </div>
  );
}

function Metadata({ label, value }: { label: string; value: ReactNode }) {
  return (
    <div>
      <dt>{label}</dt>
      <dd>{value}</dd>
    </div>
  );
}
