// @vitest-environment jsdom

import { cleanup, render, screen, within } from "@testing-library/react";
import axe from "axe-core";
import { afterEach, describe, expect, it, vi } from "vitest";
import { api } from "../../lib/api";
import type {
  Cluster,
  HASummary,
  Node,
  OperationalStatus,
  StatisticsReport,
} from "../../lib/types";
import { ScopeProvider } from "../../shell/ScopeContext";
import { DashboardPage } from "./DashboardPage";

afterEach(() => {
  cleanup();
  vi.restoreAllMocks();
});

const cluster = {
  id: "11111111-1111-4111-8111-111111111111",
  name: "Home",
} as Cluster;
const primaryNodeID = "22222222-2222-4222-8222-222222222222";
const secondaryNodeID = "33333333-3333-4333-8333-333333333333";
const nodes = [
  {
    id: primaryNodeID,
    name: "Primary",
    baseUrl: "https://primary.example.test",
    healthStatus: "healthy",
    compatibilityStatus: "supported",
    version: "v0.107.79",
    lastSeenAt: "2099-08-11T00:00:00Z",
    lastPolledAt: "2099-08-11T00:00:00Z",
  },
  {
    id: secondaryNodeID,
    name: "Secondary",
    baseUrl: "https://secondary.example.test",
    healthStatus: "unreachable",
    compatibilityStatus: "supported",
    version: "v0.107.78",
    lastSeenAt: "2099-08-11T00:00:00Z",
    lastPolledAt: "2099-08-11T00:00:00Z",
  },
] as Node[];

const operational = {
  summary: {
    state: "degraded",
    actionRequired: true,
    message: "One or more controller subsystems require attention.",
    healthyNodes: 1,
    expectedNodes: 2,
  },
  api: "healthy",
  ha: { state: "degraded", servingDnsNodes: 1, totalNodes: 2 },
  statistics: { state: "stale", currentNodes: 1, expectedNodes: 2 },
  queryLog: { state: "healthy", currentNodes: 2, expectedNodes: 2 },
} as OperationalStatus;

const ha = {
  state: "degraded",
  totalNodes: 2,
  servingDnsNodes: 1,
  apiReachableNodes: 1,
  convergedNodes: 2,
  maintenanceNodes: 0,
  certificateWarnings: 1,
  updateAvailableNodes: 1,
  message: "Redundancy is reduced.",
  nodes: [
    { nodeId: primaryNodeID, dnsStatus: "healthy" },
    { nodeId: secondaryNodeID, dnsStatus: "failed" },
  ],
} as HASummary;

const statistics = {
  range: "24h",
  state: "partial",
  scope: { type: "cluster" },
  generatedAt: "2026-08-24T07:00:00Z",
  freshness: {
    newestAt: "2026-08-24T07:00:00Z",
    oldestAt: "2026-08-23T07:00:00Z",
    staleAfterSeconds: 10800,
  },
  coverage: {
    status: "partial",
    expectedNodes: 2,
    includedNodes: 1,
    missingNodes: 1,
    staleNodes: 0,
    unsupportedNodes: 0,
    maintenanceNodes: 0,
  },
  totals: {
    dnsQueries: 1200,
    blockedFiltering: 150,
    blockedPercentage: 12.5,
    replacedSafeBrowsing: 2,
    replacedSafeSearch: 5,
    replacedParental: 10,
    safetyInterventions: 17,
    safetyInterventionPercentage: 1.42,
    averageProcessingMs: 16.25,
  },
  series: [
    {
      at: "2026-08-24T06:00:00Z",
      dnsQueries: 500,
      blockedFiltering: 50,
      replacedSafeBrowsing: 0,
      replacedParental: 0,
      includedNodes: 1,
    },
    {
      at: "2026-08-24T07:00:00Z",
      dnsQueries: 700,
      blockedFiltering: 100,
      replacedSafeBrowsing: 2,
      replacedParental: 10,
      includedNodes: 1,
    },
  ],
  rankings: {
    queriedDomains: [{ key: "example.com", value: 700 }],
    blockedDomains: [{ key: "ads.example", value: 100 }],
    clients: [],
    upstreamResponses: [],
    upstreamAverageLatencyMs: [],
  },
  nodes: [],
} as StatisticsReport;

