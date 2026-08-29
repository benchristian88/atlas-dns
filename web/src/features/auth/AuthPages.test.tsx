// @vitest-environment jsdom

import { cleanup, render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import axe from "axe-core";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { api } from "../../lib/api";
import { ThemeProvider } from "../../theme/ThemeProvider";
import { installMatchMedia } from "../../theme/testMatchMedia";
import { LoginPage, MFAChallengePage } from "./AuthPages";

beforeEach(() => installMatchMedia(false));

afterEach(() => {
  cleanup();
  vi.restoreAllMocks();
  window.localStorage.clear();
  window.sessionStorage.clear();
  window.history.replaceState({}, "", "/");
});

const user = {
  id: "11111111-1111-4111-8111-111111111111",
  email: "operator@example.test",
  displayName: "Operator",
  role: "administrator" as const,
};

describe("MFA authentication", () => {
  it("moves valid password login into an ephemeral MFA challenge", async () => {
    const interaction = userEvent.setup();
    vi.spyOn(api, "login").mockResolvedValue({
      mfaRequired: true,
      mfaChallenge: "opaque-challenge-value",
      challengeExpiresAt: "2026-08-26T04:05:00Z",
    });
    const onMFARequired = vi.fn();
    render(
      <ThemeProvider>
        <LoginPage onAuthenticated={vi.fn()} onMFARequired={onMFARequired} />
      </ThemeProvider>,
    );
    await interaction.type(
      screen.getByLabelText("Email"),
      "operator@example.test",
    );
    await interaction.type(
      screen.getByLabelText("Password"),
      "current secure password",
    );
    await interaction.click(screen.getByRole("button", { name: "Sign in" }));
    await waitFor(() =>
      expect(onMFARequired).toHaveBeenCalledWith(
        "opaque-challenge-value",
        "2026-08-26T04:05:00Z",
      ),
    );
    expect(window.location.href).not.toContain("opaque-challenge-value");
    expect(JSON.stringify(window.localStorage)).not.toContain(
      "opaque-challenge-value",
    );
    expect(JSON.stringify(window.sessionStorage)).not.toContain(
      "opaque-challenge-value",
    );
  });

  it("verifies TOTP, switches to recovery mode, and cancels safely", async () => {
    const interaction = userEvent.setup();
    const verify = vi
      .spyOn(api, "verifyMFA")
      .mockRejectedValueOnce(new Error("Authentication code is invalid."))
      .mockResolvedValue({ user, expiresAt: "2026-08-26T16:00:00Z" });
    const recovery = vi
      .spyOn(api, "verifyMFARecovery")
      .mockResolvedValue({ user, expiresAt: "2026-08-26T16:00:00Z" });
    const cancel = vi.spyOn(api, "cancelMFA").mockResolvedValue();
    const onAuthenticated = vi.fn();
    const onCancel = vi.fn();
    const { container, rerender } = render(
      <ThemeProvider>
        <MFAChallengePage
          challenge="opaque-challenge"
          expiresAt="2026-08-26T04:05:00Z"
          onAuthenticated={onAuthenticated}
          onCancel={onCancel}
        />
      </ThemeProvider>,
    );
    await interaction.type(
      screen.getByLabelText("Authenticator code"),
      "000000",
    );
    await interaction.click(screen.getByRole("button", { name: "Verify" }));
    expect(await screen.findByText(/invalid/i)).toBeTruthy();
    expect(verify).toHaveBeenCalledWith("opaque-challenge", "000000");

    await interaction.click(
      screen.getByRole("button", { name: "Use a recovery code" }),
    );
    await interaction.type(
      screen.getByLabelText("Recovery code"),
      "aaaaa-bbbbb-ccccc-ddddd",
    );
    await interaction.click(screen.getByRole("button", { name: "Verify" }));
    await waitFor(() => expect(onAuthenticated).toHaveBeenCalledWith(user));
    expect(recovery).toHaveBeenCalledWith(
      "opaque-challenge",
      "aaaaa-bbbbb-ccccc-ddddd",
    );

    rerender(
      <ThemeProvider>
        <MFAChallengePage
          challenge="second-challenge"
          expiresAt="2026-08-26T04:05:00Z"
          onAuthenticated={onAuthenticated}
          onCancel={onCancel}
        />
      </ThemeProvider>,
    );
    await interaction.click(screen.getByRole("button", { name: "Cancel" }));
    await waitFor(() =>
      expect(cancel).toHaveBeenCalledWith("second-challenge"),
    );
    expect(onCancel).toHaveBeenCalled();
    const result = await axe.run(container, {
      rules: { "color-contrast": { enabled: false } },
    });
    expect(result.violations).toEqual([]);
  });
});
