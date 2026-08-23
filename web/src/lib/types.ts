export interface User {
  id: string;
  email: string;
  displayName: string;
  role: "administrator";
}

export interface AdminUser extends User {
  enabled: boolean;
  createdAt: string;
  updatedAt: string;
  lastLoginAt?: string;
}

export interface VersionInfo {
  version: string;
  commit: string;
  builtAt: string;
  development: boolean;
  databaseSchemaVersion: number;
}

export interface ControllerUpdateStatus {
  installedVersion: string;
  buildIdentifier: string;
  buildDate: string;
  development: boolean;
  latestVersion?: string;
  state: "development" | "unknown" | "update_available" | "up_to_date";
  releaseUrl?: string;
  releaseNotes?: string;
  lastChecked?: string;
  errorCode?: string;
  installationType: "docker" | "native_systemd" | "custom" | "unknown";
  updateMethod: string;
  updateCommand?: string;
  backupRequired: boolean;
}

export interface SystemSettings {
  updateChecksEnabled: boolean;
  recordVersion: number;
  queryLogRetention: string;
  statisticsRetention: string;
  installationType: string;
}

export interface BackupManifest {
  backupFormatVersion: number;
  applicationVersion: string;
  buildIdentifier: string;
  databaseSchemaVersion: number;
  createdAt: string;
  type: "standard" | "full";
  includedComponents: string[];
  excludedComponents: string[];
  requiresPassphrase: boolean;
  sessionsRestored: boolean;
}

export interface RestorePreflight {
  valid: boolean;
  manifest: BackupManifest;
  sizeBytes: number;
  plan: {
    execution: "offline_cli";
    requiresRestart: boolean;
    replaces: string[];
    retains: string[];
    sessionsInvalidated: boolean;
  };
}

export interface AuthResponse {
  user: User;
  expiresAt: string;
}

export interface Cluster {
  id: string;
  name: string;
  description: string;
  version: number;
  reconciliationPolicy: "enforce" | "alert" | "manual";
  activeRevisionId?: string;
  createdAt: string;
  updatedAt: string;
}

export type HealthStatus =
  | "unknown"
  | "healthy"
  | "unreachable"
  | "incompatible"
  | "disabled";
export type CertificatePolicy = "system" | "custom_ca" | "insecure_http";

export interface Node {
  id: string;
  clusterId: string;
  name: string;
  baseUrl: string;
  certificatePolicy: CertificatePolicy;
  enabled: boolean;
  healthStatus: HealthStatus;
  compatibilityStatus: "unknown" | "supported" | "unsupported";
  version?: string;
  lastSeenAt?: string;
  lastPolledAt?: string;
  latencyMs?: number;
  lastErrorCode?: string;
  maintenanceMode: boolean;
  appliedRevisionId?: string;
  appliedHash?: string;
  convergenceStatus:
    | "pending"
    | "converged"
    | "drifted"
    | "applying"
    | "verifying"
    | "apply_failed"
    | "observation_failed"
    | "unsupported"
    | "maintenance";
  lastReconciledAt?: string;
  recordVersion: number;
  createdAt: string;
  updatedAt: string;
}

export interface DesiredConfigurationDocument {
  schemaVersion: number;
  shared: ConfigurationDocument["shared"];
  nodeOverrides: Record<string, ConfigurationDocument["nodeSpecific"]>;
  unsupported: ConfigurationDocument["unsupported"];
}

export interface AuditEvent {
  id: string;
  actorType: "user" | "system" | "anonymous";
  actorUserId?: string;
  action: string;
  resourceType: string;
  resourceId?: string;
  requestId: string;
  metadata: Record<string, unknown>;
  createdAt: string;
}

