import type {
  AdminUser,
  AllowlistPresentation,
  ApiErrorBody,
  AuditEvent,
  AuthResponse,
  BlockedServicesCatalogue,
  BlocklistPresentation,
  CapabilityProfile,
  CertificateHealth,
  CertificatePolicy,
  Cluster,
  ConfigurationDifference,
  ConfigurationDraft,
  ConfigurationRevision,
  ConfigurationSnapshot,
  ControllerUpdateStatus,
  Deployment,
  DeploymentPreview,
  DesiredConfigurationDocument,
  DhcpActiveCheckResult,
  DhcpInterfaces,
  DhcpOperation,
  DNSOperationalCommand,
  DriftEvent,
  HAHistoryPage,
  HASummary,
  LifecycleSettings,
  MaintenancePreflight,
  Node,
  NodeLifecycle,
  NotificationChannel,
  NotificationTestResult,
  OperationalStatus,
  OperationalTarget,
  QueryEvent,
  QueryEventPage,
  RestorePreflight,
  StatisticsReport,
  SystemSettings,
  UpgradeOperation,
  ValidationIssue,
  VersionHealth,
  VersionInfo,
} from "./types";

export class ApiError extends Error {
  readonly code: string;
  readonly field?: string;
  readonly requestId: string;
  readonly status: number;

  constructor(status: number, body: ApiErrorBody) {
    super(body.message);
    this.name = "ApiError";
    this.code = body.code;
    this.field = body.field;
    this.requestId = body.requestId;
    this.status = status;
  }
}

function cookie(name: string): string {
  const prefix = `${encodeURIComponent(name)}=`;
  for (const part of document.cookie.split(";")) {
    const value = part.trim();
    if (value.startsWith(prefix)) {
      return decodeURIComponent(value.slice(prefix.length));
    }
  }
  return "";
}

async function request<T>(path: string, options: RequestInit = {}): Promise<T> {
  const method = options.method ?? "GET";
  const headers = new Headers(options.headers);
  headers.set("Accept", "application/json");
  if (options.body !== undefined) {
    headers.set("Content-Type", "application/json");
  }
  if (!["GET", "HEAD", "OPTIONS"].includes(method.toUpperCase())) {
    headers.set("X-CSRF-Token", cookie("atlas_dns_csrf"));
  }
  const response = await fetch(path, {
    ...options,
    headers,
    credentials: "same-origin",
  });
  if (!response.ok) {
    const fallback: ApiErrorBody = {
      code: "HTTP_ERROR",
      message: `Request failed with status ${response.status}.`,
      requestId: response.headers.get("X-Request-ID") ?? "unknown",
    };
    let body = fallback;
    try {
      const payload = (await response.json()) as { error?: ApiErrorBody };
      if (payload.error !== undefined) body = payload.error;
    } catch {
      // The stable fallback avoids exposing an untrusted HTML error body.
    }
    throw new ApiError(response.status, body);
  }
  if (response.status === 204) return undefined as T;
  return (await response.json()) as T;
}

async function checkedResponse(
  path: string,
  options: RequestInit,
): Promise<Response> {
  const headers = new Headers(options.headers);
  headers.set("X-CSRF-Token", cookie("atlas_dns_csrf"));
  const response = await fetch(path, {
    ...options,
    headers,
    credentials: "same-origin",
  });
  if (!response.ok) {
    let body: ApiErrorBody = {
      code: "HTTP_ERROR",
      message: `Request failed with status ${response.status}.`,
      requestId: response.headers.get("X-Request-ID") ?? "unknown",
    };
    try {
      body = ((await response.json()) as { error: ApiErrorBody }).error ?? body;
    } catch {
      /* safe fallback */
    }
    throw new ApiError(response.status, body);
  }
  return response;
}

export interface NodePayload {
  name: string;
  baseUrl: string;
  certificatePolicy: CertificatePolicy;
  customCaPem?: string;
  credentials?: { username: string; password: string };
  enabled: boolean;
  recordVersion?: number;
}

