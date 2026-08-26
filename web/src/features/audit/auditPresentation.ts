import type { AuditEvent } from "../../lib/types";
import { nodeDetailPath } from "../../routing/routes";

export interface AuditEvidenceField {
  label: string;
  value: string;
}

export interface AuditChangePresentation {
  title: string;
  description: string;
  fields: AuditEvidenceField[];
  fallback: boolean;
}

const actionLabels: Readonly<Record<string, string>> = {
  "auth.login.failed": "Login failed",
  "auth.login.succeeded": "Login succeeded",
  "auth.logout": "User signed out",
  "backup.created": "Backup created",
  "backup.restored": "Backup restored",
  "cluster.created": "Cluster created",
  "cluster.reconciliation_policy_changed":
    "Cluster reconciliation policy changed",
  "cluster.updated": "Cluster updated",
  "configuration.draft_imported": "Configuration imported to draft",
  "configuration.draft_updated": "Configuration draft updated",
  "configuration.revision_archived": "Configuration revision archived",
  "configuration.revision_published": "Configuration revision published",
  "configuration.revision_restored": "Configuration revision restored",
  "deployment.cancelled": "Deployment cancelled",
  "deployment.cancellation_requested": "Deployment cancellation requested",
  "deployment.failed": "Deployment failed",
  "deployment.partially_succeeded": "Deployment partially succeeded",
  "deployment.succeeded": "Deployment completed",
  "drift.detected": "Configuration drift detected",
  "drift.resolved": "Configuration drift resolved",
  "node.connection_tested": "Node connection tested",
  "node.created": "Node added",
  "node.credentials_rotated": "Node credentials rotated",
  "node.lifecycle_settings_changed": "Node lifecycle settings changed",
  "node.maintenance_changed": "Node maintenance state changed",
  "node.removed": "Node removed",
  "node.updated": "Node updated",
  "notification.channel_created": "Notification channel created",
  "notification.channel_disabled": "Notification channel disabled",
  "notification.channel_enabled": "Notification channel enabled",
  "notification.channel_removed": "Notification channel removed",
  "notification.channel_updated": "Notification channel updated",
  "notification.policy_changed": "Notification policy changed",
  "operational_history.cleared": "Operational History cleared",
  "system_settings.updated": "System settings changed",
  "user.created": "User created",
  "user.disabled": "User disabled",
  "user.enabled": "User enabled",
  "user.login_identifier_changed": "User login identifier changed",
  "user.password_changed": "Own password changed",
  "user.password_reset": "User password reset",
  "user.updated": "User updated",
};

const sensitiveKeys = new Set([
  "authorization",
  "certificatebody",
  "certificatepem",
  "clientidentifier",
  "clientidentity",
  "credential",
  "credentialencryptionmaterial",
  "credentialnonce",
  "credentials",
  "csrftoken",
  "customcapem",
  "destination",
  "encryptedcredentials",
  "nodecredentials",
  "password",
  "passwordhash",
  "privatekey",
  "query",
  "querycontents",
  "queryname",
  "rawerror",
  "responsebody",
  "rule",
  "rules",
  "secret",
  "secrets",
  "sessiontoken",
  "token",
  "username",
  "webhooksecret",
]);

export function auditActionLabel(action: string): string {
  return actionLabels[action] ?? titleCase(action.replaceAll(/[._]/g, " "));
}

export function auditResourceLabel(resourceType: string): string {
  const labels: Record<string, string> = {
    authentication: "Authentication",
    cluster: "Cluster",
    configuration_draft: "Configuration draft",
    configuration_revision: "Configuration revision",
    deployment: "Deployment",
    drift_event: "Drift incident",
    node: "Node",
    notification_channel: "Notification channel",
    notification_policy: "Notification policy",
    operational_command: "Operational command",
    operational_history: "Operational History",
    session: "Session",
    system_settings: "System settings",
    upgrade: "Upgrade",
    user: "User",
  };
  return labels[resourceType] ?? titleCase(resourceType.replaceAll("_", " "));
}

export function auditActorLabel(event: AuditEvent): string {
  if (event.actorType === "system") return "System";
  if (event.actorType === "anonymous") return "Anonymous";
  if (event.actorDisplayName) return event.actorDisplayName;
  return event.actorUserId ? `User ${shortID(event.actorUserId)}` : "User";
}

