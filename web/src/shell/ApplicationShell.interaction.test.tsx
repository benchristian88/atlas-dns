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
import { api } from "../lib/api";
import type {
  Cluster,
  ConfigurationRevision,
  Deployment,
  Node,
} from "../lib/types";
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
}: {
  children?: ReactNode;
  pathname?: string;
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
        clusters={[]}
        pathname={pathname}
        onSelectCluster={() => undefined}
        onLogout={() => undefined}
      >
        {children}
      </ApplicationShell>
    </ThemeProvider>
  );
}

describe("v1.1 application shell", () => {
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
    await interaction.keyboard("{Escape}");
    expect(
      screen.queryByRole("dialog", { name: "Navigation drawer" }),
    ).toBeNull();
    await waitFor(() => expect(document.activeElement).toBe(trigger));
  });

  it("shows cluster, scope, revision, health, refresh, and deployment context", async () => {
    const cluster = {
      id: "cluster-1",
      name: "Home DNS",
      activeRevisionId: "revision-1",
    } as Cluster;
    const node = {
      id: "node-1",
      clusterId: cluster.id,
      name: "Primary",
      enabled: true,
      healthStatus: "healthy",
    } as Node;
    const revision = {
      id: "revision-1",
      revisionNumber: 24,
      active: true,
    } as ConfigurationRevision;
    const deployment = {
      id: "deployment-12345678",
      status: "running",
      nodes: [{ id: "task-1", nodeId: node.id, status: "applying" }],
    } as Deployment;
    vi.spyOn(api, "nodes").mockResolvedValue({
      items: [node],
      refreshedAt: "2026-08-24T07:00:00Z",
      staleAfterSeconds: 60,
    });
    vi.spyOn(api, "configurationRevisions").mockResolvedValue({
      items: [revision],
    });
    vi.spyOn(api, "deployments").mockResolvedValue({ items: [deployment] });
    vi.spyOn(api, "deployment").mockResolvedValue(deployment);

    render(
      <ThemeProvider>
        <ApplicationShell
          user={{
            id: "user-1",
            email: "operator@example.test",
            displayName: "Operator",
            role: "administrator",
          }}
          clusters={[cluster]}
          selected={cluster}
          pathname="/ha/drift"
          onSelectCluster={() => undefined}
          onLogout={() => undefined}
        >
          <p>Page</p>
        </ApplicationShell>
      </ThemeProvider>,
    );

    await waitFor(() => expect(screen.getByText("#24")).toBeTruthy());
    const context = screen.getByRole("region", { name: "Controller context" });
    expect(
      within(context).getByRole("option", { name: "Home DNS" }),
    ).toBeTruthy();
    expect(
      within(context).getByRole("option", { name: "Entire Cluster" }),
    ).toBeTruthy();
    expect(
      within(context).getByRole("option", { name: "Primary" }),
    ).toBeTruthy();
    expect(within(context).getByText("Healthy")).toBeTruthy();
    expect(
      within(context).getByRole("link", { name: "applying Primary" }),
    ).toBeTruthy();
    expect(
      screen
        .getAllByRole("link", { name: "Notifications" })
        .some((link) => link.classList.contains("topbar-icon-button")),
    ).toBe(true);
  });
});
