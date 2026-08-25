import type { ReactNode } from "react";
import type { HealthStatus } from "../lib/types";

export type StatusKind =
  | HealthStatus
  | "empty"
  | "stale"
  | "degraded"
  | "converged"
  | "drifted"
  | "pending"
  | "applying"
  | "verifying"
  | "failed"
  | "maintenance"
  | "paused"
  | "not_applicable"
  | "observed"
  | "unsupported"
  | "success"
  | "warning"
  | "info";

const labels: Record<StatusKind, string> = {
  healthy: "Healthy",
  unreachable: "Unreachable",
  incompatible: "Incompatible",
  disabled: "Disabled",
  unknown: "Unknown",
  empty: "No nodes",
  stale: "Stale",
  degraded: "Degraded",
  converged: "Converged",
  drifted: "Drifted",
  pending: "Pending",
  applying: "Applying",
  verifying: "Verifying",
  failed: "Failed",
  maintenance: "Maintenance",
  paused: "Paused",
  not_applicable: "Not Applicable",
  observed: "Observed Only",
  unsupported: "Unsupported",
  success: "Success",
  warning: "Warning",
  info: "Information",
};

export function StatusBadge({
  status,
  label,
  icon,
}: {
  status: StatusKind;
  label?: ReactNode;
  icon?: ReactNode;
}) {
  return (
    <span className={`status status--${status}`} data-size="compact">
      {icon !== undefined && <span aria-hidden="true">{icon}</span>}
      {label ?? labels[status]}
    </span>
  );
}
