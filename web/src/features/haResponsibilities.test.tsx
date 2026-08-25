// @vitest-environment jsdom

import {
  cleanup,
  fireEvent,
  render,
  screen,
  waitFor,
} from "@testing-library/react";
import axe from "axe-core";
import { afterEach, describe, expect, it, vi } from "vitest";
import { api } from "../lib/api";
import type { Cluster, Node } from "../lib/types";
import { ConfigurationPage } from "./configuration/ConfigurationPage";
import { DeploymentsPage } from "./deployments/DeploymentsPage";
import { DriftPage } from "./drift/DriftPage";
import { RevisionsPage } from "./history/HistoryPage";
import { NodesPage } from "./nodes/NodesPage";

const cluster: Cluster = {
  id: "11111111-1111-4111-8111-111111111111",
  name: "Home",
  description: "",
  version: 1,
  reconciliationPolicy: "manual",
  createdAt: "2026-08-03T00:00:00Z",
  updatedAt: "2026-08-03T00:00:00Z",
};

const node: Node = {
  id: "22222222-2222-4222-8222-222222222222",
  clusterId: cluster.id,
  name: "Primary DNS",
  baseUrl: "https://dns.example.test",
  certificatePolicy: "system",
  enabled: true,
  healthStatus: "healthy",
  compatibilityStatus: "supported",
  version: "v0.107.78",
  lastPolledAt: "2026-08-03T00:00:00Z",
  latencyMs: 12,
  maintenanceMode: false,
  appliedRevisionId: "33333333-3333-4333-8333-333333333333",
  convergenceStatus: "drifted",
  recordVersion: 1,
  createdAt: "2026-08-03T00:00:00Z",
  updatedAt: "2026-08-03T00:00:00Z",
};

afterEach(() => {
  cleanup();
  vi.restoreAllMocks();
});

function mockNodes() {
  return vi.spyOn(api, "nodes").mockResolvedValue({
    items: [],
    refreshedAt: "2026-08-03T00:00:00Z",
    staleAfterSeconds: 60,
  });
}

function mockInventory() {
  return vi.spyOn(api, "configurationInventory").mockResolvedValue({
    schemaVersion: 2,
    snapshots: [],
    capabilities: [],
  });
}

