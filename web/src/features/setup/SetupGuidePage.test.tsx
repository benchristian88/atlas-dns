// @vitest-environment jsdom

import { cleanup, render, screen } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { api } from "../../lib/api";
import type { Cluster, OnboardingStatus } from "../../lib/types";
import { SetupGuidePage } from "./SetupGuidePage";

afterEach(() => {
  cleanup();
  vi.restoreAllMocks();
});

const cluster = {
  id: "11111111-1111-4111-8111-111111111111",
  name: "Home",
} as Cluster;

function onboardingStatus(
  overrides: Partial<OnboardingStatus> = {},
): OnboardingStatus {
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
      sessionDurationSeconds: 43200,
      nodeHealthIntervalSeconds: 30,
      nodeRequestTimeoutSeconds: 10,
      statisticsPollIntervalSeconds: 3600,
      queryLogCollectionEnabled: true,
      queryLogPollIntervalSeconds: 30,
      queryLogRetentionSeconds: 604800,
      logLevel: "info",
      operationalHistoryRetentionDays: 90,
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

describe("SetupGuidePage", () => {
  it("renders canonical incomplete state and links back to guided onboarding", async () => {
    vi.spyOn(api, "onboardingStatus").mockResolvedValue(onboardingStatus());
    const { container } = render(<SetupGuidePage cluster={cluster} />);

    await screen.findByText("First compatible AdGuard Home node added");
    expect(
      screen
        .getByRole("link", { name: "Continue onboarding" })
        .getAttribute("href"),
    ).toBe("/onboarding");
    expect(container.querySelectorAll(".setup-guide__complete")).toHaveLength(
      1,
    );
  });

  it("keeps redundancy recommended while core setup may continue with a deliberate skip", async () => {
    vi.spyOn(api, "onboardingStatus").mockResolvedValue(
      onboardingStatus({
        eligibleNodeCount: 1,
        nodeCount: 1,
        resumeStep: "secondary_node",
      }),
    );
    const { container } = render(<SetupGuidePage cluster={cluster} />);

    await screen.findByText("Redundant node configured");
    expect(screen.getByRole("link", { name: "Add redundancy" })).toBeTruthy();
    expect(container.querySelectorAll(".setup-guide__complete")).toHaveLength(
      2,
    );
  });

  it("uses completed onboarding status while keeping deployment as separate guidance", async () => {
    vi.spyOn(api, "onboardingStatus").mockResolvedValue(
      onboardingStatus({
        setupRequired: false,
        completed: true,
        resumeStep: "completed",
        eligibleNodeCount: 2,
        nodeCount: 2,
        redundant: true,
        topologyReady: true,
        authoritativeReady: true,
        state: {
          clusterId: cluster.id,
          recordVersion: 4,
          monitoringReviewedAt: "2026-08-24T00:00:00Z",
          completedAt: "2026-08-24T00:01:00Z",
        },
        notificationCount: 1,
        notificationsReady: true,
        canFinish: true,
      }),
    );
    const { container } = render(<SetupGuidePage cluster={cluster} />);

    await screen.findByText("Core setup complete");
    expect(
      screen.getByRole("link", { name: "Review onboarding" }),
    ).toBeTruthy();
    expect(container.querySelectorAll(".setup-guide__complete")).toHaveLength(
      7,
    );
    expect(
      screen.getByText("Initial revision deployed and active").closest("li")
        ?.className,
    ).not.toContain("setup-guide__complete");
  });

  it("keeps loading distinct from an empty result", () => {
    vi.spyOn(api, "onboardingStatus").mockReturnValue(
      new Promise(() => undefined),
    );
    render(<SetupGuidePage cluster={cluster} />);
    expect(screen.getByRole("status").textContent).toContain(
      "Checking setup progress",
    );
  });

  it("renders genuine canonical-status failures with retry", async () => {
    vi.spyOn(api, "onboardingStatus").mockRejectedValue(
      new Error("onboarding status unavailable"),
    );
    render(<SetupGuidePage cluster={cluster} />);
    expect((await screen.findByRole("alert")).textContent).toContain(
      "onboarding status unavailable",
    );
    expect(screen.getByRole("button", { name: "Try again" })).toBeTruthy();
  });
});
