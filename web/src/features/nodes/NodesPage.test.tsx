// @vitest-environment jsdom

import {
  cleanup,
  fireEvent,
  render,
  screen,
  waitFor,
} from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { api } from "../../lib/api";
import type { Cluster, Node } from "../../lib/types";
import { NodesPage } from "./NodesPage";

const cluster: Cluster = {
  id: "11111111-1111-4111-8111-111111111111",
  name: "Home",
  description: "",
  version: 1,
  reconciliationPolicy: "manual",
  createdAt: "2026-08-16T00:00:00Z",
  updatedAt: "2026-08-16T00:00:00Z",
};

const node: Node = {
  id: "22222222-2222-4222-8222-222222222222",
  clusterId: cluster.id,
  name: "Primary DNS",
  baseUrl: "https://dns.example.test",
  certificatePolicy: "system",
  enabled: true,
  healthStatus: "healthy",
  compatibilityStatus: "supported",
  version: "v0.107.78",
  maintenanceMode: false,
  convergenceStatus: "converged",
  recordVersion: 4,
  createdAt: "2026-08-16T00:00:00Z",
  updatedAt: "2026-08-16T00:00:00Z",
};

afterEach(() => {
  cleanup();
  vi.restoreAllMocks();
});

function mockSupportingRequests() {
  vi.spyOn(api, "configurationInventory").mockResolvedValue({
    schemaVersion: 2,
    snapshots: [],
    capabilities: [],
  });
  vi.spyOn(api, "configurationRevisions").mockResolvedValue({ items: [] });
  vi.spyOn(api, "driftEvents").mockResolvedValue({ items: [] });
}

function nodeResponse(value: Node) {
  return {
    items: [value],
    refreshedAt: "2026-08-16T00:00:00Z",
    staleAfterSeconds: 60,
  };
}

describe("Nodes page ownership", () => {
  it("uses the Nodes-only five-card summary layout", async () => {
    mockSupportingRequests();
    vi.spyOn(api, "nodes").mockResolvedValue(nodeResponse(node));

    render(<NodesPage cluster={cluster} />);

    const summary = await screen.findByLabelText("Cluster node summary");
    expect(summary.classList.contains("convergence-summary--five")).toBe(true);
    expect(summary.querySelectorAll("dl > div")).toHaveLength(5);
  });

  it("routes existing-node operational work to exact Node Detail", async () => {
    mockSupportingRequests();
    vi.spyOn(api, "nodes").mockResolvedValue(nodeResponse(node));
    const testConnection = vi.spyOn(api, "testNode");
    const preflight = vi.spyOn(api, "maintenancePreflight");
    const enter = vi.spyOn(api, "enterMaintenance");
    const leave = vi.spyOn(api, "returnToService");

    render(<NodesPage cluster={cluster} />);
    const manage = await screen.findByRole("link", { name: "Manage" });
    expect(manage.getAttribute("href")).toBe(`/ha/nodes/${node.id}`);
    expect(screen.queryByRole("button", { name: "Test" })).toBeNull();
    expect(screen.queryByRole("button", { name: "Maintenance" })).toBeNull();
    expect(
      screen.queryByRole("button", { name: "Leave maintenance" }),
    ).toBeNull();
    expect(testConnection).not.toHaveBeenCalled();
    expect(preflight).not.toHaveBeenCalled();
    expect(enter).not.toHaveBeenCalled();
    expect(leave).not.toHaveBeenCalled();
  });

  it("keeps candidate validation for add and probed save for edit", async () => {
    mockSupportingRequests();
    vi.spyOn(api, "nodes").mockResolvedValue(nodeResponse(node));
    const validate = vi.spyOn(api, "validateNodeCandidate").mockResolvedValue({
      version: "v0.107.78",
      compatibility: "supported",
      onboardingCompatibility: "supported",
      running: true,
      latencyMs: 4,
    });
    const create = vi.spyOn(api, "createNode").mockResolvedValue(node);
    const update = vi.spyOn(api, "updateNode").mockResolvedValue(node);

    render(<NodesPage cluster={cluster} />);
    fireEvent.click(await screen.findByRole("button", { name: "Add node" }));
    fireEvent.change(screen.getByLabelText("Name"), {
      target: { value: "Candidate" },
    });
    fireEvent.change(screen.getByLabelText("Administration URL"), {
      target: { value: "https://candidate.example.test" },
    });
    fireEvent.change(screen.getByLabelText("Username"), {
      target: { value: "admin" },
    });
    fireEvent.change(screen.getByLabelText("Password"), {
      target: { value: "secret" },
    });
    fireEvent.click(screen.getByRole("button", { name: "Test and save" }));
    await waitFor(() => expect(validate).toHaveBeenCalled());
    expect(create).toHaveBeenCalled();

    fireEvent.click(await screen.findByRole("button", { name: "Edit" }));
    fireEvent.click(screen.getByRole("button", { name: "Test and save" }));
    await waitFor(() =>
      expect(update).toHaveBeenCalledWith(node.id, expect.any(Object)),
    );
  });
});