export interface ConfigurationDocument {
  schemaVersion: number;
  shared: {
    dns: {
      upstreamDns: string[];
      bootstrapDns: string[];
      fallbackDns: string[];
      privateReverseDns: string[];
      protectionEnabled: boolean;
      rateLimit: number;
      rateLimitSubnetLengthIpv4: number;
      rateLimitSubnetLengthIpv6: number;
      rateLimitAllowlist: string[];
      blockingMode:
        | ""
        | "default"
        | "refused"
        | "nxdomain"
        | "null_ip"
        | "custom_ip";
      blockingIpv4: string;
      blockingIpv6: string;
      blockedResponseTtl: number;
      ednsClientSubnet: boolean;
      ednsUseCustom: boolean;
      ednsCustomIp: string;
      disableIpv6: boolean;
      dnssecEnabled: boolean;
      cacheSize: number;
      cacheEnabled: boolean;
      cacheTtlMin: number;
      cacheTtlMax: number;
      cacheOptimistic: boolean;
      upstreamMode: "" | "load_balance" | "fastest_addr" | "parallel";
      usePrivateReverseResolvers: boolean;
      resolveClients: boolean;
      upstreamTimeoutSeconds: number;
    };
    filtering: {
      enabled: boolean;
      updateIntervalHours: number;
      filterUrls: string[];
      whitelistUrls: string[];
      userRules: string[];
    };
    clients: PersistentClient[];
    rewritesEnabled: boolean;
    rewrites: Rewrite[];
    services: ServicesConfiguration;
    queryLog: QueryLogPolicy;
    statistics: StatisticsPolicy;
  };
  nodeSpecific: NodeSpecificConfiguration;
  observedOnly: {
    productVersion: string;
    tls: TlsStatus;
    dhcpLeases: DhcpLease[];
  };
  unsupported: { section: string; reason: string }[];
}

export interface SafeSearchConfiguration {
  enabled: boolean;
  bing: boolean;
  duckDuckGo: boolean;
  ecosia: boolean;
  google: boolean;
  pixabay: boolean;
  yandex: boolean;
  youTube: boolean;
}

export interface PersistentClient {
  name: string;
  ids: string[];
  useGlobalSettings: boolean;
  filteringEnabled: boolean;
  parentalEnabled: boolean;
  safeBrowsingEnabled: boolean;
  safeSearch: SafeSearchConfiguration;
  useGlobalBlockedServices: boolean;
  blockedServices: string[];
  blockedServicesSchedule: Schedule;
  upstreams: string[];
  upstreamsCacheEnabled: boolean;
  upstreamsCacheSize: number;
  tags: string[];
  ignoreQueryLog: boolean;
  ignoreStatistics: boolean;
}

