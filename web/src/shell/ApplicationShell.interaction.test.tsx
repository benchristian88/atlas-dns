// @vitest-environment jsdom

import {
  cleanup,
  render,
  screen,
  waitFor,
  within,
} from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import type { ReactNode } from "react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { ThemeProvider } from "../theme/ThemeProvider";
import { installMatchMedia } from "../theme/testMatchMedia";
import { ApplicationShell } from "./ApplicationShell";

afterEach(() => {
  cleanup();
  vi.restoreAllMocks();
  vi.unstubAllGlobals();
  window.localStorage.clear();
});

beforeEach(() => installMatchMedia());

function ShellTest({
  children = <p>Page</p>,
  pathname = "/settings/dns",
  onLogout = () => undefined,
}: {
  children?: ReactNode;
  pathname?: string;
  onLogout?: () => void;
}) {
  return (
    <ThemeProvider>
      <ApplicationShell
        user={{
          id: "user-1",
          email: "operator@example.test",
          displayName: "Operator",
          role: "administrator",
        }}
        pathname={pathname}
        onLogout={onLogout}
      >
        {children}
      </ApplicationShell>
    </ThemeProvider>
  );
}

describe("v1.1 application shell", () => {
  it("uses the resolved appearance for sidebar branding", () => {
    window.localStorage.setItem("atlas-dns.theme", "light");
    const light = render(<ShellTest />);
    expect(
      light.container.querySelector<HTMLImageElement>(
        ".app-sidebar .atlas-brand__lockup",
      )?.src,
    ).toContain("atlas-dns-lockup-light.svg");
    light.unmount();

    window.localStorage.setItem("atlas-dns.theme", "dark");
    const dark = render(<ShellTest />);
    expect(
      dark.container.querySelector<HTMLImageElement>(
        ".app-sidebar .atlas-brand__lockup",
      )?.src,
    ).toContain("atlas-dns-lockup-dark.svg");
  });

  it("keeps the active child current and its owning sidebar group open", () => {
    render(<ShellTest />);
    const primary = screen.getByRole("navigation", {
      name: "Primary navigation",
    });
    expect(
      within(primary)
        .getByRole("button", { name: "Settings" })
        .getAttribute("aria-expanded"),
    ).toBe("true");
    expect(
      within(primary)
        .getByRole("link", { name: "DNS" })
        .getAttribute("aria-current"),
    ).toBe("page");
    expect(screen.getByRole("link", { name: "Setup Guide" })).toBeTruthy();
  });

  it("expands groups and moves keyboard focus into the first child", async () => {
    const interaction = userEvent.setup();
    render(<ShellTest />);
    const filters = screen.getByRole("button", { name: "Filters" });
    filters.focus();
    await interaction.keyboard("{ArrowDown}");
    await waitFor(() =>
      expect(document.activeElement?.textContent).toBe("DNS Blocklists"),
    );
    expect(filters.getAttribute("aria-expanded")).toBe("true");
  });

  it("collapses to labelled icons and expands when a group is selected", async () => {
    const interaction = userEvent.setup();
    const { container } = render(<ShellTest />);
    await interaction.click(
      screen.getByRole("button", { name: "Collapse sidebar" }),
    );
    expect(
      container
        .querySelector(".app-shell")
        ?.getAttribute("data-sidebar-collapsed"),
    ).toBe("true");
    expect(window.localStorage.getItem("atlas-dns.sidebar-collapsed")).toBe(
      "true",
    );
    const filters = screen.getByRole("button", { name: "Filters" });
    expect(filters.getAttribute("title")).toBe("Filters");
    await interaction.click(filters);
    expect(
      container
        .querySelector(".app-shell")
        ?.hasAttribute("data-sidebar-collapsed"),
    ).toBe(false);
    expect(filters.getAttribute("aria-expanded")).toBe("true");
  });

  it("uses the same hierarchy in the mobile drawer and restores trigger focus", async () => {
    const interaction = userEvent.setup();
    render(<ShellTest />);
    const trigger = screen.getByRole("button", { name: "Open navigation" });
    await interaction.click(trigger);
    const drawer = screen.getByRole("dialog", { name: "Navigation drawer" });
    const mobileNavigation = within(drawer).getByRole("navigation", {
      name: "Mobile navigation",
    });
    expect(
      within(mobileNavigation)
        .getByRole("button", { name: "Settings" })
        .getAttribute("aria-expanded"),
    ).toBe("true");
    await interaction.click(
      within(mobileNavigation).getByRole("button", { name: "Filters" }),
    );
    expect(
      within(mobileNavigation)
        .getByRole("button", { name: "Filters" })
        .getAttribute("aria-expanded"),
    ).toBe("true");
    expect(
      within(mobileNavigation)
        .getByRole("button", { name: "Settings" })
        .getAttribute("aria-expanded"),
    ).toBe("false");
    await interaction.click(
      within(drawer).getByRole("button", { name: /Operator Administrator/ }),
    );
    expect(
      within(drawer).getByRole("menuitem", { name: "My Account" }),
    ).toBeTruthy();
    expect(
      within(drawer).getByRole("menuitem", { name: "Preferences" }),
    ).toBeTruthy();
    expect(
      within(drawer).getByRole("menuitem", { name: "Sign out" }),
    ).toBeTruthy();
    await interaction.keyboard("{Escape}");
    expect(
      screen.queryByRole("dialog", { name: "Navigation drawer" }),
    ).toBeNull();
    await waitFor(() => expect(document.activeElement).toBe(trigger));
  });

  it("removes the top bar and exposes account destinations at the rail bottom", async () => {
    const interaction = userEvent.setup();
    const logout = vi.fn();
    const { container } = render(
      <ShellTest pathname="/ha/drift" onLogout={logout} />,
    );

    expect(container.querySelector(".app-topbar")).toBeNull();
    expect(container.querySelector(".topbar-context")).toBeNull();
    expect(
      screen.queryByLabelText("Notifications", {
        selector: ".topbar-icon-button",
      }),
    ).toBeNull();
    expect(
      container.querySelector(".content")?.getAttribute("style"),
    ).toBeNull();

    const sidebar = screen.getByRole("complementary", {
      name: "Application sidebar",
    });
    const account = within(sidebar).getByRole("button", {
      name: /Operator Administrator/,
    });
    await interaction.click(account);
    expect(
      within(sidebar).getByRole("menuitem", { name: "My Account" }),
    ).toBeTruthy();
    expect(
      within(sidebar).getByRole("menuitem", { name: "Preferences" }),
    ).toBeTruthy();
    await interaction.click(
      within(sidebar).getByRole("menuitem", { name: "Sign out" }),
    );
    expect(logout).toHaveBeenCalledOnce();
  });
});
