import {
  type FormEvent,
  useCallback,
  useEffect,
  useMemo,
  useState,
} from "react";
import { HealthSummaryCard } from "../../components/DataDisplay";
import { EmptyState, ErrorState, Loading } from "../../components/Feedback";
import { PageHeader } from "../../components/Page";
import { StatusBadge } from "../../components/StatusBadge";
import { api, type NodePayload } from "../../lib/api";
import type {
  CapabilityProfile,
  CertificatePolicy,
  Cluster,
  ConfigurationRevision,
  ConfigurationSnapshot,
  DriftEvent,
  Node,
} from "../../lib/types";
import { nodeDetailPath } from "../../routing/routes";

export function NodesPage({ cluster }: { cluster: Cluster }) {
  const [nodes, setNodes] = useState<Node[]>();
  const [capabilities, setCapabilities] = useState<CapabilityProfile[]>([]);
  const [snapshots, setSnapshots] = useState<ConfigurationSnapshot[]>([]);
  const [revisions, setRevisions] = useState<ConfigurationRevision[]>([]);
  const [drift, setDrift] = useState<DriftEvent[]>([]);
  const [error, setError] = useState<unknown>();
  const [showAdd, setShowAdd] = useState(false);
  const [editing, setEditing] = useState<Node>();

  const load = useCallback(async () => {
    try {
      const [nodeResult, inventory, revisionResult, driftResult] =
        await Promise.all([
          api.nodes(cluster.id),
          api.configurationInventory(cluster.id),
          api.configurationRevisions(cluster.id, true),
          api.driftEvents(cluster.id),
        ]);
      setNodes(nodeResult.items);
      setCapabilities(inventory.capabilities);
      setSnapshots(inventory.snapshots);
      setRevisions(revisionResult.items);
      setDrift(driftResult.items);
      setError(undefined);
    } catch (caught) {
      setError(caught);
    }
  }, [cluster.id]);

  useEffect(() => {
    void load();
  }, [load]);

  const revisionNumbers = useMemo(
    () =>
      new Map(
        revisions.map((revision) => [revision.id, revision.revisionNumber]),
      ),
    [revisions],
  );
  const openDriftNodes = useMemo(
    () =>
      new Set(
        drift
          .filter((item) => item.status === "open")
          .map((item) => item.nodeId),
      ),
    [drift],
  );
  const currentNodes = nodes ?? [];
  const healthyNodes = currentNodes.filter(
    (node) => node.healthStatus === "healthy",
  ).length;
  const degradedNodes = currentNodes.filter(
    (node) => node.enabled && node.healthStatus === "unknown",
  ).length;
  const unreachableNodes = currentNodes.filter(
    (node) => node.healthStatus === "unreachable",
  ).length;
  const incompatibleNodes = currentNodes.filter(
    (node) =>
      node.healthStatus === "incompatible" ||
      node.compatibilityStatus === "unsupported",
  ).length;
  const latestObservation = latestTime(
    snapshots.map((snapshot) => snapshot.observedAt),
  );

  return (
    <>
      <PageHeader
        eyebrow="HA Controller"
        title="Nodes"
        description={`Inventory, identity, and controller connectivity for ${cluster.name}. Open a node for operational tests and maintenance.`}
        primaryAction={
          <button
            type="button"
            className="button"
            onClick={() => {
              setShowAdd((value) => !value);
              setEditing(undefined);
            }}
          >
            {showAdd ? "Cancel" : "Add node"}
          </button>
        }
      />
      {showAdd && (
        <NodeForm
          cluster={cluster}
          onSaved={() => {
            setShowAdd(false);
            void load();
          }}
        />
      )}
      {editing !== undefined && (
        <NodeForm
          cluster={cluster}
          node={editing}
          onSaved={() => {
            setEditing(undefined);
            void load();
          }}
        />
      )}
      {nodes === undefined && error === undefined && (
        <Loading label="Loading nodes…" />
      )}
      {nodes === undefined && error !== undefined && (
        <ErrorState error={error} retry={() => void load()} />
      )}
      {nodes?.length === 0 && !showAdd && (
        <EmptyState title="No managed nodes">
          <p>Add an AdGuard Home node to begin automatic status polling.</p>
        </EmptyState>
      )}
      {nodes !== undefined && nodes.length > 0 && (
        <>
          <section
            className="health-summary-grid health-summary-grid--five"
            aria-label="Cluster node summary"
          >
            <HealthSummaryCard
              icon="ha"
              label="Fleet health"
              value={`${healthyNodes} / ${nodes.length}`}
              status={
                nodes.every(
                  (node) =>
                    node.healthStatus === "healthy" ||
                    node.healthStatus === "disabled",
                )
                  ? "healthy"
                  : "degraded"
              }
              detail="nodes healthy"
            />
            <HealthSummaryCard
              icon="attention"
              label="Degraded"
              value={degradedNodes}
              status={degradedNodes === 0 ? "healthy" : "degraded"}
              detail="enabled nodes unknown"
            />
            <HealthSummaryCard
              icon="nodes"
              label="Unreachable"
              value={unreachableNodes}
              status={unreachableNodes === 0 ? "healthy" : "unreachable"}
              detail="nodes unreachable"
            />
            <HealthSummaryCard
              icon="updates"
              label="Incompatible"
              value={incompatibleNodes}
              status={incompatibleNodes === 0 ? "healthy" : "incompatible"}
              detail="unsupported nodes"
            />
            <HealthSummaryCard
              icon="activity"
              label="Last observation"
              value={latestObservation}
              status={snapshots.length === 0 ? "unknown" : "success"}
              statusLabel={snapshots.length === 0 ? undefined : "Recorded"}
              detail="latest inventory snapshot"
            />
          </section>
          <div className="table-wrap">
            <table>
              <thead>
                <tr>
                  <th>Node</th>
                  <th>Status</th>
                  <th>Version</th>
                  <th>Capabilities</th>
                  <th>Active revision</th>
                  <th>Drift</th>
                  <th>Latency</th>
                  <th>Last observation</th>
                  <th>
                    <span className="visually-hidden">Actions</span>
                  </th>
                </tr>
              </thead>
              <tbody>
                {nodes.map((node) => (
                  <tr key={node.id}>
                    <td>
                      <a href={nodeDetailPath(node.id)}>
                        <strong>{node.name}</strong>
                      </a>
                      <span className="table-subtitle">{node.baseUrl}</span>
                    </td>
                    <td>
                      <StatusBadge status={node.healthStatus} />
                    </td>
                    <td>
                      {node.version ?? "—"}
                      <span className="table-subtitle">
                        {node.compatibilityStatus}
                      </span>
                    </td>
                    <td>
                      {capabilityLabel(
                        capabilities.find((item) => item.nodeId === node.id),
                      )}
                    </td>
                    <td>
                      {node.appliedRevisionId ? (
                        <a
                          href={`/ha/revisions?revisionId=${encodeURIComponent(node.appliedRevisionId)}`}
                        >
                          #
                          {revisionNumbers.get(node.appliedRevisionId) ??
                            "Unknown"}
                        </a>
                      ) : (
                        "Not applied"
                      )}
                      <span className="table-subtitle">
                        {node.maintenanceMode
                          ? "maintenance"
                          : node.convergenceStatus.replaceAll("_", " ")}
                      </span>
                    </td>
                    <td>
                      {openDriftNodes.has(node.id) ||
                      node.convergenceStatus === "drifted" ? (
                        <a href="/ha/drift">
                          <StatusBadge status="drifted" />
                        </a>
                      ) : (
                        <StatusBadge
                          status={
                            node.convergenceStatus === "converged"
                              ? "converged"
                              : "pending"
                          }
                        />
                      )}
                    </td>
                    <td>
                      {node.latencyMs === undefined
                        ? "—"
                        : `${node.latencyMs} ms`}
                    </td>
                    <td>
                      {formatTime(
                        snapshots.find(
                          (snapshot) => snapshot.nodeId === node.id,
                        )?.observedAt ?? node.lastPolledAt,
                      )}
                    </td>
                    <td>
                      <div className="row-actions">
                        <a
                          className="button button--quiet"
                          href={nodeDetailPath(node.id)}
                        >
                          Manage
                        </a>
                        <button
                          type="button"
                          className="button button--quiet"
                          onClick={() => {
                            setEditing(node);
                            setShowAdd(false);
                          }}
                        >
                          Edit
                        </button>
                        <button
                          type="button"
                          className="button button--danger"
                          onClick={() => void remove(node)}
                        >
                          Remove
                        </button>
                      </div>
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        </>
      )}
    </>
  );

  async function remove(node: Node) {
    const confirmName = window.prompt(
      `Type ${node.name} to remove this node. Its stored credentials will be destroyed.`,
    );
    if (confirmName === null) return;
    try {
      await api.deleteNode(node, confirmName);
      await load();
    } catch (caught) {
      setError(caught);
    }
  }
}

export function NodeForm({
  cluster,
  node,
  onSaved,
  requireOnboardingCompatibility = false,
  onValidated,
}: {
  cluster: Cluster;
  node?: Node;
  onSaved: () => void;
  requireOnboardingCompatibility?: boolean;
  onValidated?: (result: {
    version: string;
    compatibility: string;
    running: boolean;
    latencyMs: number;
  }) => void;
}) {
  const [name, setName] = useState(node?.name ?? "");
  const [baseUrl, setBaseUrl] = useState(node?.baseUrl ?? "https://");
  const [policy, setPolicy] = useState<CertificatePolicy>(
    node?.certificatePolicy ?? "system",
  );
  const [customCaPem, setCustomCaPem] = useState("");
  const [username, setUsername] = useState("");
  const [password, setPassword] = useState("");
  const [enabled, setEnabled] = useState(node?.enabled ?? true);
  const [error, setError] = useState<unknown>();
  const [submitting, setSubmitting] = useState(false);

  async function submit(event: FormEvent) {
    event.preventDefault();
    setSubmitting(true);
    setError(undefined);
    const payload: NodePayload = {
      name,
      baseUrl,
      certificatePolicy: policy,
      enabled,
    };
    if (customCaPem.trim() !== "") payload.customCaPem = customCaPem;
    if (username !== "" || password !== "")
      payload.credentials = { username, password };
    if (node !== undefined) payload.recordVersion = node.recordVersion;
    try {
      if (node === undefined) {
        if (payload.credentials === undefined)
          throw new Error("Username and password are required for a new node.");
        const result = await api.validateNodeCandidate(cluster.id, payload);
        onValidated?.(result);
        if (
          requireOnboardingCompatibility &&
          result.onboardingCompatibility !== "supported"
        )
          throw new Error(
            `AdGuard Home ${result.version || "version unknown"} is below the v1.1 onboarding minimum of 0.107.78 or outside the compatible 0.107 API generation.`,
          );
        await api.createNode(cluster.id, payload);
      } else {
        await api.updateNode(node.id, payload);
      }
      setPassword("");
      onSaved();
    } catch (caught) {
      setError(caught);
    } finally {
      setSubmitting(false);
    }
  }

  return (
    <form
      className="card form-stack node-form"
      onSubmit={(event) => void submit(event)}
    >
      <div>
        <p className="eyebrow">
          {node === undefined ? "Onboarding" : "Node settings"}
        </p>
        <h2>
          {node === undefined ? "Add AdGuard Home node" : `Edit ${node.name}`}
        </h2>
      </div>
      <div className="form-grid">
        <label>
          Name
          <input
            value={name}
            onChange={(event) => setName(event.target.value)}
            required
            maxLength={120}
          />
        </label>
        <label>
          Administration URL
          <input
            type="url"
            value={baseUrl}
            onChange={(event) => setBaseUrl(event.target.value)}
            required
          />
        </label>
        <label>
          TLS trust policy
          <select
            value={policy}
            onChange={(event) =>
              setPolicy(event.target.value as CertificatePolicy)
            }
          >
            <option value="system">System trust</option>
            <option value="custom_ca">Custom private CA</option>
            <option value="insecure_http">Explicit plaintext HTTP</option>
          </select>
        </label>
        <label className="checkbox">
          <input
            type="checkbox"
            checked={enabled}
            onChange={(event) => setEnabled(event.target.checked)}
          />
          Enable automatic health polling
        </label>
      </div>
      {policy === "custom_ca" && (
        <label>
          Custom CA certificate
          <textarea
            rows={6}
            value={customCaPem}
            onChange={(event) => setCustomCaPem(event.target.value)}
            placeholder={
              node === undefined
                ? "-----BEGIN CERTIFICATE-----"
                : "Leave blank to retain the stored CA"
            }
          />
        </label>
      )}
      {policy === "insecure_http" && (
        <div className="notice notice--warning">
          Plain HTTP exposes node credentials on the management network. TLS
          certificate verification cannot otherwise be disabled.
        </div>
      )}
      <fieldset>
        <legend>
          {node === undefined
            ? "Node credentials"
            : "Rotate credentials (optional)"}
        </legend>
        <div className="form-grid">
          <label>
            Username
            <input
              autoComplete="off"
              value={username}
              onChange={(event) => setUsername(event.target.value)}
              required={node === undefined}
            />
          </label>
          <label>
            Password
            <input
              type="password"
              autoComplete="new-password"
              value={password}
              onChange={(event) => setPassword(event.target.value)}
              required={node === undefined}
            />
          </label>
        </div>
      </fieldset>
      <p className="muted">
        The controller tests authentication and status before it stores
        encrypted credentials. It does not change node configuration.
      </p>
      {error !== undefined && <ErrorState error={error} />}
      <button type="submit" className="button" disabled={submitting}>
        {submitting ? "Testing and saving…" : "Test and save"}
      </button>
    </form>
  );
}

function formatTime(value?: string): string {
  if (value === undefined) return "Never";
  const date = new Date(value);
  return Number.isNaN(date.valueOf()) ? "Unknown" : date.toLocaleString();
}

function latestTime(values: string[]): string {
  const latest = values
    .map((value) => new Date(value))
    .filter((value) => !Number.isNaN(value.valueOf()))
    .sort((left, right) => right.valueOf() - left.valueOf())[0];
  return latest ? latest.toLocaleString() : "Never";
}

function capabilityLabel(profile?: CapabilityProfile): string {
  if (!profile) return "Unknown";
  const supported = Object.values(profile.features).filter(Boolean).length;
  return `${profile.compatibility} · ${supported} features`;
}
