// @vitest-environment jsdom

import {
  cleanup,
  render,
  screen,
  waitFor,
  within,
} from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import axe from "axe-core";
import { afterEach, describe, expect, it, vi } from "vitest";
import { api } from "../../lib/api";
import type { AuditEvent } from "../../lib/types";
import { AuditPage } from "./AuditPage";

const actorID = "11111111-1111-4111-8111-111111111111";
const eventID = "22222222-2222-4222-8222-222222222222";
const nodeID = "33333333-3333-4333-8333-333333333333";

const systemSettingsEvent: AuditEvent = {
  id: eventID,
  actorType: "user",
  actorUserId: actorID,
  actorDisplayName: "Current Admin",
  action: "system_settings.updated",
  resourceType: "system_settings",
  requestId: "44444444-4444-4444-8444-444444444444",
  metadata: {
    queryLogRetentionSeconds: 2_592_000,
    queryLogCollectionEnabled: true,
  },
  createdAt: "2026-08-25T08:00:00Z",
};

afterEach(() => {
  cleanup();
  vi.restoreAllMocks();
  window.history.replaceState(null, "", "/system/audit");
  document.documentElement.removeAttribute("data-theme");
  document.documentElement.removeAttribute("data-theme-preference");
});

function mockPage(
  items: AuditEvent[] = [systemSettingsEvent],
  nextCursor?: string,
) {
  vi.spyOn(api, "auditEvents").mockResolvedValue({
    items,
    nextCursor,
    hasMore: nextCursor !== undefined,
  });
  vi.spyOn(api, "auditEvent").mockImplementation(async (id) => {
    const event = items.find((item) => item.id === id);
    if (!event) throw new Error("not found");
    return event;
  });
}

