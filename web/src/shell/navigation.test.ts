import { describe, expect, it } from "vitest";
import { resolveRoute } from "../routing/routes";
import {
  ADMINISTRATION_NAVIGATION,
  HA_NAVIGATION,
  isGroupActive,
  isNavigationGroup,
  MONITORING_NAVIGATION,
  PRIMARY_NAVIGATION,
  UTILITY_NAVIGATION,
} from "./navigation";

describe("application navigation", () => {
  it("uses only canonical, explicitly handled routes", () => {
    const primaryLinks = PRIMARY_NAVIGATION.flatMap((item) =>
      isNavigationGroup(item) ? item.children : [item],
    );
    for (const item of [...primaryLinks, ...UTILITY_NAVIGATION]) {
      expect(["redirect", "not-found"]).not.toContain(
        resolveRoute(item.href).kind,
      );
    }
  });

  it("highlights a parent when any active child is selected", () => {
    const cases = [
      ["Settings", "/settings/dns"],
      ["Filters", "/filters/rewrites"],
      ["HA Controller", "/ha/drift"],
      ["Monitoring", "/system/operational-status"],
      ["Administration", "/system/audit"],
    ] as const;
    for (const [label, pathname] of cases) {
      const group = PRIMARY_NAVIGATION.find((item) => item.label === label);
      expect(group && isNavigationGroup(group)).toBe(true);
      if (!group || !isNavigationGroup(group)) continue;
      expect(isGroupActive(group, pathname)).toBe(true);
      expect(isGroupActive(group, "/")).toBe(false);
    }
  });

  it("orders the HA lifecycle consistently with Revisions terminology", () => {
    const group = PRIMARY_NAVIGATION.find(
      (item) => item.label === "HA Controller",
    );
    expect(group && isNavigationGroup(group)).toBe(true);
    if (!group || !isNavigationGroup(group)) return;
    expect(group.children.map(({ label, href }) => ({ label, href }))).toEqual([
      { label: "Nodes", href: "/ha/nodes" },
      { label: "HA Operations", href: "/ha/operations" },
      { label: "Notifications", href: "/ha/notifications" },
      { label: "Configuration Control", href: "/ha/configuration" },
      { label: "Revisions", href: "/ha/revisions" },
      { label: "Deployments", href: "/ha/deployments" },
      { label: "Drift", href: "/ha/drift" },
    ]);
  });

  it("keeps observation and controller administration in their v1.1 owners", () => {
    expect(MONITORING_NAVIGATION.map((item) => item.label)).toEqual([
      "Statistics",
      "Query Log",
      "Operational Status",
    ]);
    expect(HA_NAVIGATION.some((item) => item.label === "Notifications")).toBe(
      true,
    );
    expect(ADMINISTRATION_NAVIGATION.map((item) => item.label)).toEqual([
      "Users",
      "Audit Log",
      "System Settings",
      "Backups",
      "Updates",
      "About",
    ]);
    expect(UTILITY_NAVIGATION.map((item) => item.label)).toEqual([
      "Setup Guide",
    ]);
    expect(
      [...PRIMARY_NAVIGATION, ...UTILITY_NAVIGATION].some(
        (item) => item.label === "Integrations",
      ),
    ).toBe(false);
  });
});