export interface Rewrite {
  domain: string;
  answer: string;
  enabled: boolean;
}
export interface DayRange {
  start: number;
  end: number;
}
export interface Schedule {
  timeZone: string;
  days: Record<string, DayRange>;
}
export interface ServicesConfiguration {
  blockedServiceIds: string[];
  blockedSchedule: Schedule;
  safeBrowsing: boolean;
  parentalControl: boolean;
  safeSearch: SafeSearchConfiguration;
}
export interface BlockedServiceCatalogueService {
  id: string;
  name: string;
  groupId?: string;
  supportedNodeIds: string[];
  unsupportedNodeIds: string[];
}
export interface BlockedServiceCatalogueNode {
  nodeId: string;
  nodeName: string;
  version?: string;
  status: "available" | "stale" | "error" | "unsupported";
  serviceCount: number;
  fetchedAt?: string;
  errorCode?: string;
}
export interface BlockedServicesCatalogue {
  services: BlockedServiceCatalogueService[];
  groups: { id: string }[];
  nodes: BlockedServiceCatalogueNode[];
  generatedAt: string;
  stale: boolean;
  partial: boolean;
}
export interface FilterListMetadata {
  id: number;
  url: string;
  name: string;
  enabled: boolean;
  ruleCount: number;
  lastUpdated?: string;
  portable: boolean;
}
export interface FilterListPresentationNode {
  nodeId: string;
  nodeName: string;
  version?: string;
  status: "available" | "stale" | "error" | "unsupported";
  fetchedAt?: string;
  errorCode?: string;
  lists: FilterListMetadata[];
}
export interface FilterListPresentation {
  nodes: FilterListPresentationNode[];
  generatedAt: string;
  stale: boolean;
  partial: boolean;
}
export type BlocklistPresentation = FilterListPresentation;
export type AllowlistPresentation = FilterListPresentation;
export interface QueryLogPolicy {
  enabled: boolean;
  intervalMillis: number;
  anonymizeClientIp: boolean;
  ignored: string[];
  ignoredEnabled: boolean;
}
export interface StatisticsPolicy {
  enabled: boolean;
  intervalMillis: number;
  ignored: string[];
  ignoredEnabled: boolean;
}
export interface DhcpStaticLease {
  mac: string;
  ip: string;
  hostname: string;
}
export interface DhcpConfiguration {
  enabled: boolean;
  interfaceName: string;
  ipv4: {
    gateway: string;
    subnetMask: string;
    rangeStart: string;
    rangeEnd: string;
    leaseDurationSeconds: number;
  };
  ipv6: { rangeStart: string; leaseDurationSeconds: number };
  staticLeases: DhcpStaticLease[];
}
export interface NodeSpecificConfiguration {
  bindHosts: string[];
  dnsPort: number;
  dhcp?: DhcpConfiguration;
}
export interface DhcpLease {
  mac: string;
  ip: string;
  hostname: string;
  expiresAt: string;
}
export interface DhcpInterface {
  name: string;
  hardwareAddress?: string;
  ipv4Addresses: string[];
  ipv6Addresses: string[];
  gatewayIp?: string;
  flags: string[];
  available: boolean;
}
export interface DhcpInterfaces {
  nodeId: string;
  nodeName: string;
  interfaces: DhcpInterface[];
  fetchedAt: string;
}
export type DhcpCheckValueStatus = "yes" | "no" | "error" | "unavailable";
export interface DhcpCheckValue {
  status: DhcpCheckValueStatus;
  message?: string;
}
export interface DhcpActiveCheckResult {
  nodeId: string;
  nodeName: string;
  interfaceName: string;
  status: "none" | "found" | "multiple" | "partial" | "error";
  ipv4: DhcpCheckValue;
  ipv4StaticIp: { status: DhcpCheckValueStatus; ip?: string };
  ipv6: DhcpCheckValue;
  checkedAt: string;
}
export type DhcpOperationCommand =
  | "dhcp_reset_leases"
  | "dhcp_reset_configuration";
export interface DhcpOperationNodeResult {
  id: string;
  nodeId: string;
  nodeName: string;
  status: "running" | "succeeded" | "failed";
  errorCode?: string;
  startedAt: string;
  completedAt?: string;
}
export interface DhcpOperation {
  id: string;
  clusterId: string;
  clusterName: string;
  command: DhcpOperationCommand;
  status: "running" | "succeeded" | "failed";
  requestId: string;
  observationStatus: "not_run" | "succeeded" | "failed";
  observationSnapshotId?: string;
  observationErrorCode?: string;
  auditReference?: string;
  requestedAt: string;
  completedAt?: string;
  nodeResults: DhcpOperationNodeResult[];
  duplicate?: boolean;
}
export interface TlsStatus {
  enabled: boolean;
  serverName: string;
  forceHttps: boolean;
  httpsPort: number;
  dnsOverTlsPort: number;
  dnsOverQuicPort: number;
  servePlainDns: boolean;
  validCertificate: boolean;
  validChain: boolean;
  validKey: boolean;
  validPair: boolean;
  subject?: string;
  issuer?: string;
  notBefore?: string;
  notAfter?: string;
  dnsNames?: string[];
  warning?: string;
}

