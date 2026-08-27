import { useCallback, useEffect, useMemo, useState } from "react";
import { DataTable, type DataTableColumn } from "../../components/DataDisplay";
import {
  Banner,
  EmptyState,
  ErrorState,
  LoadingSkeleton,
} from "../../components/Feedback";
import { ConfirmDialog, Dialog } from "../../components/Overlays";
import { PageContainer, PageHeader } from "../../components/Page";
import { ScheduleEditor } from "../../components/ScheduleEditor";
import {
  Field,
  SettingRow,
  SettingsGroup,
  UnsavedChangesNotice,
} from "../../components/Settings";
import { StatusBadge, type StatusKind } from "../../components/StatusBadge";
import {
  IdentifierListEditor,
  TagMultiSelect,
  UpstreamEditor,
} from "../../components/StructuredInputs";
import { api } from "../../lib/api";
import type {
  BlockedServicesCatalogue,
  CapabilityProfile,
  Cluster,
  ConfigurationDraft,
  Node,
  PersistentClient,
  SafeSearchConfiguration,
  ValidationIssue,
} from "../../lib/types";
import { ServiceCatalogue } from "../blockedservices/ServiceCatalogue";
import {
  type CacheSizeUnit,
  cacheSizeForDisplay,
  cacheSizeToBytes,
  cleanClientForDraft,
  clientChangeState,
  clientMatchesSearch,
  hasClientValidationErrors,
  validatePersistentClient,
} from "./model";

const safeSearchProviders: ReadonlyArray<{
  key: Exclude<keyof SafeSearchConfiguration, "enabled">;
  label: string;
}> = [
  { key: "bing", label: "Bing" },
  { key: "duckDuckGo", label: "DuckDuckGo" },
  { key: "ecosia", label: "Ecosia" },
  { key: "google", label: "Google" },
  { key: "pixabay", label: "Pixabay" },
  { key: "yandex", label: "Yandex" },
  { key: "youTube", label: "YouTube" },
];

const emptySafeSearch = (): SafeSearchConfiguration => ({
  enabled: false,
  bing: true,
  duckDuckGo: true,
  ecosia: true,
  google: true,
  pixabay: true,
  yandex: true,
  youTube: true,
});

function emptyPersistentClient(): PersistentClient {
  return {
    name: "",
    ids: [],
    useGlobalSettings: true,
    filteringEnabled: true,
    parentalEnabled: false,
    safeBrowsingEnabled: false,
    safeSearch: emptySafeSearch(),
    useGlobalBlockedServices: true,
    blockedServices: [],
    blockedServicesSchedule: { timeZone: "Local", days: {} },
    upstreams: [],
    upstreamsCacheEnabled: false,
    upstreamsCacheSize: 0,
    tags: [],
    ignoreQueryLog: false,
    ignoreStatistics: false,
  };
}

type ClientEditor = {
  mode: "add" | "edit";
  index?: number;
  client: PersistentClient;
};

type ClientRow = { client: PersistentClient; index: number };

