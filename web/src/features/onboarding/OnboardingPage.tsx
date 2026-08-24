import {
  type FormEvent,
  type ReactNode,
  useCallback,
  useEffect,
  useMemo,
  useState,
} from "react";
import { Banner, ErrorState, Loading } from "../../components/Feedback";
import { PageHeader } from "../../components/Page";
import { api } from "../../lib/api";
import type {
  Cluster,
  OnboardingStatus,
  OnboardingStep,
  SystemSettings,
} from "../../lib/types";
import { ClusterCreate } from "../clusters/ClusterCreate";
import { NodeForm } from "../nodes/NodesPage";

const orderedSteps: Array<[OnboardingStep, string]> = [
  ["welcome", "Welcome"],
  ["controller_identity", "Controller"],
  ["primary_node", "First node"],
  ["secondary_node", "Redundancy"],
  ["topology_validation", "Topology"],
  ["baseline_selection", "Baseline"],
  ["monitoring", "Monitoring"],
  ["notifications", "Notifications"],
  ["review", "Review"],
];

const categories = [
  "dns",
  "redundancy",
  "certificates",
  "versions",
  "maintenance",
  "upgrades",
];

export function OnboardingPage({
  cluster,
  onClusterCreated,
}: {
  cluster?: Cluster;
  onClusterCreated?: (cluster: Cluster) => void;
}) {
  const [status, setStatus] = useState<OnboardingStatus>();
  const [active, setActive] = useState<OnboardingStep>(
    cluster ? "welcome" : "controller_identity",
  );
  const [initialized, setInitialized] = useState(false);
  const [error, setError] = useState<unknown>();
  const [busy, setBusy] = useState("");
  const [detected, setDetected] = useState("");
  const [sourceSnapshot, setSourceSnapshot] = useState("");
  const [differences, setDifferences] = useState<number>();
  const [comparisonReviewed, setComparisonReviewed] = useState(false);
  const [monitoring, setMonitoring] = useState<SystemSettings>();
  const [webhookName, setWebhookName] = useState("Atlas HA alerts");
  const [webhookURL, setWebhookURL] = useState("");
  const [webhookCategories, setWebhookCategories] =
    useState<string[]>(categories);
  const [webhookResult, setWebhookResult] = useState("");

  const load = useCallback(async () => {
    try {
      const value = await api.onboardingStatus(cluster?.id);
      setStatus(value);
      setMonitoring(value.monitoring);
      if (!initialized) {
        setActive(value.completed ? "completed" : value.resumeStep);
        setInitialized(true);
      }
      setError(undefined);
      return value;
    } catch (caught) {
      setError(caught);
      return undefined;
    }
  }, [cluster?.id, initialized]);

  useEffect(() => {
    void load();
  }, [load]);

  const successfulSources = useMemo(
    () =>
      status?.nodes.filter(
        (item) =>
          item.onboardingCompatible &&
          item.configurationAvailable &&
          item.snapshot?.document,
      ) ?? [],
    [status],
  );

  if (!cluster) {
    return (
      <div className="onboarding-page">
        <PageHeader
          eyebrow="First-run onboarding"
          title="Name your DNS cluster"
          description="A cluster groups the AdGuard Home nodes that provide one resilient DNS service."
        />
        {error !== undefined && <ErrorState error={error} />}
        <ClusterCreate
          layout="onboarding"
          onCreated={(created) => {
            onClusterCreated?.(created);
            window.history.replaceState({}, "", "/onboarding");
          }}
        />
        <a className="button button--quiet" href="/">
          Exit onboarding
        </a>
      </div>
    );
  }

  if (!status && error === undefined)
    return <Loading label="Loading onboarding progress…" />;
  if (!status) return <ErrorState error={error} retry={() => void load()} />;

  const currentIndex = orderedSteps.findIndex(([id]) => id === active);
  const clusterID = cluster.id;
  const currentStatus = status;

  return (
    <div className="onboarding-page">
      <PageHeader
        eyebrow={status.completed ? "Setup review" : "First-run onboarding"}
        title={
          status.completed ? "Atlas setup is complete" : "Set up Atlas DNS"
        }
        description="Guided setup uses the same nodes, observations, drafts, and immutable revisions as the rest of Atlas."
        primaryAction={
          <a className="button button--secondary" href="/">
            Exit safely
          </a>
        }
      />
      <ol className="onboarding-progress" aria-label="Onboarding progress">
        {orderedSteps.map(([id, label], index) => (
          <li
            key={id}
            aria-current={id === active ? "step" : undefined}
            className={index < currentIndex ? "is-complete" : ""}
          >
            <span>{index + 1}</span>
            {label}
          </li>
        ))}
      </ol>
      {error !== undefined && (
        <ErrorState error={error} retry={() => void load()} />
      )}
      {active === "welcome" && (
        <Step title="Welcome to Atlas DNS Controller">
          <p>
            Atlas centrally manages configuration, observes health, aggregates
            telemetry, and coordinates HA operations across AdGuard Home nodes.
            It never proxies DNS, so your nodes continue serving independently.
          </p>
          <Next onClick={() => setActive("controller_identity")} />
        </Step>
      )}
      {active === "controller_identity" && (
        <Step title="Controller identity">
          <dl className="onboarding-summary">
            <div>
              <dt>Cluster display name</dt>
              <dd>{cluster.name}</dd>
            </div>
            <div>
              <dt>Description</dt>
              <dd>{cluster.description || "Not set"}</dd>
            </div>
            <div>
              <dt>Public base URL</dt>
              <dd>{status.publicBaseUrl}</dd>
            </div>
          </dl>
          <p className="muted">
            The public URL remains protected deployment configuration. Atlas
            does not create tenant, site, or multi-controller identities here.
          </p>
          <Next onClick={() => setActive("primary_node")} />
        </Step>
      )}
      {active === "primary_node" && (
        <Step title="Add the first AdGuard Home node">
          {status.eligibleNodeCount > 0 ? (
            <>
              <NodeEvidence status={status} />
              <Next onClick={() => setActive("secondary_node")} />
            </>
          ) : (
            <>
              <p>
                Guided onboarding requires AdGuard Home 0.107.78 or later in the
                compatible 0.107 API generation.
              </p>
              {status.nodeCount > 0 && (
                <>
                  <Banner
                    tone="warning"
                    title="Existing node cannot satisfy onboarding"
                  >
                    The enabled node is unhealthy, below the guided baseline, or
                    outside the evaluated API generation. Correct it, disable
                    it, or add a compatible node before continuing.
                  </Banner>
                  <NodeEvidence status={status} />
                </>
              )}
              {detected && (
                <Banner tone="info" title="Detected node">
                  {detected}
                </Banner>
              )}
              <NodeForm
                cluster={cluster}
                requireOnboardingCompatibility
                onValidated={(result) =>
                  setDetected(
                    `${result.version} · ${result.running ? "DNS running" : "DNS stopped"} · ${result.latencyMs} ms`,
                  )
                }
                onSaved={() => void load()}
              />
            </>
          )}
        </Step>
      )}
      {active === "secondary_node" && (
        <Step title="Add a redundant node">
          {status.redundant ? (
            <>
              <Banner tone="success" title="Redundancy configured">
                Two onboarding-compatible nodes are registered.
              </Banner>
              <Next onClick={() => setActive("topology_validation")} />
            </>
          ) : (
            <>
              <Banner tone="warning" title="One node is not redundant">
                Atlas supports deliberate single-node operation, but it cannot
                tolerate a node outage.
              </Banner>
              <NodeForm
                cluster={cluster}
                requireOnboardingCompatibility
                onSaved={() => void load()}
              />
              <button
                className="button button--secondary"
                type="button"
                disabled={busy !== ""}
                onClick={() =>
                  void progress(
                    { redundancySkipped: true },
                    "topology_validation",
                  )
                }
              >
                Continue with one node
              </button>
            </>
          )}
        </Step>
      )}
      {active === "topology_validation" && (
        <Step title="Validate topology">
          <NodeEvidence status={status} />
          {!status.topologyReady && (
            <Banner tone="warning" title="Configuration observation required">
              Refresh each compatible node so Atlas can validate schema-v2
              configuration and capability availability.
            </Banner>
          )}
          <button
            className="button"
            type="button"
            disabled={busy !== ""}
            onClick={() => void validateTopology()}
          >
            {busy === "topology" ? "Validating…" : "Validate topology"}
          </button>
          {status.topologyReady && (
            <Next onClick={() => setActive("baseline_selection")} />
          )}
        </Step>
      )}
      {active === "baseline_selection" && (
        <Step title="Establish the initial authoritative revision">
          {status.authoritativeReady ? (
            <>
              <Banner tone="success" title="Immutable revision published">
                Revision #{status.revision?.revisionNumber} is established. It
                is not active on nodes until a normal verified deployment.
              </Banner>
              <Next onClick={() => setActive("monitoring")} />
            </>
          ) : (
            <>
              <p>
                Select the node whose shared configuration should become the
                source of truth. Atlas will retain every node&apos;s listener
                identity in the draft.
              </p>
              {successfulSources.map((item) => {
                const selected = sourceSnapshot === item.snapshot?.id;
                return (
                  <label
                    className={`onboarding-source${selected ? " is-selected" : ""}`}
                    key={item.node.id}
                  >
                    <input
                      type="radio"
                      name="baseline-source"
                      value={item.snapshot?.id}
                      checked={sourceSnapshot === item.snapshot?.id}
                      onChange={() =>
                        setSourceSnapshot(item.snapshot?.id ?? "")
                      }
                    />
                    <span className="onboarding-source__details">
                      <strong>{item.node.name}</strong>
                      <small>{item.node.version}</small>
                    </span>
                    <span className="onboarding-source__selection">
                      {selected ? "Selected" : "Select"}
                    </span>
                  </label>
                );
              })}
              {successfulSources.length > 1 && (
                <button
                  className="button button--secondary"
                  type="button"
                  disabled={busy !== ""}
                  onClick={() => void compareSources()}
                >
                  Review differences first
                </button>
              )}
              {differences !== undefined && (
                <Banner
                  tone={differences === 0 ? "success" : "warning"}
                  title={
                    differences === 0
                      ? "Configurations match"
                      : "Configurations differ"
                  }
                >
                  {differences === 0
                    ? "The supported configuration is semantically equal. You must still select a source."
                    : `${differences} managed or observed differences were found. Review Configuration Control later for full field detail.`}
                </Banner>
              )}
              <button
                className="button"
                type="button"
                disabled={
                  busy !== "" ||
                  !sourceSnapshot ||
                  (successfulSources.length > 1 && !comparisonReviewed)
                }
                onClick={() => void establishBaseline()}
              >
                {busy === "baseline"
                  ? "Validating and publishing…"
                  : "Use selected node and publish revision"}
              </button>
            </>
          )}
        </Step>
      )}
      {active === "monitoring" && monitoring && (
        <Step title="Monitoring defaults">
          <form
            className="form-stack"
            onSubmit={(event) => void saveMonitoring(event)}
          >
            <div className="form-grid">
              <DurationInput
                label="Node health interval"
                min={5}
                max={3600}
                value={monitoring.nodeHealthIntervalSeconds}
                onChange={(value) =>
                  setMonitoring({
                    ...monitoring,
                    nodeHealthIntervalSeconds: value,
                  })
                }
              />
              <DurationInput
                label="Statistics poll interval"
                min={60}
                max={86400}
                value={monitoring.statisticsPollIntervalSeconds}
                onChange={(value) =>
                  setMonitoring({
                    ...monitoring,
                    statisticsPollIntervalSeconds: value,
                  })
                }
              />
              <DurationInput
                label="Query Log poll interval"
                min={5}
                max={3600}
                value={monitoring.queryLogPollIntervalSeconds}
                onChange={(value) =>
                  setMonitoring({
                    ...monitoring,
                    queryLogPollIntervalSeconds: value,
                  })
                }
              />
              <DurationInput
                label="Query Log retention"
                min={3600}
                max={7776000}
                value={monitoring.queryLogRetentionSeconds}
                onChange={(value) =>
                  setMonitoring({
                    ...monitoring,
                    queryLogRetentionSeconds: value,
                  })
                }
              />
            </div>
            <label className="checkbox">
              <input
                type="checkbox"
                checked={monitoring.queryLogCollectionEnabled}
                onChange={(event) =>
                  setMonitoring({
                    ...monitoring,
                    queryLogCollectionEnabled: event.target.checked,
                  })
                }
              />{" "}
              Collect Query Log centrally
            </label>
            <div className="row-actions row-actions--start">
              <button className="button" type="submit" disabled={busy !== ""}>
                Save monitoring settings
              </button>
              <button
                className="button button--secondary"
                type="button"
                onClick={() => setMonitoring(recommendedMonitoring(monitoring))}
              >
                Use recommended defaults
              </button>
            </div>
          </form>
        </Step>
      )}
      {active === "notifications" && (
        <Step title="Notifications">
          {status.notificationCount > 0 ? (
            <>
              <Banner tone="success" title="Webhook configured">
                {status.notificationCount} encrypted webhook channel(s) are
                configured.
              </Banner>
              <Next onClick={() => setActive("review")} />
            </>
          ) : (
            <form
              className="form-stack"
              onSubmit={(event) => void saveWebhook(event)}
            >
              <label>
                Channel name
                <input
                  value={webhookName}
                  onChange={(event) => setWebhookName(event.target.value)}
                  required
                />
              </label>
              <label>
                HTTPS webhook URL
                <input
                  type="url"
                  value={webhookURL}
                  onChange={(event) => setWebhookURL(event.target.value)}
                  required
                  autoComplete="off"
                />
              </label>
              <fieldset className="notification-categories">
                <legend>Event defaults</legend>
                {categories.map((category) => (
                  <label className="checkbox" key={category}>
                    <input
                      type="checkbox"
                      checked={webhookCategories.includes(category)}
                      onChange={(event) =>
                        setWebhookCategories((current) =>
                          event.target.checked
                            ? [...current, category]
                            : current.filter((item) => item !== category),
                        )
                      }
                    />{" "}
                    {category}
                  </label>
                ))}
              </fieldset>
              {webhookResult && (
                <Banner tone="info" title="Test Webhook">
                  {webhookResult}
                </Banner>
              )}
              <div className="row-actions row-actions--start">
                <button
                  className="button"
                  type="submit"
                  disabled={busy !== "" || webhookCategories.length === 0}
                >
                  Save and Test Webhook
                </button>
                <button
                  className="button button--secondary"
                  type="button"
                  disabled={busy !== ""}
                  onClick={() =>
                    void progress({ notificationsSkipped: true }, "review")
                  }
                >
                  Skip notifications
                </button>
              </div>
            </form>
          )}
        </Step>
      )}
      {active === "review" && (
        <Step title="Review and finish">
          <dl className="onboarding-summary">
            <div>
              <dt>Nodes</dt>
              <dd>{status.eligibleNodeCount}</dd>
            </div>
            <div>
              <dt>Topology</dt>
              <dd>{status.redundant ? "Redundant" : "Single node"}</dd>
            </div>
            <div>
              <dt>Revision</dt>
              <dd>
                {status.revision
                  ? `#${status.revision.revisionNumber}`
                  : "Missing"}
              </dd>
            </div>
            <div>
              <dt>Monitoring</dt>
              <dd>
                {status.state.monitoringReviewedAt ? "Reviewed" : "Incomplete"}
              </dd>
            </div>
            <div>
              <dt>Notifications</dt>
              <dd>{status.notificationCount > 0 ? "Configured" : "Skipped"}</dd>
            </div>
          </dl>
          <button
            className="button"
            type="button"
            disabled={!status.canFinish || busy !== ""}
            onClick={() => void finish()}
          >
            {busy === "finish" ? "Preparing dashboard…" : "Finish onboarding"}
          </button>
        </Step>
      )}
      {active === "completed" && (
        <Step title="Atlas is ready">
          <Banner tone="success" title="Onboarding complete">
            Setup is recorded using current controller state. Relaunching this
            page is a safe review and never resets nodes or configuration.
          </Banner>
          <div className="row-actions row-actions--start">
            <a className="button" href="/">
              Go to Dashboard
            </a>
            <a className="button button--secondary" href="/ha/nodes">
              Review Nodes
            </a>
            <a className="button button--secondary" href="/ha/configuration">
              Configuration Control
            </a>
            <a className="button button--secondary" href="/setup-guide">
              Open Setup Guide
            </a>
          </div>
        </Step>
      )}
      {currentIndex > 0 && active !== "completed" && (
        <button
          className="button button--quiet onboarding-back"
          type="button"
          onClick={() =>
            setActive(orderedSteps[currentIndex - 1]?.[0] ?? "welcome")
          }
        >
          Back
        </button>
      )}
    </div>
  );

  async function progress(
    input: Parameters<typeof api.updateOnboardingProgress>[2],
    next: OnboardingStep,
  ) {
    setBusy("progress");
    setError(undefined);
    try {
      const value = await api.updateOnboardingProgress(
        clusterID,
        currentStatus.state.recordVersion,
        input,
      );
      setStatus(value);
      setActive(next);
    } catch (caught) {
      setError(caught);
    } finally {
      setBusy("");
    }
  }

  async function validateTopology() {
    setBusy("topology");
    setError(undefined);
    try {
      for (const item of currentStatus.nodes.filter(
        (node) => node.onboardingCompatible,
      ))
        await api.observeNode(item.node.id);
      await load();
    } catch (caught) {
      setError(caught);
    } finally {
      setBusy("");
    }
  }

  async function compareSources() {
    if (successfulSources.length < 2) return;
    setBusy("compare");
    try {
      const left = successfulSources[0]?.snapshot?.id;
      const right = successfulSources[1]?.snapshot?.id;
      if (!left || !right)
        throw new Error("Two successful observations are required.");
      const result = await api.compareConfigurations(left, right);
      setDifferences(result.differences.length);
      setComparisonReviewed(true);
      setError(undefined);
    } catch (caught) {
      setError(caught);
    } finally {
      setBusy("");
    }
  }

  async function establishBaseline() {
    setBusy("baseline");
    setError(undefined);
    try {
      const inventory = await api.configurationInventory(clusterID);
      let version = inventory.draft?.version ?? 0;
      const snapshots = successfulSources
        .map((item) => item.snapshot?.id)
        .filter((value): value is string => Boolean(value));
      const ordered = [
        ...snapshots.filter((id) => id !== sourceSnapshot),
        sourceSnapshot,
      ];
      for (const snapshot of ordered) {
        const draft = await api.importConfiguration(
          clusterID,
          snapshot,
          version,
        );
        version = draft.version;
      }
      const preview = await api.validateConfigurationDraft(clusterID);
      if (!preview.valid)
        throw new Error(
          preview.issues
            .map((issue) => `${issue.field}: ${issue.message}`)
            .join("; ") ||
            "The imported draft did not pass capability validation.",
        );
      await api.publishConfigurationRevision(
        clusterID,
        version,
        "Initial authoritative configuration from guided onboarding",
      );
      await load();
    } catch (caught) {
      setError(caught);
    } finally {
      setBusy("");
    }
  }

  async function saveMonitoring(event: FormEvent) {
    event.preventDefault();
    if (!monitoring) return;
    setBusy("monitoring");
    try {
      const saved = await api.updateSystemSettings(monitoring);
      setMonitoring(saved);
      const value = await api.updateOnboardingProgress(
        clusterID,
        currentStatus.state.recordVersion,
        { monitoringReviewed: true },
      );
      setStatus(value);
      setActive("notifications");
      setError(undefined);
    } catch (caught) {
      setError(caught);
    } finally {
      setBusy("");
    }
  }

  async function saveWebhook(event: FormEvent) {
    event.preventDefault();
    setBusy("webhook");
    try {
      const channel = await api.createNotificationChannel(clusterID, {
        name: webhookName,
        destination: webhookURL,
        enabled: true,
        subscribedCategories: webhookCategories,
      });
      const result = await api.testNotificationChannel(channel.id);
      setWebhookURL("");
      setWebhookResult(
        result.success
          ? "The endpoint accepted the bounded test."
          : `The test failed safely (${result.errorCode ?? "NOTIFICATION_TEST_FAILED"}).`,
      );
      const value = await load();
      if (value) setStatus(value);
    } catch (caught) {
      setError(caught);
    } finally {
      setBusy("");
    }
  }

  async function finish() {
    setBusy("finish");
    try {
      const value = await api.finishOnboarding(
        clusterID,
        currentStatus.state.recordVersion,
      );
      setStatus(value);
      setActive("completed");
      sessionStorage.removeItem(`atlas-dns.onboarding-dismissed.${clusterID}`);
      setError(undefined);
    } catch (caught) {
      setError(caught);
    } finally {
      setBusy("");
    }
  }
}