export interface ConfigurationSnapshot {
  id: string;
  nodeId: string;
  observedAt: string;
  schemaVersion: number;
  document?: ConfigurationDocument;
  canonicalHash?: string;
  nodeVersion?: string;
  collectionStatus: "succeeded" | "failed";
  errorCode?: string;
}

export interface CapabilityProfile {
  nodeId: string;
  productVersion: string;
  compatibility: string;
  schemaVersion: number;
  features: Record<string, boolean>;
  warnings: string[];
  refreshedAt: string;
}

export interface ConfigurationDraft {
  id: string;
  clusterId: string;
  sourceSnapshotId: string;
  schemaVersion: number;
  baseRevisionId?: string;
  document: DesiredConfigurationDocument;
  canonicalHash: string;
  version: number;
  updatedAt: string;
}

export interface ValidationIssue {
  field: string;
  message: string;
}

export interface ConfigurationRevision {
  id: string;
  clusterId: string;
  revisionNumber: number;
  schemaVersion: number;
  document: DesiredConfigurationDocument;
  canonicalHash: string;
  summary: string;
  createdBy: string;
  createdAt: string;
  active: boolean;
  archivedAt?: string;
  archivedBy?: string;
  lifecycle: RecordLifecycle;
}

export interface RecordLifecycle {
  canArchive: boolean;
  canRestore: boolean;
  canDelete: boolean;
}

export interface DeploymentNode {
  id: string;
  deploymentId: string;
  nodeId: string;
  position: number;
  effectiveHash: string;
  status: string;
  attemptCount: number;
  startedAt?: string;
  completedAt?: string;
  errorCode?: string;
  errorMessage?: string;
  verificationSnapshotId?: string;
}

export interface Deployment {
  id: string;
  clusterId: string;
  revisionId: string;
  status: string;
  strategy: "sequential";
  failurePolicy: "stop";
  origin: "manual" | "rollback" | "reconciliation";
  rollbackOfRevisionId?: string;
  requestedBy?: string;
  requestId: string;
  cancelRequested: boolean;
  errorCode?: string;
  requestedAt: string;
  startedAt?: string;
  completedAt?: string;
  archivedAt?: string;
  archivedBy?: string;
  lifecycle: RecordLifecycle;
  nodes: DeploymentNode[];
}

export interface DeploymentPreview {
  revisionId: string;
  strategy: string;
  failurePolicy: string;
  differences: ConfigurationDifference[];
  restartRequired: boolean;
  valid: boolean;
  issues: ValidationIssue[];
  nodes: {
    nodeId: string;
    position: number;
    effectiveHash: string;
    valid: boolean;
    warning?: string;
  }[];
}

export interface DriftEvent {
  id: string;
  clusterId: string;
  nodeId: string;
  desiredRevisionId: string;
  desiredHash: string;
  observedSnapshotId: string;
  observedHash: string;
  fingerprint: string;
  status: "open" | "resolved";
  policy: "enforce" | "alert" | "manual";
  reconciliationStatus: string;
  differences: ConfigurationDifference[];
  detectedAt: string;
  lastSeenAt: string;
  resolvedAt?: string;
  resolution?: string;
  relatedDeploymentId?: string;
}

export interface ConfigurationDifference {
  section: string;
  field: string;
  scope:
    | "shared_managed"
    | "node_specific_managed"
    | "observed_only"
    | "unsupported";
  left: unknown;
  right: unknown;
  summary: string;
}

export interface ApiErrorBody {
  code: string;
  message: string;
  field?: string;
  requestId: string;
}

export type StatisticsRange = "24h" | "7d" | "30d";

export interface StatisticsRanking {
  key: string;
  value: number;
  percentage?: number;
}