export function ClientsPage({ cluster }: { cluster: Cluster }) {
  const [draft, setDraft] = useState<ConfigurationDraft>();
  const [savedDocument, setSavedDocument] = useState("");
  const [savedClients, setSavedClients] = useState<PersistentClient[]>([]);
  const [nodes, setNodes] = useState<Node[]>([]);
  const [capabilities, setCapabilities] = useState<CapabilityProfile[]>([]);
  const [catalogue, setCatalogue] = useState<BlockedServicesCatalogue>();
  const [catalogueError, setCatalogueError] = useState<unknown>();
  const [issues, setIssues] = useState<ValidationIssue[]>([]);
  const [loading, setLoading] = useState(true);
  const [saving, setSaving] = useState(false);
  const [saved, setSaved] = useState(false);
  const [error, setError] = useState<unknown>();
  const [search, setSearch] = useState(
    () => new URLSearchParams(window.location.search).get("client") ?? "",
  );
  const [editor, setEditor] = useState<ClientEditor>();
  const [removeIndex, setRemoveIndex] = useState<number>();

  const load = useCallback(async () => {
    setLoading(true);
    try {
      const [inventory, nodeResult] = await Promise.all([
        api.configurationInventory(cluster.id),
        api.nodes(cluster.id),
      ]);
      setDraft(inventory.draft);
      setSavedDocument(
        inventory.draft ? JSON.stringify(inventory.draft.document) : "",
      );
      setSavedClients(
        inventory.draft?.document.shared.clients.map(cloneClient) ?? [],
      );
      setNodes(nodeResult.items);
      setCapabilities(inventory.capabilities);
      setIssues([]);
      setSaved(false);
      setError(undefined);
      try {
        setCatalogue(await api.blockedServicesCatalogue(cluster.id));
        setCatalogueError(undefined);
      } catch (caught) {
        setCatalogue(undefined);
        setCatalogueError(caught);
      }
    } catch (caught) {
      setError(caught);
    } finally {
      setLoading(false);
    }
  }, [cluster.id]);

  useEffect(() => void load(), [load]);

  const clients = draft?.document.shared.clients ?? [];
  const dirty =
    draft !== undefined && JSON.stringify(draft.document) !== savedDocument;
  const rows = clients
    .map((client, index) => ({ client, index }))
    .filter(({ client }) => clientMatchesSearch(client, search));
  const tagOptions = useMemo(
    () => [...new Set(clients.flatMap((client) => client.tags))],
    [clients],
  );
  const pendingRemovals = savedClients.filter(
    (savedClient) =>
      !clients.some(
        (client) =>
          client.name.localeCompare(savedClient.name, undefined, {
            sensitivity: "accent",
          }) === 0,
      ),
  );

  function setClients(value: PersistentClient[]) {
    if (draft === undefined) return;
    setSaved(false);
    setDraft({
      ...draft,
      document: {
        ...draft.document,
        shared: { ...draft.document.shared, clients: value },
      },
    });
  }

  function commitEditor(client: PersistentClient) {
    if (editor === undefined) return;
    const cleaned = cleanClientForDraft(client);
    if (editor.mode === "add") setClients([...clients, cleaned]);
    else if (editor.index !== undefined)
      setClients(
        clients.map((current, index) =>
          index === editor.index ? cleaned : current,
        ),
      );
    setEditor(undefined);
  }

  async function saveDraft() {
    if (draft === undefined) return;
    setSaving(true);
    setSaved(false);
    try {
      const result = await api.updateConfigurationDraft(
        cluster.id,
        draft.version,
        draft.document,
      );
      setDraft(result.draft);
      setSavedDocument(JSON.stringify(result.draft.document));
      setSavedClients(result.draft.document.shared.clients.map(cloneClient));
      setIssues(result.issues);
      setSaved(true);
      setError(undefined);
    } catch (caught) {
      setError(caught);
    } finally {
      setSaving(false);
    }
  }

  const columns: readonly DataTableColumn<ClientRow>[] = [
    {
      id: "name",
      header: "Client name",
      render: ({ client }) => <strong>{client.name}</strong>,
    },
    {
      id: "identifiers",
      header: "Identifiers",
      render: ({ client }) => <ValueSummary values={client.ids} />,
    },
    {
      id: "tags",
      header: "Tags",
      render: ({ client }) => <TagSummary values={client.tags} />,
    },
    {
      id: "inheritance",
      header: "Policy state",
      render: ({ client }) => (
        <StatusBadge
          status={client.useGlobalSettings ? "info" : "warning"}
          label={client.useGlobalSettings ? "Inherited" : "Overrides"}
        />
      ),
    },
    {
      id: "safety",
      header: "Safety overrides",
      render: ({ client }) => <SafetySummary client={client} />,
    },
    {
      id: "services",
      header: "Blocked services",
      render: ({ client }) =>
        client.useGlobalBlockedServices
          ? "Global policy"
          : `${client.blockedServices.length} selected`,
    },
    {
      id: "compatibility",
      header: "Node compatibility",
      render: ({ client }) => {
        const compatibility = clientCompatibility(
          client,
          nodes,
          capabilities,
          catalogue,
          catalogueError,
        );
        return (
          <StatusBadge
            status={compatibility.status}
            label={compatibility.label}
          />
        );
      },
    },
    {
      id: "draft",
      header: "Draft state",
      render: ({ client }) => {
        const state = clientChangeState(client, savedClients);
        return (
          <StatusBadge
            status={
              state === "added"
                ? "success"
                : state === "modified"
                  ? "warning"
                  : "info"
            }
            label={
              state === "unchanged"
                ? "In draft"
                : state === "added"
                  ? "Added"
                  : "Modified"
            }
          />
        );
      },
    },
    {
      id: "actions",
      header: "Actions",
      align: "right",
      render: ({ client, index }) => (
        <div className="row-actions client-table__actions">
          <button
            type="button"
            className="button button--quiet"
            aria-label={`Edit ${client.name}`}
            onClick={() =>
              setEditor({ mode: "edit", index, client: cloneClient(client) })
            }
          >
            Edit
          </button>
          <button
            type="button"
            className="button button--danger"
            aria-label={`Remove ${client.name}`}
            onClick={() => setRemoveIndex(index)}
          >
            Remove
          </button>
        </div>
      ),
    },
  ];

  if (loading && draft === undefined)
    return (
      <PageContainer size="wide">
        <PageHeader title="Persistent Clients" />
        <LoadingSkeleton label="Loading persistent clients" rows={6} />
      </PageContainer>
    );
  if (error !== undefined && draft === undefined)
    return (
      <PageContainer size="wide">
        <PageHeader title="Persistent Clients" />
        <ErrorState error={error} retry={() => void load()} />
      </PageContainer>
    );

  const removeClient =
    removeIndex === undefined ? undefined : clients[removeIndex];

  return (
    <PageContainer size="wide">
      <PageHeader
        eyebrow="Settings"
        title="Persistent Clients"
        description="Manage client identities and per-client policy in the cluster draft. Nodes change only after publication and deployment."
        primaryAction={
          <button
            type="button"
            className="button"
            disabled={draft === undefined || saving}
            onClick={() => void saveDraft()}
          >
            {saving ? "Saving…" : "Save Draft"}
          </button>
        }
        secondaryActions={
          <button
            type="button"
            className="button button--secondary"
            disabled={draft === undefined || draft.document.schemaVersion !== 2}
            onClick={() =>
              setEditor({ mode: "add", client: emptyPersistentClient() })
            }
          >
            Add Client
          </button>
        }
      />

      {error !== undefined && (
        <Banner tone="danger" title="The latest request failed">
          {error instanceof Error ? error.message : "Something went wrong."}
        </Banner>
      )}
      {draft === undefined ? (
        <EmptyState title="Import a node configuration first">
          <p>
            Open Configuration Control, refresh a node, and import its
            observation to create the cluster draft.
          </p>
        </EmptyState>
      ) : draft.document.schemaVersion !== 2 ? (
        <Banner tone="danger" title="Unsupported draft format">
          Import a current schema-v2 observation before editing persistent
          clients.
        </Banner>
      ) : (
        <>
          <UnsavedChangesNotice
            dirty={dirty}
            saving={saving}
            saved={saved}
            onSave={() => void saveDraft()}
          />
          {catalogueError !== undefined && (
            <Banner
              tone="warning"
              title="Blocked-services catalogue unavailable"
            >
              Client identities and other policy remain editable. Existing
              service IDs are retained, but named service selection is limited
              until catalogue metadata is available.
            </Banner>
          )}
          {catalogue?.partial && (
            <Banner tone="warning" title="Blocked-services metadata is partial">
              Existing selections are retained. Publication preflight remains
              authoritative for node compatibility.
            </Banner>
          )}
          {pendingRemovals.length > 0 && (
            <Banner tone="warning" title="Client removal pending in this draft">
              {pendingRemovals.map((client) => client.name).join(", ")} will
              remain on every node until this draft is saved, published, and
              deployed.
            </Banner>
          )}
          <div className="client-table-toolbar">
            <label>
              Search clients
              <input
                type="search"
                value={search}
                placeholder="Name, identifier, or tag"
                onChange={(event) => setSearch(event.target.value)}
              />
            </label>
            <p className="muted">
              {rows.length} of {clients.length} clients shown
            </p>
          </div>
          <DataTable
            columns={columns}
            rows={rows}
            rowKey={({ client, index }) => `${index}-${client.name}`}
            caption="Persistent clients in the current desired-state draft"
            emptyTitle={
              clients.length === 0
                ? "No persistent clients"
                : "No clients match this search"
            }
            filteredEmpty={clients.length > 0}
            emptyDescription={
              clients.length === 0 ? (
                <>
                  <p>
                    Add a client to manage its identity and policy across every
                    node.
                  </p>
                  <button
                    type="button"
                    className="button"
                    onClick={() =>
                      setEditor({
                        mode: "add",
                        client: emptyPersistentClient(),
                      })
                    }
                  >
                    Add Client
                  </button>
                </>
              ) : (
                <button
                  type="button"
                  className="button button--secondary"
                  onClick={() => setSearch("")}
                >
                  Clear search
                </button>
              )
            }
          />
          {issues.length > 0 && (
            <Banner tone="warning" title="Validation needs attention">
              <ul className="compact-list">
                {issues.map((issue) => (
                  <li key={`${issue.field}-${issue.message}`}>
                    {issue.field}: {issue.message}
                  </li>
                ))}
              </ul>
            </Banner>
          )}
        </>
      )}

      {editor !== undefined && (
        <ClientDialog
          editor={editor}
          clients={clients}
          tagOptions={tagOptions}
          catalogue={catalogue ?? emptyCatalogue(nodes)}
          catalogueUnavailable={catalogueError !== undefined}
          onClose={() => setEditor(undefined)}
          onSave={commitEditor}
        />
      )}
      <ConfirmDialog
        open={removeClient !== undefined}
        onClose={() => setRemoveIndex(undefined)}
        onConfirm={() => {
          if (removeIndex !== undefined)
            setClients(clients.filter((_, index) => index !== removeIndex));
          setRemoveIndex(undefined);
        }}
        title={`Remove ${removeClient?.name ?? "client"}?`}
        description="This updates the configuration draft only."
        confirmLabel="Remove from Draft"
      >
        <p>
          The client will not be deleted from any AdGuard Home node until you
          save this draft, publish a revision, and deploy it.
        </p>
      </ConfirmDialog>
    </PageContainer>
  );
}

