// @vitest-environment jsdom

import { cleanup, render } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import axe from "axe-core";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { NotFoundPage } from "./routing/RouteStatePages";
import { ApplicationShell } from "./shell/ApplicationShell";
import { ThemeProvider } from "./theme/ThemeProvider";
import { installMatchMedia } from "./theme/testMatchMedia";

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
});

beforeEach(() => installMatchMedia());

const user = {
  id: "user-1",
  email: "operator@example.test",
  displayName: "Operator",
  role: "administrator" as const,
};

async function expectNoStructuralWcagViolations(container: HTMLElement) {
  const result = await axe.run(container, {
    runOnly: { type: "tag", values: ["wcag2a", "wcag2aa", "wcag21aa"] },
    rules: {
      // jsdom has no layout or computed colour engine. Browser contrast is
      // checked with the committed light/dark visual baselines.
      "color-contrast": { enabled: false },
    },
  });
  expect(result.violations).toEqual([]);
}

describe("WCAG structural regression", () => {
  it("keeps route-state pages free of automated WCAG A/AA violations", async () => {
    const { container } = render(
      <ThemeProvider>
        <ApplicationShell
          user={user}
          pathname="/mistyped"
          onLogout={() => undefined}
        >
          <NotFoundPage pathname="/mistyped" />
        </ApplicationShell>
      </ThemeProvider>,
    );

    await expectNoStructuralWcagViolations(container);
  });

  it("keeps the expanded mobile hierarchy structurally accessible", async () => {
    const interaction = userEvent.setup();
    const { container, getByRole } = render(
      <ThemeProvider>
        <ApplicationShell
          user={user}
          pathname="/statistics"
          onLogout={() => undefined}
        >
          <h1>Statistics</h1>
        </ApplicationShell>
      </ThemeProvider>,
    );
    await interaction.click(getByRole("button", { name: "Open navigation" }));

    await expectNoStructuralWcagViolations(container);
  });
});