function mockSources(
  options: { nodeItems?: Node[]; statisticsReport?: StatisticsReport } = {},
) {
  vi.spyOn(api, "nodes").mockResolvedValue({
    items: options.nodeItems ?? nodes,
    refreshedAt: "2026-08-24T07:00:00Z",
    staleAfterSeconds: 90,
  });
  vi.spyOn(api, "operationalStatus").mockResolvedValue(operational);
  vi.spyOn(api, "statistics").mockResolvedValue(
    options.statisticsReport ?? statistics,
  );
  vi.spyOn(api, "haStatus").mockResolvedValue(ha);
  vi.spyOn(api, "versions").mockResolvedValue({
    items: [
      {
        nodeId: primaryNodeID,
        updateAvailable: false,
        releaseCheckStale: false,
      },
      {
        nodeId: secondaryNodeID,
        updateAvailable: true,
        releaseCheckStale: false,
      },
    ] as never[],
  });
  vi.spyOn(api, "configurationRevisions").mockResolvedValue({
    items: [
      {
        id: "revision-1",
        clusterId: cluster.id,
        revisionNumber: 7,
        summary: "Blocking policy updated",
        createdAt: "2026-08-24T06:30:00Z",
        active: true,
      },
    ] as never[],
  });
  vi.spyOn(api, "deployments").mockResolvedValue({ items: [] });
  vi.spyOn(api, "driftEvents").mockResolvedValue({ items: [] });
  vi.spyOn(api, "auditEvents").mockResolvedValue({
    items: [
      {
        id: "audit-1",
        action: "node.updated",
        resourceType: "node",
        actorType: "user",
        createdAt: "2026-08-24T06:00:00Z",
      },
    ] as never[],
  });
}

function renderDashboard(nodeItems = nodes, nodeId = "") {
  return render(
    <ScopeProvider value={{ nodeId, nodes: nodeItems }}>
      <DashboardPage cluster={cluster} />
    </ScopeProvider>,
  );
}

