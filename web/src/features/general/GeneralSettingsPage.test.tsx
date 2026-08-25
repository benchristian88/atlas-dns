// @vitest-environment jsdom

import {
  cleanup,
  fireEvent,
  render,
  screen,
  waitFor,
  within,
} from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, describe, expect, it, vi } from "vitest";
import { api } from "../../lib/api";
import type {
  CapabilityProfile,
  Cluster,
  ConfigurationDraft,
  ConfigurationRevision,
  DesiredConfigurationDocument,
  Node,
} from "../../lib/types";
import { GeneralSettingsPage } from "./GeneralSettingsPage";
import { DAY_MILLIS, HOUR_MILLIS } from "./model";

afterEach(() => {
  cleanup();
  vi.restoreAllMocks();
  window.sessionStorage.clear();
  delete document.documentElement.dataset.theme;
});

const cluster: Cluster = {
  id: "11111111-1111-4111-8111-111111111111",
  name: "Home",
  description: "",
  version: 1,
  reconciliationPolicy: "manual",
  activeRevisionId: "66666666-6666-4666-8666-666666666666",
  createdAt: "2026-08-01T00:00:00Z",
  updatedAt: "2026-08-01T00:00:00Z",
};

const primary: Node = {
  id: "22222222-2222-4222-8222-222222222222",
  clusterId: cluster.id,
  name: "Primary",
  baseUrl: "http://primary.test",
  certificatePolicy: "insecure_http",
  enabled: true,
  healthStatus: "healthy",
  compatibilityStatus: "supported",
  version: "v0.107.78",
  maintenanceMode: false,
  convergenceStatus: "converged",
  recordVersion: 1,
  createdAt: "2026-08-01T00:00:00Z",
  updatedAt: "2026-08-01T00:00:00Z",
};

const secondary: Node = {
  ...primary,
  id: "33333333-3333-4333-8333-333333333333",
  name: "Secondary",
  version: "v0.107.78",
};

const desiredDocument = {
  schemaVersion: 2,
  shared: {
    dns: { protectionEnabled: true },
    filtering: {
      enabled: true,
      updateIntervalHours: 5,
      filterUrls: [],
      whitelistUrls: [],
      userRules: [],
    },
    clients: [],
    rewritesEnabled: true,
    rewrites: [],
    services: {
      blockedServiceIds: [],
      blockedSchedule: { timeZone: "Local", days: {} },
      safeBrowsing: true,
      parentalControl: false,
      safeSearch: {
        enabled: true,
        bing: true,
        duckDuckGo: false,
        ecosia: true,
        google: true,
        pixabay: false,
        yandex: true,
        youTube: true,
      },
    },
    queryLog: {
      enabled: true,
      intervalMillis: HOUR_MILLIS + 1,
      anonymizeClientIp: true,
      ignored: ["health.example"],
      ignoredEnabled: false,
    },
    statistics: {
      enabled: true,
      intervalMillis: 13 * HOUR_MILLIS,
      ignored: ["metrics.example"],
      ignoredEnabled: true,
    },
  },
  nodeOverrides: {},
  unsupported: [],
} as unknown as DesiredConfigurationDocument;

const draft: ConfigurationDraft = {
  id: "44444444-4444-4444-8444-444444444444",
  clusterId: cluster.id,
  sourceSnapshotId: "55555555-5555-4555-8555-555555555555",
  schemaVersion: 2,
  document: desiredDocument,
  canonicalHash: "draft-hash",
  version: 4,
  updatedAt: "2026-08-01T00:00:00Z",
};

const activeRevision = {
  id: cluster.activeRevisionId,
  clusterId: cluster.id,
  revisionNumber: 12,
  schemaVersion: 2,
  document: desiredDocument,
  canonicalHash: "active-hash",
  summary: "Active",
  createdBy: "user",
  createdAt: "2026-08-01T00:00:00Z",
  active: true,
} as ConfigurationRevision;

function profile(
  node: Node,
  features: Record<string, boolean>,
): CapabilityProfile {
  return {
    nodeId: node.id,
    productVersion: node.version ?? "",
    compatibility: "supported",
    schemaVersion: 2,
    features,
    warnings: [],
    refreshedAt: "2026-08-01T00:00:00Z",
  };
}