export interface StatisticsReport {
  range: StatisticsRange;
  scope: { type: "cluster" | "node"; nodeId?: string };
  state: "ready" | "partial" | "unavailable";
  generatedAt: string;
  freshness: {
    newestAt?: string;
    oldestAt?: string;
    staleAfterSeconds: number;
  };
  coverage: {
    status: "complete" | "partial" | "unavailable";
    expectedNodes: number;
    includedNodes: number;
    missingNodes: number;
    staleNodes: number;
    unsupportedNodes: number;
    maintenanceNodes: number;
  };
  totals: {
    dnsQueries: number;
    blockedFiltering: number;
    blockedPercentage: number;
    replacedSafeBrowsing: number;
    replacedSafeSearch: number;
    replacedParental: number;
    safetyInterventions: number;
    safetyInterventionPercentage: number;
    averageProcessingMs: number;
  };
  series: {
    at: string;
    dnsQueries: number;
    blockedFiltering: number;
    replacedSafeBrowsing: number;
    replacedParental: number;
    includedNodes: number;
  }[];
  rankings: {
    queriedDomains: StatisticsRanking[];
    blockedDomains: StatisticsRanking[];
    clients: StatisticsRanking[];
    upstreamResponses: StatisticsRanking[];
    upstreamAverageLatencyMs: StatisticsRanking[];
  };
  nodes: {
    nodeId: string;
    nodeName: string;
    status: string;
    reasonCode?: string;
    collectedAt?: string;
    dnsQueries?: number;
  }[];
}

export type QueryEventStatus =
  | "allowed"
  | "blocked"
  | "rewritten"
  | "safe_search"
  | "safe_browsing"
  | "parental"
  | "error"
  | "other";

export interface QueryEvent {
  id: string;
  nodeId: string;
  nodeName: string;
  timestamp: string;
  ingestedAt: string;
  query: string;
  queryType: string;
  clientIdentifier: string;
  clientDisplayName?: string;
  clientProtocol?: string;
  status: QueryEventStatus;
  responseCode?: string;
  processingTimeMs: number;
  upstream?: string;
  filteringReason?: string;
  serviceName?: string;
  rules: { text: string; filterListId?: number }[];
  answers: { type: string; value: string; ttl?: number }[];
  cached: boolean;
  answerDnssec: boolean;
}

export interface QueryLogCoverage {
  status: "complete" | "partial" | "unavailable";
  collectionEnabled: boolean;
  retentionSeconds: number;
  expectedNodes: number;
  includedNodes: number;
  staleNodes: number;
  unsupportedNodes: number;
  disabledNodes: number;
  maintenanceNodes: number;
  errorNodes: number;
  gapNodes: number;
  currentThrough?: string;
  staleAfterSeconds: number;
  nodes: {
    nodeId: string;
    nodeName: string;
    status: string;
    reasonCode?: string;
    lastAttemptAt?: string;
    lastSuccessAt?: string;
    currentThrough?: string;
    gapDetected: boolean;
  }[];
}

export interface QueryEventPage {
  items: QueryEvent[];
  nextCursor?: string;
  generatedAt: string;
  coverage: QueryLogCoverage;
  filters: { statuses: QueryEventStatus[]; queryTypes: string[] };
}

export type OperationalHealthState =
  | "healthy"
  | "degraded"
  | "stale"
  | "failed"
  | "paused"
  | "unsupported"
  | "maintenance"
  | "unknown";

export interface OperationalNodeHealth {
  nodeId: string;
  nodeName: string;
  state: OperationalHealthState;
  lastAttemptAt?: string;
  lastSuccessAt?: string;
  nextScheduledAt?: string;
  lagSeconds?: number;
  consecutiveFailures: number;
  errorCode?: string;
  gapDetected?: boolean;
  gapReason?: string;
  recordsReceived?: number;
  capabilityState?: OperationalHealthState;
  capabilityRefreshedAt?: string;
}

export interface OperationalCollectionHealth {
  state: OperationalHealthState;
  expectedNodes: number;
  currentNodes: number;
  staleNodes: number;
  unsupportedNodes: number;
  coveragePercent: number;
  nodes: OperationalNodeHealth[];
}

