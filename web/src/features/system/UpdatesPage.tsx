import { useCallback, useEffect, useState } from "react";
import { Banner, ErrorState, Loading } from "../../components/Feedback";
import { PageContainer, PageHeader } from "../../components/Page";
import { SettingRow, SettingsGroup } from "../../components/Settings";
import { api } from "../../lib/api";
import type { ControllerUpdateStatus } from "../../lib/types";

export function UpdatesPage() {
  const [status, setStatus] = useState<ControllerUpdateStatus>();
  const [error, setError] = useState<unknown>();
  const [checking, setChecking] = useState(false);
  const [copied, setCopied] = useState(false);
  const load = useCallback(async (force = false) => {
    setChecking(true);
    try {
      setStatus(
        force
          ? await api.checkControllerUpdate()
          : await api.controllerUpdate(),
      );
      setError(undefined);
    } catch (caught) {
      setError(caught);
    } finally {
      setChecking(false);
    }
  }, []);
  useEffect(() => {
    void load();
  }, [load]);
  return (
    <PageContainer size="wide">
      <PageHeader
        eyebrow="System"
        title="Updates"
        description="Stable release awareness and safe host-guided controller updates."
        primaryAction={
          <button
            className="button"
            type="button"
            disabled={checking}
            onClick={() => void load(true)}
          >
            {checking ? "Checking…" : "Check now"}
          </button>
        }
      />
      {error !== undefined && (
        <ErrorState error={error} retry={() => void load()} />
      )}
      {!status && !error && <Loading label="Loading update status…" />}
      {status?.errorCode && (
        <Banner tone="warning" title="Latest check unavailable">
          The last successful release information is retained. Error code:{" "}
          {status.errorCode}
        </Banner>
      )}
      {status && (
        <>
          <SettingsGroup title="Release status">
            <SettingRow
              title="Installed version"
              control={<code>{status.installedVersion}</code>}
            />
            <SettingRow
              title="Latest stable"
              control={<code>{status.latestVersion ?? "Unknown"}</code>}
            />
            <SettingRow
              title="Status"
              control={<span>{status.state.replaceAll("_", " ")}</span>}
            />
            <SettingRow
              title="Last checked"
              control={
                <span>
                  {status.lastChecked
                    ? new Date(status.lastChecked).toLocaleString()
                    : "Never"}
                </span>
              }
            />
          </SettingsGroup>
          <SettingsGroup
            title="Update method"
            description="Atlas DNS Controller does not execute host commands or access the Docker socket."
          >
            <SettingRow
              title="Installation type"
              control={
                <span className="settings-group-copy">
                  {status.installationType}
                </span>
              }
            />
            <div className="settings-group-content update-instructions">
              <p className="settings-group-copy">{status.updateMethod}</p>
              {status.updateCommand && (
                <>
                  <pre>
                    <code>{status.updateCommand}</code>
                  </pre>
                  <button
                    className="button button--secondary"
                    type="button"
                    onClick={async () => {
                      try {
                        await navigator.clipboard.writeText(
                          status.updateCommand ?? "",
                        );
                        setCopied(true);
                      } catch (caught) {
                        setError(caught);
                      }
                    }}
                  >
                    {copied ? "Copied" : "Copy update instructions"}
                  </button>
                </>
              )}
            </div>
            <SettingRow
              title="Migration and compatibility"
              description="Only the target release notes and tested compatibility matrix establish a supported path; development metadata is never treated as a release guarantee."
              control="Review required"
            />
            <Banner tone="warning" title="Create a backup first">
              Review release notes and compatibility information before
              migrations, then verify readiness, login, nodes, collectors, and
              convergence after restart.
            </Banner>
          </SettingsGroup>
          {status.releaseNotes && (
            <SettingsGroup title="Release notes">
              <pre className="release-notes">{status.releaseNotes}</pre>
              {status.releaseUrl && (
                <a href={status.releaseUrl} target="_blank" rel="noreferrer">
                  Open release on GitHub
                </a>
              )}
            </SettingsGroup>
          )}
        </>
      )}
    </PageContainer>
  );
}