describe("AuditPage", () => {
  it("expands an adjacent accessible row with typed change and actor evidence", async () => {
    const user = userEvent.setup();
    mockPage();
    const { container } = render(<AuditPage />);

    const disclosure = await screen.findByRole("button", {
      name: "View audit evidence for System settings changed",
    });
    expect(disclosure.getAttribute("aria-expanded")).toBe("false");
    expect(disclosure.textContent).toBe("+");
    await user.click(disclosure);
    expect(disclosure.getAttribute("aria-expanded")).toBe("true");
    expect(disclosure.textContent).toBe("−");
    expect(window.location.search).toBe(`?auditEventId=${eventID}`);

    const detail = document.getElementById(`audit-detail-${eventID}`);
    expect(detail).toBeTruthy();
    expect(
      within(detail as HTMLElement).getByText("Query Log retention"),
    ).toBeTruthy();
    expect(
      within(detail as HTMLElement).getByText("2592000 seconds"),
    ).toBeTruthy();
    expect(within(detail as HTMLElement).getByText(actorID)).toBeTruthy();
    expect(
      within(detail as HTMLElement).getByText("Current Admin"),
    ).toBeTruthy();
    expect(
      within(detail as HTMLElement)
        .getByRole("link", {
          name: "Open resource",
        })
        .getAttribute("href"),
    ).toBe("/system/settings");

    const accessibility = await axe.run(container, {
      runOnly: { type: "tag", values: ["wcag2a", "wcag2aa", "wcag21aa"] },
      rules: { "color-contrast": { enabled: false } },
    });
    expect(accessibility.violations).toEqual([]);
  });

  it("uses a bounded redacted fallback for unknown metadata", async () => {
    const user = userEvent.setup();
    const unknown: AuditEvent = {
      ...systemSettingsEvent,
      id: "55555555-5555-4555-8555-555555555555",
      action: "future.integration_changed",
      resourceType: "integration",
      metadata: {
        safeCode: "SAFE_RESULT",
        password: "must-not-render",
        apiToken: "must-not-render",
        nested: {
          responseBody: "must-not-render",
          webhookDestination: "must-not-render",
          outcome: "complete",
        },
      },
    };
    mockPage([unknown]);
    render(<AuditPage />);
    await user.click(
      await screen.findByRole("button", {
        name: "View audit evidence for Future Integration Changed",
      }),
    );
    expect(screen.getByText("SAFE_RESULT")).toBeTruthy();
    expect(screen.getAllByText("[Redacted]")).toHaveLength(4);
    expect(screen.queryByText("must-not-render")).toBeNull();
  });

  it("preserves a deep-linked selection across refresh and fetches an older exact event", async () => {
    const older = {
      ...systemSettingsEvent,
      id: "66666666-6666-4666-8666-666666666666",
      action: "node.updated",
      resourceType: "node",
      resourceId: nodeID,
      metadata: { name: "Primary", enabled: true },
    };
    window.history.replaceState(
      null,
      "",
      `/system/audit?auditEventId=${older.id}`,
    );
    vi.spyOn(api, "auditEvents").mockResolvedValue({
      items: [systemSettingsEvent],
      hasMore: false,
    });
    vi.spyOn(api, "auditEvent").mockResolvedValue(older);
    const first = render(<AuditPage />);
    expect(
      await screen.findByText("Selected audit event loaded directly"),
    ).toBeTruthy();
    expect(
      screen.getByRole("button", {
        name: "Hide audit evidence for Node updated",
      }),
    ).toBeTruthy();
    expect(
      screen.getByRole("link", { name: "Open resource" }).getAttribute("href"),
    ).toBe(`/ha/nodes/${nodeID}`);
    first.unmount();

    render(<AuditPage />);
    expect(
      await screen.findByRole("button", {
        name: "Hide audit evidence for Node updated",
      }),
    ).toBeTruthy();
    expect(window.location.search).toContain(`auditEventId=${older.id}`);
  });

  it("pages with opaque cursors and returns to the prior stable page", async () => {
    const user = userEvent.setup();
    const auditEvents = vi
      .spyOn(api, "auditEvents")
      .mockImplementation(async (options = {}) =>
        options.cursor === "older-page"
          ? {
              items: [
                {
                  ...systemSettingsEvent,
                  id: "77777777-7777-4777-8777-777777777777",
                  createdAt: "2026-08-24T08:00:00Z",
                },
              ],
              hasMore: false,
            }
          : {
              items: [systemSettingsEvent],
              nextCursor: "older-page",
              hasMore: true,
            },
      );
    vi.spyOn(api, "auditEvent").mockRejectedValue(new Error("not found"));
    render(<AuditPage />);
    await screen.findByText("System settings changed");
    await user.click(screen.getByRole("button", { name: "Next" }));
    await waitFor(() =>
      expect(auditEvents).toHaveBeenLastCalledWith({
        cursor: "older-page",
        limit: 50,
      }),
    );
    expect(
      await screen.findByText("Inspecting older audit evidence"),
    ).toBeTruthy();
    await user.click(screen.getByRole("button", { name: "Previous" }));
    await waitFor(() =>
      expect(auditEvents).toHaveBeenLastCalledWith({ cursor: "", limit: 50 }),
    );
  });

  for (const [preference, resolved, width] of [
    ["light", "light", 1280],
    ["dark", "dark", 390],
    ["system", "dark", 500],
  ] as const) {
    it(`renders detail in ${preference} theme at ${width}px`, async () => {
      Object.defineProperty(window, "innerWidth", {
        value: width,
        configurable: true,
      });
      document.documentElement.dataset.theme = resolved;
      document.documentElement.dataset.themePreference = preference;
      mockPage();
      const user = userEvent.setup();
      const { container } = render(<AuditPage />);
      await user.click(
        await screen.findByRole("button", {
          name: "View audit evidence for System settings changed",
        }),
      );
      expect(container.querySelector(".audit-event-detail")).toBeTruthy();
      expect(document.documentElement.dataset.theme).toBe(resolved);
      expect(document.documentElement.dataset.themePreference).toBe(preference);
    });
  }
});
