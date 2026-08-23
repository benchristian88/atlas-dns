// @vitest-environment jsdom

import { cleanup, render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import axe from "axe-core";
import { afterEach, describe, expect, it, vi } from "vitest";
import { api } from "../../lib/api";
import type { Cluster, Node } from "../../lib/types";
import { HAOperationsPage } from "./HAOperationsPage";
import { NodeLifecyclePage } from "./NodeLifecyclePage";

afterEach(() => {
  cleanup();
  vi.restoreAllMocks();
});
const cluster = {
  id: "11111111-1111-4111-8111-111111111111",
  name: "Home",
  description: "",
  reconciliationPolicy: "manual",
  version: 1,
  createdAt: "2026-08-09T00:00:00Z",
  updatedAt: "2026-08-09T00:00:00Z",
} as Cluster;
const node = {
  id: "22222222-2222-4222-8222-222222222222",
  clusterId: cluster.id,
  name: "Primary",
  baseUrl: "https://node.test",
  certificatePolicy: "system",
  enabled: true,
  healthStatus: "healthy",
  compatibilityStatus: "supported",
  version: "v0.107.78",
  maintenanceMode: false,
  convergenceStatus: "converged",
  recordVersion: 2,
  createdAt: "2026-08-09T00:00:00Z",
  updatedAt: "2026-08-09T00:00:00Z",
} as Node;

function commonMocks() {
  vi.spyOn(api, "nodes").mockResolvedValue({
    items: [node],
    refreshedAt: "2026-08-09T01:00:00Z",
    staleAfterSeconds: 90,
  });
  vi.spyOn(api, "upgrades").mockResolvedValue({ items: [] });
}

describe("HA operations", () => {
  it("renders separate DNS, API, convergence, certificates, versions, notifications, and history dimensions", async () => {
    commonMocks();
    vi.spyOn(api, "haStatus").mockResolvedValue({
      state: "at_risk",
      totalNodes: 1,
      servingDnsNodes: 1,
      apiReachableNodes: 1,
      convergedNodes: 1,
      maintenanceNodes: 0,
      certificateWarnings: 1,
      updateAvailableNodes: 1,
      message: "No redundancy.",
      nodes: [
        {
          nodeId: node.id,
          dnsStatus: "healthy",
          udpStatus: "healthy",
          tcpStatus: "healthy",
          dnsProbedAt: "2026-08-09T01:00:00Z",
        },
      ],
    });
    vi.spyOn(api, "certificates").mockResolvedValue({
      items: [
        {
          nodeId: node.id,
          nodeName: node.name,
          subject: "DNS certificate",
          daysRemaining: 5,
          state: "critical",
        },
      ],
    });
    vi.spyOn(api, "versions").mockResolvedValue({
      items: [
        {
          nodeId: node.id,
          nodeName: node.name,
          installedVersion: "v0.107.78",
          latestVersion: "v0.107.79",
          compatibility: "supported",
          installationType: "docker",
          upgradeSupport: "guided",
          updateAvailable: true,
          releaseCheckStale: false,
        },
      ],
    });
    const history = vi
      .spyOn(api, "haHistory")
      .mockImplementation(async (_clusterId, options) =>
        options?.cursor
          ? {
              items: [
                {
                  id: "66666666-6666-4666-8666-666666666666",
                  kind: "notification" as const,
                  clusterId: cluster.id,
                  nodeId: node.id,
                  eventType: "dns.recovered",
                  severity: "info" as const,
                  summary: "DNS service recovered",
                  details: {},
                  occurredAt: "2026-08-09T00:58:00Z",
                  notification: {
                    channelName: "Operations",
                    status: "failed" as const,
                    attemptCount: 5,
                    errorCode: "NOTIFICATION_HTTP_REJECTED",
                    errorSummary: "HTTP 500",
                    httpStatus: 500,
                    test: false,
                  },
                },
              ],
              hasMore: false,
            }
          : {
              items: [
                {
                  id: "33333333-3333-4333-8333-333333333333",
                  kind: "event" as const,
                  clusterId: cluster.id,
                  nodeId: node.id,
                  eventType: "dns.failed",
                  severity: "critical",
                  summary: "DNS failed",
                  details: {},
                  occurredAt: "2026-08-09T01:00:00Z",
                },
                {
                  id: "44444444-4444-4444-8444-444444444444",
                  kind: "notification" as const,
                  clusterId: cluster.id,
                  nodeId: node.id,
                  eventType: "dns.failed",
                  severity: "critical" as const,
                  summary: "DNS failed",
                  details: {},
                  occurredAt: "2026-08-09T01:00:00Z",
                  notification: {
                    channelName: "Operations",
                    status: "delivered" as const,
                    attemptCount: 1,
                    httpStatus: 204,
                    test: false,
                  },
                },
                {
                  id: "55555555-5555-4555-8555-555555555555",
                  kind: "notification" as const,
                  clusterId: cluster.id,
                  eventType: "notification.test",
                  severity: "info" as const,
                  summary: "Atlas DNS Controller webhook test",
                  details: {},
                  occurredAt: "2026-08-09T00:59:00Z",
                  notification: {
                    channelName: "Operations",
                    status: "delivered" as const,
                    attemptCount: 1,
                    httpStatus: 204,
                    test: true,
                  },
                },
              ],
              nextCursor: "cursor-next",
              hasMore: true,
            },
      );
    vi.spyOn(api, "notificationChannels").mockResolvedValue({ items: [] });
    const { container } = render(<HAOperationsPage cluster={cluster} />);
    expect(
      await screen.findByRole("heading", { name: "HA Operations" }),
    ).toBeTruthy();
    expect(
      screen.getAllByText("1 / 1", { selector: "strong" }).length,
    ).toBeGreaterThan(0);
    const summary = container.querySelector(
      '[aria-label="HA redundancy summary"]',
    );
    expect(summary?.querySelectorAll(":scope > .metric-card")).toHaveLength(4);
    expect(summary?.querySelector(".metric")).toBeNull();
    expect(screen.getByText("DNS certificate")).toBeTruthy();
    expect(screen.getAllByText("DNS failed")).toHaveLength(2);
    expect(screen.getByText("Webhook test")).toBeTruthy();
    expect(screen.getAllByText("Delivered").length).toBe(2);
    await userEvent.click(screen.getByRole("button", { name: "Next" }));
    await waitFor(() =>
      expect(history).toHaveBeenCalledWith(cluster.id, {
        cursor: "cursor-next",
        limit: 50,
      }),
    );
    expect(await screen.findByText(/HTTP 500/)).toBeTruthy();
    expect(
      (screen.getByRole("button", { name: "Next" }) as HTMLButtonElement)
        .disabled,
    ).toBe(true);
    expect(
      (screen.getByRole("button", { name: "Previous" }) as HTMLButtonElement)
        .disabled,
    ).toBe(false);
    const accessibility = await axe.run(container, {
      runOnly: { type: "tag", values: ["wcag2a", "wcag2aa", "wcag21aa"] },
      rules: { "color-contrast": { enabled: false } },
    });
    expect(accessibility.violations).toEqual([]);
  });

  it("completes add, preserved/replaced-secret edit, enable state, test, and delete webhook workflows", async () => {
    const user = userEvent.setup();
    commonMocks();
    vi.spyOn(api, "haStatus").mockResolvedValue({
      state: "healthy",
      totalNodes: 1,
      servingDnsNodes: 1,
      apiReachableNodes: 1,
      convergedNodes: 1,
      maintenanceNodes: 0,
      certificateWarnings: 0,
      updateAvailableNodes: 0,
      message: "Healthy",
      nodes: [],
    });
    vi.spyOn(api, "certificates").mockResolvedValue({ items: [] });
    vi.spyOn(api, "versions").mockResolvedValue({ items: [] });
    vi.spyOn(api, "haHistory").mockResolvedValue({
      items: [],
      hasMore: false,
    });
    const channel = {
      id: "44444444-4444-4444-8444-444444444444",
      clusterId: cluster.id,
      name: "Operations",
      channelType: "webhook" as const,
      enabled: true,
      destinationSet: true,
      destinationSummary: "https://hooks.example.test",
      subscribedEvents: ["all_ha_transitions"],
      recordVersion: 3,
      createdAt: "2026-08-09T00:00:00Z",
      updatedAt: "2026-08-09T01:00:00Z",
    };
    vi.spyOn(api, "notificationChannels").mockResolvedValue({
      items: [channel],
    });
    const create = vi
      .spyOn(api, "createNotificationChannel")
      .mockResolvedValue(channel);
    const update = vi
      .spyOn(api, "updateNotificationChannel")
      .mockResolvedValue({ ...channel, recordVersion: 4 });
    const test = vi.spyOn(api, "testNotificationChannel").mockResolvedValue({
      channelId: channel.id,
      success: true,
      testedAt: "2026-08-09T02:00:00Z",
    });
    const remove = vi
      .spyOn(api, "deleteNotificationChannel")
      .mockResolvedValue(undefined);
    vi.spyOn(window, "prompt").mockReturnValue(channel.name);

    const { container } = render(<HAOperationsPage cluster={cluster} />);
    expect(await screen.findByText("https://hooks.example.test")).toBeTruthy();
    expect(screen.queryByText(/token=/)).toBeNull();
    const notifications = screen
      .getByRole("heading", { name: "Notifications" })
      .closest(".settings-group");
    expect(
      notifications?.querySelector(".settings-group__body--padded"),
    ).toBeTruthy();

    await user.click(screen.getByRole("button", { name: "Edit" }));
    expect(container.querySelector("form.panel-form")).toBeTruthy();
    const name = screen.getByLabelText(/Channel name/);
    await user.clear(name);
    await user.type(name, "Operations renamed");
    expect(screen.queryByLabelText(/HTTPS webhook URL/)).toBeNull();
    await user.click(screen.getByRole("button", { name: "Save webhook" }));
    await waitFor(() =>
      expect(update).toHaveBeenCalledWith(channel.id, {
        name: "Operations renamed",
        enabled: true,
        recordVersion: 3,
      }),
    );

    await user.click(screen.getByRole("button", { name: "Edit" }));
    await user.click(screen.getByLabelText("Replace destination secret"));
    await user.type(
      screen.getByLabelText(/HTTPS webhook URL/),
      "https://replacement.example.test/private?token=new-hidden",
    );
    await user.click(screen.getByRole("button", { name: "Save webhook" }));
    await waitFor(() =>
      expect(update).toHaveBeenCalledWith(channel.id, {
        name: channel.name,
        enabled: true,
        recordVersion: 3,
        destination:
          "https://replacement.example.test/private?token=new-hidden",
        replaceDestination: true,
      }),
    );

    await user.click(screen.getByRole("button", { name: "Disable" }));
    await waitFor(() =>
      expect(update).toHaveBeenCalledWith(channel.id, {
        name: channel.name,
        enabled: false,
        recordVersion: 3,
      }),
    );
    await user.click(screen.getByRole("button", { name: "Test" }));
    await waitFor(() => expect(test).toHaveBeenCalledWith(channel.id));
    expect(
      await screen.findByText(/endpoint accepted the bounded test/i),
    ).toBeTruthy();

    await user.click(screen.getByRole("button", { name: "Delete" }));
    await waitFor(() =>
      expect(remove).toHaveBeenCalledWith(channel.id, 3, channel.name),
    );

    await user.click(screen.getByRole("button", { name: "Add webhook" }));
    await user.type(screen.getByLabelText(/Channel name/), "Pager");
    await user.type(
      screen.getByLabelText(/HTTPS webhook URL/),
      "https://pager.example.test/hook?token=hidden",
    );
    await user.click(
      screen.getByRole("button", { name: "Add encrypted webhook" }),
    );
    await waitFor(() =>
      expect(create).toHaveBeenCalledWith(cluster.id, {
        name: "Pager",
        destination: "https://pager.example.test/hook?token=hidden",
        enabled: true,
      }),
    );
  }, 15_000);

  it("shows bounded loading and a retryable error when the node no longer exists", async () => {
    vi.spyOn(api, "nodes").mockResolvedValue({
      items: [],
      refreshedAt: "2026-08-09T01:00:00Z",
      staleAfterSeconds: 90,
    });
    vi.spyOn(api, "nodeLifecycle").mockResolvedValue({} as never);
    vi.spyOn(api, "maintenancePreflight").mockResolvedValue({} as never);
    vi.spyOn(api, "upgrades").mockResolvedValue({ items: [] });

    render(<NodeLifecyclePage cluster={cluster} nodeId={node.id} />);
    expect(screen.getByText("Loading node detail…")).toBeTruthy();
    expect(
      await screen.findByText(
        "This managed node no longer exists in the selected cluster.",
      ),
    ).toBeTruthy();
    expect(screen.getByRole("button", { name: "Try again" })).toBeTruthy();
  });

  it("uses the common responsive page system and shows blocking DHCP maintenance preflight", async () => {
    commonMocks();
    vi.spyOn(api, "nodeLifecycle").mockResolvedValue({
      generatedAt: "2026-08-09T01:00:00Z",
      settings: {
        nodeId: node.id,
        dnsProbeHost: "",
        dnsProbePort: 53,
        dnsProbeName: ".",
        dnsProbeType: "NS",
        expectedRcode: 0,
        probeUdp: true,
        probeTcp: true,
        installationType: "docker",
        recordVersion: 1,
        createdAt: "2026-08-09T00:00:00Z",
        updatedAt: "2026-08-09T00:00:00Z",
      },
      dns: {
        id: "44444444-4444-4444-8444-444444444444",
        clusterId: cluster.id,
        nodeId: node.id,
        status: "healthy",
        udpStatus: "healthy",
        tcpStatus: "healthy",
        responseCode: 0,
        latencyMs: 3,
        probedAt: "2026-08-09T01:00:00Z",
      },
      certificate: { nodeId: node.id, nodeName: node.name, state: "healthy" },
      version: {
        nodeId: node.id,
        nodeName: node.name,
        installedVersion: "v0.107.78",
        compatibility: "supported",
        installationType: "docker",
        upgradeSupport: "guided",
        updateAvailable: false,
        releaseCheckStale: false,
      },
      events: [],
    });
    vi.spyOn(api, "maintenancePreflight").mockResolvedValue({
      nodeId: node.id,
      allowed: false,
      breakGlassRequired: false,
      healthyDnsNodesRemaining: 1,
      expectedRedundancy: "at_risk",
      activeDeployment: false,
      openDrift: false,
      activeDhcp: true,
      checks: [
        {
          name: "dhcp",
          status: "fail",
          required: true,
          message: "Complete a handoff",
        },
      ],
    });
    const { container } = render(
      <NodeLifecyclePage cluster={cluster} nodeId={node.id} />,
    );
    expect(
      await screen.findByRole("heading", { name: "Maintenance and DHCP" }),
    ).toBeTruthy();
    expect(container.querySelector(".page-container--wide")).toBeTruthy();
    expect(
      container.querySelectorAll(".settings-group").length,
    ).toBeGreaterThanOrEqual(7);
    expect(
      container.querySelectorAll(".settings-group__body--padded").length,
    ).toBeGreaterThanOrEqual(7);
    expect(container.querySelector("form.panel-form")).toBeTruthy();
    expect(
      screen
        .getByRole("link", { name: "Configuration Control" })
        .getAttribute("href"),
    ).toBe("/ha/configuration");
    expect(
      screen.getByRole("link", { name: "View drift" }).getAttribute("href"),
    ).toBe("/ha/drift");
    expect(
      screen
        .getByRole("link", { name: "View deployments" })
        .getAttribute("href"),
    ).toBe("/ha/deployments");
    expect(
      screen.getByRole("link", { name: "Statistics" }).getAttribute("href"),
    ).toBe("/statistics");
    expect(
      screen.getByRole("link", { name: "Query Log" }).getAttribute("href"),
    ).toBe("/query-log");
    expect(screen.getByText("Handoff required")).toBeTruthy();
    expect(
      (
        screen.getByRole("button", {
          name: "Enter maintenance",
        }) as HTMLButtonElement
      ).disabled,
    ).toBe(true);
    const accessibility = await axe.run(container, {
      runOnly: { type: "tag", values: ["wcag2a", "wcag2aa", "wcag21aa"] },
      rules: { "color-contrast": { enabled: false } },
    });
    expect(accessibility.violations).toEqual([]);
  });
});
