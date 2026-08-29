import { readFileSync } from "node:fs";
import { describe, expect, it } from "vitest";

const css = readFileSync(new URL("../styles/theme.css", import.meta.url), "utf8");

describe("responsive shell contract", () => {
  it("contains the shell at desktop, tablet, and iPhone-class widths", () => {
    expect(css).toContain(
      "grid-template-columns: var(--atlas-sidebar-width) minmax(0, 1fr)",
    );
    expect(css).toContain("grid-template-rows: minmax(100vh, auto)");
    expect(css).not.toContain(".app-topbar {");
    expect(css).toContain("@media (max-width: 1080px)");
    expect(css).toContain("@media (max-width: 560px)");
    expect(css).toContain(".app-shell > .content { min-width: 0");
    expect(css).not.toMatch(
      /\.app-shell > \.content \{[^}]*overflow-x?: hidden/,
    );
    expect(css).toMatch(
      /\.mobile-shell-header \{[^}]*position: sticky[^}]*top: 0[^}]*display: flex/,
    );
    expect(css).toMatch(
      /\.shell-drawer-toggle \{[^}]*width: 44px[^}]*height: 44px/,
    );
    expect(css).toContain(
      ".mobile-shell-brand .atlas-brand__lockup, .drawer-brand .atlas-brand__lockup { display: block; }",
    );
    expect(css).not.toContain(
      ".app-shell > .content .page-header:first-child .page-header__content",
    );
  });

  it("reflows dashboard cards while keeping wide node data locally scrollable", () => {
    expect(css).toContain("grid-template-columns: repeat(5, minmax(0, 1fr))");
    expect(css).toContain("@media (max-width: 1400px)");
    expect(css).toContain("@media (max-width: 900px)");
    expect(css).toContain(".dashboard-node-table { min-width: 760px; }");
    expect(css).toMatch(
      /\.table-wrap \{[^}]*position: relative[^}]*overflow-x: auto/,
    );
    expect(css).toMatch(
      /\.dashboard-panel-header \{[^}]*flex-wrap: wrap[^}]*gap: var\(--atlas-space-2\) var\(--atlas-space-3\)/,
    );
    expect(css).toMatch(
      /\.dashboard-panel-header > div \{[^}]*min-width: 0[^}]*flex: 1 1 10rem/,
    );
    expect(css).toMatch(
      /\.dashboard-panel-metadata \{[^}]*var\(--atlas-space-1\)[^}]*color: var\(--atlas-text-muted\)/,
    );
    expect(css).toMatch(
      /caption \{[^}]*padding: var\(--atlas-space-3\)[^}]*color: var\(--atlas-text-muted\)/,
    );
    expect(css).toMatch(
      /\.dashboard-health-card__body em \{[^}]*overflow: visible[^}]*white-space: normal/,
    );
  });

  it("keeps mobile drawer groups compact without reducing primary touch targets", () => {
    expect(css).toMatch(
      /\.mobile-drawer nav \{[^}]*align-content: start[^}]*gap: 2px/,
    );
    expect(css).toContain(
      ".mobile-drawer .sidebar-link, .mobile-drawer .sidebar-group__trigger { min-height: 44px; }",
    );
    expect(css).toMatch(
      /\.mobile-drawer \{[^}]*env\(safe-area-inset-top\)[^}]*env\(safe-area-inset-bottom\)[^}]*env\(safe-area-inset-left\)/,
    );
  });

  it("keeps guided onboarding usable at phone widths", () => {
    expect(css).toMatch(
      /@media \(max-width: 620px\)[\s\S]*\.onboarding-progress \{[^}]*overflow-x: auto/,
    );
    expect(css).toContain(".onboarding-source { grid-template-columns: auto minmax(0, 1fr);");
    expect(css).toContain(".onboarding-page { display: grid;");
  });

  it("keeps compact health summaries responsive and theme-token based", () => {
    expect(css).toContain(
      ".health-summary-grid { display: grid; grid-template-columns: repeat(6, minmax(0, 1fr));",
    );
    expect(css).toContain(
      ".health-summary-grid--five { grid-template-columns: repeat(5, minmax(0, 1fr)); }",
    );
    expect(css).toContain(
      ".health-summary-grid--four { grid-template-columns: repeat(4, minmax(0, 1fr)); }",
    );
    expect(css).toContain(
      ".dashboard-health-grid, .health-summary-grid { grid-template-columns: repeat(3, minmax(0, 1fr)); }",
    );
    expect(css).toContain(
      ".dashboard-health-grid, .health-summary-grid { grid-template-columns: repeat(2, minmax(0, 1fr)); }",
    );
    expect(css).toMatch(
      /\.operational-section-heading \{[^}]*flex-wrap: wrap[^}]*border-bottom: 1px solid var\(--atlas-border\)/,
    );
    expect(css).toContain(
      ".operational-section-heading > div:first-child { min-width: min(100%, 280px);",
    );
    expect(css).toContain(
      ".operational-section-heading__aside { flex: 0 0 auto; color: var(--atlas-text-muted);",
    );
  });
});