export interface OperationalStatus {
  generatedAt: string;
  clusterId: string;
  summary: {
    state: OperationalHealthState;
    actionRequired: boolean;
    message: string;
    healthyNodes: number;
    expectedNodes: number;
  };
  api: OperationalHealthState;
  database: {
    state: OperationalHealthState;
    pingLatencyMs: number;
    schemaVersion: number;
    databaseBytes: number;
    poolTotal: number;
    poolAcquired: number;
    poolMax: number;
    errorCode?: string;
    datasets: {
      name: string;
      estimatedRows: number;
      approximateBytes: number;
      retentionSeconds: number;
      oldestRetainedAt?: string;
      newestRetainedAt?: string;
    }[];
  };
  nodes: OperationalNodeHealth[];
  dnsService: OperationalCollectionHealth;
  ha: HASummary;
  observation: OperationalCollectionHealth;
  statistics: OperationalCollectionHealth;
  queryLog: OperationalCollectionHealth;
  workers: {
    name: string;
    state: OperationalHealthState;
    running: boolean;
    lastAttemptAt?: string;
    lastSuccessAt?: string;
    lastFailureAt?: string;
    consecutiveFailures: number;
    nextScheduledAt?: string;
    currentDurationMs?: number;
    errorCode?: string;
    runsTotal: number;
    failuresTotal: number;
  }[];
}

export interface HASummary {
  state: "healthy" | "degraded" | "at_risk";
  totalNodes: number;
  servingDnsNodes: number;
  apiReachableNodes: number;
  convergedNodes: number;
  maintenanceNodes: number;
  certificateWarnings: number;
  updateAvailableNodes: number;
  message: string;
  nodes: {
    nodeId: string;
    dnsStatus:
      | "healthy"
      | "failed"
      | "stale"
      | "maintenance"
      | "disabled"
      | "unknown";
    udpStatus: "healthy" | "failed" | "disabled" | "unknown";
    tcpStatus: "healthy" | "failed" | "disabled" | "unknown";
    dnsProbedAt?: string;
    errorCode?: string;
  }[];
}

export interface DNSProbeResult {
  id: string;
  clusterId: string;
  nodeId: string;
  status: "healthy" | "failed";
  udpStatus: "healthy" | "failed" | "disabled";
  tcpStatus: "healthy" | "failed" | "disabled";
  responseCode?: number;
  latencyMs?: number;
  addressFamily?: "ipv4" | "ipv6";
  errorCode?: string;
  probedAt: string;
}

export interface LifecycleSettings {
  nodeId: string;
  dnsProbeHost: string;
  dnsProbePort: number;
  dnsProbeName: string;
  dnsProbeType: "A" | "AAAA" | "NS";
  expectedRcode: number;
  probeUdp: boolean;
  probeTcp: boolean;
  installationType:
    | "native_systemd"
    | "docker"
    | "home_assistant_addon"
    | "custom"
    | "unknown";
  recordVersion: number;
  createdAt: string;
  updatedAt: string;
}

export interface CertificateHealth {
  nodeId: string;
  nodeName: string;
  subject?: string;
  issuer?: string;
  notAfter?: string;
  daysRemaining?: number;
  state: "healthy" | "warning" | "critical" | "expired" | "unknown";
  observedAt?: string;
}

export interface VersionHealth {
  nodeId: string;
  nodeName: string;
  installedVersion: string;
  latestVersion?: string;
  compatibility: string;
  installationType: LifecycleSettings["installationType"];
  upgradeSupport: "guided" | "unsupported";
  updateAvailable: boolean;
  releaseCheckStale: boolean;
}

export interface HAEvent {
  id: string;
  clusterId: string;
  nodeId?: string;
  eventType: string;
  severity: "info" | "warning" | "critical";
  summary: string;
  details: Record<string, unknown>;
  occurredAt: string;
}

