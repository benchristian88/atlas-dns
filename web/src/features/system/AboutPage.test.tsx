// @vitest-environment jsdom

import { cleanup, render, screen } from "@testing-library/react";
import axe from "axe-core";
import { afterEach, describe, expect, it, vi } from "vitest";
import { api } from "../../lib/api";
import { AboutPage } from "./AboutPage";

afterEach(() => {
  cleanup();
  vi.restoreAllMocks();
});

describe("AboutPage", () => {
  it("shows consistent product, build, schema, compatibility, and attribution", async () => {
    vi.spyOn(api, "versionInfo").mockResolvedValue({
      version: "0.9.0-dev",
      commit: "abc123",
      builtAt: "2026-08-09T00:00:00Z",
      development: true,
      databaseSchemaVersion: 13,
    });
    const { container } = render(<AboutPage />);
    expect(await screen.findByText("0.9.0-dev")).toBeTruthy();
    expect(screen.getByText("13")).toBeTruthy();
    expect(screen.getByText(/independent project/)).toBeTruthy();
    const sections = screen.getAllByRole("heading", { level: 2 });
    expect(sections.map((heading) => heading.textContent)).toEqual([
      "About Atlas Project",
      "Installation / Build Information",
      "Licensing / Attribution",
    ]);
    expect(container.querySelector(".about-metadata")).toBeTruthy();
    expect(
      screen.getByText("Administration", { selector: ".eyebrow" }),
    ).toBeTruthy();
    const result = await axe.run(container, {
      runOnly: { type: "tag", values: ["wcag2a", "wcag2aa", "wcag21aa"] },
      rules: { "color-contrast": { enabled: false } },
    });
    expect(result.violations).toEqual([]);
  });
});
