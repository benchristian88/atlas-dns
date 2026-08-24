// @vitest-environment jsdom

import { cleanup, render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, describe, expect, it, vi } from "vitest";
import { api } from "../../lib/api";
import type { Cluster, OnboardingStatus } from "../../lib/types";
import { OnboardingPage } from "./OnboardingPage";

afterEach(() => {
  cleanup();
  vi.restoreAllMocks();
  sessionStorage.clear();
});

const cluster = {
  id: "11111111-1111-4111-8111-111111111111",
  name: "Home DNS",
  description: "Primary home service",
} as Cluster;

function status(overrides: Partial<OnboardingStatus> = {}): OnboardingStatus {
  return {
    setupRequired: true,
    completed: false,
    resumeStep: "primary_node",
    publicBaseUrl: "https://atlas.example.test",
    cluster,
    state: { clusterId: cluster.id, recordVersion: 0 },
    nodes: [],
    nodeCount: 0,
    eligibleNodeCount: 0,
    redundant: false,
    topologyReady: false,
    authoritativeReady: false,
    monitoring: {
      updateChecksEnabled: true,
      recordVersion: 1,
      nodeHealthIntervalSeconds: 30,
      statisticsPollIntervalSeconds: 3600,
      queryLogCollectionEnabled: true,
      queryLogPollIntervalSeconds: 30,
      queryLogRetentionSeconds: 604800,
      queryLogRetention: "168h0m0s",
      statisticsRetention: "32 days detailed; 400 days daily",
      installationType: "docker",
    },
    notificationCount: 0,
    notificationsReady: false,
    canFinish: false,
    ...overrides,
  };
}

