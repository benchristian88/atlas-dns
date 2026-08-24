import { type ReactNode, useCallback, useEffect, useState } from "react";
import { Banner, EmptyState, ErrorState, Loading } from "./components/Feedback";
import { PageContainer } from "./components/Page";
import { AllowlistsPage } from "./features/allowlists/AllowlistsPage";
import { AuditPage } from "./features/audit/AuditPage";
import { LoginPage, SetupPage } from "./features/auth/AuthPages";
import { BlockedServicesPage } from "./features/blockedservices/BlockedServicesPage";
import { BlocklistsPage } from "./features/blocklists/BlocklistsPage";
import { ClientsPage } from "./features/clients/ClientsPage";
import { ClusterCreate } from "./features/clusters/ClusterCreate";
import { ConfigurationPage } from "./features/configuration/ConfigurationPage";
import { DashboardPage } from "./features/dashboard/DashboardPage";
import { DeploymentsPage } from "./features/deployments/DeploymentsPage";
import { DHCPPage } from "./features/dhcp/DHCPPage";
import { DNSSettingsPage } from "./features/dns/DNSSettingsPage";
import { DriftPage } from "./features/drift/DriftPage";
import { EncryptionPage } from "./features/encryption/EncryptionPage";
import { CustomRulesPage } from "./features/filters/CustomRulesPage";
import { GeneralSettingsPage } from "./features/general/GeneralSettingsPage";
import { HAOperationsPage } from "./features/ha-operations/HAOperationsPage";
import { NodeLifecyclePage } from "./features/ha-operations/NodeLifecyclePage";
import { RevisionsPage } from "./features/history/HistoryPage";
import { NodesPage } from "./features/nodes/NodesPage";
import { NotificationsPage } from "./features/notifications/NotificationsPage";
import { OnboardingPage } from "./features/onboarding/OnboardingPage";
import { OperationalStatusPage } from "./features/operational-status/OperationalStatusPage";
import { QueryLogPage } from "./features/query-log/QueryLogPage";
import { RewritesPage } from "./features/rewrites/RewritesPage";
import { SetupGuidePage } from "./features/setup/SetupGuidePage";
import { StatisticsPage } from "./features/statistics/StatisticsPage";
import { AboutPage } from "./features/system/AboutPage";
import { BackupPage } from "./features/system/BackupPage";
import { SystemSettingsPage } from "./features/system/SystemSettingsPage";
import { UpdatesPage } from "./features/system/UpdatesPage";
import { UsersPage } from "./features/users/UsersPage";
import { ApiError, api } from "./lib/api";
import type { Cluster, User } from "./lib/types";
import { NotFoundPage } from "./routing/RouteStatePages";
import {
  preserveRouteState,
  resolveRoute,
  routePageWidth,
} from "./routing/routes";
import { ApplicationShell } from "./shell/ApplicationShell";

type BootState =
  | { kind: "loading" }
  | {
      kind: "setup";
      publicBaseUrl: string;
      controllerTime: string;
      secureCookies: boolean;
    }
  | { kind: "login" }
  | { kind: "authenticated"; user: User }
  | { kind: "error"; error: unknown };

export function App() {
  const [state, setState] = useState<BootState>({ kind: "loading" });

  const boot = useCallback(async () => {
    setState({ kind: "loading" });
    try {
      const setup = await api.setupStatus();
      if (setup.setupRequired) {
        setState({
          kind: "setup",
          publicBaseUrl: setup.publicBaseUrl,
          controllerTime: setup.controllerTime,
          secureCookies: setup.secureCookies,
        });
        return;
      }
      try {
        const auth = await api.me();
        setState({ kind: "authenticated", user: auth.user });
      } catch (caught) {
        if (caught instanceof ApiError && caught.status === 401)
          setState({ kind: "login" });
        else throw caught;
      }
    } catch (caught) {
      setState({ kind: "error", error: caught });
    }
  }, []);

  useEffect(() => {
    void boot();
  }, [boot]);

  switch (state.kind) {
    case "loading":
      return (
        <main className="centred-state">
          <Loading label="Starting Atlas DNS Controller…" />
        </main>
      );
    case "setup":
      return (
        <SetupPage
          publicBaseUrl={state.publicBaseUrl}
          controllerTime={state.controllerTime}
          secureCookies={state.secureCookies}
          onAuthenticated={(user) => setState({ kind: "authenticated", user })}
        />
      );
    case "login":
      return (
        <LoginPage
          onAuthenticated={(user) => setState({ kind: "authenticated", user })}
        />
      );
    case "authenticated":
      return (
        <Application
          user={state.user}
          onLogout={() => setState({ kind: "login" })}
        />
      );
    case "error":
      return (
        <main className="centred-state">
          <ErrorState error={state.error} retry={() => void boot()} />
        </main>
      );
  }
}

