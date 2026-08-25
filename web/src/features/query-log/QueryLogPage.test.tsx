// @vitest-environment jsdom

import { cleanup, render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import axe from "axe-core";
import { afterEach, describe, expect, it, vi } from "vitest";
import { api } from "../../lib/api";
import type { Cluster, Node, QueryEventPage } from "../../lib/types";
import { ScopeProvider } from "../../shell/ScopeContext";
import { QueryLogPage } from "./QueryLogPage";

afterEach(() => {
  cleanup();
  vi.restoreAllMocks();
  delete document.documentElement.dataset.theme;
  window.innerWidth = 1024;
});

const cluster = {
  id: "11111111-1111-4111-8111-111111111111",
  name: "Home",
} as Cluster;
const node = {
  id: "22222222-2222-4222-8222-222222222222",
  clusterId: cluster.id,
  name: "dns-a",
} as Node;

const page: QueryEventPage = {
  generatedAt: "2026-08-09T01:03:00Z",
  nextCursor: "older-page",
  filters: { statuses: ["allowed", "blocked"], queryTypes: ["A", "AAAA"] },
  coverage: {
    status: "partial",
    collectionEnabled: true,
    retentionSeconds: 604800,
    expectedNodes: 2,
    includedNodes: 1,
    staleNodes: 0,
    unsupportedNodes: 1,
    disabledNodes: 0,
    maintenanceNodes: 0,
    errorNodes: 0,
    gapNodes: 0,
    staleAfterSeconds: 120,
    nodes: [],
  },
  items: [
    {
      id: "33333333-3333-4333-8333-333333333333",
      nodeId: node.id,
      nodeName: node.name,
      timestamp: "2026-08-09T01:02:03Z",
      ingestedAt: "2026-08-09T01:02:04Z",
      query: "ads.example.org",
      queryType: "A",
      clientIdentifier: "192.0.2.10",
      clientDisplayName: "Living room",
      status: "blocked",
      responseCode: "NOERROR",
      processingTimeMs: 1.25,
      upstream: "1.1.1.1:53",
      filteringReason: "FilteredBlackList",
      rules: [{ text: "||ads.example.org^", filterListId: 7 }],
      answers: [{ type: "A", value: "0.0.0.0", ttl: 10 }],
      cached: false,
      answerDnssec: true,
    },
  ],
};

describe("QueryLogPage", () => {
  it("renders node-attributed rows, coverage, detail, and safe draft links", async () => {
    const queryEvents = vi.spyOn(api, "queryEvents").mockResolvedValue(page);
    const { container } = render(
      <ScopeProvider value={{ nodeId: "", nodes: [node] }}>
        <QueryLogPage cluster={cluster} />
      </ScopeProvider>,
    );

    expect(await screen.findByText("ads.example.org")).toBeTruthy();
    expect(screen.getByText("dns-a")).toBeTruthy();
    expect(screen.getByText(/1 of 2 enabled nodes/)).toBeTruthy();
    expect(screen.getByText(/Central retention: 7 days/)).toBeTruthy();
    await userEvent.click(
      screen.getByRole("button", { name: "View details for ads.example.org" }),
    );
    expect(screen.getByText("FilteredBlackList")).toBeTruthy();
    expect(
      screen.getByRole("link", { name: "View node" }).getAttribute("href"),
    ).toBe(`/ha/nodes/${node.id}`);
    expect(
      screen.getByText("Monitoring", { selector: ".eyebrow" }),
    ).toBeTruthy();
    expect(
      screen.getByRole("link", { name: "Block domain" }).getAttribute("href"),
    ).toContain("/filters/custom-rules?action=block&domain=ads.example.org");
    expect(screen.getByText(/never publish, deploy/i)).toBeTruthy();

    const accessibility = await axe.run(container, {
      runOnly: { type: "tag", values: ["wcag2a", "wcag2aa", "wcag21aa"] },
      rules: { "color-contrast": { enabled: false } },
    });
    expect(accessibility.violations).toEqual([]);
    expect(queryEvents).toHaveBeenCalledWith(
      cluster.id,
      expect.objectContaining({ nodeId: "", limit: 50 }),
    );
  });

  it("debounces search, applies filters, and pages with the server cursor", async () => {
    const queryEvents = vi.spyOn(api, "queryEvents").mockResolvedValue(page);
    render(<QueryLogPage cluster={cluster} />);
    await screen.findByText("ads.example.org");

    await userEvent.type(
      screen.getByRole("searchbox", { name: "Search queries or clients" }),
      "router",
    );
    await userEvent.selectOptions(
      screen.getByRole("combobox", { name: "Response status" }),
      "blocked",
    );
    await waitFor(() =>
      expect(queryEvents).toHaveBeenLastCalledWith(
        cluster.id,
        expect.objectContaining({
          search: "router",
          status: "blocked",
          cursor: "",
        }),
      ),
    );
    await userEvent.click(screen.getByRole("button", { name: "Next" }));
    await waitFor(() =>
      expect(queryEvents).toHaveBeenLastCalledWith(
        cluster.id,
        expect.objectContaining({ cursor: "older-page", search: "router" }),
      ),
    );
    expect(screen.getByText("Page 2")).toBeTruthy();
  });

  it("presents collection-disabled state without discarding retained events", async () => {
    vi.spyOn(api, "queryEvents").mockResolvedValue({
      ...page,
      coverage: {
        ...page.coverage,
        status: "unavailable",
        collectionEnabled: false,
      },
    });
    render(<QueryLogPage cluster={cluster} />);
    expect(
      await screen.findByText("Central collection is disabled"),
    ).toBeTruthy();
    expect(screen.getByText("ads.example.org")).toBeTruthy();
  });

  it.each([
    ["light", 1440],
    ["dark", 390],
  ] as const)(
    "retains node attribution in %s theme at %dpx",
    async (theme, width) => {
      document.documentElement.dataset.theme = theme;
      window.innerWidth = width;
      window.dispatchEvent(new Event("resize"));
      vi.spyOn(api, "queryEvents").mockResolvedValue(page);
      render(<QueryLogPage cluster={cluster} />);
      expect(await screen.findByText("ads.example.org")).toBeTruthy();
      expect(screen.getByRole("columnheader", { name: "Node" })).toBeTruthy();
      expect(screen.getByText("dns-a")).toBeTruthy();
      expect(document.documentElement.dataset.theme).toBe(theme);
    },
  );
});