describe("HA Controller page responsibilities", () => {
  it("keeps node health, compatibility, observation, latency, revision, and drift on Nodes", async () => {
    vi.spyOn(api, "nodes").mockResolvedValue({
      items: [node],
      refreshedAt: "2026-08-03T00:00:00Z",
      staleAfterSeconds: 60,
    });
    vi.spyOn(api, "configurationInventory").mockResolvedValue({
      schemaVersion: 2,
      snapshots: [
        {
          id: "44444444-4444-4444-8444-444444444444",
          nodeId: node.id,
          observedAt: "2026-08-03T00:00:00Z",
          schemaVersion: 2,
          collectionStatus: "succeeded",
        },
      ],
      capabilities: [
        {
          nodeId: node.id,
          productVersion: "v0.107.78",
          compatibility: "supported",
          schemaVersion: 2,
          features: { dns: true, filtering: true },
          warnings: [],
          refreshedAt: "2026-08-03T00:00:00Z",
        },
      ],
    });
    vi.spyOn(api, "configurationRevisions").mockResolvedValue({
      items: [
        {
          id: node.appliedRevisionId ?? "",
          clusterId: cluster.id,
          revisionNumber: 27,
          schemaVersion: 2,
          document: {} as never,
          canonicalHash: "hash",
          summary: "Current policy",
          createdBy: "user",
          createdAt: "2026-08-03T00:00:00Z",
          active: true,
          lifecycle: { canArchive: false, canRestore: false, canDelete: false },
        },
      ],
    });
    vi.spyOn(api, "driftEvents").mockResolvedValue({ items: [] });

    render(<NodesPage cluster={cluster} />);

    expect(await screen.findByText("Primary DNS")).toBeTruthy();
    expect(screen.getByText("12 ms")).toBeTruthy();
    expect(screen.getByText("#27")).toBeTruthy();
    expect(screen.getByText("supported · 2 features")).toBeTruthy();
    expect(
      screen.getByRole("link", { name: "Drifted" }).getAttribute("href"),
    ).toBe("/ha/drift");
  });

  it("keeps Configuration Control forward-looking and moves adoption under Advanced", async () => {
    mockNodes();
    mockInventory();
    vi.spyOn(api, "configurationRevisions").mockResolvedValue({ items: [] });

    render(<ConfigurationPage cluster={cluster} />);

    expect(
      await screen.findByRole("heading", { name: "Configuration Control" }),
    ).toBeTruthy();
    expect(screen.getByText("Advanced configuration adoption")).toBeTruthy();
    expect(
      screen.queryByRole("heading", { name: "Revision history" }),
    ).toBeNull();
    expect(screen.queryByRole("button", { name: "Rollback" })).toBeNull();
  });

  it("keeps Revisions backward-looking", async () => {
    vi.spyOn(api, "configurationRevisions").mockResolvedValue({ items: [] });
    vi.spyOn(api, "deployments").mockResolvedValue({ items: [] });

    render(<RevisionsPage cluster={cluster} />);

    expect(
      await screen.findByRole("heading", { name: "Configuration Revisions" }),
    ).toBeTruthy();
    expect(
      screen.getByRole("heading", { name: "Published revisions" }),
    ).toBeTruthy();
    expect(screen.queryByRole("button", { name: /Publish/ })).toBeNull();
    expect(screen.queryByText("Import into draft")).toBeNull();
  });

  it("loads deployment execution without loading drift incidents", async () => {
    vi.spyOn(api, "deployments").mockResolvedValue({ items: [] });
    vi.spyOn(api, "configurationRevisions").mockResolvedValue({ items: [] });
    mockNodes();
    const drift = vi.spyOn(api, "driftEvents");

    render(<DeploymentsPage cluster={cluster} />);

    expect(
      await screen.findByRole("heading", { name: "Deployments" }),
    ).toBeTruthy();
    expect(
      screen.getByRole("heading", { name: "All deployments" }),
    ).toBeTruthy();
    expect(
      screen.queryByRole("heading", { name: "Drift incidents" }),
    ).toBeNull();
    expect(drift).not.toHaveBeenCalled();
  });

  it("loads current drift without loading deployment history", async () => {
    vi.spyOn(api, "driftEvents").mockResolvedValue({ items: [] });
    mockNodes();
    mockInventory();
    const deployments = vi.spyOn(api, "deployments");

    render(<DriftPage cluster={cluster} />);

    expect(await screen.findByRole("heading", { name: "Drift" })).toBeTruthy();
    expect(
      screen.getByRole("heading", { name: "Drift incidents" }),
    ).toBeTruthy();
    expect(
      screen.queryByRole("heading", { name: "Deployment history" }),
    ).toBeNull();
    expect(deployments).not.toHaveBeenCalled();
    await waitFor(() =>
      expect(
        screen.getByText("Cluster has no open drift incidents"),
      ).toBeTruthy(),
    );
  });

  it("links drift evidence to exact Node Detail without owning maintenance", async () => {
    vi.spyOn(api, "driftEvents").mockResolvedValue({
      items: [
        {
          id: "44444444-4444-4444-8444-444444444444",
          clusterId: cluster.id,
          nodeId: node.id,
          desiredRevisionId: node.appliedRevisionId ?? "",
          desiredHash: "desired-hash",
          observedSnapshotId: "55555555-5555-4555-8555-555555555555",
          observedHash: "observed-hash",
          fingerprint: "fingerprint",
          status: "open",
          policy: "manual",
          reconciliationStatus: "pending",
          differences: [],
          detectedAt: "2026-08-03T00:00:00Z",
          lastSeenAt: "2026-08-03T00:05:00Z",
        },
      ],
    });
    vi.spyOn(api, "nodes").mockResolvedValue({
      items: [node],
      refreshedAt: "2026-08-03T00:00:00Z",
      staleAfterSeconds: 60,
    });
    mockInventory();
    const preflight = vi.spyOn(api, "maintenancePreflight");
    const enter = vi.spyOn(api, "enterMaintenance");
    const leave = vi.spyOn(api, "returnToService");

    render(<DriftPage cluster={cluster} />);
    fireEvent.click(
      await screen.findByRole("button", {
        name: "View drift incident details for Primary DNS",
      }),
    );
    expect(
      screen.getByRole("link", { name: "View node" }).getAttribute("href"),
    ).toBe(`/ha/nodes/${node.id}`);
    expect(screen.queryByRole("button", { name: /maintenance/i })).toBeNull();
    expect(preflight).not.toHaveBeenCalled();
    expect(enter).not.toHaveBeenCalled();
    expect(leave).not.toHaveBeenCalled();
  });

  it("keeps the separated HA task pages structurally accessible", async () => {
    mockNodes();
    mockInventory();
    vi.spyOn(api, "configurationRevisions").mockResolvedValue({ items: [] });
    vi.spyOn(api, "deployments").mockResolvedValue({ items: [] });
    vi.spyOn(api, "driftEvents").mockResolvedValue({ items: [] });

    for (const page of [
      <ConfigurationPage key="configuration" cluster={cluster} />,
      <RevisionsPage key="revisions" cluster={cluster} />,
      <DeploymentsPage key="deployments" cluster={cluster} />,
      <DriftPage key="drift" cluster={cluster} />,
    ]) {
      const { container, unmount } = render(page);
      await screen.findByRole("heading", { level: 1 });
      const result = await axe.run(container, {
        runOnly: { type: "tag", values: ["wcag2a", "wcag2aa", "wcag21aa"] },
        rules: { "color-contrast": { enabled: false } },
      });
      expect(result.violations).toEqual([]);
      unmount();
    }
  });
});
