// @vitest-environment jsdom

import { cleanup, render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import axe from "axe-core";
import { afterEach, describe, expect, it, vi } from "vitest";
import { api } from "../../lib/api";
import type { AdminUser, User } from "../../lib/types";
import { UsersPage } from "./UsersPage";

afterEach(() => {
  cleanup();
  vi.restoreAllMocks();
});

const currentUser: User = {
  id: "11111111-1111-4111-8111-111111111111",
  email: "primary@example.test",
  displayName: "Primary",
  role: "administrator",
};

const primary: AdminUser = {
  ...currentUser,
  enabled: true,
  createdAt: "2026-08-09T00:00:00Z",
  updatedAt: "2026-08-09T00:00:00Z",
  mfaEnabled: true,
};
const secondary: AdminUser = {
  id: "22222222-2222-4222-8222-222222222222",
  email: "secondary@example.test",
  displayName: "Secondary",
  role: "administrator",
  enabled: true,
  createdAt: "2026-08-09T00:00:00Z",
  updatedAt: "2026-08-09T00:00:00Z",
  mfaEnabled: false,
};
const users: AdminUser[] = [primary, secondary];

describe("UsersPage", () => {
  it("shows safe account state, prevents self-disable, and disables another administrator", async () => {
    vi.spyOn(api, "users").mockResolvedValue({ items: users });
    const update = vi
      .spyOn(api, "updateUser")
      .mockResolvedValue({ ...secondary, enabled: false });
    render(<UsersPage currentUser={currentUser} />);

    expect(
      await screen.findByText(
        "secondary@example.test · Administrator · 2FA: Not enabled",
      ),
    ).toBeTruthy();
    const disableButtons = screen.getAllByRole("button", { name: "Disable" });
    expect((disableButtons[0] as HTMLButtonElement).disabled).toBe(true);
    const secondaryDisable = disableButtons.at(1);
    if (secondaryDisable === undefined)
      throw new Error("secondary disable button missing");
    await userEvent.click(secondaryDisable);
    expect(update).toHaveBeenCalledWith(secondary, false);
    expect(await screen.findByText("Secondary disabled.")).toBeTruthy();
  });

  it("has no automated structural WCAG A/AA violations", async () => {
    vi.spyOn(api, "users").mockResolvedValue({ items: users });
    const { container } = render(<UsersPage currentUser={currentUser} />);
    await screen.findByText(
      "secondary@example.test · Administrator · 2FA: Not enabled",
    );
    const result = await axe.run(container, {
      runOnly: { type: "tag", values: ["wcag2a", "wcag2aa", "wcag21aa"] },
      rules: { "color-contrast": { enabled: false } },
    });
    expect(result.violations).toEqual([]);
  });
});