export function auditResourceHref(event: AuditEvent): string | undefined {
  if (!event.resourceId) {
    if (event.resourceType === "system_settings") return "/system/settings";
    if (event.resourceType === "notification_policy")
      return "/ha/notifications";
    return undefined;
  }
  const id = encodeURIComponent(event.resourceId);
  switch (event.resourceType) {
    case "node":
      return nodeDetailPath(event.resourceId);
    case "configuration_revision":
      return `/ha/revisions?revisionId=${id}`;
    case "deployment":
      return `/ha/deployments?deploymentId=${id}`;
    case "drift_event":
      return `/ha/drift?driftId=${id}`;
    case "configuration_draft":
      return "/ha/configuration";
    case "notification_channel":
      return "/ha/notifications";
    case "upgrade":
      return "/ha/operations";
    case "user":
      return "/system/users";
    case "cluster":
      return "/";
    default:
      return undefined;
  }
}

export function auditEventHref(id: string): string {
  return `/system/audit?auditEventId=${encodeURIComponent(id)}`;
}

export function presentAuditChange(event: AuditEvent): AuditChangePresentation {
  const metadata = event.metadata ?? {};
  const known = knownFields(event.action, metadata);
  if (known !== undefined) {
    return {
      title: auditActionLabel(event.action),
      description: known.description,
      fields: known.fields,
      fallback: false,
    };
  }
  return {
    title: auditActionLabel(event.action),
    description:
      "Bounded recorded metadata is shown below. Suspicious fields are redacted defensively.",
    fields: fallbackFields(metadata),
    fallback: true,
  };
}

function knownFields(
  action: string,
  metadata: Record<string, unknown>,
): { description: string; fields: AuditEvidenceField[] } | undefined {
  if (action === "system_settings.updated") {
    const labels: [string, string, (value: unknown) => string][] = [
      ["updateChecksEnabled", "Update checks", enabled],
      ["sessionDurationSeconds", "Session duration", seconds],
      ["nodeHealthIntervalSeconds", "Node health interval", seconds],
      ["nodeRequestTimeoutSeconds", "Node request timeout", seconds],
      ["statisticsPollIntervalSeconds", "Statistics poll interval", seconds],
      ["queryLogCollectionEnabled", "Query Log collection", enabled],
      ["queryLogPollIntervalSeconds", "Query Log poll interval", seconds],
      ["queryLogRetentionSeconds", "Query Log retention", seconds],
      ["logLevel", "Log level", displayValue],
      [
        "operationalHistoryRetentionDays",
        "Operational History retention",
        days,
      ],
    ];
    return {
      description:
        "The resulting controller settings recorded by this action are shown; previous values were not recorded.",
      fields: labels.flatMap(([key, label, format]) =>
        key in metadata ? [{ label, value: format(metadata[key]) }] : [],
      ),
    };
  }
  if (action.startsWith("notification.channel_")) {
    return resultingFields(metadata, [
      ["enabled", "Enabled", enabled],
      ["channelType", "Channel type", displayValue],
      ["destinationSummary", "Safe destination summary", displayValue],
      ["destinationReplaced", "Destination replaced", yesNo],
      ["subscribedCategories", "Subscribed categories", displayValue],
      ["success", "Test succeeded", yesNo],
      ["errorCode", "Error code", displayValue],
    ]);
  }
  if (action === "notification.policy_changed") {
    return resultingFields(metadata, [
      ["enabledEventTypes", "Enabled event types", displayValue],
    ]);
  }
  if (action.startsWith("node.")) {
    return resultingFields(metadata, [
      ["name", "Node", displayValue],
      ["enabled", "Enabled", enabled],
      ["certificatePolicy", "Certificate policy", displayValue],
      ["installationType", "Installation type", displayValue],
      ["outcome", "Outcome", displayValue],
      ["compatibility", "Compatibility", displayValue],
      ["version", "Observed version", displayValue],
      ["errorCode", "Error code", displayValue],
      ["credentialsDestroyed", "Credentials destroyed", yesNo],
    ]);
  }
  if (action === "cluster.reconciliation_policy_changed") {
    return {
      description: "The recorded policy transition is shown.",
      fields: [
        field(
          "Previous policy",
          metadata.previousReconciliationPolicy,
          displayValue,
        ),
        field("New policy", metadata.reconciliationPolicy, displayValue),
      ].filter((item): item is AuditEvidenceField => item !== undefined),
    };
  }
  if (action.startsWith("cluster.")) {
    return resultingFields(metadata, [
      ["name", "Cluster", displayValue],
      ["reconciliationPolicy", "Reconciliation policy", displayValue],
    ]);
  }
  if (action.startsWith("configuration.")) {
    return resultingFields(metadata, [
      ["summary", "Summary", displayValue],
      ["revisionNumber", "Revision number", displayValue],
      ["valid", "Draft valid", yesNo],
      ["sourceSnapshotId", "Source snapshot ID", displayValue],
    ]);
  }
  if (action.startsWith("deployment.")) {
    return resultingFields(metadata, [
      ["revisionId", "Revision ID", displayValue],
      ["targetCount", "Target nodes", displayValue],
      ["mutatedNodes", "Mutated nodes", displayValue],
      ["errorCode", "Error code", displayValue],
      ["safeBoundary", "Stopped at safe boundary", yesNo],
    ]);
  }
  if (action.startsWith("drift.")) {
    return resultingFields(metadata, [
      ["nodeId", "Node ID", displayValue],
      ["revisionId", "Desired revision ID", displayValue],
      ["policy", "Policy", displayValue],
      ["differenceCount", "Differences", displayValue],
      ["resolution", "Resolution", displayValue],
    ]);
  }
  if (action.startsWith("auth.")) {
    return resultingFields(metadata, [
      ["sourceIp", "Source IP", displayValue],
      ["identifierHash", "Identifier hash", displayValue],
      ["outcome", "Outcome", displayValue],
    ]);
  }
  if (action.startsWith("user.")) {
    return resultingFields(metadata, [
      ["role", "Role", displayValue],
      ["enabled", "Enabled", enabled],
      ["loginIdentifierChanged", "Login identifier changed", yesNo],
      ["displayNameChanged", "Display name changed", yesNo],
      ["sessionsRevoked", "Sessions revoked", yesNo],
      ["otherSessionsRevoked", "Other sessions revoked", yesNo],
    ]);
  }
  return undefined;
}