function ClientDialog({
  editor,
  clients,
  tagOptions,
  catalogue,
  catalogueUnavailable,
  onClose,
  onSave,
}: {
  editor: ClientEditor;
  clients: PersistentClient[];
  tagOptions: string[];
  catalogue: BlockedServicesCatalogue;
  catalogueUnavailable: boolean;
  onClose: () => void;
  onSave: (client: PersistentClient) => void;
}) {
  const [client, setClient] = useState(editor.client);
  const validation = validatePersistentClient(client, clients, editor.index);
  const invalid = hasClientValidationErrors(validation);
  const update = (patch: Partial<PersistentClient>) =>
    setClient((current) => ({ ...current, ...patch }));
  const policyDisabled = client.useGlobalSettings;

  return (
    <Dialog
      open
      onClose={onClose}
      title={
        editor.mode === "add"
          ? "Add Persistent Client"
          : "Edit Persistent Client"
      }
      description="Changes are staged in the browser until you save the cluster draft."
      size="large"
      actions={
        <>
          <button
            type="button"
            className="button button--secondary"
            onClick={onClose}
          >
            Cancel
          </button>
          <button
            type="button"
            className="button"
            disabled={invalid}
            onClick={() => onSave(client)}
          >
            {editor.mode === "add" ? "Add to Draft" : "Update Draft"}
          </button>
        </>
      }
    >
      <div className="client-dialog__sections">
        <SettingsGroup
          title="Identity"
          description="A client needs a unique name and at least one address or identifier."
        >
          <div className="client-dialog__group-content">
            <Field
              label="Name"
              htmlFor="persistent-client-name"
              required
              error={validation.name}
            >
              <input
                id="persistent-client-name"
                value={client.name}
                autoComplete="off"
                aria-invalid={validation.name !== undefined}
                onChange={(event) => update({ name: event.target.value })}
              />
            </Field>
            <IdentifierListEditor
              label="Identifiers"
              value={client.ids}
              onChange={(ids) => update({ ids })}
              placeholder="IP, CIDR, MAC, or ClientID"
              addLabel="Add identifier"
              help="Browser checks are guidance; AdGuard Home validation remains authoritative. Case and identifier semantics are preserved."
            />
            {validation.identifiers.length > 0 && (
              <Banner tone="danger" title="Identifiers need attention">
                <ul className="compact-list">
                  {[...new Set(validation.identifiers)].map((issue) => (
                    <li key={issue}>{issue}</li>
                  ))}
                </ul>
              </Banner>
            )}
          </div>
        </SettingsGroup>

        <SettingsGroup
          title="Tags"
          description="Tags already present in this draft are offered as suggestions. Free-entry values and unknown legacy tags are retained."
        >
          <div className="client-dialog__group-content">
            <TagMultiSelect
              label="Client tags"
              value={client.tags}
              options={tagOptions}
              onChange={(tags) => update({ tags })}
            />
          </div>
        </SettingsGroup>

        <SettingsGroup title="Inheritance">
          <SettingRow
            title="Use global filtering settings"
            description="Inherit cluster filtering and safety policy. Turn this off to configure client-specific overrides below."
            status={
              <StatusBadge
                status={client.useGlobalSettings ? "info" : "warning"}
                label={client.useGlobalSettings ? "Inherited" : "Overridden"}
              />
            }
            control={
              <Switch
                label="Use global filtering settings"
                checked={client.useGlobalSettings}
                onChange={(useGlobalSettings) => update({ useGlobalSettings })}
              />
            }
          />
        </SettingsGroup>

        <SettingsGroup
          title="Filtering and safety"
          description={
            policyDisabled
              ? "These values are preserved but inactive while global filtering settings are inherited."
              : "These settings override the global policy for this client."
          }
          disabled={policyDisabled}
        >
          <SettingRow
            title="Filtering"
            control={
              <Switch
                label="Filtering"
                checked={client.filteringEnabled}
                disabled={policyDisabled}
                onChange={(filteringEnabled) => update({ filteringEnabled })}
              />
            }
          />
          <SettingRow
            title="Safe Browsing"
            control={
              <Switch
                label="Safe Browsing"
                checked={client.safeBrowsingEnabled}
                disabled={policyDisabled}
                onChange={(safeBrowsingEnabled) =>
                  update({ safeBrowsingEnabled })
                }
              />
            }
          />
          <SettingRow
            title="Parental control"
            control={
              <Switch
                label="Parental control"
                checked={client.parentalEnabled}
                disabled={policyDisabled}
                onChange={(parentalEnabled) => update({ parentalEnabled })}
              />
            }
          />
          <SettingRow
            title="Safe Search"
            description="Enable supported search providers as one grouped policy."
            control={
              <Switch
                label="Safe Search"
                checked={client.safeSearch.enabled}
                disabled={policyDisabled}
                onChange={(enabled) =>
                  update({ safeSearch: { ...client.safeSearch, enabled } })
                }
              />
            }
          />
          {client.safeSearch.enabled && (
            <fieldset
              className="safe-search-providers"
              disabled={policyDisabled}
            >
              <legend>Safe Search providers</legend>
              <div className="toggle-grid">
                {safeSearchProviders.map((provider) => (
                  <Switch
                    key={provider.key}
                    label={provider.label}
                    checked={client.safeSearch[provider.key]}
                    disabled={policyDisabled}
                    onChange={(enabled) =>
                      update({
                        safeSearch: {
                          ...client.safeSearch,
                          [provider.key]: enabled,
                        },
                      })
                    }
                  />
                ))}
              </div>
            </fieldset>
          )}
        </SettingsGroup>

        <SettingsGroup
          title="Logging and statistics"
          description="Choose whether this client's activity contributes to node-local logs and statistics."
        >
          <SettingRow
            title="Include in query log"
            description="When enabled, matching queries remain visible in each node's query log."
            control={
              <Switch
                label="Include in query log"
                checked={!client.ignoreQueryLog}
                onChange={(included) => update({ ignoreQueryLog: !included })}
              />
            }
          />
          <SettingRow
            title="Include in statistics"
            description="When enabled, matching queries contribute to each node's statistics."
            control={
              <Switch
                label="Include in statistics"
                checked={!client.ignoreStatistics}
                onChange={(included) => update({ ignoreStatistics: !included })}
              />
            }
          />
        </SettingsGroup>

        <SettingsGroup
          title="Blocked services"
          description="Inherit the cluster selection or choose a client-specific service set and inactivity schedule."
        >
          <SettingRow
            title="Use global blocked services"
            control={
              <Switch
                label="Use global blocked services"
                checked={client.useGlobalBlockedServices}
                onChange={(useGlobalBlockedServices) =>
                  update({ useGlobalBlockedServices })
                }
              />
            }
          />
          {!client.useGlobalBlockedServices && (
            <div className="client-dialog__group-content">
              {catalogueUnavailable && (
                <Banner tone="warning" title="Catalogue metadata unavailable">
                  Existing service IDs remain selected and can be retained or
                  removed. Reload the page to restore named selection.
                </Banner>
              )}
              <ServiceCatalogue
                catalogue={catalogue}
                selectedIDs={client.blockedServices}
                onChange={(blockedServices) => update({ blockedServices })}
              />
              <ScheduleEditor
                label="Client blocked-services inactivity schedule"
                value={client.blockedServicesSchedule}
                onChange={(blockedServicesSchedule) =>
                  update({ blockedServicesSchedule })
                }
              />
            </div>
          )}
        </SettingsGroup>

        <SettingsGroup
          title="Upstreams"
          description="Client-specific resolvers use AdGuard Home upstream syntax and retain their order."
        >
          <div className="client-dialog__group-content">
            <UpstreamEditor
              label="Client-specific upstreams"
              value={client.upstreams}
              onChange={(upstreams) => update({ upstreams })}
              placeholder={"1.1.1.1\n[/example.org/]tls://dns.example"}
              help="One resolver per line. Plain IPs and hostnames, encrypted tls://, https://, quic:// or sdns:// resolvers, and [/domain/]resolver selectors are supported. Use # after a selector for the default upstream. Order is preserved; AdGuard Home remains authoritative."
            />
            {validation.upstreams.length > 0 && (
              <Banner tone="danger" title="Upstreams need attention">
                {validation.upstreams[0]}
              </Banner>
            )}
          </div>
        </SettingsGroup>

        <SettingsGroup title="Response cache">
          <SettingRow
            title="Cache client-specific upstream responses"
            control={
              <Switch
                label="Enable response cache"
                checked={client.upstreamsCacheEnabled}
                onChange={(upstreamsCacheEnabled) =>
                  update({ upstreamsCacheEnabled })
                }
              />
            }
          />
          <div className="client-dialog__group-content">
            <CacheSizeField
              value={client.upstreamsCacheSize}
              disabled={!client.upstreamsCacheEnabled}
              error={validation.cacheSize}
              onChange={(upstreamsCacheSize) => update({ upstreamsCacheSize })}
            />
          </div>
        </SettingsGroup>
      </div>
    </Dialog>
  );
}