function mockLoad(overrides?: {
  draft?: ConfigurationDraft;
  capabilities?: CapabilityProfile[];
}) {
  vi.spyOn(api, "configurationInventory").mockResolvedValue({
    schemaVersion: 2,
    snapshots: [],
    capabilities: overrides?.capabilities ?? [
      profile(primary, {
        filter_interval_arbitrary: true,
        safe_search_ecosia: true,
        ignored_lists_toggle: true,
        querylog_clear: true,
        stats_reset: true,
      }),
      profile(secondary, {
        filter_interval_arbitrary: false,
        safe_search_ecosia: false,
        ignored_lists_toggle: false,
        querylog_clear: true,
        stats_reset: true,
      }),
    ],
    draft: overrides && "draft" in overrides ? overrides.draft : draft,
  });
  vi.spyOn(api, "nodes").mockResolvedValue({
    items: [primary, secondary],
    refreshedAt: "2026-08-01T00:00:00Z",
    staleAfterSeconds: 60,
  });
  vi.spyOn(api, "configurationRevisions").mockResolvedValue({
    items: [activeRevision],
  });
}

describe("General Settings", () => {
  it("loads preset and custom values without conversion loss", async () => {
    mockLoad();
    const update = vi
      .spyOn(api, "updateConfigurationDraft")
      .mockImplementation(async (_clusterID, _version, nextDocument) => ({
        draft: { ...draft, version: 5, document: nextDocument },
        issues: [],
      }));
    render(<GeneralSettingsPage cluster={cluster} />);

    expect(await screen.findByText("Protection and filtering")).not.toBeNull();
    expect(screen.queryByText("Draft and scope")).toBeNull();
    expect(screen.queryByText("Revision #12")).toBeNull();
    expect(
      (screen.getByLabelText("Filter update interval") as HTMLSelectElement)
        .value,
    ).toBe("custom");
    expect(
      (
        screen.getByLabelText(
          "Custom filter update interval",
        ) as HTMLInputElement
      ).value,
    ).toBe("5");
    expect(
      (screen.getByLabelText("Custom query log retention") as HTMLInputElement)
        .value,
    ).toBe(String(HOUR_MILLIS + 1));
    expect(
      (
        screen.getByLabelText(
          "Custom query log retention unit",
        ) as HTMLSelectElement
      ).value,
    ).toBe("1");
    expect(
      (screen.getByLabelText("Custom statistics retention") as HTMLInputElement)
        .value,
    ).toBe("13");
    expect(
      (
        screen.getByLabelText(
          "Custom statistics retention unit",
        ) as HTMLSelectElement
      ).value,
    ).toBe(String(HOUR_MILLIS));

    fireEvent.change(screen.getByLabelText("Custom query log retention unit"), {
      target: { value: String(DAY_MILLIS) },
    });
    fireEvent.click(screen.getByLabelText("Protection enabled"));
    fireEvent.click(screen.getByRole("button", { name: "Save Draft" }));
    await waitFor(() => expect(update).toHaveBeenCalledOnce());
    expect(update.mock.calls[0]?.[2].shared.queryLog.intervalMillis).toBe(
      HOUR_MILLIS + 1,
    );
    expect(update.mock.calls[0]?.[2].shared.statistics.intervalMillis).toBe(
      13 * HOUR_MILLIS,
    );
  });

  it("maps presets, Statistics ignored state, providers, and Save Draft only", async () => {
    mockLoad({
      capabilities: [
        profile(primary, {
          filter_interval_arbitrary: true,
          safe_search_ecosia: true,
          ignored_lists_toggle: true,
        }),
        profile(secondary, {
          filter_interval_arbitrary: true,
          safe_search_ecosia: true,
          ignored_lists_toggle: true,
        }),
      ],
    });
    const update = vi
      .spyOn(api, "updateConfigurationDraft")
      .mockImplementation(async (_clusterID, _version, nextDocument) => ({
        draft: { ...draft, version: 5, document: nextDocument },
        issues: [],
      }));
    const publish = vi.spyOn(api, "publishConfigurationRevision");
    const deploy = vi.spyOn(api, "startDeployment");
    render(<GeneralSettingsPage cluster={cluster} />);

    await screen.findByText("Query Log policy");
    fireEvent.change(screen.getByLabelText("Query Log retention"), {
      target: { value: String(7 * DAY_MILLIS) },
    });
    fireEvent.click(screen.getByLabelText("Apply Statistics ignored domains"));
    fireEvent.click(screen.getByLabelText("DuckDuckGo"));
    fireEvent.click(screen.getByRole("button", { name: "Save Draft" }));

    await waitFor(() => expect(update).toHaveBeenCalledOnce());
    const savedDocument = update.mock.calls[0]?.[2];
    expect(savedDocument?.shared.queryLog.intervalMillis).toBe(7 * DAY_MILLIS);
    expect(savedDocument?.shared.queryLog.ignoredEnabled).toBe(false);
    expect(savedDocument?.shared.statistics.ignoredEnabled).toBe(false);
    expect(savedDocument?.shared.services.safeSearch.duckDuckGo).toBe(true);
    expect(publish).not.toHaveBeenCalled();
    expect(deploy).not.toHaveBeenCalled();
    expect(
      await screen.findByText("Draft saved. Nodes are unchanged."),
    ).not.toBeNull();
  });

  it("validates domains inline and keeps invalid rows out of Save Draft", async () => {
    mockLoad();
    const update = vi.spyOn(api, "updateConfigurationDraft");
    render(<GeneralSettingsPage cluster={cluster} />);

    const row = await screen.findByLabelText(
      "Query Log ignored domains entry 1",
    );
    fireEvent.change(row, { target: { value: "bad domain" } });
    expect(screen.getByText("Enter a valid domain name.")).not.toBeNull();
    expect(
      (screen.getByRole("button", { name: "Save Draft" }) as HTMLButtonElement)
        .disabled,
    ).toBe(true);
    expect(update).not.toHaveBeenCalled();
  });

  it("groups provider controls and explains version and future-release boundaries", async () => {
    mockLoad();
    render(<GeneralSettingsPage cluster={cluster} />);

    expect(await screen.findByText("Safe Search providers")).not.toBeNull();
    expect(screen.getByLabelText("DuckDuckGo")).not.toBeNull();
    expect(screen.getByLabelText("YouTube")).not.toBeNull();
    expect((screen.getByLabelText("Ecosia") as HTMLInputElement).disabled).toBe(
      true,
    );
    expect(screen.getByText(/Ecosia cannot be changed/)).not.toBeNull();
    expect(
      screen.getByText(
        /Controller collection and retention are configured separately/,
      ),
    ).not.toBeNull();
    expect(
      screen.getByText(
        /central query-event retention is configured separately/i,
      ),
    ).not.toBeNull();
    expect(
      screen.getByRole("button", { name: "Clear Query Log" }),
    ).not.toBeNull();
    expect(
      screen.getByRole("button", { name: "Reset Statistics" }),
    ).not.toBeNull();
  });

  it("supports keyboard controls and light/dark/mobile-safe shared layout", async () => {
    mockLoad();
    document.documentElement.dataset.theme = "light";
    Object.defineProperty(window, "innerWidth", {
      value: 1440,
      configurable: true,
    });
    const desktop = render(<GeneralSettingsPage cluster={cluster} />);
    expect(await screen.findByText("Protection and filtering")).not.toBeNull();
    expect(document.documentElement.dataset.theme).toBe("light");
    expect(
      desktop.container.querySelector(".page-container--wide"),
    ).not.toBeNull();
    desktop.unmount();

    document.documentElement.dataset.theme = "dark";
    Object.defineProperty(window, "innerWidth", {
      value: 390,
      configurable: true,
    });
    const user = userEvent.setup();
    const { container } = render(<GeneralSettingsPage cluster={cluster} />);

    const protection = await screen.findByLabelText("Protection enabled");
    protection.focus();
    await user.keyboard(" ");
    expect((protection as HTMLInputElement).checked).toBe(false);
    expect(document.documentElement.dataset.theme).toBe("dark");
    expect(container.querySelector(".page-container--wide")).not.toBeNull();
  });

  it("renders loading, retryable error, and missing-draft states", async () => {
    vi.spyOn(api, "configurationInventory").mockReturnValue(
      new Promise(() => undefined),
    );
    vi.spyOn(api, "nodes").mockReturnValue(new Promise(() => undefined));
    vi.spyOn(api, "configurationRevisions").mockReturnValue(
      new Promise(() => undefined),
    );
    const loading = render(<GeneralSettingsPage cluster={cluster} />);
    expect(screen.getByLabelText("Loading General Settings")).not.toBeNull();
    loading.unmount();

    vi.restoreAllMocks();
    vi.spyOn(api, "configurationInventory").mockRejectedValue(
      new Error("General request failed"),
    );
    vi.spyOn(api, "nodes").mockResolvedValue({
      items: [],
      refreshedAt: "",
      staleAfterSeconds: 60,
    });
    vi.spyOn(api, "configurationRevisions").mockResolvedValue({ items: [] });
    const failed = render(<GeneralSettingsPage cluster={cluster} />);
    expect(await screen.findByText("General request failed")).not.toBeNull();
    expect(screen.getByRole("button", { name: "Try again" })).not.toBeNull();
    failed.unmount();

    vi.restoreAllMocks();
    mockLoad({ draft: undefined });
    const missing = render(<GeneralSettingsPage cluster={cluster} />);
    expect(
      await screen.findByText("Import a node configuration first"),
    ).not.toBeNull();
    missing.unmount();

    vi.restoreAllMocks();
    mockLoad({
      draft: {
        ...draft,
        schemaVersion: 1,
        document: { ...desiredDocument, schemaVersion: 1 },
      },
    });
    render(<GeneralSettingsPage cluster={cluster} />);
    expect(await screen.findByText("Unsupported draft format")).not.toBeNull();
  });

  it("clears one node query log only after strong confirmation without changing policy", async () => {
    mockLoad();
    const update = vi.spyOn(api, "updateConfigurationDraft");
    const clear = vi.spyOn(api, "clearQueryLog").mockResolvedValue({
      id: "77777777-7777-4777-8777-777777777777",
      clusterId: cluster.id,
      clusterName: cluster.name,
      command: "clear_query_log",
      target: { scope: "node", nodeId: primary.id },
      status: "succeeded",
      requestId: "request-querylog",
      requestedAt: "2026-08-02T00:00:00Z",
      completedAt: "2026-08-02T00:00:01Z",
      excludedNodes: [],
      nodeResults: [
        {
          id: "result-querylog",
          nodeId: primary.id,
          nodeName: primary.name,
          position: 1,
          status: "succeeded",
          observationStatus: "succeeded",
        },
      ],
    });
    const user = userEvent.setup();
    render(<GeneralSettingsPage cluster={cluster} />);
    await user.click(
      await screen.findByRole("button", { name: "Clear Query Log" }),
    );
    const dialog = screen.getByRole("dialog", { name: "Clear Query Log" });
    expect(within(dialog).getAllByText("Selected node").length).toBeGreaterThan(
      0,
    );
    expect(within(dialog).getAllByText("Primary").length).toBeGreaterThan(0);
    expect(
      within(dialog).getByText(/deleted immediately and permanently/),
    ).not.toBeNull();
    expect(
      within(dialog).getByText(/enabled state, retention policy/),
    ).not.toBeNull();
    const confirm = within(dialog).getByRole("button", {
      name: "Clear Query Log",
    }) as HTMLButtonElement;
    expect(confirm.disabled).toBe(true);
    await user.type(
      within(dialog).getByLabelText(/Type CLEAR_QUERY_LOG/),
      "CLEAR_QUERY_LOG",
    );
    expect(confirm.disabled).toBe(false);
    await user.click(confirm);
    await waitFor(() => expect(clear).toHaveBeenCalledOnce());
    expect(clear.mock.calls[0]?.[1]).toEqual({
      scope: "node",
      nodeId: primary.id,
    });
    expect(await screen.findByText("Query Log clear result")).not.toBeNull();
    expect(
      (screen.getByLabelText("Query logging enabled") as HTMLInputElement)
        .checked,
    ).toBe(true);
    expect(update).not.toHaveBeenCalled();
  });

  it("resets statistics with explicit fleet scope and durable partial results", async () => {
    mockLoad();
    const reset = vi.spyOn(api, "resetStatistics").mockResolvedValue({
      id: "88888888-8888-4888-8888-888888888888",
      clusterId: cluster.id,
      clusterName: cluster.name,
      command: "reset_statistics",
      target: { scope: "all_compatible_enabled_nodes" },
      status: "partial_success",
      requestId: "request-statistics",
      requestedAt: "2026-08-02T00:00:00Z",
      completedAt: "2026-08-02T00:00:01Z",
      excludedNodes: [],
      nodeResults: [
        {
          id: "result-a",
          nodeId: primary.id,
          nodeName: primary.name,
          position: 1,
          status: "succeeded",
          observationStatus: "succeeded",
        },
        {
          id: "result-b",
          nodeId: secondary.id,
          nodeName: secondary.name,
          position: 2,
          status: "failed",
          errorCode: "NODE_UNREACHABLE",
        },
      ],
    });
    const user = userEvent.setup();
    render(<GeneralSettingsPage cluster={cluster} />);
    await user.click(
      await screen.findByRole("button", { name: "Reset Statistics" }),
    );
    const dialog = screen.getByRole("dialog", { name: "Reset Statistics" });
    await user.selectOptions(
      within(dialog).getByLabelText("Target scope"),
      "all_compatible_enabled_nodes",
    );
    expect(
      within(dialog).getByText("All compatible enabled nodes (2)"),
    ).not.toBeNull();
    await user.type(
      within(dialog).getByLabelText(/Type RESET_STATISTICS/),
      "RESET_STATISTICS",
    );
    await user.click(
      within(dialog).getByRole("button", { name: "Reset Statistics" }),
    );
    await waitFor(() => expect(reset).toHaveBeenCalledOnce());
    expect(reset.mock.calls[0]?.[1]).toEqual({
      scope: "all_compatible_enabled_nodes",
    });
    expect(
      await screen.findByText("Statistics reset partially completed"),
    ).not.toBeNull();
    expect(screen.getAllByText(/NODE_UNREACHABLE/).length).toBeGreaterThan(0);
    expect(
      window.sessionStorage.getItem(`atlas-policy-operation:${cluster.id}`),
    ).not.toBeNull();
    await user.click(screen.getByRole("button", { name: "Dismiss result" }));
    expect(
      window.sessionStorage.getItem(`atlas-policy-operation:${cluster.id}`),
    ).toBeNull();
  });
});