export const api = {
  setupStatus: () =>
    request<{
      setupRequired: boolean;
      publicBaseUrl: string;
      controllerTime: string;
      secureCookies: boolean;
      checks: Record<string, string>;
    }>("/api/v1/setup/status"),
  setup: (input: { email: string; displayName: string; password: string }) =>
    request<AuthResponse>("/api/v1/setup", {
      method: "POST",
      body: JSON.stringify(input),
    }),
  login: (input: { email: string; password: string }) =>
    request<AuthResponse>("/api/v1/auth/login", {
      method: "POST",
      body: JSON.stringify(input),
    }),
  me: () => request<AuthResponse>("/api/v1/auth/me"),
  logout: () =>
    request<void>("/api/v1/auth/logout", { method: "POST", body: "{}" }),
  users: () => request<{ items: AdminUser[] }>("/api/v1/users"),
  createUser: (input: {
    email: string;
    displayName: string;
    password: string;
  }) =>
    request<AdminUser>("/api/v1/users", {
      method: "POST",
      body: JSON.stringify({ ...input, role: "administrator" }),
    }),
  updateUser: (user: AdminUser, enabled: boolean) =>
    request<AdminUser>(`/api/v1/users/${user.id}`, {
      method: "PATCH",
      body: JSON.stringify({
        email: user.email,
        displayName: user.displayName,
        role: user.role,
        enabled,
      }),
    }),
  resetUserPassword: (userId: string, password: string) =>
    request<void>(`/api/v1/users/${userId}/password-reset`, {
      method: "POST",
      body: JSON.stringify({ password }),
    }),
  createBackup: async (type: "standard" | "full", passphrase: string) => {
    const response = await checkedResponse("/api/v1/system/backups", {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ type, passphrase }),
    });
    const disposition = response.headers.get("Content-Disposition") ?? "";
    const filename =
      disposition.match(/filename="([^"]+)"/)?.[1] ??
      `atlas-dns-${type}.atlasdnsbackup`;
    return {
      blob: await response.blob(),
      filename,
      type: response.headers.get("X-Backup-Type") ?? type,
      applicationVersion:
        response.headers.get("X-Backup-Application-Version") ?? "unknown",
      databaseSchemaVersion:
        response.headers.get("X-Backup-Schema-Version") ?? "unknown",
      createdAt: response.headers.get("X-Backup-Created-At") ?? "",
    };
  },
  restorePreflight: async (archive: File, passphrase: string) => {
    const form = new FormData();
    form.set("archive", archive);
    form.set("passphrase", passphrase);
    const response = await checkedResponse("/api/v1/system/restore-preflight", {
      method: "POST",
      body: form,
    });
    return (await response.json()) as RestorePreflight;
  },
  controllerUpdate: () =>
    request<ControllerUpdateStatus>("/api/v1/system/update"),
  checkControllerUpdate: () =>
    request<ControllerUpdateStatus>("/api/v1/system/update/check", {
      method: "POST",
      body: "{}",
    }),
  versionInfo: () => request<VersionInfo>("/api/v1/system/version"),
  systemSettings: () => request<SystemSettings>("/api/v1/system/settings"),
  updateSystemSettings: (
    settings: SystemSettings,
    updateChecksEnabled: boolean,
  ) =>
    request<SystemSettings>("/api/v1/system/settings", {
      method: "PATCH",
      body: JSON.stringify({
        updateChecksEnabled,
        recordVersion: settings.recordVersion,
      }),
    }),
  clusters: () => request<{ items: Cluster[] }>("/api/v1/clusters"),
  createCluster: (input: { name: string; description: string }) =>
    request<Cluster>("/api/v1/clusters", {
      method: "POST",
      body: JSON.stringify(input),
    }),
  updateCluster: (cluster: Cluster) =>
    request<Cluster>(`/api/v1/clusters/${cluster.id}`, {
      method: "PATCH",
      body: JSON.stringify({
        name: cluster.name,
        description: cluster.description,
        reconciliationPolicy: cluster.reconciliationPolicy,
        version: cluster.version,
      }),
    }),
  nodes: (clusterId: string) =>
    request<{
      items: Node[];
      refreshedAt: string;
      staleAfterSeconds: number;
    }>(`/api/v1/clusters/${clusterId}/nodes`),
  operationalStatus: (clusterId: string) =>
    request<OperationalStatus>(
      `/api/v1/clusters/${clusterId}/operational-status`,
    ),
  haStatus: (clusterId: string) =>
    request<HASummary>(`/api/v1/clusters/${clusterId}/ha-status`),
  haHistory: (
    clusterId: string,
    options: { nodeId?: string; cursor?: string; limit?: number } = {},
  ) => {
    const query = new URLSearchParams({ limit: String(options.limit ?? 50) });
    if (options.nodeId) query.set("nodeId", options.nodeId);
    if (options.cursor) query.set("cursor", options.cursor);
    return request<HAHistoryPage>(
      `/api/v1/clusters/${clusterId}/ha-history?${query.toString()}`,
    );
  },
  certificates: (clusterId: string) =>
    request<{ items: CertificateHealth[] }>(
      `/api/v1/clusters/${clusterId}/certificates`,
    ),
  versions: (clusterId: string) =>
    request<{ items: VersionHealth[] }>(
      `/api/v1/clusters/${clusterId}/versions`,
    ),
  nodeLifecycle: (nodeId: string) =>
    request<NodeLifecycle>(`/api/v1/nodes/${nodeId}/lifecycle`),
  maintenancePreflight: (nodeId: string) =>
    request<MaintenancePreflight>(
      `/api/v1/nodes/${nodeId}/maintenance-preflight`,
    ),
  probeDNS: (nodeId: string) =>
    request<import("./types").DNSProbeResult>(
      `/api/v1/nodes/${nodeId}/dns-probe`,
      { method: "POST", body: "{}" },
    ),
  saveLifecycleSettings: (nodeId: string, settings: LifecycleSettings) =>
    request<LifecycleSettings>(`/api/v1/nodes/${nodeId}/lifecycle-settings`, {
      method: "PUT",
      body: JSON.stringify(settings),
    }),
  enterMaintenance: (node: Node, breakGlass = false, confirmation = "") =>
    request<Node>(`/api/v1/nodes/${node.id}/maintenance`, {
      method: "POST",
      body: JSON.stringify({
        enabled: true,
        recordVersion: node.recordVersion,
        breakGlass,
        confirmation,
      }),
    }),
  returnToService: (node: Node) =>
    request<{
      nodeId: string;
      succeeded: boolean;
      checks: import("./types").LifecycleCheck[];
    }>(`/api/v1/nodes/${node.id}/return-to-service`, {
      method: "POST",
      body: JSON.stringify({ recordVersion: node.recordVersion }),
    }),
  upgrades: (clusterId: string) =>
    request<{ items: UpgradeOperation[] }>(
      `/api/v1/clusters/${clusterId}/upgrades`,
    ),
  startUpgrade: (nodeId: string, targetVersion: string) =>
    request<UpgradeOperation>(`/api/v1/nodes/${nodeId}/upgrades`, {
      method: "POST",
      body: JSON.stringify({ targetVersion }),
    }),
  validateUpgrade: (upgradeId: string, recordVersion: number) =>
    request<UpgradeOperation>(`/api/v1/upgrades/${upgradeId}/validate`, {
      method: "POST",
      body: JSON.stringify({ recordVersion }),
    }),
  notificationChannels: (clusterId: string) =>
    request<{ items: NotificationChannel[] }>(
      `/api/v1/clusters/${clusterId}/notification-channels`,
    ),
  createNotificationChannel: (
    clusterId: string,
    input: {
      name: string;
      destination: string;
      enabled: boolean;
    },
  ) =>
    request<NotificationChannel>(
      `/api/v1/clusters/${clusterId}/notification-channels`,
      {
        method: "POST",
        body: JSON.stringify(input),
      },
    ),
  updateNotificationChannel: (
    channelId: string,
    input: {
      name: string;
      enabled: boolean;
      recordVersion: number;
      destination?: string;
      replaceDestination?: boolean;
    },
  ) =>
    request<NotificationChannel>(`/api/v1/notification-channels/${channelId}`, {
      method: "PATCH",
      body: JSON.stringify(input),
    }),
  deleteNotificationChannel: (
    channelId: string,
    recordVersion: number,
    confirmation: string,
  ) =>
    request<void>(`/api/v1/notification-channels/${channelId}`, {
      method: "DELETE",
      body: JSON.stringify({ recordVersion, confirmation }),
    }),
  testNotificationChannel: (channelId: string) =>
    request<NotificationTestResult>(
      `/api/v1/notification-channels/${channelId}/test`,
      { method: "POST", body: JSON.stringify({}) },
    ),
  statistics: (clusterId: string, range: string, nodeId = "") => {
    const query = new URLSearchParams({ range, limit: "10" });
    if (nodeId !== "") query.set("nodeId", nodeId);
    return request<StatisticsReport>(
      `/api/v1/clusters/${clusterId}/statistics?${query.toString()}`,
    );
  },
  queryEvents: (
    clusterId: string,
    input: {
      nodeId?: string;
      cursor?: string;
      limit?: number;
      search?: string;
      status?: string;
      queryType?: string;
      client?: string;
    } = {},
  ) => {
    const query = new URLSearchParams({ limit: String(input.limit ?? 50) });
    if (input.nodeId) query.set("nodeId", input.nodeId);
    if (input.cursor) query.set("cursor", input.cursor);
    if (input.search) query.set("search", input.search);
    if (input.status) query.set("status", input.status);
    if (input.queryType) query.set("queryType", input.queryType);
    if (input.client) query.set("client", input.client);
    return request<QueryEventPage>(
      `/api/v1/clusters/${clusterId}/query-events?${query.toString()}`,
    );
  },
  queryEvent: (clusterId: string, eventId: string) =>
    request<QueryEvent>(
      `/api/v1/clusters/${clusterId}/query-events/${eventId}`,
    ),
  createNode: (clusterId: string, input: NodePayload) =>
    request<Node>(`/api/v1/clusters/${clusterId}/nodes`, {
      method: "POST",
      body: JSON.stringify(input),
    }),
  updateNode: (nodeId: string, input: NodePayload) =>
    request<Node>(`/api/v1/nodes/${nodeId}`, {
      method: "PATCH",
      body: JSON.stringify(input),
    }),
  deleteNode: (node: Node, confirmName: string) =>
    request<void>(`/api/v1/nodes/${node.id}`, {
      method: "DELETE",
      body: JSON.stringify({ recordVersion: node.recordVersion, confirmName }),
    }),
  testNode: (nodeId: string) =>
    request<{
      version: string;
      compatibility: string;
      running: boolean;
      latencyMs: number;
    }>(`/api/v1/nodes/${nodeId}/test-connection`, {
      method: "POST",
      body: "{}",
    }),
  observeNode: (nodeId: string) =>
    request<ConfigurationSnapshot>(`/api/v1/nodes/${nodeId}/observations`, {
      method: "POST",
      body: "{}",
    }),
  refreshFilters: (nodeId: string, whitelist: boolean) =>
    request<{ nodeId: string; whitelist: boolean; status: "succeeded" }>(
      `/api/v1/nodes/${nodeId}/filter-refresh`,
      { method: "POST", body: JSON.stringify({ whitelist }) },
    ),
  testUpstreamDNS: (
    clusterId: string,
    target: OperationalTarget,
    input: {
      draftVersion: number;
      upstreamDns: string[];
      bootstrapDns: string[];
      fallbackDns: string[];
      privateReverseDns: string[];
      upstreamMode: string;
      usePrivateReverseResolvers: boolean;
    },
    idempotencyKey: string,
  ) =>
    request<DNSOperationalCommand>(
      `/api/v1/clusters/${clusterId}/operational-commands/test-upstream-dns`,
      {
        method: "POST",
        headers: { "Idempotency-Key": idempotencyKey },
        body: JSON.stringify({ target, input }),
      },
    ),
  clearDNSCache: (
    clusterId: string,
    target: OperationalTarget,
    idempotencyKey: string,
  ) =>
    request<DNSOperationalCommand>(
      `/api/v1/clusters/${clusterId}/operational-commands/clear-dns-cache`,
      {
        method: "POST",
        headers: { "Idempotency-Key": idempotencyKey },
        body: JSON.stringify({ target, confirmation: "CLEAR_DNS_CACHE" }),
      },
    ),
  testHostFiltering: (
    clusterId: string,
    target: OperationalTarget,
    input: { hostname: string; client?: string; queryType?: string },
    idempotencyKey: string,
  ) =>
    request<DNSOperationalCommand>(
      `/api/v1/clusters/${clusterId}/operational-commands/test-host-filtering`,
      {
        method: "POST",
        headers: { "Idempotency-Key": idempotencyKey },
        body: JSON.stringify({ target, input }),
      },
    ),
  clearQueryLog: (
    clusterId: string,
    target: OperationalTarget,
    idempotencyKey: string,
  ) =>
    request<DNSOperationalCommand>(
      `/api/v1/clusters/${clusterId}/operational-commands/clear-query-log`,
      {
        method: "POST",
        headers: { "Idempotency-Key": idempotencyKey },
        body: JSON.stringify({ target, confirmation: "CLEAR_QUERY_LOG" }),
      },
    ),
  resetStatistics: (
    clusterId: string,
    target: OperationalTarget,
    idempotencyKey: string,
  ) =>
    request<DNSOperationalCommand>(
      `/api/v1/clusters/${clusterId}/operational-commands/reset-statistics`,
      {
        method: "POST",
        headers: { "Idempotency-Key": idempotencyKey },
        body: JSON.stringify({ target, confirmation: "RESET_STATISTICS" }),
      },
    ),
  dnsOperation: (operationId: string) =>
    request<DNSOperationalCommand>(
      `/api/v1/operational-commands/${operationId}`,
    ),
  dnsOperations: (
    clusterId: string,
    command?: DNSOperationalCommand["command"],
  ) =>
    request<{ items: DNSOperationalCommand[] }>(
      `/api/v1/clusters/${clusterId}/operational-commands?limit=10${command ? `&command=${command}` : ""}`,
    ),
  dhcpInterfaces: (nodeId: string) =>
    request<DhcpInterfaces>(`/api/v1/nodes/${nodeId}/dhcp/interfaces`),
  checkActiveDhcp: (nodeId: string, interfaceName: string) =>
    request<DhcpActiveCheckResult>(
      `/api/v1/nodes/${nodeId}/dhcp/active-check`,
      {
        method: "POST",
        body: JSON.stringify({ interfaceName }),
      },
    ),
  resetDhcpLeases: (
    nodeId: string,
    confirmation: string,
    idempotencyKey: string,
  ) =>
    request<DhcpOperation>(`/api/v1/nodes/${nodeId}/dhcp/reset-leases`, {
      method: "POST",
      headers: { "Idempotency-Key": idempotencyKey },
      body: JSON.stringify({ confirmation }),
    }),
  resetDhcpConfiguration: (
    nodeId: string,
    confirmation: string,
    idempotencyKey: string,
  ) =>
    request<DhcpOperation>(`/api/v1/nodes/${nodeId}/dhcp/reset-configuration`, {
      method: "POST",
      headers: { "Idempotency-Key": idempotencyKey },
      body: JSON.stringify({ confirmation }),
    }),
  dhcpOperations: (nodeId: string) =>
    request<{ items: DhcpOperation[] }>(
      `/api/v1/nodes/${nodeId}/dhcp/operations?limit=10`,
    ),
  configurationInventory: (clusterId: string) =>
    request<{
      schemaVersion: number;
      snapshots: ConfigurationSnapshot[];
      capabilities: CapabilityProfile[];
      draft?: ConfigurationDraft;
    }>(`/api/v1/clusters/${clusterId}/configuration-inventory`),
  blockedServicesCatalogue: (clusterId: string) =>
    request<BlockedServicesCatalogue>(
      `/api/v1/clusters/${clusterId}/blocked-services/catalogue`,
    ),
  blocklistPresentation: (clusterId: string) =>
    request<BlocklistPresentation>(
      `/api/v1/clusters/${clusterId}/blocklists/presentation`,
    ),
  allowlistPresentation: (clusterId: string) =>
    request<AllowlistPresentation>(
      `/api/v1/clusters/${clusterId}/allowlists/presentation`,
    ),
  compareConfigurations: (leftSnapshotId: string, rightSnapshotId: string) =>
    request<{ equal: boolean; differences: ConfigurationDifference[] }>(
      `/api/v1/configuration-comparisons?leftSnapshotId=${encodeURIComponent(leftSnapshotId)}&rightSnapshotId=${encodeURIComponent(rightSnapshotId)}`,
    ),
  importConfiguration: (
    clusterId: string,
    snapshotId: string,
    expectedVersion: number,
  ) =>
    request<ConfigurationDraft>(
      `/api/v1/clusters/${clusterId}/configuration-draft/import`,
      {
        method: "POST",
        body: JSON.stringify({ snapshotId, expectedVersion, confirmed: true }),
      },
    ),
  updateConfigurationDraft: (
    clusterId: string,
    expectedVersion: number,
    document: DesiredConfigurationDocument,
  ) =>
    request<{ draft: ConfigurationDraft; issues: ValidationIssue[] }>(
      `/api/v1/clusters/${clusterId}/configuration-draft`,
      {
        method: "PUT",
        body: JSON.stringify({ expectedVersion, document }),
      },
    ),
  validateConfigurationDraft: (clusterId: string) =>
    request<DeploymentPreview>(
      `/api/v1/clusters/${clusterId}/configuration-draft/validate`,
      { method: "POST", body: "{}" },
    ),
  publishConfigurationRevision: (
    clusterId: string,
    expectedVersion: number,
    summary: string,
  ) =>
    request<ConfigurationRevision>(
      `/api/v1/clusters/${clusterId}/configuration-revisions`,
      {
        method: "POST",
        body: JSON.stringify({ expectedVersion, summary }),
      },
    ),
  configurationRevisions: (clusterId: string, includeArchived = false) =>
    request<{ items: ConfigurationRevision[] }>(
      `/api/v1/clusters/${clusterId}/configuration-revisions${includeArchived ? "?includeArchived=true" : ""}`,
    ),
  archiveConfigurationRevision: (revisionId: string) =>
    request<void>(`/api/v1/configuration-revisions/${revisionId}/archive`, {
      method: "POST",
      body: JSON.stringify({ confirmed: true }),
    }),
  restoreConfigurationRevision: (revisionId: string) =>
    request<void>(`/api/v1/configuration-revisions/${revisionId}/restore`, {
      method: "POST",
      body: JSON.stringify({ confirmed: true }),
    }),
  deleteConfigurationRevision: (revisionId: string, confirmation: string) =>
    request<void>(`/api/v1/configuration-revisions/${revisionId}`, {
      method: "DELETE",
      body: JSON.stringify({ confirmation }),
    }),
  compareConfigurationRevisions: (
    leftRevisionId: string,
    rightRevisionId: string,
  ) =>
    request<{ equal: boolean; differences: ConfigurationDifference[] }>(
      `/api/v1/configuration-revision-comparisons?leftRevisionId=${encodeURIComponent(leftRevisionId)}&rightRevisionId=${encodeURIComponent(rightRevisionId)}`,
    ),
  deploymentPreview: (clusterId: string, revisionId: string) =>
    request<DeploymentPreview>(
      `/api/v1/clusters/${clusterId}/configuration-revisions/${revisionId}/deployment-preview`,
      { method: "POST", body: "{}" },
    ),
  startDeployment: (clusterId: string, revisionId: string) =>
    request<Deployment>(
      `/api/v1/clusters/${clusterId}/configuration-revisions/${revisionId}/deployments`,
      { method: "POST", body: JSON.stringify({ targetNodeIds: [] }) },
    ),
  rollback: (clusterId: string, revisionId: string) =>
    request<Deployment>(
      `/api/v1/clusters/${clusterId}/configuration-revisions/${revisionId}/rollback`,
      { method: "POST", body: JSON.stringify({ confirmed: true }) },
    ),
  deployments: (clusterId: string, includeArchived = false) =>
    request<{ items: Deployment[] }>(
      `/api/v1/clusters/${clusterId}/deployments${includeArchived ? "?includeArchived=true" : ""}`,
    ),
  deployment: (deploymentId: string) =>
    request<Deployment>(`/api/v1/deployments/${deploymentId}`),
  cancelDeployment: (deploymentId: string) =>
    request<void>(`/api/v1/deployments/${deploymentId}/cancel`, {
      method: "POST",
      body: "{}",
    }),
  archiveDeployment: (deploymentId: string) =>
    request<void>(`/api/v1/deployments/${deploymentId}/archive`, {
      method: "POST",
      body: JSON.stringify({ confirmed: true }),
    }),
  restoreDeployment: (deploymentId: string) =>
    request<void>(`/api/v1/deployments/${deploymentId}/restore`, {
      method: "POST",
      body: JSON.stringify({ confirmed: true }),
    }),
  deleteDeployment: (deploymentId: string, confirmation: string) =>
    request<void>(`/api/v1/deployments/${deploymentId}`, {
      method: "DELETE",
      body: JSON.stringify({ confirmation }),
    }),
  driftEvents: (clusterId: string) =>
    request<{ items: DriftEvent[] }>(
      `/api/v1/clusters/${clusterId}/drift-events`,
    ),
  restoreDrift: (driftId: string) =>
    request<Deployment>(`/api/v1/drift-events/${driftId}/restore`, {
      method: "POST",
      body: "{}",
    }),
  adoptDrift: (driftId: string, expectedDraftVersion: number) =>
    request<ConfigurationDraft>(`/api/v1/drift-events/${driftId}/adopt`, {
      method: "POST",
      body: JSON.stringify({ expectedDraftVersion }),
    }),
  auditEvents: () =>
    request<{ items: AuditEvent[] }>("/api/v1/audit-events?limit=100"),
};
