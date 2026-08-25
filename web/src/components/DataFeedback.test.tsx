// @vitest-environment jsdom

import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import {
  ConvergenceSummary,
  DataTable,
  HealthSummaryCard,
  MetricCard,
  NodeBadge,
  Pagination,
  PartialSuccessPanel,
  ProgressTimeline,
  RevisionBadge,
  StructuredDiff,
} from "./DataDisplay";
import {
  Banner,
  EmptyState,
  ErrorState,
  LoadingSkeleton,
  Toast,
} from "./Feedback";
import { StatusBadge } from "./StatusBadge";

afterEach(cleanup);

describe("data primitives", () => {
  const columns = [
    {
      id: "name",
      header: "Name",
      render: (row: { id: string; name: string }) => row.name,
    },
  ] as const;

  it("renders DataTable loading, empty, error, stale, and populated states", () => {
    const { rerender } = render(
      <DataTable
        columns={columns}
        rows={[]}
        rowKey={(row) => row.id}
        loading
      />,
    );
    expect(screen.getByRole("status").getAttribute("aria-label")).toBe(
      "Loading table…",
    );
    rerender(
      <DataTable
        columns={columns}
        rows={[]}
        rowKey={(row) => row.id}
        emptyTitle="Nothing found"
      />,
    );
    expect(
      screen.getByRole("heading", { name: "Nothing found" }),
    ).not.toBeNull();
    rerender(
      <DataTable
        columns={columns}
        rows={[]}
        rowKey={(row) => row.id}
        error={new Error("offline")}
      />,
    );
    expect(screen.getByRole("alert").textContent).toContain("offline");
    rerender(
      <DataTable
        columns={columns}
        rows={[{ id: "a", name: "Primary" }]}
        rowKey={(row) => row.id}
        stale
      />,
    );
    expect(screen.getByRole("table")).not.toBeNull();
    expect(screen.getByText("Showing stale data")).not.toBeNull();
  });

  it("supports page and cursor-style pagination controls", () => {
    const previous = vi.fn();
    const next = vi.fn();
    render(
      <Pagination
        page={2}
        hasPrevious
        hasNext
        onPrevious={previous}
        onNext={next}
      />,
    );
    fireEvent.click(screen.getByRole("button", { name: "Previous" }));
    fireEvent.click(screen.getByRole("button", { name: "Next" }));
    expect(previous).toHaveBeenCalledOnce();
    expect(next).toHaveBeenCalledOnce();
  });

  it("renders status, node, revision, and convergence badges", () => {
    render(
      <>
        <StatusBadge status="verifying" />
        <NodeBadge name="Primary DNS" status="healthy" />
        <RevisionBadge number={24} active />
        <ConvergenceSummary
          counts={{ converged: 1, pending: 0, drifted: 1, failed: 0 }}
        />
      </>,
    );
    expect(screen.getByText("Verifying")).not.toBeNull();
    expect(screen.getByText("Primary DNS")).not.toBeNull();
    expect(screen.getByText(/Revision #24/)).not.toBeNull();
    expect(screen.getByText("1 of 2 nodes converged")).not.toBeNull();
  });

  it("renders summary metrics with explicit label, value, and detail layout", () => {
    const { container } = render(
      <MetricCard
        label="DNS serving"
        value="2 / 2"
        detail="All managed nodes"
      />,
    );
    const card = container.querySelector(".metric-card");
    expect(card?.tagName).toBe("ARTICLE");
    expect(card?.querySelector("span")?.textContent).toBe("DNS serving");
    expect(card?.querySelector("strong")?.textContent).toBe("2 / 2");
    expect(card?.querySelector("small")?.textContent).toBe("All managed nodes");
  });

  it("renders compact health summaries with icon, status, and detail", () => {
    render(
      <HealthSummaryCard
        icon="dns"
        label="DNS serving"
        value="2 / 2"
        status="healthy"
        detail="nodes serving DNS"
      />,
    );
    const card = screen.getByRole("article", { name: "DNS serving" });
    expect(card.classList.contains("health-summary-card")).toBe(true);
    expect(card.textContent).toContain("2 / 2");
    expect(card.textContent).toContain("Healthy");
    expect(card.textContent).toContain("nodes serving DNS");
  });

  it("renders structured differences, progress, and partial success", () => {
    render(
      <>
        <StructuredDiff
          differences={[
            {
              id: "one",
              section: "DNS",
              field: "enabled",
              before: false,
              after: true,
            },
          ]}
        />
        <ProgressTimeline
          steps={[{ id: "apply", label: "Apply", status: "applying" }]}
        />
        <PartialSuccessPanel
          results={[
            { id: "one", label: "Primary", status: "success" },
            { id: "two", label: "Secondary", status: "failed" },
          ]}
        />
      </>,
    );
    expect(screen.getByText("DNS / enabled")).not.toBeNull();
    expect(screen.getByText("Apply")).not.toBeNull();
    expect(screen.getByText("1 of 2 targets succeeded.")).not.toBeNull();
  });
});

describe("feedback primitives", () => {
  it("renders banner, toast, loading, empty, and error semantics", () => {
    const dismiss = vi.fn();
    const retry = vi.fn();
    render(
      <>
        <Banner tone="warning" title="Stale">
          Old data
        </Banner>
        <Toast tone="success" message="Saved" onDismiss={dismiss} />
        <LoadingSkeleton label="Loading nodes" rows={2} />
        <EmptyState title="No nodes" />
        <ErrorState error={new Error("Unavailable")} retry={retry} />
      </>,
    );
    expect(screen.getByText("Old data")).not.toBeNull();
    expect(screen.getByLabelText("Loading nodes")).not.toBeNull();
    expect(screen.getByRole("heading", { name: "No nodes" })).not.toBeNull();
    fireEvent.click(
      screen.getByRole("button", { name: "Dismiss notification" }),
    );
    fireEvent.click(screen.getByRole("button", { name: "Try again" }));
    expect(dismiss).toHaveBeenCalledOnce();
    expect(retry).toHaveBeenCalledOnce();
  });
});
