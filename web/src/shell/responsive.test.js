import { readFileSync } from "node:fs";
import { describe, expect, it } from "vitest";

const css = readFileSync(new URL("../styles/theme.css", import.meta.url), "utf8");

describe("responsive shell contract", () => {
  it("contains the shell at desktop, tablet, and iPhone-class widths", () => {
    expect(css).toContain(
      "grid-template-columns: var(--atlas-sidebar-width) minmax(0, 1fr)",
    );
    expect(css).toContain("@media (max-width: 1080px)");
    expect(css).toContain("@media (max-width: 760px)");
    expect(css).toContain("@media (max-width: 560px)");
    expect(css).toContain(".app-shell > .content { min-width: 0");
    expect(css).toContain("overflow: hidden");
  });

  it("reflows dashboard cards while keeping wide node data locally scrollable", () => {
    expect(css).toContain("grid-template-columns: repeat(5, minmax(0, 1fr))");
    expect(css).toContain("@media (max-width: 1400px)");
    expect(css).toContain("@media (max-width: 900px)");
    expect(css).toContain(".dashboard-node-table { min-width: 760px; }");
    expect(css).toMatch(/\.table-wrap \{[^}]*overflow-x: auto/);
  });

  it("keeps guided onboarding usable at phone widths", () => {
    expect(css).toMatch(
      /@media \(max-width: 620px\)[\s\S]*\.onboarding-progress \{[^}]*overflow-x: auto/,
    );
    expect(css).toContain(".onboarding-source { grid-template-columns: auto minmax(0, 1fr);");
    expect(css).toContain(".onboarding-page { display: grid;");
  });
});
