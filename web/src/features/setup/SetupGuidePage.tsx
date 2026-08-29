import { useCallback, useEffect, useState } from "react";
import { Banner, ErrorState, Loading } from "../../components/Feedback";
import { PageContainer, PageHeader } from "../../components/Page";
import { api } from "../../lib/api";
import type { Cluster, OnboardingStatus } from "../../lib/types";

type GuideStep = {
  label: string;
  complete: boolean;
  href: string;
  action: string;
  required: boolean;
};

export function SetupGuidePage({ cluster }: { cluster: Cluster }) {
  const [status, setStatus] = useState<OnboardingStatus>();
  const [error, setError] = useState<unknown>();
  const load = useCallback(async () => {
    setError(undefined);
    try {
      setStatus(await api.onboardingStatus(cluster.id));
    } catch (caught) {
      setError(caught);
    }
  }, [cluster.id]);
  useEffect(() => {
    void load();
  }, [load]);

  const steps: GuideStep[] = status
    ? [
        {
          label: "Administrator and controller identity configured",
          complete: status.cluster !== undefined,
          href: "/system/users",
          action: "Review administrators",
          required: true,
        },
        {
          label: "First compatible AdGuard Home node added",
          complete: status.eligibleNodeCount >= 1,
          href: "/ha/nodes",
          action: "Review nodes",
          required: true,
        },
        {
          label: "Topology and schema-v2 configuration validated",
          complete: status.topologyReady,
          href: "/ha/configuration",
          action: "Review observations",
          required: true,
        },
        {
          label: "Initial immutable revision published",
          complete: status.authoritativeReady,
          href: "/ha/revisions",
          action: "Review revisions",
          required: true,
        },
        {
          label: "Redundant node configured",
          complete: status.redundant,
          href: "/ha/nodes",
          action: "Add redundancy",
          required: false,
        },
        {
          label: "Monitoring settings reviewed",
          complete: status.state.monitoringReviewedAt !== undefined,
          href: "/system/settings",
          action: "Review monitoring",
          required: false,
        },
        {
          label: "Webhook notification configured",
          complete: status.notificationCount > 0,
          href: "/ha/notifications",
          action: "Review notifications",
          required: false,
        },
        {
          label: "Initial revision deployed and active",
          complete: Boolean(cluster.activeRevisionId),
          href: "/ha/deployments",
          action: "Deploy safely",
          required: false,
        },
      ]
    : [];

  return (
    <PageContainer size="wide">
      <PageHeader
        eyebrow="Getting started"
        title="Setup Guide"
        description="Reference and follow-up guidance backed by the same canonical status as onboarding."
        primaryAction={
          <a className="button" href="/onboarding">
            {status?.completed ? "Review onboarding" : "Continue onboarding"}
          </a>
        }
      />
      {status?.completed && (
        <Banner tone="success" title="Core setup complete">
          Optional operational recommendations remain visible below and do not
          redefine onboarding completion.
        </Banner>
      )}
      {error !== undefined && (
        <ErrorState error={error} retry={() => void load()} />
      )}
      {!status && error === undefined && (
        <Loading label="Checking setup progress…" />
      )}
      <ol className="setup-guide">
        {steps.map((step) => (
          <li
            key={step.label}
            className={step.complete ? "setup-guide__complete" : ""}
          >
            <span className="setup-guide__mark" aria-hidden="true">
              {step.complete ? "✓" : "×"}
            </span>
            <div className="setup-guide__detail">
              <span className="setup-guide__status">
                {step.complete ? "Complete" : "Incomplete"} ·{" "}
                {step.required ? "Required" : "Recommended"}
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
