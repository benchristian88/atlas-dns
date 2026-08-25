import { useCallback, useEffect, useState } from "react";
import { MetricCard } from "../../components/DataDisplay";
import { Banner, ErrorState, Loading } from "../../components/Feedback";
import { PageContainer, PageHeader } from "../../components/Page";
import { Field, SettingsGroup } from "../../components/Settings";
import { StatusBadge } from "../../components/StatusBadge";
import { api } from "../../lib/api";
import type {
  Cluster,
  LifecycleSettings,
  MaintenancePreflight,
  Node,
  NodeLifecycle,
  UpgradeOperation,
} from "../../lib/types";

export function NodeLifecyclePage({
  cluster,
  nodeId,
}: {
  cluster: Cluster;
  nodeId: string;
}) {
  const [node, setNode] = useState<Node>();
  const [lifecycle, setLifecycle] = useState<NodeLifecycle>();
  const [preflight, setPreflight] = useState<MaintenancePreflight>();
  const [settings, setSettings] = useState<LifecycleSettings>();
  const [error, setError] = useState<unknown>();
  const [busy, setBusy] = useState("");
  const [targetVersion, setTargetVersion] = useState("");
  const [upgrades, setUpgrades] = useState<UpgradeOperation[]>([]);
  const load = useCallback(async () => {
    try {
      const [nodes, value, check, upgradeResult] = await Promise.all([
        api.nodes(cluster.id),
        api.nodeLifecycle(nodeId),
        api.maintenancePreflight(nodeId),
        api.upgrades(cluster.id),
      ]);
      const selectedNode = nodes.items.find((item) => item.id === nodeId);
      if (selectedNode === undefined) {
        throw new Error(
          "This managed node no longer exists in the selected cluster.",
        );
      }
      setNode(selectedNode);
      setLifecycle(value);
      setSettings(value.settings);
      setPreflight(check);
      setUpgrades(upgradeResult.items.filter((item) => item.nodeId === nodeId));
      setError(undefined);
    } catch (caught) {
      setError(caught);
    }
  }, [cluster.id, nodeId]);
  useEffect(() => {
    void load();
  }, [load]);
  if (
    (node === undefined ||
      lifecycle === undefined ||
      settings === undefined ||
      preflight === undefined) &&
    error === undefined
  )
    return (
      <PageContainer size="wide">
        <Loading label="Loading node detail…" />
      </PageContainer>
    );
  if (
    node === undefined ||
    lifecycle === undefined ||
    settings === undefined ||
    preflight === undefined
  )
    return (
      <PageContainer size="wide">
        <ErrorState error={error} retry={() => void load()} />
      </PageContainer>
    );
  return (
    <PageContainer size="wide" className="node-detail-page">
      <nav className="breadcrumb" aria-label="Breadcrumb">
        <a href="/ha/nodes">Nodes</a> <span aria-hidden="true">/</span>{" "}
        <span aria-current="page">{node.name}</span>
      </nav>
      <PageHeader
        eyebrow="Managed node"
        title={node.name}
        description="Current operational state, lifecycle safeguards, and the next safe actions for this node."
        statusNotice={
          <StatusBadge
            status={node.maintenanceMode ? "maintenance" : node.healthStatus}
          />
        }
        primaryAction={
          <button
            className="button"
            type="button"
            disabled={busy !== ""}
            onClick={() => void testConnection()}
          >
            {busy === "test" ? "Testing…" : "Test connection"}
          </button>
        }
        secondaryActions={
          <button
            className="button button--secondary"
            type="button"
            disabled={busy !== ""}
            onClick={() => void refresh()}
          >
            {busy === "refresh" ? "Refreshing…" : "Refresh"}
          </button>
        }
      />
      {error !== undefined && (
        <Banner tone="warning" title="Operation failed">
          {error instanceof Error
            ? error.message
            : "The operation could not be completed."}
        </Banner>
      )}
      <section className="metrics" aria-label="Node lifecycle status">
        <MetricCard
          label="API"
          value={node.healthStatus.replaceAll("_", " ")}
          valueClassName="operational-value"
        />
        <MetricCard
          label="DNS"
          value={(lifecycle.dns?.status ?? "unknown").replaceAll("_", " ")}
          valueClassName="operational-value"
        />
        <MetricCard
          label="Configuration"
          value={node.convergenceStatus.replaceAll("_", " ")}
          valueClassName="operational-value"
        />
        <MetricCard
          label="Version"
          value={(node.version ?? "unknown").replaceAll("_", " ")}
        />
      </section>

      <SettingsGroup
        title="Overview"
        description="Identity, controller reachability, and desired-state position."
        bodySpacing="padded"
      >
        <dl className="detail-list">
          <div>
            <dt>Administration endpoint</dt>
            <dd className="break-anywhere">{node.baseUrl}</dd>
          </div>
          <div>
            <dt>Controller state</dt>
            <dd>{node.enabled ? "Managed" : "Disabled"}</dd>
          </div>
          <div>
            <dt>Compatibility</dt>
            <dd>{node.compatibilityStatus}</dd>
          </div>
          <div>
            <dt>Last API poll</dt>
            <dd>{formatTime(node.lastPolledAt)}</dd>
          </div>
          <div>
            <dt>Applied revision</dt>
            <dd>
              {node.appliedRevisionId ? (
                <a
                  href={`/ha/revisions?revisionId=${encodeURIComponent(node.appliedRevisionId)}`}
                >
                  View applied revision
                </a>
              ) : (
                "Not applied"
              )}
            </dd>
          </div>
          <div>
            <dt>Convergence</dt>
            <dd>{node.convergenceStatus.replaceAll("_", " ")}</dd>
          </div>
        </dl>
        <div className="row-actions row-actions--start">
          <a className="button button--secondary" href="/ha/configuration">
            Configuration Control
          </a>
          <a className="button button--secondary" href="/ha/drift">
            View drift
          </a>
          <a className="button button--secondary" href="/ha/deployments">
            View deployments
          </a>
        </div>
      </SettingsGroup>

      <SettingsGroup
        title="DNS Service"
        description="Active DNS is measured independently from the authenticated AdGuard Home API."
        bodySpacing="padded"
        actions={
          <button
            className="button button--secondary"
            type="button"
            disabled={busy !== ""}
            onClick={() => void probe()}
          >
            {busy === "probe" ? "Probing…" : "Probe now"}
          </button>
        }
      >
        {lifecycle.dns ? (
          <dl className="detail-list">
            <div>
              <dt>State</dt>
              <dd>
                <StatusBadge status={lifecycle.dns.status} />
              </dd>
            </div>
            <div>
              <dt>UDP / TCP</dt>
              <dd>
                {lifecycle.dns.udpStatus} / {lifecycle.dns.tcpStatus}
              </dd>
            </div>
            <div>
              <dt>Response / latency</dt>
              <dd>
                RCODE {lifecycle.dns.responseCode ?? "—"} ·{" "}
                {lifecycle.dns.latencyMs ?? "—"} ms
              </dd>
            </div>
            <div>
              <dt>Last probe</dt>
              <dd>{formatTime(lifecycle.dns.probedAt)}</dd>
            </div>
          </dl>
        ) : (
          <p>No durable DNS measurement yet.</p>
        )}
        <form
          className="card form-stack panel-form"
          onSubmit={(event) => void saveSettings(event)}
        >
          <h3>Probe and installation settings</h3>
          <Field
            label="Probe host"
            htmlFor="node-probe-host"
            help="Leave blank to use the node URL host."
          >
            <input
              id="node-probe-host"
              value={settings.dnsProbeHost}
              placeholder="Defaults to node URL host"
              onChange={(event) =>
                setSettings({ ...settings, dnsProbeHost: event.target.value })
              }
            />
          </Field>
          <Field label="Port" htmlFor="node-probe-port" required>
            <input
              id="node-probe-port"
              type="number"
              value={settings.dnsProbePort}
              onChange={(event) =>
                setSettings({
                  ...settings,
                  dnsProbePort: Number(event.target.value),
                })
              }
            />
          </Field>
          <Field label="Test name" htmlFor="node-probe-name" required>
            <input
              id="node-probe-name"
              value={settings.dnsProbeName}
              onChange={(event) =>
                setSettings({ ...settings, dnsProbeName: event.target.value })
              }
            />
          </Field>
          <Field
            label="Installation type"
            htmlFor="node-installation-type"
            required
          >
            <select
              id="node-installation-type"
              value={settings.installationType}
              onChange={(event) =>
                setSettings({
                  ...settings,
                  installationType: event.target
                    .value as LifecycleSettings["installationType"],
                })
              }
            >
              <option value="unknown">Unknown</option>
              <option value="native_systemd">Native / systemd</option>
              <option value="docker">Docker</option>
              <option value="home_assistant_addon">
                Home Assistant add-on
              </option>
              <option value="custom">Custom</option>
            </select>
          </Field>
          <label>
            <input
              type="checkbox"
              checked={settings.probeUdp}
              onChange={(event) =>
                setSettings({ ...settings, probeUdp: event.target.checked })
              }
            />{" "}
            Verify UDP
          </label>
          <label>
            <input
              type="checkbox"
              checked={settings.probeTcp}
              onChange={(event) =>
                setSettings({ ...settings, probeTcp: event.target.checked })
              }
            />{" "}
            Verify TCP
          </label>
          <button className="button" type="submit" disabled={busy !== ""}>
            {busy === "settings" ? "Saving…" : "Save lifecycle settings"}
          </button>
        </form>
      </SettingsGroup>

      <SettingsGroup
        title="Maintenance and DHCP"
        description="Preflight protects DNS redundancy, deployment activity, drift visibility, and active DHCP ownership."
        bodySpacing="padded"
      >
        <dl className="detail-list">
          <div>
            <dt>Healthy DNS nodes remaining</dt>
            <dd>{preflight.healthyDnsNodesRemaining}</dd>
          </div>
          <div>
            <dt>Expected redundancy</dt>
            <dd>{preflight.expectedRedundancy}</dd>
          </div>
          <div>
            <dt>Active deployment</dt>
            <dd>{preflight.activeDeployment ? "Blocking" : "None"}</dd>
          </div>
          <div>
            <dt>Open drift</dt>
            <dd>{preflight.openDrift ? "Warning" : "None"}</dd>
          </div>
          <div>
            <dt>Active DHCP</dt>
            <dd>{preflight.activeDhcp ? "Handoff required" : "No"}</dd>
          </div>
        </dl>
        <ul>
          {preflight.checks.map((check) => (
            <li key={check.name}>
              <strong>{check.name.replaceAll("_", " ")}</strong>: {check.status}{" "}
              — {check.message}
            </li>
          ))}
        </ul>
        {node.maintenanceMode ? (
          <button
            className="button"
            type="button"
            disabled={busy !== ""}
            onClick={() => void returnToService()}
          >
            Validate and return to service
          </button>
        ) : (
          <button
            className="button button--danger"
            type="button"
            disabled={busy !== "" || !preflight.allowed}
            onClick={() => void enterMaintenance()}
          >
            Enter maintenance
          </button>
        )}
        <div className="row-actions row-actions--start">
          <a className="button button--secondary" href="/settings/dhcp">
            Open DHCP settings
          </a>
        </div>
      </SettingsGroup>

      <SettingsGroup
        title="TLS / Certificates"
        description="Public certificate health only; private material stays outside controller state."
        bodySpacing="padded"
      >
        <dl className="detail-list">
          <div>
            <dt>Subject</dt>
            <dd>
              {lifecycle.certificate.state === "not_applicable"
                ? "Not configured"
                : lifecycle.certificate.subject || "Not reported"}
            </dd>
          </div>
          <div>
            <dt>Expiry</dt>
            <dd>
              {lifecycle.certificate.state === "not_applicable"
                ? "Not applicable"
                : formatTime(lifecycle.certificate.notAfter)}
            </dd>
          </div>
          <div>
            <dt>Remaining</dt>
            <dd>
              {lifecycle.certificate.daysRemaining === undefined
                ? "—"
                : `${lifecycle.certificate.daysRemaining} days`}
            </dd>
          </div>
          <div>
            <dt>State</dt>
            <dd>
              <StatusBadge
                status={
                  lifecycle.certificate.state === "critical" ||
                  lifecycle.certificate.state === "expired"
                    ? "failed"
                    : lifecycle.certificate.state
                }
                label={lifecycle.certificate.state.replaceAll("_", " ")}
              />
            </dd>
          </div>
        </dl>
        <p className="muted">
          Certificate chain, keys, and filesystem paths are never returned.
        </p>
      </SettingsGroup>

      <SettingsGroup
        title="Software Version"
        description="Compatibility and guided lifecycle coordination without remote command execution."
        bodySpacing="padded"
      >
        <dl className="detail-list">
          <div>
            <dt>Installed</dt>
            <dd>{lifecycle.version.installedVersion || "Unknown"}</dd>
          </div>
          <div>
            <dt>Latest known upstream</dt>
            <dd>{lifecycle.version.latestVersion || "Unavailable"}</dd>
          </div>
          <div>
            <dt>Controller compatibility</dt>
            <dd>{lifecycle.version.compatibility}</dd>
          </div>
          <div>
            <dt>Workflow</dt>
            <dd>{lifecycle.version.upgradeSupport}</dd>
          </div>
        </dl>
        {lifecycle.version.upgradeSupport === "guided" &&
          node.maintenanceMode && (
            <form
              className="card form-stack panel-form"
              onSubmit={(event) => void startUpgrade(event)}
            >
              <label>
                Target version
                <input
                  value={targetVersion}
                  onChange={(event) => setTargetVersion(event.target.value)}
                  required
                />
              </label>
              <button className="button" type="submit" disabled={busy !== ""}>
                Start guided upgrade
              </button>
              <p>
                Atlas DNS Controller records the operation and waits while you
                execute the documented host-side update. It never runs an
                arbitrary command.
              </p>
            </form>
          )}
        {upgrades
          .filter((upgrade) => upgrade.status === "awaiting_operator")
          .map((upgrade) => (
            <article className="card" key={upgrade.id}>
              <h3>Upgrade awaiting validation</h3>
              <p>
                Upgrade {upgrade.fromVersion} to {upgrade.targetVersion} using
                the supported host-side procedure. For Docker, update the pinned
                image and recreate only this node. For native/systemd, use the
                official AdGuard Home package/update procedure and restart only
                this node.
              </p>
              <button
                className="button"
                type="button"
                disabled={busy !== ""}
                onClick={() => void validateUpgrade(upgrade)}
              >
                Run post-upgrade validation
              </button>
            </article>
          ))}
      </SettingsGroup>

      <SettingsGroup
        title="Collectors and visibility"
        description="Canonical pages retain node attribution and detailed freshness evidence."
        bodySpacing="padded"
      >
        <div className="row-actions row-actions--start">
          <a className="button button--secondary" href="/statistics">
            Statistics
          </a>
          <a className="button button--secondary" href="/query-log">
            Query Log
          </a>
          <a
            className="button button--secondary"
            href="/system/operational-status"
          >
            Collector status
          </a>
        </div>
      </SettingsGroup>

      <SettingsGroup
        title="Operational History"
        description="Node lifecycle transitions; full configuration and security history remain on their canonical pages."
        bodySpacing="padded"
        actions={
          <a className="button button--secondary" href="/system/audit">
            Audit log
          </a>
        }
      >
        <div className="table-wrap">
          <table>
            <thead>
              <tr>
                <th>Time</th>
                <th>Event</th>
                <th>Severity</th>
                <th>Summary</th>
              </tr>
            </thead>
            <tbody>
              {lifecycle.events.map((event) => (
                <tr key={event.id}>
                  <td>{formatTime(event.occurredAt)}</td>
                  <td>{event.eventType}</td>
                  <td>{event.severity}</td>
                  <td>{event.summary}</td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      </SettingsGroup>
    </PageContainer>
  );

  async function probe() {
    setBusy("probe");
    try {
      await api.probeDNS(nodeId);
      await load();
    } catch (caught) {
      setError(caught);
    } finally {
      setBusy("");
    }
  }
  async function refresh() {
    setBusy("refresh");
    try {
      await load();
    } finally {
      setBusy("");
    }
  }
  async function testConnection() {
    setBusy("test");
    try {
      await api.testNode(nodeId);
      await load();
    } catch (caught) {
      setError(caught);
    } finally {
      setBusy("");
    }
  }
  async function saveSettings(event: React.FormEvent) {
    event.preventDefault();
    if (settings === undefined) return;
    setBusy("settings");
    try {
      await api.saveLifecycleSettings(nodeId, settings);
      await load();
    } catch (caught) {
      setError(caught);
    } finally {
      setBusy("");
    }
  }
  async function enterMaintenance() {
    if (preflight === undefined || node === undefined) return;
    let breakGlass = false,
      confirmation = "";
    if (preflight.breakGlassRequired) {
      breakGlass = window.confirm(
        "This leaves no verified healthy DNS node. Use break glass?",
      );
      if (!breakGlass) return;
      confirmation =
        window.prompt("Type CONTINUE_WITHOUT_DNS_REDUNDANCY") ?? "";
    } else if (!window.confirm(`Enter maintenance on ${node.name}?`)) return;
    setBusy("maintenance");
    try {
      await api.enterMaintenance(node, breakGlass, confirmation);
      await load();
    } catch (caught) {
      setError(caught);
    } finally {
      setBusy("");
    }
  }
  async function returnToService() {
    if (node === undefined) return;
    if (
      !window.confirm(
        "Run all return-to-service checks? The node stays in maintenance if any required check fails.",
      )
    )
      return;
    setBusy("return");
    try {
      await api.returnToService(node);
      await load();
    } catch (caught) {
      setError(caught);
      await load();
    } finally {
      setBusy("");
    }
  }
  async function startUpgrade(event: React.FormEvent) {
    event.preventDefault();
    if (!window.confirm(`Start a guided upgrade to ${targetVersion}?`)) return;
    setBusy("upgrade");
    try {
      await api.startUpgrade(nodeId, targetVersion);
      await load();
    } catch (caught) {
      setError(caught);
    } finally {
      setBusy("");
    }
  }
  async function validateUpgrade(upgrade: UpgradeOperation) {
    if (node === undefined) return;
    if (
      !window.confirm(
        "Validate the expected version, API, DNS, capabilities, configuration, drift, TLS, DHCP, and return this node to service only on success?",
      )
    )
      return;
    setBusy("validate-upgrade");
    try {
      await api.validateUpgrade(upgrade.id, node.recordVersion);
      await load();
    } catch (caught) {
      setError(caught);
      await load();
    } finally {
      setBusy("");
    }
  }
}
function formatTime(value?: string) {
  return value ? new Date(value).toLocaleString() : "—";
}