function Application({ user, onLogout }: { user: User; onLogout: () => void }) {
  const [clusters, setClusters] = useState<Cluster[]>();
  const [selectedID, setSelectedID] = useState("");
  const [error, setError] = useState<unknown>();

  const loadClusters = useCallback(
    async (preferredID?: string) => {
      try {
        const result = await api.clusters();
        setClusters(result.items);
        setSelectedID(
          (current) => preferredID ?? (current || result.items[0]?.id || ""),
        );
        setError(undefined);
      } catch (caught) {
        if (caught instanceof ApiError && caught.status === 401) onLogout();
        else setError(caught);
      }
    },
    [onLogout],
  );

  useEffect(() => {
    void loadClusters();
  }, [loadClusters]);

  async function logout() {
    try {
      await api.logout();
    } finally {
      onLogout();
    }
  }

  const selected =
    clusters?.find((cluster) => cluster.id === selectedID) ?? clusters?.[0];
  const path = window.location.pathname;
  const route = resolveRoute(path);
  let content: ReactNode;
  if (route.kind === "redirect") content = <RouteRedirect to={route.to} />;
  else if (route.kind === "not-found")
    content = <NotFoundPage pathname={path} />;
  else if (clusters === undefined && error === undefined)
    content = <Loading label="Loading clusters…" />;
  else if (clusters === undefined && error !== undefined)
    content = <ErrorState error={error} retry={() => void loadClusters()} />;
  else if (route.kind === "audit") content = <AuditPage />;
  else if (route.kind === "users") content = <UsersPage currentUser={user} />;
  else if (route.kind === "backups") content = <BackupPage />;
  else if (route.kind === "updates") content = <UpdatesPage />;
  else if (route.kind === "about") content = <AboutPage />;
  else if (route.kind === "system-settings")
    content = <SystemSettingsPage cluster={selected} />;
  else if (selected === undefined && route.kind === "onboarding")
    content = (
      <OnboardingPage
        onClusterCreated={(cluster) => void loadClusters(cluster.id)}
      />
    );
  else if (selected === undefined)
    content = (
      <EmptyState title="Create your first cluster">
        <p>
          A cluster groups the AdGuard Home nodes that provide one resilient DNS
          service.
        </p>
        <ClusterCreate onCreated={(cluster) => void loadClusters(cluster.id)} />
        <p>
          <a className="button" href="/onboarding">
            Start guided onboarding
          </a>
        </p>
      </EmptyState>
    );
  else {
    switch (route.kind) {
      case "dashboard":
        content = <DashboardPage cluster={selected} />;
        break;
      case "nodes":
        content = <NodesPage cluster={selected} />;
        break;
      case "ha-operations":
        content = <HAOperationsPage cluster={selected} />;
        break;
      case "notifications":
        content = <NotificationsPage cluster={selected} />;
        break;
      case "node-lifecycle":
        content = (
          <NodeLifecyclePage cluster={selected} nodeId={route.nodeId} />
        );
        break;
      case "statistics":
        content = <StatisticsPage cluster={selected} />;
        break;
      case "query-log":
        content = <QueryLogPage cluster={selected} />;
        break;
      case "configuration":
        content = <ConfigurationPage cluster={selected} />;
        break;
      case "revisions":
        content = <RevisionsPage cluster={selected} />;
        break;
      case "deployments":
        content = <DeploymentsPage cluster={selected} />;
        break;
      case "drift":
        content = <DriftPage cluster={selected} />;
        break;
      case "operational-status":
        content = <OperationalStatusPage cluster={selected} />;
        break;
      case "setup-guide":
        content = <SetupGuidePage cluster={selected} />;
        break;
      case "onboarding":
        content = <OnboardingPage cluster={selected} />;
        break;
      case "blocked-services":
        content = <BlockedServicesPage cluster={selected} />;
        break;
      case "blocklists":
        content = <BlocklistsPage cluster={selected} />;
        break;
      case "allowlists":
        content = <AllowlistsPage cluster={selected} />;
        break;
      case "settings":
        content =
          route.area === "clients" ? (
            <ClientsPage cluster={selected} />
          ) : route.area === "dns" ? (
            <DNSSettingsPage cluster={selected} />
          ) : route.area === "privacy" ? (
            <GeneralSettingsPage cluster={selected} />
          ) : route.area === "rewrites" ? (
            <RewritesPage cluster={selected} />
          ) : route.area === "dhcp" ? (
            <DHCPPage cluster={selected} />
          ) : route.area === "filters" ? (
            <CustomRulesPage cluster={selected} />
          ) : route.area === "infrastructure" ? (
            <EncryptionPage cluster={selected} />
          ) : (
            <NotFoundPage pathname={path} />
          );
        break;
    }
  }

  return (
    <ApplicationShell
      user={user}
      clusters={clusters ?? []}
      selected={selected}
      pathname={path}
      onSelectCluster={setSelectedID}
      onLogout={() => void logout()}
    >
      <PageContainer size={routePageWidth(route)}>
        {selected && route.kind !== "onboarding" && (
          <OnboardingOffer cluster={selected} />
        )}
        {content}
      </PageContainer>
    </ApplicationShell>
  );
}

export function OnboardingOffer({ cluster }: { cluster: Cluster }) {
  const key = `atlas-dns.onboarding-dismissed.${cluster.id}`;
  const [required, setRequired] = useState(false);
  const [dismissed, setDismissed] = useState(
    () => sessionStorage.getItem(key) === "true",
  );
  useEffect(() => {
    let active = true;
    void api
      .onboardingStatus(cluster.id)
      .then((status) => {
        if (active) setRequired(status.setupRequired);
      })
      .catch(() => undefined);
    return () => {
      active = false;
    };
  }, [cluster.id]);
  if (!required || dismissed) return null;
  return (
    <Banner tone="info" title="Atlas setup is incomplete">
      <span>
        Continue guided onboarding from your current controller state.{" "}
      </span>
      <a href="/onboarding">Continue setup</a>{" "}
      <button
        className="button button--quiet"
        type="button"
        onClick={() => {
          sessionStorage.setItem(key, "true");
          setDismissed(true);
        }}
      >
        Not now
      </button>
    </Banner>
  );
}

function RouteRedirect({ to }: { to: string }) {
  useEffect(() => {
    window.location.replace(
      preserveRouteState(to, window.location.search, window.location.hash),
    );
  }, [to]);
  return <Loading label="Redirecting to the current page…" />;
}