export interface NotificationHistoryOutcome {
  channelId?: string;
  channelName: string;
  status: "delivered" | "failed" | "suppressed" | "pending";
  attemptCount: number;
  errorCode?: string;
  errorSummary?: string;
  httpStatus?: number;
  test: boolean;
  completedAt?: string;
}

export interface HAHistoryItem extends HAEvent {
  kind: "event" | "notification";
  notification?: NotificationHistoryOutcome;
}

export interface HAHistoryPage {
  items: HAHistoryItem[];
  nextCursor?: string;
  hasMore: boolean;
}

export interface MaintenancePreflight {
  nodeId: string;
  allowed: boolean;
  breakGlassRequired: boolean;
  healthyDnsNodesRemaining: number;
  expectedRedundancy: string;
  activeDeployment: boolean;
  openDrift: boolean;
  activeDhcp: boolean;
  checks: LifecycleCheck[];
}

export interface LifecycleCheck {
  name: string;
  status: "pass" | "warning" | "fail" | "not_applicable" | "unknown";
  required: boolean;
  errorCode?: string;
  message: string;
}

export interface NodeLifecycle {
  generatedAt: string;
  settings: LifecycleSettings;
  dns?: DNSProbeResult;
  certificate: CertificateHealth;
  version: VersionHealth;
  events: HAEvent[];
}

export interface UpgradeOperation {
  id: string;
  clusterId: string;
  nodeId: string;
  fromVersion: string;
  targetVersion: string;
  installationType: LifecycleSettings["installationType"];
  mode: "guided";
  status:
    | "planned"
    | "maintenance"
    | "awaiting_operator"
    | "validating"
    | "succeeded"
    | "failed"
    | "cancelled";
  preflight: Record<string, unknown>;
  validation: Record<string, unknown>;
  errorCode?: string;
  errorSummary?: string;
  startedAt: string;
  completedAt?: string;
}

export interface NotificationChannel {
  id: string;
  clusterId: string;
  name: string;
  channelType: "webhook";
  enabled: boolean;
  destinationSet: boolean;
  destinationSummary: string;
  subscribedEvents: string[];
  recordVersion: number;
  createdAt: string;
  updatedAt: string;
}

export interface NotificationTestResult {
  channelId: string;
  success: boolean;
  errorCode?: string;
  testedAt: string;
}

export type OperationalTarget =
  | { scope: "node"; nodeId: string }
  | { scope: "all_compatible_enabled_nodes" };

export interface DNSOperationalCommand {
  id: string;
  clusterId: string;
  clusterName: string;
  command:
    | "test_upstream_dns"
    | "test_host_filtering"
    | "clear_dns_cache"
    | "clear_query_log"
    | "reset_statistics";
  target: OperationalTarget;
  status:
    | "queued"
    | "running"
    | "succeeded"
    | "partial_success"
    | "failed"
    | "interrupted";
  requestId: string;
  auditReference?: string;
  requestedAt: string;
  startedAt?: string;
  completedAt?: string;
  duplicate?: boolean;
  excludedNodes: {
    nodeId: string;
    nodeName: string;
    errorCode: string;
  }[];
  nodeResults: {
    id: string;
    nodeId: string;
    nodeName: string;
    position: number;
    status: "pending" | "running" | "succeeded" | "failed" | "skipped";
    errorCode?: string;
    upstreamResults?: {
      resolverId: string;
      status: "succeeded" | "failed";
      errorCode?: string;
    }[];
    hostFilterResult?: {
      matched: boolean;
      reason?: string;
      rules: { text: string; filterListId: number }[];
      serviceName?: string;
      canonicalName?: string;
      ipAddresses?: string[];
    };
    observationStatus?: "not_run" | "succeeded" | "failed";
    observationSnapshotId?: string;
    observationErrorCode?: string;
  }[];
}