describe("DashboardPage", () => {
  it("renders real health, activity, attention, changes, nodes, and domain rankings", async () => {
    mockSources();
    const { container } = renderDashboard();
    expect(
      await screen.findByRole("heading", { name: "DNS activity" }),
    ).toBeTruthy();
    const health = screen.getByRole("region", { name: "Primary health" });
    expect(health.querySelectorAll(".dashboard-health-card")).toHaveLength(5);
    expect(within(health).getByText("DNS Serving")).toBeTruthy();
    expect(within(health).getByText("API Reachable")).toBeTruthy();
    expect(within(health).getByText("HA Status")).toBeTruthy();
    expect(within(health).getByText("Collection")).toBeTruthy();
    expect(within(health).getByText("Attention")).toBeTruthy();
    expect(screen.getByText("1,200")).toBeTruthy();
    expect(screen.getByText("12.5%")).toBeTruthy();
    expect(
      screen.getByRole("heading", { name: "Recent changes" }),
    ).toBeTruthy();
    expect(screen.getByText("Blocking policy updated")).toBeTruthy();
    expect(screen.getByRole("heading", { name: "Nodes (2)" })).toBeTruthy();
    expect(screen.getByText("example.com")).toBeTruthy();
    expect(screen.getByText("ads.example")).toBeTruthy();
    expect(screen.getByText(/Cluster: Home/)).toBeTruthy();
    expect(screen.getAllByText("Traffic scope: Entire Cluster").length).toBe(2);
    expect(api.auditEvents).toHaveBeenCalledWith({
      clusterId: cluster.id,
      includeController: true,
      limit: 50,
    });
    expect(
      container.querySelectorAll(".dashboard-node-table tbody tr"),
    ).toHaveLength(2);
    const accessibility = await axe.run(container, {
      runOnly: { type: "tag", values: ["wcag2a", "wcag2aa", "wcag21aa"] },
      rules: { "color-contrast": { enabled: false } },
    });
    expect(accessibility.violations).toEqual([]);
  });

  it("does not create certificate attention when canonical warning count is zero", async () => {
    mockSources();
    vi.mocked(api.haStatus).mockResolvedValue({
      ...ha,
      certificateWarnings: 0,
    });
    renderDashboard();
    expect(
      await screen.findByRole("heading", { name: "Attention" }),
    ).toBeTruthy();
    expect(screen.queryByText("Certificate expiry needs review.")).toBeNull();
  });

  it("keeps partial dashboard content visible when supplementary APIs fail", async () => {
    vi.spyOn(api, "nodes").mockResolvedValue({
      items: nodes,
      refreshedAt: "2026-08-24T07:00:00Z",
      staleAfterSeconds: 90,
    });
    for (const name of [
      "operationalStatus",
      "statistics",
      "haStatus",
      "versions",
      "configurationRevisions",
      "deployments",
      "driftEvents",
      "auditEvents",
    ] as const)
      vi.spyOn(api, name).mockRejectedValue(new Error(`${name} unavailable`));
    renderDashboard();
    expect(
      await screen.findByText(
        "Some dashboard sources are unavailable. Available operational data remains visible.",
      ),
    ).toBeTruthy();
    expect(screen.getByRole("heading", { name: "DNS activity" })).toBeTruthy();
    expect(
      screen.getByText(
        "No usable 24-hour Statistics snapshot is available for this scope.",
      ),
    ).toBeTruthy();
    expect(screen.getByRole("heading", { name: "Nodes (2)" })).toBeTruthy();
    expect(screen.getByText(/Recent changes is partial:/)).toBeTruthy();
    expect(screen.getByText(/Audit Log unavailable/)).toBeTruthy();
  });

  it("keeps cluster evidence cluster-wide while labeling selected-node traffic only", async () => {
    mockSources();
    renderDashboard(nodes, primaryNodeID);
    expect(await screen.findByText("DNS Serving")).toBeTruthy();
    expect(screen.getByText(/Cluster: Home/)).toBeTruthy();
    expect(screen.getAllByText("Traffic scope: Primary").length).toBe(2);
    expect(screen.queryByText(/Scope: Primary/)).toBeNull();
    expect(api.statistics).toHaveBeenCalledWith(
      cluster.id,
      "24h",
      primaryNodeID,
    );
    expect(api.haStatus).toHaveBeenCalledWith(cluster.id);
    expect(api.nodes).toHaveBeenCalledWith(cluster.id);
  });

  it("scopes, labels, de-duplicates, and exact-links Recent Changes", async () => {
    mockSources();
    vi.mocked(api.configurationRevisions).mockResolvedValue({
      items: [
        {
          id: "revision-1",
          clusterId: cluster.id,
          revisionNumber: 7,
          summary: "Blocking policy updated",
          createdAt: "2026-08-24T06:30:00Z",
          active: true,
        },
        {
          id: "revision-other",
          clusterId: "99999999-9999-4999-8999-999999999999",
          revisionNumber: 99,
          summary: "Other cluster revision",
          createdAt: "2026-08-24T08:00:00Z",
        },
      ] as never[],
    });
    vi.mocked(api.deployments).mockResolvedValue({
      items: [
        {
          id: "deployment-1",
          clusterId: cluster.id,
          revisionId: "revision-1",
          status: "succeeded",
          origin: "manual",
          requestedAt: "2026-08-24T06:00:00Z",
          completedAt: "2026-08-24T06:20:00Z",
        },
        {
          id: "deployment-other",
          clusterId: "99999999-9999-4999-8999-999999999999",
          revisionId: "revision-other",
          status: "failed",
          origin: "manual",
          requestedAt: "2026-08-24T07:00:00Z",
          completedAt: "2026-08-24T07:10:00Z",
        },
      ] as never[],
    });
    vi.mocked(api.auditEvents).mockResolvedValue({
      items: [
        {
          id: "audit-revision",
          action: "configuration.revision_published",
          resourceType: "configuration_revision",
          resourceId: "revision-1",
          actorType: "user",
          actorDisplayName: "Current Admin",
          createdAt: "2026-08-24T06:30:00Z",
          clusterId: cluster.id,
          scope: "cluster",
        },
        {
          id: "audit-deployment",
          action: "deployment.succeeded",
          resourceType: "deployment",
          resourceId: "deployment-1",
          actorType: "system",
          createdAt: "2026-08-24T06:20:00Z",
          clusterId: cluster.id,
          scope: "cluster",
        },
        {
          id: "audit-controller",
          action: "system_settings.updated",
          resourceType: "system_settings",
          actorType: "user",
          actorDisplayName: "Current Admin",
          createdAt: "2026-08-24T06:10:00Z",
          scope: "controller",
        },
        {
          id: "audit-other-cluster",
          action: "node.updated",
          resourceType: "node",
          actorType: "user",
          createdAt: "2026-08-24T07:00:00Z",
          clusterId: "99999999-9999-4999-8999-999999999999",
          scope: "cluster",
        },
      ] as never[],
      hasMore: false,
    });
    renderDashboard();
    expect(await screen.findByText("Blocking policy updated")).toBeTruthy();
    expect(screen.getAllByText("Blocking policy updated")).toHaveLength(1);
    expect(screen.queryByText("Configuration Revision Published")).toBeNull();
    expect(screen.getByText("Deployment Succeeded")).toBeTruthy();
    expect(screen.queryByText("Node updated")).toBeNull();
    expect(screen.queryByText("Other cluster revision")).toBeNull();
    const controllerChange = screen.getByRole("link", {
      name: /Controller · System settings changed/,
    });
    expect(controllerChange.getAttribute("href")).toBe(
      "/system/audit?auditEventId=audit-controller",
    );
  });

  it("does not present unavailable statistics totals as zero", async () => {
    mockSources({ statisticsReport: { ...statistics, state: "unavailable" } });
    renderDashboard();
    expect(
      await screen.findByText(
        "No usable 24-hour Statistics snapshot is available for this scope.",
      ),
    ).toBeTruthy();
    expect(screen.queryByText("1,200")).toBeNull();
  });

  it("labels supplementary regions while their sources are loading", async () => {
    vi.spyOn(api, "nodes").mockResolvedValue({
      items: nodes,
      refreshedAt: "2026-08-24T07:00:00Z",
      staleAfterSeconds: 90,
    });
    vi.spyOn(api, "statistics").mockReturnValue(
      new Promise<StatisticsReport>(() => undefined),
    );
    vi.spyOn(api, "operationalStatus").mockReturnValue(
      new Promise<OperationalStatus>(() => undefined),
    );
    vi.spyOn(api, "haStatus").mockReturnValue(
      new Promise<HASummary>(() => undefined),
    );
    vi.spyOn(api, "versions").mockResolvedValue({ items: [] });
    vi.spyOn(api, "configurationRevisions").mockResolvedValue({ items: [] });
    vi.spyOn(api, "deployments").mockResolvedValue({ items: [] });
    vi.spyOn(api, "driftEvents").mockResolvedValue({ items: [] });
    vi.spyOn(api, "auditEvents").mockResolvedValue({ items: [] });
    renderDashboard();
    expect(await screen.findByText("DNS Serving")).toBeTruthy();
    expect(screen.getAllByText("Loading…").length).toBeGreaterThan(1);
    expect(screen.getByText("Loading DNS activity…")).toBeTruthy();
  });

  it("renders the node empty state without manufacturing metrics", async () => {
    mockSources({ nodeItems: [] });
    renderDashboard([]);
    expect(
      await screen.findByRole("heading", {
        name: "Add your first AdGuard Home node",
      }),
    ).toBeTruthy();
    expect(screen.getByRole("link", { name: "Add a node" })).toBeTruthy();
  });

  it("renders a retryable initial error when node inventory cannot load", async () => {
    vi.spyOn(api, "nodes").mockRejectedValue(new Error("nodes unavailable"));
    vi.spyOn(api, "statistics").mockRejectedValue(new Error("unavailable"));
    vi.spyOn(api, "operationalStatus").mockRejectedValue(
      new Error("unavailable"),
    );
    vi.spyOn(api, "haStatus").mockRejectedValue(new Error("unavailable"));
    vi.spyOn(api, "versions").mockRejectedValue(new Error("unavailable"));
    vi.spyOn(api, "configurationRevisions").mockRejectedValue(
      new Error("unavailable"),
    );
    vi.spyOn(api, "deployments").mockRejectedValue(new Error("unavailable"));
    vi.spyOn(api, "driftEvents").mockRejectedValue(new Error("unavailable"));
    vi.spyOn(api, "auditEvents").mockRejectedValue(new Error("unavailable"));
    renderDashboard();
    expect(
      await screen.findByRole("heading", {
        name: "Unable to load this content",
      }),
    ).toBeTruthy();
    expect(screen.getByRole("button", { name: "Try again" })).toBeTruthy();
  });
});
