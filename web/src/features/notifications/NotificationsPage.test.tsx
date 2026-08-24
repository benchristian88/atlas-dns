// @vitest-environment jsdom

import { cleanup, render, screen } from "@testing-library/react";
import axe from "axe-core";
import { afterEach, describe, expect, it, vi } from "vitest";
import { api } from "../../lib/api";
import type { Cluster, NotificationChannel } from "../../lib/types";
import { NotificationsPage } from "./NotificationsPage";

afterEach(() => {
  cleanup();
  vi.restoreAllMocks();
});

const cluster = {
  id: "11111111-1111-4111-8111-111111111111",
  name: "Home",
} as Cluster;

describe("NotificationsPage", () => {
  it("renders existing encrypted channel summaries without exposing destinations", async () => {
    vi.spyOn(api, "notificationChannels").mockResolvedValue({
      items: [
        {
          id: "channel-1",
          clusterId: cluster.id,
          name: "Operations",
          channelType: "webhook",
          enabled: true,
          destinationSet: true,
          destinationSummary: "https://hooks.example.test",
          subscribedCategories: ["ha_transition"],
          recordVersion: 1,
          createdAt: "2026-08-24T06:00:00Z",
          updatedAt: "2026-08-24T07:00:00Z",
        } as NotificationChannel,
      ],
    });
    const { container } = render(<NotificationsPage cluster={cluster} />);
    expect(
      await screen.findByRole("heading", { name: "Notifications" }),
    ).toBeTruthy();
    expect(screen.getByText("Operations")).toBeTruthy();
    expect(screen.getByText("https://hooks.example.test")).toBeTruthy();
    expect(screen.queryByText(/secret-token/)).toBeNull();
    const accessibility = await axe.run(container, {
      runOnly: { type: "tag", values: ["wcag2a", "wcag2aa", "wcag21aa"] },
      rules: { "color-contrast": { enabled: false } },
    });
    expect(accessibility.violations).toEqual([]);
  });

  it("renders an explicit empty state", async () => {
    vi.spyOn(api, "notificationChannels").mockResolvedValue({ items: [] });
    render(<NotificationsPage cluster={cluster} />);
    expect(
      await screen.findByRole("heading", { name: "No notification webhooks" }),
    ).toBeTruthy();
  });

  it("renders a retryable initial error", async () => {
    vi.spyOn(api, "notificationChannels").mockRejectedValue(
      new Error("unavailable"),
    );
    render(<NotificationsPage cluster={cluster} />);
    expect(
      await screen.findByRole("heading", {
        name: "Unable to load this content",
      }),
    ).toBeTruthy();
    expect(screen.getByRole("button", { name: "Try again" })).toBeTruthy();
  });
});
