// @vitest-environment jsdom

import { cleanup, render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, describe, expect, it, vi } from "vitest";
import { api } from "../lib/api";
import * as browserNavigation from "../lib/browserNavigation";
import type {
  Cluster,
  ConfigurationRevision,
  Deployment,
  DesiredConfigurationDocument,
  DriftEvent,
  Node,
} from "../lib/types";
import { ConfigurationPage } from "./configuration/ConfigurationPage";
import { DeploymentsPage } from "./deployments/DeploymentsPage";
import { DriftPage } from "./drift/DriftPage";
import { RevisionsPage } from "./history/HistoryPage";

const cluster: Cluster = {
  id: "cluster-1",
  name: "Home",
  description: "",
  version: 1,
  reconciliationPolicy: "manual",
  createdAt: "2026-08-09T00:00:00Z",
  updatedAt: "2026-08-09T00:00:00Z",
};

const document = {
  schemaVersion: 2,
  shared: {
    dns: {
      upstreamDns: [],
      bootstrapDns: [],
      fallbackDns: [],
      privateReverseDns: [],
      protectionEnabled: true,
      rateLimit: 0,
      rateLimitSubnetLengthIpv4: 24,
      rateLimitSubnetLengthIpv6: 56,
      rateLimitAllowlist: [],
      blockingMode: "default",
      blockingIpv4: "",
      blockingIpv6: "",
      blockedResponseTtl: 10,
      ednsClientSubnet: false,
      ednsUseCustom: false,
      ednsCustomIp: "",
      disableIpv6: false,
      dnssecEnabled: false,
      cacheSize: 0,
      cacheEnabled: true,
      cacheTtlMin: 0,
      cacheTtlMax: 0,
      cacheOptimistic: false,
      upstreamMode: "",
      usePrivateReverseResolvers: false,
      resolveClients: true,
      upstreamTimeoutSeconds: 10,
    },
    filtering: {
      enabled: true,
      updateIntervalHours: 24,
      filterUrls: [],
      whitelistUrls: [],
      userRules: [],
    },
    clients: [],
    rewritesEnabled: true,
    rewrites: [],
    services: {
      blockedServiceIds: [],
      blockedSchedule: { timeZone: "UTC", days: {} },
      safeBrowsing: false,
      parentalControl: false,
      safeSearch: {
        enabled: false,
        bing: false,
        duckDuckGo: false,
        ecosia: false,
        google: false,
        pixabay: false,
        yandex: false,
        youTube: false,
      },
    },
    queryLog: {
      enabled: true,
      intervalMillis: 86400000,
      anonymizeClientIp: false,
      ignored: [],
      ignoredEnabled: false,
    },
    statistics: {
      enabled: true,
      intervalMillis: 86400000,
      ignored: [],
      ignoredEnabled: false,
    },
  },
  nodeOverrides: {},
  unsupported: [],
} as DesiredConfigurationDocument;

const revision: ConfigurationRevision = {
  id: "revision-42",
  clusterId: cluster.id,
  revisionNumber: 42,
  schemaVersion: 2,
  document,
  canonicalHash: "revision-hash",
  summary: "Harden resolver policy",
  createdBy: "operator-1",
  createdAt: "2026-08-09T01:00:00Z",
  active: false,
  lifecycle: { canArchive: true, canRestore: false, canDelete: true },
};

const deployment: Deployment = {
  id: "deployment-108",
  clusterId: cluster.id,
  revisionId: revision.id,
  status: "running",
  strategy: "sequential",
  failurePolicy: "stop",
  origin: "manual",
  requestId: "request-1",
  cancelRequested: false,
  requestedAt: "2026-08-09T02:00:00Z",
  lifecycle: { canArchive: false, canRestore: false, canDelete: false },
  nodes: [],
};

const node: Node = {
  id: "node-1",
  clusterId: cluster.id,
  name: "Primary DNS",
  baseUrl: "https://dns.test",
  certificatePolicy: "system",
  enabled: true,
  healthStatus: "healthy",
  compatibilityStatus: "supported",
  maintenanceMode: false,
  convergenceStatus: "drifted",
  recordVersion: 1,
  createdAt: "2026-08-09T00:00:00Z",
  updatedAt: "2026-08-09T00:00:00Z",
};

