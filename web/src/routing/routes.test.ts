import { describe, expect, it } from "vitest";
import {
  CANONICAL_PATHS,
  LEGACY_REDIRECTS,
  preserveRouteState,
  resolveRoute,
  routePageWidth,
} from "./routes";

describe("canonical route safety", () => {
  it("resolves every canonical route without a redirect or fallback", () => {
    for (const path of CANONICAL_PATHS) {
      expect(["redirect", "not-found"]).not.toContain(resolveRoute(path).kind);
    }
  });

  it("keeps the complete canonical route table stable", () => {
    const expectedKinds = {
      "/": "dashboard",
      "/statistics": "statistics",
      "/settings/general": "settings",
      "/settings/dns": "settings",
      "/settings/encryption": "settings",
      "/settings/clients": "settings",
      "/settings/dhcp": "settings",
      "/filters/blocklists": "blocklists",
      "/filters/allowlists": "allowlists",
      "/filters/rewrites": "settings",
      "/filters/blocked-services": "blocked-services",
      "/filters/custom-rules": "settings",
      "/query-log": "query-log",
      "/ha/nodes": "nodes",
      "/ha/operations": "ha-operations",
      "/ha/notifications": "notifications",
      "/ha/configuration": "configuration",
      "/ha/revisions": "revisions",
      "/ha/deployments": "deployments",
      "/ha/drift": "drift",
      "/setup-guide": "setup-guide",
      "/system/users": "users",
      "/system/audit": "audit",
      "/system/operational-status": "operational-status",
      "/system/settings": "system-settings",
      "/system/backups": "backups",
      "/system/updates": "updates",
      "/system/about": "about",
    } as const;

    expect(Object.keys(expectedKinds)).toEqual([...CANONICAL_PATHS]);
    for (const [path, kind] of Object.entries(expectedKinds))
      expect(resolveRoute(path).kind).toBe(kind);
    expect(resolveRoute("/ha/deployments")).toEqual({ kind: "deployments" });
    expect(resolveRoute("/ha/drift")).toEqual({ kind: "drift" });
    expect(
      resolveRoute("/ha/nodes/11111111-1111-4111-8111-111111111111"),
    ).toEqual({
      kind: "node-lifecycle",
      nodeId: "11111111-1111-4111-8111-111111111111",
    });
  });

  it("renders an explicit not-found result for unknown paths", () => {
    expect(resolveRoute("/mistyped-dashboard")).toEqual({ kind: "not-found" });
    expect(resolveRoute("/settings/not-real")).toEqual({ kind: "not-found" });
  });

  it("assigns every route family its documented page width", () => {
    const expectedWidths = {
      "/": "wide",
      "/statistics": "wide",
      "/settings/general": "wide",
      "/settings/dns": "wide",
      "/settings/encryption": "wide",
      "/settings/clients": "wide",
      "/settings/dhcp": "wide",
      "/filters/blocklists": "wide",
      "/filters/allowlists": "wide",
      "/filters/rewrites": "wide",
      "/filters/blocked-services": "wide",
      "/filters/custom-rules": "wide",
      "/query-log": "wide",
      "/ha/nodes": "wide",
      "/ha/operations": "wide",
      "/ha/notifications": "wide",
      "/ha/configuration": "wide",
      "/ha/revisions": "wide",
      "/ha/deployments": "wide",
      "/ha/drift": "wide",
      "/setup-guide": "standard",
      "/system/users": "standard",
      "/system/audit": "standard",
      "/system/operational-status": "standard",
      "/system/settings": "standard",
      "/system/backups": "standard",
      "/system/updates": "standard",
      "/system/about": "standard",
    } as const;

    expect(Object.keys(expectedWidths)).toEqual([...CANONICAL_PATHS]);
    for (const [path, width] of Object.entries(expectedWidths)) {
      expect(routePageWidth(resolveRoute(path))).toBe(width);
    }
  });
});

describe("route migration", () => {
  it("resolves General Settings canonically and redirects its broad legacy route", () => {
    expect(resolveRoute("/settings/general")).toMatchObject({
      kind: "settings",
      area: "privacy",
    });
    expect(resolveRoute("/settings/privacy")).toEqual({
      kind: "redirect",
      to: "/settings/general",
    });
  });

  it("redirects every legacy bookmark to its approved canonical route", () => {
    for (const [from, to] of Object.entries(LEGACY_REDIRECTS)) {
      expect(resolveRoute(from)).toEqual({ kind: "redirect", to });
      expect(resolveRoute(to).kind).not.toBe("not-found");
    }
  });

  it("makes revisions canonical and preserves history as a compatibility redirect", () => {
    expect(resolveRoute("/ha/revisions")).toEqual({ kind: "revisions" });
    expect(resolveRoute("/ha/history")).toEqual({
      kind: "redirect",
      to: "/ha/revisions",
    });
    expect(
      preserveRouteState(
        "/ha/revisions",
        "?revisionId=revision-42&source=bookmark",
        "#full-configuration",
      ),
    ).toBe(
      "/ha/revisions?revisionId=revision-42&source=bookmark#full-configuration",
    );
  });

  it("normalises trailing slashes without changing route identity", () => {
    expect(resolveRoute("/ha/configuration/")).toEqual({
      kind: "redirect",
      to: "/ha/configuration",
    });
  });

  it("resolves Blocked Services to its dedicated feature page", () => {
    expect(resolveRoute("/filters/blocked-services")).toEqual({
      kind: "blocked-services",
    });
    expect(resolveRoute("/settings/services")).toEqual({
      kind: "redirect",
      to: "/filters/blocked-services",
    });
  });

  it("resolves DNS Blocklists to its dedicated feature page", () => {
    expect(resolveRoute("/filters/blocklists")).toEqual({
      kind: "blocklists",
    });
  });

  it("resolves DNS Allowlists to its dedicated feature page", () => {
    expect(resolveRoute("/filters/allowlists")).toEqual({
      kind: "allowlists",
    });
  });
});
