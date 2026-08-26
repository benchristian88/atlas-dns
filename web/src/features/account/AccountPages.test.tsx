// @vitest-environment jsdom

import { cleanup, render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import axe from "axe-core";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { api } from "../../lib/api";
import { THEME_STORAGE_KEY, ThemeProvider } from "../../theme/ThemeProvider";
import { installMatchMedia } from "../../theme/testMatchMedia";
import { MyAccountPage, PreferencesPage } from "./AccountPages";

const currentUser = {
  id: "11111111-1111-4111-8111-111111111111",
  email: "operator@example.test",
  displayName: "Nicholas Operator",
  role: "administrator" as const,
};

beforeEach(() => installMatchMedia(false));

afterEach(() => {
  cleanup();
  vi.restoreAllMocks();
  window.localStorage.clear();
  document.documentElement.removeAttribute("data-theme");
});

describe("account surfaces", () => {
  it("shows current identity and changes only the signed-in user's password", async () => {
    const interaction = userEvent.setup();
    const change = vi.spyOn(api, "changeOwnPassword").mockResolvedValue();
    const { container } = render(<MyAccountPage user={currentUser} />);

    expect(screen.getByText("Nicholas Operator")).toBeTruthy();
    expect(screen.getByText("operator@example.test")).toBeTruthy();
    expect(screen.getByText("Administrator")).toBeTruthy();
    await interaction.type(
      screen.getByLabelText(/Current password/),
      "current secure password",
    );
    await interaction.type(
      screen.getByLabelText(/^New password/),
      "replacement secure password",
    );
    await interaction.type(
      screen.getByLabelText(/Confirm new password/),
      "replacement secure password",
    );
    await interaction.click(
      screen.getByRole("button", { name: "Change password" }),
    );

    await waitFor(() =>
      expect(change).toHaveBeenCalledWith(
        "current secure password",
        "replacement secure password",
      ),
    );
    expect(
      await screen.findByText(/Other active sessions were revoked/),
    ).toBeTruthy();
    const result = await axe.run(container, {
      rules: { "color-contrast": { enabled: false } },
    });
    expect(result.violations).toEqual([]);
  });

  it("offers only System, Light, and Dark and persists the resolved logo preference model", async () => {
    const interaction = userEvent.setup();
    const { container } = render(
      <ThemeProvider>
        <PreferencesPage />
      </ThemeProvider>,
    );

    expect(screen.getAllByRole("radio")).toHaveLength(3);
    expect(screen.getByRole("radio", { name: /System/ })).toBeTruthy();
    expect(screen.getByRole("radio", { name: /Light/ })).toBeTruthy();
    expect(screen.getByRole("radio", { name: /Dark/ })).toBeTruthy();
    expect(screen.queryByText(/accent|colour picker|color picker/i)).toBeNull();

    await interaction.click(screen.getByRole("radio", { name: /Dark/ }));
    expect(window.localStorage.getItem(THEME_STORAGE_KEY)).toBe("dark");
    expect(document.documentElement.dataset.theme).toBe("dark");
    const result = await axe.run(container, {
      rules: { "color-contrast": { enabled: false } },
    });
    expect(result.violations).toEqual([]);
  });
});