function Step({ title, children }: { title: string; children: ReactNode }) {
  return (
    <section className="card onboarding-step">
      <h2>{title}</h2>
      {children}
    </section>
  );
}

function Next({ onClick }: { onClick: () => void }) {
  return (
    <button className="button" type="button" onClick={onClick}>
      Continue
    </button>
  );
}

function NodeEvidence({ status }: { status: OnboardingStatus }) {
  return (
    <div className="onboarding-node-grid">
      {status.nodes.map((item) => (
        <article className="card" key={item.node.id}>
          <strong>{item.node.name}</strong>
          <span>{item.node.version || "Version unknown"}</span>
          <span>API: {item.node.healthStatus}</span>
          <span>
            Compatibility: {item.onboardingCompatible ? "supported" : "blocked"}
          </span>
          <span>
            Configuration:{" "}
            {item.configurationAvailable ? "available" : "not observed"}
          </span>
        </article>
      ))}
    </div>
  );
}

function DurationInput({
  label,
  value,
  min,
  max,
  onChange,
}: {
  label: string;
  value: number;
  min: number;
  max: number;
  onChange: (value: number) => void;
}) {
  return (
    <label>
      {label} (seconds)
      <input
        type="number"
        min={min}
        max={max}
        value={value}
        onChange={(event) => onChange(Number(event.target.value))}
        required
      />
    </label>
  );
}

function recommendedMonitoring(settings: SystemSettings): SystemSettings {
  return {
    ...settings,
    nodeHealthIntervalSeconds: 30,
    statisticsPollIntervalSeconds: 3600,
    queryLogCollectionEnabled: true,
    queryLogPollIntervalSeconds: 30,
    queryLogRetentionSeconds: 7 * 24 * 3600,
  };
}