function Switch({
  label,
  checked,
  disabled = false,
  onChange,
}: {
  label: string;
  checked: boolean;
  disabled?: boolean;
  onChange: (checked: boolean) => void;
}) {
  return (
    <label className="checkbox">
      <input
        type="checkbox"
        checked={checked}
        disabled={disabled}
        onChange={(event) => onChange(event.target.checked)}
      />
      {label}
    </label>
  );
}

function CacheSizeField({
  value,
  disabled,
  error,
  onChange,
}: {
  value: number;
  disabled: boolean;
  error?: string;
  onChange: (bytes: number) => void;
}) {
  const initial = cacheSizeForDisplay(value);
  const [unit, setUnit] = useState<CacheSizeUnit>(initial.unit);
  const multiplier = cacheSizeToBytes(1, unit);
  return (
    <Field
      label="Cache size"
      htmlFor="client-cache-size"
      help="Stored in bytes in the desired-state document."
      error={error}
    >
      <div className="cache-size-field">
        <input
          id="client-cache-size"
          type="number"
          min="0"
          step="any"
          value={Number.isFinite(value) ? value / multiplier : ""}
          disabled={disabled}
          aria-invalid={error !== undefined}
          onChange={(event) =>
            onChange(
              event.target.value === ""
                ? Number.NaN
                : cacheSizeToBytes(Number(event.target.value), unit),
            )
          }
        />
        <select
          aria-label="Cache size unit"
          value={unit}
          disabled={disabled}
          onChange={(event) => setUnit(event.target.value as CacheSizeUnit)}
        >
          <option value="bytes">bytes</option>
          <option value="KiB">KiB</option>
          <option value="MiB">MiB</option>
        </select>
      </div>
    </Field>
  );
}

