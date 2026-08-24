import { useCallback, useEffect, useState } from "react";
import { ErrorState, Loading } from "../../components/Feedback";
import { PageContainer, PageHeader } from "../../components/Page";
import { api } from "../../lib/api";
import type { Cluster } from "../../lib/types";

type Step = { label: string; complete: boolean; href: string; action: string };
export function SetupGuidePage({ cluster }: { cluster: Cluster }) {
  const [steps, setSteps] = useState<Step[]>();
  const [error, setError] = useState<unknown>();
  const load = useCallback(async () => {
    setSteps(undefined);
    setError(undefined);
    try {
      const [
        nodes,
        inventory,
        revisions,
        deployments,
        statistics,
        queries,
        health,
        operational,
      ] = await Promise.all([
        api.nodes(cluster.id),
        api.configurationInventory(cluster.id),
        api.configurationRevisions(cluster.id),
        api.deployments(cluster.id),
        api.statistics(cluster.id, "24h"),
        api.queryEvents(cluster.id, { limit: 1 }),
        api.haStatus(cluster.id),
        api.operationalStatus(cluster.id),
      ]);
      const nodeItems = nodes.items;
      const draft = inventory.draft;
      const revisionItems = revisions.items;
      const deploymentItems = deployments.items;
      setSteps([
        {
          label: "Administrator configured",
          complete: true,
          href: "/system/users",
          action: "Manage administrators",
        },
        {
          label: "First AdGuard Home node added",
          complete: nodeItems.filter((node) => node.enabled).length >= 1,
          href: "/ha/nodes",
          action: "Add first node",
        },
        {
          label: "Redundant node added",
          complete: nodeItems.filter((node) => node.enabled).length >= 2,
          href: "/ha/nodes",
          action: "Add redundant node",
        },
        {
          label: "Configuration observed",
          complete: inventory.snapshots.length > 0,
          href: "/ha/configuration",
          action: "Observe configuration",
        },
        {
          label: "Desired draft adopted",
          complete: draft !== undefined,
          href: "/ha/configuration",
          action: "Import desired state",
        },
        {
          label: "Immutable revision published",
          complete: revisionItems.length > 0,
          href: "/ha/revisions",
          action: "Validate and publish",
        },
        {
          label: "Active revision deployed",
          complete:
            revisionItems.some((revision) => revision.active) &&
            deploymentItems.some(
              (deployment) => deployment.status === "succeeded",
            ),
          href: "/ha/deployments",
          action: "Deploy revision",
        },
        {
          label: "Statistics current",
          complete: statistics.totals.dnsQueries > 0,
          href: "/statistics",
          action: "Check Statistics",
        },
        {
          label: "Query Log current",
          complete: queries.items.length > 0,
          href: "/query-log",
          action: "Check Query Log",
        },
        {
          label: "HA/DNS health available",
          complete: health.nodes.length > 0,
          href: "/ha/operations",
          action: "Check HA Operations",
        },
        {
          label: "Operational Status available",
          complete: operational.summary !== undefined,
          href: "/system/operational-status",
          action: "Open Operational Status",
        },
      ]);
    } catch (caught) {
      setError(caught);
    }
  }, [cluster.id]);
  useEffect(() => {
    void load();
  }, [load]);
  return (
    <PageContainer size="wide">
      <PageHeader
        eyebrow="Getting started"
        title="Setup Guide"
        description="Completion is derived from controller state, not page visits."
      />
      {error !== undefined && (
        <ErrorState error={error} retry={() => void load()} />
      )}
      {!steps && !error && <Loading label="Checking setup progress…" />}
      <ol className="setup-guide">
        {steps?.map((step) => (
          <li
            key={step.label}
            className={step.complete ? "setup-guide__complete" : ""}
          >
            <span className="setup-guide__mark" aria-hidden="true">
              {step.complete ? "✓" : "×"}
            </span>
            <div className="setup-guide__detail">
              <span className="setup-guide__status">
                {step.complete ? "Complete" : "Incomplete"}
              </span>
              <strong className="setup-guide__label">{step.label}</strong>
            </div>
            <a className="setup-guide__action" href={step.href}>
              {step.action}
            </a>
          </li>
        ))}
      </ol>
    </PageContainer>
  );
}