const drift: DriftEvent = {
  id: "drift-1",
  clusterId: cluster.id,
  nodeId: node.id,
  desiredRevisionId: revision.id,
  desiredHash: "desired-hash",
  observedSnapshotId: "snapshot-1",
  observedHash: "observed-hash",
  fingerprint: "fingerprint",
  status: "open",
  policy: "manual",
  reconciliationStatus: "pending",
  differences: [],
  detectedAt: "2026-08-09T03:00:00Z",
  lastSeenAt: "2026-08-09T03:05:00Z",
};

afterEach(() => {
  cleanup();
  vi.restoreAllMocks();
  window.history.replaceState(null, "", "/");
});

function mockNodes(items: Node[] = []) {
  vi.spyOn(api, "nodes").mockResolvedValue({
    items,
    refreshedAt: "2026-08-09T00:00:00Z",
    staleAfterSeconds: 60,
  });
}

describe("revision lifecycle workflow", () => {
  it("uses the exact published revision for a persistent review handoff without deploying", async () => {
    const user = userEvent.setup();
    mockNodes();
    vi.spyOn(api, "configurationInventory").mockResolvedValue({
      schemaVersion: 2,
      snapshots: [],
      capabilities: [],
      draft: {
        id: "draft-1",
        clusterId: cluster.id,
        sourceSnapshotId: "snapshot-1",
        schemaVersion: 2,
        document,
        canonicalHash: "draft-hash",
        version: 7,
        updatedAt: "2026-08-09T00:30:00Z",
      },
    });
    vi.spyOn(api, "configurationRevisions").mockResolvedValue({ items: [] });
    const publish = vi
      .spyOn(api, "publishConfigurationRevision")
      .mockResolvedValue(revision);
    const start = vi.spyOn(api, "startDeployment");

    render(<ConfigurationPage cluster={cluster} />);
    await user.type(
      await screen.findByLabelText("Revision summary"),
      "Harden resolver policy",
    );
    await user.click(
      screen.getByRole("button", { name: "Publish immutable revision" }),
    );

    expect(
      await screen.findByText("Revision #42 published successfully"),
    ).toBeTruthy();
    expect(
      screen
        .getByRole("link", { name: "Review and deploy revision #42" })
        .getAttribute("href"),
    ).toBe("/ha/revisions?revisionId=revision-42");
    expect(publish).toHaveBeenCalledWith(
      cluster.id,
      7,
      "Harden resolver policy",
    );
    expect(start).not.toHaveBeenCalled();
    expect(window.location.pathname).toBe("/");
  });

  it("deep-links the exact revision inline and blocks invalid deployment previews", async () => {
    const user = userEvent.setup();
    window.history.replaceState(
      null,
      "",
      "/ha/revisions?revisionId=revision-42",
    );
    vi.spyOn(api, "configurationRevisions").mockResolvedValue({
      items: [revision],
    });
    vi.spyOn(api, "deployments").mockResolvedValue({ items: [] });
    vi.spyOn(api, "deploymentPreview").mockResolvedValue({
      revisionId: revision.id,
      strategy: "sequential",
      failurePolicy: "stop",
      differences: [],
      restartRequired: false,
      valid: false,
      issues: [
        {
          field: "nodeOverrides.replacement-node-id",
          message:
            "Replacement DNS is not present in this immutable revision; refresh and import its latest observation into the draft, then validate and publish a new revision",
        },
      ],
      nodes: [
        {
          nodeId: "replacement-node-id",
          position: 1,
          effectiveHash: "",
          valid: false,
          warning:
            "This revision has no listener override for the current node identity.",
        },
      ],
    });
    const start = vi.spyOn(api, "startDeployment");

    render(<RevisionsPage cluster={cluster} />);
    const disclosure = await screen.findByRole("button", {
      name: "Hide revision 42 details",
    });
    expect(disclosure.getAttribute("aria-expanded")).toBe("true");
    expect(disclosure.textContent).toBe("−");
    expect(
      disclosure
        .closest("tr")
        ?.nextElementSibling?.classList.contains("table-inline-detail-row"),
    ).toBe(true);
    const full = screen
      .getByText("View full immutable configuration for revision #42")
      .closest("details");
    expect(full?.hasAttribute("open")).toBe(false);

    await user.click(screen.getByRole("button", { name: "Deploy revision" }));
    expect(
      await screen.findByRole("dialog", {
        name: "Review deployment of revision #42",
      }),
    ).toBeTruthy();
    expect(
      await screen.findByText(
        "Replacement DNS is not present in this immutable revision; refresh and import its latest observation into the draft, then validate and publish a new revision",
      ),
    ).toBeTruthy();
    expect(
      screen.queryByText(/nodeOverrides\.replacement-node-id:/),
    ).toBeNull();
    expect(
      screen
        .getByRole("button", { name: "Confirm deployment" })
        .hasAttribute("disabled"),
    ).toBe(true);
    expect(start).not.toHaveBeenCalled();
  });

  it("starts only after confirmation and navigates with the returned deployment ID", async () => {
    const user = userEvent.setup();
    window.history.replaceState(
      null,
      "",
      "/ha/revisions?revisionId=revision-42",
    );
    vi.spyOn(api, "configurationRevisions").mockResolvedValue({
      items: [revision],
    });
    vi.spyOn(api, "deployments").mockResolvedValue({ items: [] });
    vi.spyOn(api, "deploymentPreview").mockResolvedValue({
      revisionId: revision.id,
      strategy: "sequential",
      failurePolicy: "stop",
      differences: [],
      restartRequired: false,
      valid: true,
      issues: [],
      nodes: [],
    });
    const start = vi
      .spyOn(api, "startDeployment")
      .mockResolvedValue(deployment);
    const navigate = vi
      .spyOn(browserNavigation, "navigateTo")
      .mockImplementation(() => undefined);

    render(<RevisionsPage cluster={cluster} />);
    await user.click(
      await screen.findByRole("button", { name: "Deploy revision" }),
    );
    expect(start).not.toHaveBeenCalled();
    await user.click(
      await screen.findByRole("button", { name: "Confirm deployment" }),
    );

    await waitFor(() =>
      expect(start).toHaveBeenCalledWith(cluster.id, revision.id),
    );
    expect(navigate).toHaveBeenCalledWith(
      "/ha/deployments?deploymentId=deployment-108",
    );
  });

  it("uses explicit revision archive and strongly confirmed unused-delete actions", async () => {
    const user = userEvent.setup();
    window.history.replaceState(
      null,
      "",
      "/ha/revisions?revisionId=revision-42",
    );
    vi.spyOn(api, "configurationRevisions").mockResolvedValue({
      items: [revision],
    });
    vi.spyOn(api, "deployments").mockResolvedValue({ items: [] });
    const archive = vi
      .spyOn(api, "archiveConfigurationRevision")
      .mockResolvedValue(undefined);
    const remove = vi
      .spyOn(api, "deleteConfigurationRevision")
      .mockResolvedValue(undefined);
    vi.spyOn(window, "confirm").mockReturnValue(true);
    vi.spyOn(window, "prompt").mockReturnValue("DELETE REVISION #42");

    render(<RevisionsPage cluster={cluster} />);
    await user.click(
      await screen.findByRole("button", { name: "Archive Revision" }),
    );
    await waitFor(() => expect(archive).toHaveBeenCalledWith(revision.id));

    await user.click(
      await screen.findByRole("button", { name: "Delete Unused Revision" }),
    );
    await waitFor(() =>
      expect(remove).toHaveBeenCalledWith(revision.id, "DELETE REVISION #42"),
    );
  });

  it("auto-expands the active deployment only when no explicit selection exists", async () => {
    window.history.replaceState(null, "", "/ha/deployments");
    vi.spyOn(api, "deployments").mockResolvedValue({ items: [deployment] });
    vi.spyOn(api, "deployment").mockResolvedValue(deployment);
    vi.spyOn(api, "configurationRevisions").mockResolvedValue({
      items: [revision],
    });
    mockNodes();
    render(<DeploymentsPage cluster={cluster} />);

    const disclosure = await screen.findByRole("button", {
      name: /Hide deployment/,
    });
    expect(disclosure.getAttribute("aria-expanded")).toBe("true");
    expect(disclosure.textContent).toBe("−");
    expect(
      new URLSearchParams(window.location.search).get("deploymentId"),
    ).toBe(deployment.id);
    expect(screen.getAllByText(/Active deployment/)).toHaveLength(1);
    expect(
      screen.getByRole("link", { name: "View revision" }).getAttribute("href"),
    ).toBe("/ha/revisions?revisionId=revision-42");
  });

  it("archives terminal deployment history and only offers hard delete for an unstarted deployment", async () => {
    const user = userEvent.setup();
    const terminal = {
      ...deployment,
      status: "succeeded",
      lifecycle: { canArchive: true, canRestore: false, canDelete: false },
    } as Deployment;
    window.history.replaceState(
      null,
      "",
      "/ha/deployments?deploymentId=deployment-108",
    );
    vi.spyOn(api, "deployments").mockResolvedValue({ items: [terminal] });
    vi.spyOn(api, "deployment").mockResolvedValue(terminal);
    vi.spyOn(api, "configurationRevisions").mockResolvedValue({
      items: [revision],
    });
    mockNodes();
    const archive = vi
      .spyOn(api, "archiveDeployment")
      .mockResolvedValue(undefined);
    vi.spyOn(window, "confirm").mockReturnValue(true);

    const rendered = render(<DeploymentsPage cluster={cluster} />);
    await user.click(
      await screen.findByRole("button", { name: "Archive Deployment" }),
    );
    await waitFor(() => expect(archive).toHaveBeenCalledWith(terminal.id));
    expect(
      screen.queryByRole("button", { name: "Delete Unstarted Deployment" }),
    ).toBeNull();

    rendered.unmount();
    const unstarted = {
      ...deployment,
      status: "queued",
      lifecycle: { canArchive: false, canRestore: false, canDelete: true },
    } as Deployment;
    vi.mocked(api.deployments).mockResolvedValue({ items: [unstarted] });
    vi.mocked(api.deployment).mockResolvedValue(unstarted);
    const remove = vi
      .spyOn(api, "deleteDeployment")
      .mockResolvedValue(undefined);
    vi.spyOn(window, "prompt").mockReturnValue(
      "DELETE DEPLOYMENT deployment-108",
    );
    render(<DeploymentsPage cluster={cluster} />);
    await user.click(
      await screen.findByRole("button", {
        name: "Delete Unstarted Deployment",
      }),
    );
    await waitFor(() =>
      expect(remove).toHaveBeenCalledWith(
        unstarted.id,
        "DELETE DEPLOYMENT deployment-108",
      ),
    );
  });

  it("presents drift as collapsed rows with exact related-resource links", async () => {
    const user = userEvent.setup();
    window.history.replaceState(null, "", "/ha/drift");
    vi.spyOn(api, "driftEvents").mockResolvedValue({
      items: [{ ...drift, relatedDeploymentId: deployment.id }],
    });
    mockNodes([node]);
    vi.spyOn(api, "configurationInventory").mockResolvedValue({
      schemaVersion: 2,
      snapshots: [],
      capabilities: [],
    });
    render(<DriftPage cluster={cluster} />);

    const disclosure = await screen.findByRole("button", {
      name: "View drift incident details for Primary DNS",
    });
    expect(disclosure.getAttribute("aria-expanded")).toBe("false");
    expect(disclosure.textContent).toBe("+");
    expect(
      screen.queryByRole("button", { name: "Restore desired state" }),
    ).toBeNull();
    await user.click(disclosure);
    expect(disclosure.getAttribute("aria-expanded")).toBe("true");
    expect(disclosure.textContent).toBe("−");
    expect(
      screen.getByRole("button", { name: "Restore desired state" }),
    ).toBeTruthy();
    expect(
      screen
        .getByRole("link", { name: "View desired revision" })
        .getAttribute("href"),
    ).toBe("/ha/revisions?revisionId=revision-42");
    expect(
      screen
        .getByRole("link", { name: /View deployment/ })
        .getAttribute("href"),
    ).toBe("/ha/deployments?deploymentId=deployment-108");
    await waitFor(() =>
      expect(new URLSearchParams(window.location.search).get("driftId")).toBe(
        drift.id,
      ),
    );
  });
});
