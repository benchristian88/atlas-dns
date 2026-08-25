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
    fireEvent.click(await firstSaveButton());

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
    fireEvent.click(await firstSaveButton());

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
      "Operational History retention",
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

  it("groups editable runtime settings and keeps contextual data with its collection", async () => {
    vi.spyOn(api, "systemSettings").mockResolvedValue(settings);

    render(<SystemSettingsPage />);

    const runtimeHeading = await screen.findByRole("heading", {
      name: "Runtime configuration",
    });
    const runtimeCard = runtimeHeading.closest("section");
    expect(runtimeCard).not.toBeNull();
    expect(screen.queryByRole("heading", { name: "Data" })).toBeNull();
    expect(runtimeCard?.textContent).toContain("Stored statistics");
    expect(runtimeCard?.textContent).toContain(
      "32 days detailed; 400 days daily",
    );
    expect(runtimeCard?.textContent).toContain("Current central retention");
    expect(runtimeCard?.textContent).toContain("168h0m0s");
    expect(
      runtimeCard?.contains(
        screen.getByRole("heading", { name: "Clear Operational History" }),
      ),
    ).toBe(true);
    expect(
      runtimeCard?.contains(
        screen.getByRole("button", { name: "Use recommended defaults" }),
      ),
    ).toBe(true);
    expect(
      screen.getAllByRole("button", { name: "Save runtime settings" }),
    ).toHaveLength(2);
    expect(screen.getByRole("heading", { name: "Updates" })).toBeTruthy();
    expect(screen.getByRole("checkbox", { name: "Enabled" })).toBeTruthy();
    for (const removed of [
      "General",
      "Backup & Restore",
      "Operations",
      "Security",
    ]) {
      expect(screen.queryByRole("heading", { name: removed })).toBeNull();
    }
    expect(
      screen.getByText("Administration", { selector: ".eyebrow" }),
    ).toBeTruthy();
  });
});

async function firstSaveButton() {
  const [button] = await screen.findAllByRole("button", {
    name: "Save runtime settings",
  });
  if (!button) throw new Error("expected a Save runtime settings button");
  return button;
}
