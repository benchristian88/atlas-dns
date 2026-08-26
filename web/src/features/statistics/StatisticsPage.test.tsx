// @vitest-environment jsdom

import { cleanup, render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import axe from "axe-core";
import { afterEach, describe, expect, it, vi } from "vitest";
import { api } from "../../lib/api";
import type {
  Cluster,
  Node as ManagedNode,
  StatisticsReport,
} from "../../lib/types";
import { StatisticsPage } from "./StatisticsPage";

afterEach(() => {
  cleanup();
  vi.restoreAllMocks();
});

const cluster = {
  id: "11111111-1111-4111-8111-111111111111",
  name: "Home",
} as Cluster;
const node = {
  id: "22222222-2222-4222-8222-222222222222",
  clusterId: cluster.id,
  name: "Primary",
} as ManagedNode;

const report: StatisticsReport = {
  range: "24h",
  scope: { type: "node", nodeId: node.id },
  state: "partial",
  generatedAt: "2026-08-09T00:00:00Z",
  freshness: { newestAt: "2026-08-09T00:00:00Z", staleAfterSeconds: 10800 },
  coverage: {
    status: "partial",
    expectedNodes: 2,
    includedNodes: 1,
    missingNodes: 0,
    staleNodes: 0,
    unsupportedNodes: 1,
    maintenanceNodes: 0,
  },
  totals: {
    dnsQueries: 1000,
    blockedFiltering: 125,
    blockedPercentage: 12.5,
    replacedSafeBrowsing: 2,
    replacedSafeSearch: 1,
    replacedParental: 1,
    safetyInterventions: 4,
    safetyInterventionPercentage: 0.4,
    averageProcessingMs: 16.25,
  },
  series: [
    {
      at: "2026-08-08T22:00:00Z",
      dnsQueries: 400,
      blockedFiltering: 50,
      replacedSafeBrowsing: 1,
      replacedParental: 0,
      includedNodes: 1,
    },
    {
      at: "2026-08-08T23:00:00Z",
      dnsQueries: 600,
      blockedFiltering: 75,
      replacedSafeBrowsing: 1,
      replacedParental: 1,
      includedNodes: 1,
    },
  ],
  rankings: {
    queriedDomains: [{ key: "example.com", value: 700, percentage: 70 }],
    blockedDomains: [{ key: "ads.example", value: 100, percentage: 80 }],
    clients: [{ key: "192.0.2.10", value: 800, percentage: 80 }],
    upstreamResponses: [{ key: "1.1.1.1:53", value: 900, percentage: 90 }],
    upstreamAverageLatencyMs: [{ key: "1.1.1.1:53", value: 12.4 }],
  },
  nodes: [
    {
      nodeId: node.id,
      nodeName: node.name,
      status: "included",
      collectedAt: "2026-08-09T00:00:00Z",
      dnsQueries: 1000,
    },
    {
      nodeId: "33333333-3333-4333-8333-333333333333",
      nodeName: "Older",
      status: "unsupported",
      reasonCode: "STATISTICS_EXACT_RANGE_UNSUPPORTED",
    },
  ],
};

describe("StatisticsPage", () => {
  it("renders weighted presentation data and owns cluster/node scope", async () => {
    const statistics = vi.spyOn(api, "statistics").mockResolvedValue(report);
    vi.spyOn(api, "nodes").mockResolvedValue({
      items: [node],
      refreshedAt: "2026-08-09T00:00:00Z",
      staleAfterSeconds: 60,
    });
    const { container } = render(<StatisticsPage cluster={cluster} />);

    expect(
      await screen.findByRole("heading", { name: "Statistics" }),
    ).toBeTruthy();
    const trafficScope = screen.getByRole("heading", {
      name: "Traffic scope",
    });
    const toolbar = trafficScope.closest("section");
    const firstMetric = screen.getByText("DNS queries").closest("article");
    if (
      !(toolbar instanceof HTMLElement) ||
      !(firstMetric instanceof HTMLElement)
    ) {
      throw new Error("Expected the traffic scope and metrics sections");
    }
    expect(
      toolbar.compareDocumentPosition(firstMetric) &
        Node.DOCUMENT_POSITION_FOLLOWING,
    ).toBeTruthy();
    expect(screen.getAllByText("1,000").length).toBeGreaterThanOrEqual(1);
    expect(screen.getByText("12.5% of queries")).toBeTruthy();
    expect(screen.getByText("example.com")).toBeTruthy();
    expect(screen.getByText("STATISTICS_EXACT_RANGE_UNSUPPORTED")).toBeTruthy();
    expect(
      screen.getByRole("img", {
        name: /DNS query and blocked query activity/i,
      }),
    ).toBeTruthy();
    const accessibility = await axe.run(container, {
      runOnly: { type: "tag", values: ["wcag2a", "wcag2aa", "wcag21aa"] },
      rules: { "color-contrast": { enabled: false } },
    });
    expect(accessibility.violations).toEqual([]);
    expect(statistics).toHaveBeenCalledWith(cluster.id, "24h", "");
    const scope = await screen.findByRole("combobox", { name: "Node" });
    expect(screen.getByRole("option", { name: "Entire Cluster" })).toBeTruthy();
    expect(screen.getByRole("option", { name: "Primary" })).toBeTruthy();
    await userEvent.selectOptions(scope, node.id);
    await waitFor(() =>
      expect(statistics).toHaveBeenLastCalledWith(cluster.id, "24h", node.id),
    );

    await userEvent.click(screen.getByRole("button", { name: "7 days" }));
    await waitFor(() =>
      expect(statistics).toHaveBeenLastCalledWith(cluster.id, "7d", node.id),
    );
  });

  it("renders the explicit unavailable state", async () => {
    vi.spyOn(api, "nodes").mockResolvedValue({
      items: [],
      refreshedAt: "2026-08-09T00:00:00Z",
      staleAfterSeconds: 60,
    });
    vi.spyOn(api, "statistics").mockResolvedValue({
      ...report,
      state: "unavailable",
      series: [],
    });
    render(<StatisticsPage cluster={cluster} />);
    expect(
      await screen.findByText("Statistics are not available yet"),
    ).toBeTruthy();
  });
});