function resultingFields(
  metadata: Record<string, unknown>,
  definitions: [string, string, (value: unknown) => string][],
) {
  return {
    description:
      "The resulting values recorded by this action are shown; unrecorded previous values are not inferred.",
    fields: definitions.flatMap(([key, label, format]) =>
      key in metadata ? [{ label, value: format(metadata[key]) }] : [],
    ),
  };
}

function fallbackFields(metadata: Record<string, unknown>) {
  const fields: AuditEvidenceField[] = [];
  flattenFallback(metadata, "", 0, fields);
  return fields.slice(0, 20);
}

function flattenFallback(
  value: unknown,
  path: string,
  depth: number,
  fields: AuditEvidenceField[],
) {
  if (fields.length >= 20) return;
  if (depth > 3) {
    fields.push({ label: path || "Metadata", value: "[Truncated]" });
    return;
  }
  if (value !== null && typeof value === "object" && !Array.isArray(value)) {
    for (const [key, nested] of Object.entries(value).slice(0, 32)) {
      const label = path ? `${path}.${key}` : key;
      if (sensitiveKey(key)) {
        fields.push({ label, value: "[Redacted]" });
      } else {
        flattenFallback(nested, label, depth + 1, fields);
      }
      if (fields.length >= 20) break;
    }
    return;
  }
  fields.push({
    label: path || "Metadata",
    value: displayValue(value).slice(0, 240),
  });
}

function sensitiveKey(key: string) {
  const normalized = key.toLowerCase().replaceAll(/[^a-z0-9]/g, "");
  if (sensitiveKeys.has(normalized)) return true;
  if (
    normalized === "destinationsummary" ||
    normalized === "destinationreplaced" ||
    normalized.startsWith("querylog")
  )
    return false;
  if (
    [
      "password",
      "passwd",
      "token",
      "secret",
      "credential",
      "authorization",
      "privatekey",
      "certificatebody",
      "certificatepem",
      "customcapem",
      "rawerror",
      "rawnodeerror",
      "responsebody",
      "querycontents",
      "queryname",
      "querytext",
      "clientidentity",
      "clientidentifier",
      "destination",
      "webhookurl",
    ].some((fragment) => normalized.includes(fragment))
  )
    return true;
  return normalized.endsWith("rule") || normalized.endsWith("rules");
}

function field(
  label: string,
  value: unknown,
  format: (value: unknown) => string,
) {
  return value === undefined ? undefined : { label, value: format(value) };
}

function displayValue(value: unknown): string {
  if (value === null || value === undefined || value === "") return "—";
  if (Array.isArray(value))
    return value.slice(0, 16).map(displayValue).join(", ");
  if (typeof value === "object") return "[Structured value]";
  return String(value);
}

function enabled(value: unknown) {
  return value === true
    ? "Enabled"
    : value === false
      ? "Disabled"
      : displayValue(value);
}

function yesNo(value: unknown) {
  return value === true ? "Yes" : value === false ? "No" : displayValue(value);
}

function seconds(value: unknown) {
  return typeof value === "number" ? `${value} seconds` : displayValue(value);
}

function days(value: unknown) {
  return typeof value === "number" ? `${value} days` : displayValue(value);
}

function shortID(value: string) {
  return value.slice(0, 8);
}

function titleCase(value: string) {
  return value.replace(/\b\w/g, (letter) => letter.toUpperCase());
}
