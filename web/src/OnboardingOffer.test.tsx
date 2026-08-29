// @vitest-environment jsdom

import { cleanup, render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, describe, expect, it, vi } from "vitest";
import { OnboardingOffer } from "./App";
import { api } from "./lib/api";
import type { Cluster } from "./lib/types";

const cluster = {
  id: "11111111-1111-4111-8111-111111111111",
  name: "Home DNS",
} as Cluster;

afterEach(() => {
  cleanup();
  vi.restoreAllMocks();
  sessionStorage.clear();
});

describe("OnboardingOffer", () => {
  it("automatically offers incomplete setup and permits a safe local dismissal", async () => {
    const user = userEvent.setup();
    vi.spyOn(api, "onboardingStatus").mockResolvedValue({
      setupRequired: true,
    } as never);
    render(<OnboardingOffer cluster={cluster} />);

    expect(await screen.findByText("Atlas setup is incomplete")).toBeTruthy();
    expect(
      screen.getByRole("link", { name: "Continue setup" }).getAttribute("href"),
    ).toBe("/onboarding");
    await user.click(screen.getByRole("button", { name: "Not now" }));
    expect(screen.queryByText("Atlas setup is incomplete")).toBeNull();
    expect(
      sessionStorage.getItem(`atlas-dns.onboarding-dismissed.${cluster.id}`),
    ).toBe("true");
  });

  it("does not offer onboarding after canonical completion", async () => {
    vi.spyOn(api, "onboardingStatus").mockResolvedValue({
      setupRequired: false,
    } as never);
    render(<OnboardingOffer cluster={cluster} />);
    await Promise.resolve();
    expect(screen.queryByText("Atlas setup is incomplete")).toBeNull();
  });
});