function ValueSummary({ values }: { values: string[] }) {
  if (values.length === 0) return <span className="muted">None</span>;
  return (
    <span>
      <span className="monospace">{values.slice(0, 2).join(", ")}</span>
      {values.length > 2 && (
        <small className="table-subtitle">+{values.length - 2} more</small>
      )}
    </span>
  );
}

function TagSummary({ values }: { values: string[] }) {
  if (values.length === 0) return <span className="muted">None</span>;
  return (
    <span className="client-tag-summary">
      {values.slice(0, 2).map((tag) => (
        <span className="tag-chip" key={tag}>
          {tag}
        </span>
      ))}
      {values.length > 2 && <small>+{values.length - 2}</small>}
    </span>
  );
}

function SafetySummary({ client }: { client: PersistentClient }) {
  if (client.useGlobalSettings) return <span className="muted">Inherited</span>;
  const enabled = [
    client.filteringEnabled && "Filtering",
    client.safeBrowsingEnabled && "Safe Browsing",
    client.parentalEnabled && "Parental",
    client.safeSearch.enabled && "Safe Search",
  ].filter(Boolean);
  return enabled.length > 0 ? enabled.join(", ") : "All off";
}

function clientCompatibility(
  client: PersistentClient,
  nodes: Node[],
  capabilities: CapabilityProfile[],
  catalogue: BlockedServicesCatalogue | undefined,
  catalogueError: unknown,
): { status: StatusKind; label: string } {
  const enabledNodeIDs = new Set(
    nodes.filter((node) => node.enabled).map((node) => node.id),
  );
  const profiles = capabilities.filter((profile) =>
    enabledNodeIDs.has(profile.nodeId),
  );
  if (enabledNodeIDs.size === 0)
    return { status: "empty", label: "No enabled nodes" };
  if (profiles.some((profile) => !profile.features.clients))
    return { status: "unsupported", label: "Unsupported" };
  if (profiles.length < enabledNodeIDs.size)
    return { status: "warning", label: "Partial metadata" };
  if (
    !client.useGlobalSettings &&
    client.safeSearch.enabled &&
    client.safeSearch.ecosia &&
    profiles.some((profile) => !profile.features.safe_search_ecosia)
  )
    return { status: "unsupported", label: "Ecosia unsupported" };
  if (!client.useGlobalBlockedServices && catalogue !== undefined) {
    const selected = new Set(client.blockedServices);
    if (
      catalogue.services.some(
        (service) =>
          selected.has(service.id) && service.unsupportedNodeIds.length > 0,
      )
    )
      return { status: "unsupported", label: "Service mismatch" };
    if (
      client.blockedServices.some(
        (id) => !catalogue.services.some((service) => service.id === id),
      )
    )
      return { status: "warning", label: "Needs preflight" };
  }
  if (catalogueError !== undefined || catalogue?.partial)
    return { status: "warning", label: "Partial metadata" };
  return { status: "success", label: "Compatible" };
}

function cloneClient(client: PersistentClient): PersistentClient {
  return {
    ...client,
    ids: [...client.ids],
    safeSearch: { ...client.safeSearch },
    blockedServices: [...client.blockedServices],
    blockedServicesSchedule: {
      ...client.blockedServicesSchedule,
      days: { ...client.blockedServicesSchedule.days },
    },
    upstreams: [...client.upstreams],
    tags: [...client.tags],
  };
}

function emptyCatalogue(nodes: Node[]): BlockedServicesCatalogue {
  return {
    services: [],
    groups: [],
    nodes: nodes.map((node) => ({
      nodeId: node.id,
      nodeName: node.name,
      version: node.version,
      status: "error",
      serviceCount: 0,
    })),
    generatedAt: new Date(0).toISOString(),
    stale: true,
    partial: true,
  };
}
