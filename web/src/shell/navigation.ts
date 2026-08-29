import type { IconName } from "../components/Icon";

export interface NavigationLink {
  label: string;
  href: string;
  icon: IconName;
}

export interface NavigationGroup {
  id: string;
  label: string;
  icon: IconName;
  children: readonly NavigationLink[];
}

export const MONITORING_NAVIGATION: readonly NavigationLink[] = [
  { label: "Statistics", href: "/statistics", icon: "statistics" },
  { label: "Query Log", href: "/query-log", icon: "activity" },
  {
    label: "Operational Status",
    href: "/system/operational-status",
    icon: "activity",
  },
];

const SETTINGS_NAVIGATION: readonly NavigationLink[] = [
  { label: "General", href: "/settings/general", icon: "general" },
  { label: "DNS", href: "/settings/dns", icon: "dns" },
  { label: "Encryption", href: "/settings/encryption", icon: "encryption" },
  { label: "Clients", href: "/settings/clients", icon: "clients" },
  { label: "DHCP", href: "/settings/dhcp", icon: "dhcp" },
];

const FILTERS_NAVIGATION: readonly NavigationLink[] = [
  { label: "DNS Blocklists", href: "/filters/blocklists", icon: "block" },
  { label: "DNS Allowlists", href: "/filters/allowlists", icon: "dns" },
  { label: "DNS Rewrites", href: "/filters/rewrites", icon: "rewrites" },
  {
    label: "Blocked Services",
    href: "/filters/blocked-services",
    icon: "block",
  },
  {
    label: "Custom Filter Rules",
    href: "/filters/custom-rules",
    icon: "filters",
  },
];

export const HA_NAVIGATION: readonly NavigationLink[] = [
  { label: "Nodes", href: "/ha/nodes", icon: "nodes" },
  { label: "HA Operations", href: "/ha/operations", icon: "ha" },
  { label: "Notifications", href: "/ha/notifications", icon: "notifications" },
  {
    label: "Configuration Control",
    href: "/ha/configuration",
    icon: "settings",
  },
  { label: "Revisions", href: "/ha/revisions", icon: "revisions" },
  { label: "Deployments", href: "/ha/deployments", icon: "deployments" },
  { label: "Drift", href: "/ha/drift", icon: "drift" },
];

export const ADMINISTRATION_NAVIGATION: readonly NavigationLink[] = [
  { label: "Users", href: "/system/users", icon: "users" },
  { label: "Audit Log", href: "/system/audit", icon: "audit" },
  { label: "System Settings", href: "/system/settings", icon: "system" },
  { label: "Backups", href: "/system/backups", icon: "backups" },
  { label: "Updates", href: "/system/updates", icon: "updates" },
  { label: "About", href: "/system/about", icon: "help" },
];

export const PRIMARY_NAVIGATION: readonly (NavigationLink | NavigationGroup)[] =
  [
    { label: "Dashboard", href: "/", icon: "dashboard" },
    {
      id: "monitoring",
      label: "Monitoring",
      icon: "activity",
      children: MONITORING_NAVIGATION,
    },
    {
      id: "settings",
      label: "Settings",
      icon: "settings",
      children: SETTINGS_NAVIGATION,
    },
    {
      id: "filters",
      label: "Filters",
      icon: "filters",
      children: FILTERS_NAVIGATION,
    },
    {
      id: "ha-controller",
      label: "HA Controller",
      icon: "ha",
      children: HA_NAVIGATION,
    },
    {
      id: "administration",
      label: "Administration",
      icon: "admin",
      children: ADMINISTRATION_NAVIGATION,
    },
  ];

export const UTILITY_NAVIGATION: readonly NavigationLink[] = [
  { label: "Setup Guide", href: "/setup-guide", icon: "help" },
];

export function isNavigationGroup(
  item: NavigationLink | NavigationGroup,
): item is NavigationGroup {
  return "children" in item;
}

export function isLinkActive(link: NavigationLink, pathname: string): boolean {
  return (
    link.href === pathname ||
    (link.href === "/ha/nodes" && pathname.startsWith("/ha/nodes/"))
  );
}

export function isGroupActive(
  group: NavigationGroup,
  pathname: string,
): boolean {
  return group.children.some((child) => isLinkActive(child, pathname));
}

export function groupForPath(pathname: string): NavigationGroup | undefined {
  return PRIMARY_NAVIGATION.find(
    (item): item is NavigationGroup =>
      isNavigationGroup(item) && isGroupActive(item, pathname),
  );
}
