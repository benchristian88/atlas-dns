export type SettingsArea =
  | "dns"
  | "filters"
  | "clients"
  | "rewrites"
  | "privacy"
  | "dhcp"
  | "infrastructure";

export type RouteResolution =
  | { kind: "dashboard" }
  | { kind: "nodes" }
  | { kind: "ha-operations" }
  | { kind: "notifications" }
  | { kind: "node-lifecycle"; nodeId: string }
  | { kind: "statistics" }
  | { kind: "query-log" }
  | { kind: "configuration" }
  | { kind: "deployments" }
  | { kind: "drift" }
  | { kind: "revisions" }
  | { kind: "blocked-services" }
  | { kind: "blocklists" }
  | { kind: "allowlists" }
  | {
      kind: "settings";
      area: SettingsArea;
      heading: readonly [title: string, description: string];
    }
  | { kind: "audit" }
  | { kind: "operational-status" }
  | { kind: "users" }
  | { kind: "backups" }
  | { kind: "updates" }
  | { kind: "setup-guide" }
  | { kind: "system-settings" }
  | { kind: "about" }
  | { kind: "redirect"; to: string }
  | { kind: "not-found" };

export type RoutePageWidth = "narrow" | "standard" | "wide" | "full";

export const CANONICAL_PATHS = [
  "/",
  "/statistics",
  "/settings/general",
  "/settings/dns",
  "/settings/encryption",
  "/settings/clients",
  "/settings/dhcp",
  "/filters/blocklists",
  "/filters/allowlists",
  "/filters/rewrites",
  "/filters/blocked-services",
  "/filters/custom-rules",
  "/query-log",
  "/ha/nodes",
  "/ha/operations",
  "/ha/notifications",
  "/ha/configuration",
  "/ha/revisions",
  "/ha/deployments",
  "/ha/drift",
  "/setup-guide",
  "/system/users",
  "/system/audit",
  "/system/operational-status",
  "/system/settings",
  "/system/backups",
  "/system/updates",
  "/system/about",
] as const;

export const LEGACY_REDIRECTS: Readonly<Record<string, string>> = {
  "/settings/filters": "/filters/blocklists",
  "/settings/rewrites": "/filters/rewrites",
  "/settings/services": "/filters/blocked-services",
  "/settings/privacy": "/settings/general",
  "/settings/infrastructure": "/settings/encryption",
  "/ha/history": "/ha/revisions",
};

const settingsRoutes: Readonly<
  Record<string, { area: SettingsArea; heading: readonly [string, string] }>
> = {
  "/settings/general": {
    area: "privacy",
    heading: [
      "General settings",
      "Safety services, Safe Search, query-log, and statistics policy managed across the cluster.",
    ],
  },
  "/settings/dns": {
    area: "dns",
    heading: [
      "DNS settings",
      "Shared resolver, cache, blocking, and privacy behavior.",
    ],
  },
  "/settings/encryption": {
    area: "infrastructure",
    heading: [
      "Encryption",
      "Redacted, node-attributed TLS inventory. Certificate secrets remain outside desired state.",
    ],
  },
  "/settings/clients": {
    area: "clients",
    heading: [
      "Persistent clients",
      "Client identities and per-client filtering policy shared by every node.",
    ],
  },
  "/settings/dhcp": {
    area: "dhcp",
    heading: [
      "DHCP",
      "Guarded node-specific DHCP configuration with a single active node.",
    ],
  },
  "/filters/rewrites": {
    area: "rewrites",
    heading: [
      "DNS rewrites",
      "Cluster-wide domain answers managed as an unordered set.",
    ],
  },
  "/filters/custom-rules": {
    area: "filters",
    heading: [
      "Custom filter rules",
      "Ordered cluster-wide custom filtering rules.",
    ],
  },
};

export function resolveRoute(pathname: string): RouteResolution {
  if (pathname.length > 1 && pathname.endsWith("/")) {
    return { kind: "redirect", to: pathname.replace(/\/+$/, "") };
  }

  const redirect = LEGACY_REDIRECTS[pathname];
  if (redirect !== undefined) return { kind: "redirect", to: redirect };

  const settings = settingsRoutes[pathname];
  if (settings !== undefined) return { kind: "settings", ...settings };

  const nodeLifecycle = pathname.match(/^\/ha\/nodes\/([0-9a-f-]{36})$/i);
  if (nodeLifecycle?.[1] !== undefined)
    return { kind: "node-lifecycle", nodeId: nodeLifecycle[1] };

  switch (pathname) {
    case "/":
      return { kind: "dashboard" };
    case "/statistics":
      return { kind: "statistics" };
    case "/query-log":
      return { kind: "query-log" };
    case "/ha/nodes":
      return { kind: "nodes" };
    case "/ha/operations":
      return { kind: "ha-operations" };
    case "/ha/notifications":
      return { kind: "notifications" };
    case "/ha/configuration":
      return { kind: "configuration" };
    case "/ha/deployments":
      return { kind: "deployments" };
    case "/ha/drift":
      return { kind: "drift" };
    case "/ha/revisions":
      return { kind: "revisions" };
    case "/filters/blocklists":
      return { kind: "blocklists" };
    case "/filters/allowlists":
      return { kind: "allowlists" };
    case "/filters/blocked-services":
      return { kind: "blocked-services" };
    case "/system/audit":
      return { kind: "audit" };
    case "/system/operational-status":
      return { kind: "operational-status" };
    case "/system/users":
      return { kind: "users" };
    case "/system/backups":
      return { kind: "backups" };
    case "/system/updates":
      return { kind: "updates" };
    case "/setup-guide":
      return { kind: "setup-guide" };
    case "/system/settings":
      return { kind: "system-settings" };
    case "/system/about":
      return { kind: "about" };
    default:
      return { kind: "not-found" };
  }
}

export function routePageWidth(route: RouteResolution): RoutePageWidth {
  switch (route.kind) {
    case "audit":
    case "operational-status":
    case "setup-guide":
    case "users":
    case "backups":
    case "updates":
    case "system-settings":
    case "about":
      return "standard";
    case "not-found":
    case "redirect":
      return "narrow";
    default:
      return "wide";
  }
}

export function preserveRouteState(
  pathname: string,
  search: string,
  hash: string,
): string {
  return `${pathname}${search}${hash}`;
}
