import { type FormEvent, useState } from "react";
import { Banner, ErrorState } from "../../components/Feedback";
import { PageContainer, PageHeader } from "../../components/Page";
import { Field, SettingsGroup } from "../../components/Settings";
import { api } from "../../lib/api";
import type { RestorePreflight } from "../../lib/types";

export function BackupPage() {
  const [type, setType] = useState<"standard" | "full">("standard");
  const [passphrase, setPassphrase] = useState("");
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<unknown>();
  const [preflight, setPreflight] = useState<RestorePreflight>();
  const [created, setCreated] = useState<{
    filename: string;
    type: string;
    applicationVersion: string;
    databaseSchemaVersion: string;
    createdAt: string;
  }>();
  async function create(event: FormEvent) {
    event.preventDefault();
    setBusy(true);
    setError(undefined);
    try {
      const result = await api.createBackup(type, passphrase);
      const href = URL.createObjectURL(result.blob);
      const link = document.createElement("a");
      link.href = href;
      link.download = result.filename;
      link.click();
      URL.revokeObjectURL(href);
      setCreated(result);
      setPassphrase("");
    } catch (caught) {
      setError(caught);
    } finally {
      setBusy(false);
    }
  }
  async function validate(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    setBusy(true);
    setError(undefined);
    setPreflight(undefined);
    const data = new FormData(event.currentTarget);
    const archive = data.get("archive");
    try {
      if (!(archive instanceof File))
        throw new Error("Choose a backup archive.");
      setPreflight(
        await api.restorePreflight(
          archive,
          String(data.get("passphrase") ?? ""),
        ),
      );
    } catch (caught) {
      setError(caught);
    } finally {
      setBusy(false);
    }
  }
  return (
    <PageContainer size="wide">
      <PageHeader
        eyebrow="System · Data"
        title="Backup & Restore"
        description="Create portable, passphrase-encrypted controller recovery archives. Restore execution is deliberately offline and CLI-controlled."
      />
      {error !== undefined && <ErrorState error={error} />}
      <SettingsGroup
        title="Create backup"
        description="The credential key is protected inside the encrypted archive. Keep the passphrase separately; it cannot be recovered."
      >
        <div className="settings-group-content backup-section">
          <p className="backup-section__intro">
            <strong>Standard</strong> includes users, encrypted credentials,
            desired state, revisions, deployments, drift, audit, lifecycle, and
            system settings. <strong>Full</strong> also includes retained
            Statistics, Query Log, DNS probe, HA event, and
            notification-delivery history. Browser sessions and release caches
            are excluded from both.
          </p>
          <form className="form-stack" onSubmit={(event) => void create(event)}>
            <Field label="Backup type" htmlFor="backup-type">
              <select
                id="backup-type"
                className="backup-type-select"
                value={type}
                onChange={(event) =>
                  setType(event.target.value as "standard" | "full")
                }
              >
                <option value="standard">Standard — control plane only</option>
                <option value="full">Full — include operational history</option>
              </select>
            </Field>
            <Field
              label="Archive passphrase"
              htmlFor="backup-passphrase"
              help="At least 16 characters. It is used only for this download."
              required
            >
              <input
                id="backup-passphrase"
                type="password"
                minLength={16}
                maxLength={1024}
                value={passphrase}
                onChange={(event) => setPassphrase(event.target.value)}
                autoComplete="new-password"
                required
              />
            </Field>
            <button className="button" disabled={busy} type="submit">
              {busy
                ? "Creating encrypted backup…"
                : "Create and download backup"}
            </button>
          </form>
          {created && (
            <Banner tone="success" title="Encrypted backup downloaded">
              {created.filename} · {created.type} · application{" "}
              {created.applicationVersion} · schema{" "}
              {created.databaseSchemaVersion} · created{" "}
              {created.createdAt
                ? new Date(created.createdAt).toLocaleString()
                : "just now"}
            </Banner>
          )}
        </div>
      </SettingsGroup>
      <SettingsGroup
        title="Restore preflight"
        description="Validation checks the bounded archive, checksum, passphrase, authenticated manifest, entry integrity, and target compatibility without changing controller state."
      >
        <div className="settings-group-content backup-section">
          <form
            className="form-stack"
            onSubmit={(event) => void validate(event)}
          >
            <Field label="Backup archive" required>
              <input
                name="archive"
                type="file"
                accept=".atlasdnsbackup,application/vnd.atlas-dns.backup"
                required
              />
            </Field>
            <Field label="Passphrase" required>
              <input
                name="passphrase"
                type="password"
                maxLength={1024}
                required
              />
            </Field>
            <button
              className="button button--secondary"
              disabled={busy}
              type="submit"
            >
              Validate and show restore plan
            </button>
          </form>
          {preflight && (
            <Banner tone="success" title="Backup is valid">
              <p>
                {preflight.manifest.type} backup from{" "}
                {preflight.manifest.applicationVersion}, schema{" "}
                {preflight.manifest.databaseSchemaVersion}, created{" "}
                {new Date(preflight.manifest.createdAt).toLocaleString()}.
              </p>
              <p>
                Restore uses <code>atlas-dns-backup restore</code> against a new
                empty database, invalidates browser sessions, and requires a
                controller restart.
              </p>
            </Banner>
          )}
        </div>
      </SettingsGroup>
    </PageContainer>
  );
}
