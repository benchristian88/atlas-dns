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
    vi.spyOn(api, "mfaStatus").mockResolvedValue({
      enabled: false,
      recoveryCodesRemaining: 0,
    });
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
        "",
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

  it("enrolls MFA and displays the secret and recovery codes only once", async () => {
    const interaction = userEvent.setup();
    vi.spyOn(api, "mfaStatus").mockResolvedValue({
      enabled: false,
      recoveryCodesRemaining: 0,
    });
    const start = vi.spyOn(api, "startMFAEnrollment").mockResolvedValue({
      secret: "JBSWY3DPEHPK3PXP",
      provisioningUri:
        "otpauth://totp/Atlas%20DNS:operator@example.test?secret=JBSWY3DPEHPK3PXP",
      qrCodeDataUrl: "data:image/png;base64,cXItY29kZQ==",
      issuer: "Atlas DNS",
      accountLabel: "operator@example.test",
      expiresAt: "2026-08-26T04:10:00Z",
    });
    vi.spyOn(api, "verifyMFAEnrollment").mockResolvedValue({
      recoveryCodes: ["aaaaa-bbbbb-ccccc-ddddd", "11111-22222-33333-44444"],
      recoveryCodesRemaining: 10,
    });
    render(<MyAccountPage user={currentUser} />);

    await screen.findByText("Not enabled");
    expect(screen.queryByText("JBSWY3DPEHPK3PXP")).toBeNull();
    await interaction.click(
      screen.getByRole("button", {
        name: "Enable two-factor authentication",
      }),
    );
    await interaction.type(
      document.getElementById("mfa-enroll-password") as HTMLInputElement,
      "current secure password",
    );
    await interaction.click(screen.getByRole("button", { name: "Continue" }));
    await waitFor(() =>
      expect(start).toHaveBeenCalledWith("current secure password"),
    );
    expect(await screen.findByText("JBSWY3DPEHPK3PXP")).toBeTruthy();
    expect(screen.getByRole("img", { name: /QR code/ })).toBeTruthy();
    await interaction.type(
      document.getElementById("mfa-enrollment-code") as HTMLInputElement,
      "123456",
    );
    await interaction.click(
      screen.getByRole("button", { name: "Verify and enable" }),
    );
    expect(await screen.findByText("aaaaa-bbbbb-ccccc-ddddd")).toBeTruthy();
    expect(window.localStorage.length).toBe(0);
    expect(window.sessionStorage.length).toBe(0);
    expect(window.location.href).not.toContain("JBSWY3DPEHPK3PXP");
    await interaction.click(
      screen.getByRole("button", { name: "I have saved these codes" }),
    );
    expect(screen.queryByText("aaaaa-bbbbb-ccccc-ddddd")).toBeNull();
    expect(screen.queryByText("JBSWY3DPEHPK3PXP")).toBeNull();
    expect(screen.getByText("Recovery codes remaining: 10")).toBeTruthy();
  });

  it("requires TOTP for password changes and MFA regeneration and disable", async () => {
    const interaction = userEvent.setup();
    vi.spyOn(api, "mfaStatus").mockResolvedValue({
      enabled: true,
      recoveryCodesRemaining: 7,
    });
    const change = vi.spyOn(api, "changeOwnPassword").mockResolvedValue();
    const regenerate = vi
      .spyOn(api, "regenerateMFARecoveryCodes")
      .mockResolvedValue({
        recoveryCodes: ["aaaaa-bbbbb-ccccc-ddddd"],
        recoveryCodesRemaining: 10,
      });
    const disable = vi.spyOn(api, "disableMFA").mockResolvedValue();
    render(<MyAccountPage user={currentUser} />);
    await screen.findByText("Recovery codes remaining: 7");

    await interaction.type(
      document.getElementById("account-current-password") as HTMLInputElement,
      "current secure password",
    );
    await interaction.type(
      document.getElementById("account-password-totp") as HTMLInputElement,
      "123456",
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
        "123456",
      ),
    );

    await interaction.click(
      screen.getByRole("button", { name: "Regenerate recovery codes" }),
    );
    await interaction.type(
      document.getElementById("mfa-manage-password") as HTMLInputElement,
      "current secure password",
    );
    await interaction.type(
      document.getElementById("mfa-manage-code") as HTMLInputElement,
      "654321",
    );
    await interaction.click(
      screen.getByRole("button", { name: "Regenerate recovery codes" }),
    );
    await waitFor(() =>
      expect(regenerate).toHaveBeenCalledWith(
        "current secure password",
        "654321",
      ),
    );
    await interaction.click(
      screen.getByRole("button", { name: "I have saved these codes" }),
    );

    await interaction.click(
      screen.getByRole("button", {
        name: "Disable two-factor authentication",
      }),
    );
    await interaction.type(
      document.getElementById("mfa-manage-password") as HTMLInputElement,
      "current secure password",
    );
    await interaction.type(
      document.getElementById("mfa-manage-code") as HTMLInputElement,
      "111222",
    );
    await interaction.click(
      screen.getByRole("button", {
        name: "Disable two-factor authentication",
      }),
    );
    await waitFor(() =>
      expect(disable).toHaveBeenCalledWith("current secure password", "111222"),
    );
    expect(await screen.findByText("Not enabled")).toBeTruthy();
  });
});