describe("OnboardingPage", () => {
  it("uses the full onboarding card width for initial cluster creation", async () => {
    vi.spyOn(api, "onboardingStatus").mockResolvedValue(status());
    render(<OnboardingPage />);

    const heading = await screen.findByRole("heading", {
      name: "Create a cluster",
    });
    const form = heading.closest("form");
    expect(form?.classList.contains("onboarding-cluster-form")).toBe(true);
    expect(form?.classList.contains("compact-form")).toBe(false);
  });

  it("resumes at the first node and blocks an old AdGuard version before storage", async () => {
    const user = userEvent.setup();
    vi.spyOn(api, "onboardingStatus").mockResolvedValue(status());
    vi.spyOn(api, "validateNodeCandidate").mockResolvedValue({
      version: "v0.107.77",
      compatibility: "supported",
      onboardingCompatibility: "unsupported",
      running: true,
      latencyMs: 8,
    });
    const create = vi.spyOn(api, "createNode");
    render(<OnboardingPage cluster={cluster} />);

    await screen.findByRole("heading", {
      name: "Add the first AdGuard Home node",
    });
    await user.type(screen.getByLabelText("Name"), "Primary");
    await user.clear(screen.getByLabelText("Administration URL"));
    await user.type(
      screen.getByLabelText("Administration URL"),
      "https://dns-one.example.test",
    );
    await user.type(screen.getByLabelText("Username"), "admin");
    await user.type(screen.getByLabelText("Password"), "not-returned");
    await user.click(screen.getByRole("button", { name: "Test and save" }));

    expect((await screen.findByRole("alert")).textContent).toContain(
      "minimum of 0.107.78",
    );
    expect(create).not.toHaveBeenCalled();
    expect(document.body.textContent).not.toContain("not-returned");
  });

  it("records a deliberate single-node skip without duplicating node state", async () => {
    const user = userEvent.setup();
    const initial = status({
      resumeStep: "secondary_node",
      nodeCount: 1,
      eligibleNodeCount: 1,
    });
    vi.spyOn(api, "onboardingStatus").mockResolvedValue(initial);
    const progress = vi
      .spyOn(api, "updateOnboardingProgress")
      .mockResolvedValue({
        ...initial,
        resumeStep: "topology_validation",
        state: {
          ...initial.state,
          recordVersion: 1,
          redundancySkippedAt: "2026-08-24T08:00:00Z",
        },
      });
    render(<OnboardingPage cluster={cluster} />);

    await user.click(
      await screen.findByRole("button", { name: "Continue with one node" }),
    );
    expect(progress).toHaveBeenCalledWith(cluster.id, 0, {
      redundancySkipped: true,
    });
    expect(
      await screen.findByRole("heading", { name: "Validate topology" }),
    ).toBeTruthy();
  });

  it("requires comparison review and explicit source selection before publishing", async () => {
    const user = userEvent.setup();
    const baseline = status({
      resumeStep: "baseline_selection",
      nodeCount: 2,
      eligibleNodeCount: 2,
      redundant: true,
      topologyReady: true,
      nodes: [
        {
          node: {
            id: "node-one",
            name: "Primary",
            version: "v0.107.78",
          } as never,
          onboardingCompatible: true,
          configurationAvailable: true,
          snapshot: { id: "snapshot-one", document: {} } as never,
        },
        {
          node: {
            id: "node-two",
            name: "Secondary",
            version: "v0.107.79",
          } as never,
          onboardingCompatible: true,
          configurationAvailable: true,
          snapshot: { id: "snapshot-two", document: {} } as never,
        },
      ],
    });
    vi.spyOn(api, "onboardingStatus").mockResolvedValue(baseline);
    vi.spyOn(api, "compareConfigurations").mockResolvedValue({
      differences: [{}],
    } as never);
    vi.spyOn(api, "configurationInventory").mockResolvedValue({
      snapshots: [],
    } as never);
    const importDraft = vi
      .spyOn(api, "importConfiguration")
      .mockResolvedValueOnce({ version: 1 } as never)
      .mockResolvedValueOnce({ version: 2 } as never);
    vi.spyOn(api, "validateConfigurationDraft").mockResolvedValue({
      valid: true,
      issues: [],
    } as never);
    const publish = vi
      .spyOn(api, "publishConfigurationRevision")
      .mockResolvedValue({} as never);
    render(<OnboardingPage cluster={cluster} />);

    const establish = await screen.findByRole("button", {
      name: "Use selected node and publish revision",
    });
    expect((establish as HTMLButtonElement).disabled).toBe(true);
    await user.click(screen.getByLabelText(/Secondary/));
    const selectedSource = screen.getByLabelText(/Secondary/).closest("label");
    expect(selectedSource?.classList.contains("is-selected")).toBe(true);
    expect(selectedSource?.textContent).toContain("Selected");
    expect((establish as HTMLButtonElement).disabled).toBe(true);
    await user.click(
      screen.getByRole("button", { name: "Review differences first" }),
    );
    await screen.findByText(/managed or observed differences were found/);
    expect((establish as HTMLButtonElement).disabled).toBe(false);
    await user.click(establish);

    await waitFor(() =>
      expect(publish).toHaveBeenCalledWith(
        cluster.id,
        2,
        expect.stringContaining("Initial authoritative configuration"),
      ),
    );
    expect(importDraft.mock.calls).toEqual([
      [cluster.id, "snapshot-one", 0],
      [cluster.id, "snapshot-two", 1],
    ]);
  });

  it("supports notification skip and safe completed re-entry", async () => {
    const user = userEvent.setup();
    const notifications = status({
      resumeStep: "notifications",
      state: { clusterId: cluster.id, recordVersion: 4 },
    });
    vi.spyOn(api, "onboardingStatus").mockResolvedValue(notifications);
    const progress = vi
      .spyOn(api, "updateOnboardingProgress")
      .mockResolvedValue({
        ...notifications,
        resumeStep: "review",
        state: {
          ...notifications.state,
          recordVersion: 5,
          notificationsSkippedAt: "2026-08-24T08:00:00Z",
        },
      });
    const { unmount } = render(<OnboardingPage cluster={cluster} />);
    await user.click(
      await screen.findByRole("button", { name: "Skip notifications" }),
    );
    expect(progress).toHaveBeenCalledWith(cluster.id, 4, {
      notificationsSkipped: true,
    });
    unmount();

    vi.restoreAllMocks();
    vi.spyOn(api, "onboardingStatus").mockResolvedValue(
      status({
        setupRequired: false,
        completed: true,
        resumeStep: "completed",
      }),
    );
    render(<OnboardingPage cluster={cluster} />);
    expect(
      await screen.findByRole("heading", { name: "Atlas is ready" }),
    ).toBeTruthy();
    expect(screen.getByRole("link", { name: "Open Setup Guide" })).toBeTruthy();
  });
});
