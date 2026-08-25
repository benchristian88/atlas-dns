// @vitest-environment jsdom

import { cleanup, render, screen, waitFor } from "@testing-library/react";
import axe from "axe-core";
import { afterEach, describe, expect, it, vi } from "vitest";
import { api } from "../../lib/api";
import type { Cluster, OperationalStatus } from "../../lib/types";
import { OperationalStatusPage } from "./OperationalStatusPage";

afterEach(() => {
  cleanup();
  vi.restoreAllMocks();
});

const cluster: Cluster = {
  id: "11111111-1111-4111-8111-111111111111",
  name: "Home",
  description: "",
  reconciliationPolicy: "manual",
  version: 1,
  createdAt: "2026-08-09T00:00:00Z",
  updatedAt: "2026-08-09T00:00:00Z",
};

const status: OperationalStatus = {
  generatedAt: "2026-08-09T01:00:00Z",
  clusterId: cluster.id,
  summary: {
    state: "degraded",
    actionRequired: true,
    message: "Query Log is stale.",
    healthyNodes: 1,
    expectedNodes: 2,
  },
  api: "healthy",
  database: {
    state: "healthy",
    pingLatencyMs: 2,
    schemaVersion: 10,
    databaseBytes: 2048,
    poolTotal: 3,
    poolAcquired: 1,
    poolMax: 10,
    datasets: [
      {
        name: "statistics",
        estimatedRows: 20,
        approximateBytes: 1024,
        retentionSeconds: 86400,
        oldestRetainedAt: "2026-08-08T00:00:00Z",
        newestRetainedAt: "2026-08-09T00:00:00Z",
      },
    ],
  },
  nodes: [],
  dnsService: {
    state: "healthy",
    expectedNodes: 2,
    currentNodes: 2,
    staleNodes: 0,
    unsupportedNodes: 0,
    coveragePercent: 100,
    nodes: [],
  },
  ha: {
    state: "healthy",
    totalNodes: 2,
    servingDnsNodes: 2,
    apiReachableNodes: 2,
    convergedNodes: 2,
    maintenanceNodes: 0,
    certificateWarnings: 0,
    updateAvailableNodes: 0,
    message: "Healthy redundancy.",
    nodes: [],
  },
  observation: {
    state: "healthy",
    expectedNodes: 2,
    currentNodes: 2,
    staleNodes: 0,
    unsupportedNodes: 0,
    coveragePercent: 100,
    nodes: [],
  },
  statistics: {
    state: "healthy",
    expectedNodes: 2,
    currentNodes: 2,
    staleNodes: 0,
    unsupportedNodes: 0,
    coveragePercent: 100,
    nodes: [],
  },
  queryLog: {
    state: "degraded",
    expectedNodes: 2,
    currentNodes: 1,
    staleNodes: 1,
    unsupportedNodes: 0,
    coveragePercent: 50,
    nodes: [
      {
        nodeId: "22222222-2222-4222-8222-222222222222",
        nodeName: "dns-secondary",
        state: "stale",
        lagSeconds: 1080,
        consecutiveFailures: 2,
        gapDetected: true,
        gapReason: "QUERY_LOG_NODE_RETENTION_GAP",
      },
    ],
  },
  workers: [
    {
      name: "query_log_collection",
      state: "healthy",
      running: false,
      consecutiveFailures: 0,
      runsTotal: 4,
      failuresTotal: 0,
    },
  ],
};

describe("OperationalStatusPage", () => {
  it("renders degraded collectors, safe gaps, workers, and storage", async () => {
    vi.spyOn(api, "operationalStatus").mockResolvedValue(status);
    const { container } = render(<OperationalStatusPage cluster={cluster} />);
    expect(
      await screen.findByRole("heading", { name: "Operational Status" }),
    ).not.toBeNull();
    expect(
      screen
        .getByText(
          "Health of the controller, collectors, storage, and background work.",
        )
        .closest(".page-header__description"),
    ).not.toBeNull();
    expect(
      screen.getByText("Monitoring", { selector: ".eyebrow" }),
    ).not.toBeNull();
    const overallHealth = container.querySelector(
      '[aria-label="Overall controller health"]',
    );
    expect(overallHealth).not.toBeNull();
    expect(
      overallHealth?.querySelectorAll(".operational-health-card"),
    ).toHaveLength(6);
    expect(overallHealth?.querySelector(".metric-card")).toBeNull();
    expect(
      screen.getByRole("article", { name: "Query Log" }).textContent,
    ).toContain("1 / 2");
    expect(screen.getByText("dns-secondary")).not.toBeNull();
    expect(screen.getByText("QUERY_LOG_NODE_RETENTION_GAP")).not.toBeNull();
    expect(screen.getByText("query log collection")).not.toBeNull();
    expect(screen.getByText("1 KiB")).not.toBeNull();
    const coreServices = screen
      .getByRole("heading", { name: "Core Services" })
      .closest(".operational-section");
    expect(coreServices).not.toBeNull();
    expect(
      coreServices?.querySelector(".operational-section-heading"),
    ).not.toBeNull();
    expect(coreServices?.querySelector(".settings-group__header")).toBeNull();
    expect(
      coreServices?.querySelectorAll(".summary-tile-grid > div"),
    ).toHaveLength(4);
    expect(
      container.querySelector('[aria-label="Core service status"]'),
    ).not.toBeNull();
    expect(
      container.querySelectorAll(".operational-section-heading h2"),
    ).toHaveLength(7);
  });

  it("has no automated structural WCAG A/AA violations", async () => {
    vi.spyOn(api, "operationalStatus").mockResolvedValue(status);
    const { container } = render(<OperationalStatusPage cluster={cluster} />);
    await screen.findByRole("heading", { name: "Operational Status" });
    const result = await axe.run(container, {
      runOnly: { type: "tag", values: ["wcag2a", "wcag2aa", "wcag21aa"] },
      rules: { "color-contrast": { enabled: false } },
    });
    expect(result.violations).toEqual([]);
  });

  it("renders a retryable loading failure", async () => {
    vi.spyOn(api, "operationalStatus").mockRejectedValue(
      new Error("Unavailable"),
    );
    render(<OperationalStatusPage cluster={cluster} />);
    await waitFor(() => expect(screen.getByText("Unavailable")).not.toBeNull());
    expect(screen.getByRole("button", { name: "Try again" })).not.toBeNull();
  });
});
