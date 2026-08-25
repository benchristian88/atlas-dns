// @vitest-environment jsdom

import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { api } from "../../lib/api";
import type {
  Cluster,
  OnboardingStatus,
  SystemSettings,
} from "../../lib/types";
import { SystemSettingsPage } from "./SystemSettingsPage";

afterEach(() => {
  cleanup();
  vi.restoreAllMocks();
});

const cluster = {
  id: "11111111-1111-4111-8111-111111111111",
  name: "Home",
} as Cluster;

const settings: SystemSettings = {
  updateChecksEnabled: true,
  recordVersion: 4,
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
};

describe("SystemSettingsPage", () => {
  it("records monitoring review for the selected cluster after settings save", async () => {
    vi.spyOn(api, "systemSettings").mockResolvedValue(settings);
    vi.spyOn(api, "updateSystemSettings").mockResolvedValue({
      ...settings,
      recordVersion: 5,
    });
    vi.spyOn(api, "onboardingStatus").mockResolvedValue({
      state: { clusterId: cluster.id, recordVersion: 2 },
    } as OnboardingStatus);
    const progress = vi
      .spyOn(api, "updateOnboardingProgress")
      .mockResolvedValue({} as OnboardingStatus);

    render(<SystemSettingsPage cluster={cluster} />);
    fireEvent.click(
      await screen.findByRole("button", { name: "Save runtime settings" }),
    );

    await vi.waitFor(() =>
      expect(progress).toHaveBeenCalledWith(cluster.id, 2, {
        monitoringReviewed: true,
      }),
    );
  });

  it("does not rewrite an existing monitoring acknowledgement", async () => {
    vi.spyOn(api, "systemSettings").mockResolvedValue(settings);
    vi.spyOn(api, "updateSystemSettings").mockResolvedValue({
      ...settings,
      recordVersion: 5,
    });
    vi.spyOn(api, "onboardingStatus").mockResolvedValue({
      state: {
        clusterId: cluster.id,
        recordVersion: 3,
        monitoringReviewedAt: "2026-08-24T00:00:00Z",
      },
    } as OnboardingStatus);
    const progress = vi.spyOn(api, "updateOnboardingProgress");

    render(<SystemSettingsPage cluster={cluster} />);
    fireEvent.click(
      await screen.findByRole("button", { name: "Save runtime settings" }),
    );

    await vi.waitFor(() => expect(api.onboardingStatus).toHaveBeenCalled());
    expect(progress).not.toHaveBeenCalled();
  });

  it("requires the exact confirmation before clearing Operational History", async () => {
    vi.spyOn(api, "systemSettings").mockResolvedValue(settings);
    const clear = vi.spyOn(api, "clearOperationalHistory").mockResolvedValue({
      eventsDeleted: 4,
      deliveriesDeleted: 2,
    });
    render(<SystemSettingsPage />);
    for (const group of [
      "Session & Security",
      "Node Monitoring",
      "Statistics Collection",
      "Query Log Collection",
      "Operational History",
    ]) {
      expect(await screen.findByRole("heading", { name: group })).toBeTruthy();
    }
    expect(screen.queryByRole("option", { name: /Forever/i })).toBeNull();
    const button = await screen.findByRole("button", {
      name: "Clear Operational History",
    });
    expect(button.hasAttribute("disabled")).toBe(true);
    fireEvent.change(screen.getByLabelText(/Type CLEAR OPERATIONAL HISTORY/), {
      target: { value: "CLEAR OPERATIONAL HISTORY" },
    });
    expect(button.hasAttribute("disabled")).toBe(false);
    fireEvent.click(button);
    await vi.waitFor(() =>
      expect(clear).toHaveBeenCalledWith("CLEAR OPERATIONAL HISTORY"),
    );
    expect((await screen.findByRole("status")).textContent).toContain(
      "Audit Log entries were retained",
    );
  });
});
