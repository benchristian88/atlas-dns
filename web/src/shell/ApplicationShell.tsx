import {
  type KeyboardEvent as ReactKeyboardEvent,
  type ReactNode,
  useCallback,
  useEffect,
  useMemo,
  useRef,
  useState,
} from "react";
import { AtlasBrand } from "../components/Brand";
import { Icon } from "../components/Icon";
import { StatusBadge } from "../components/StatusBadge";
import { api } from "../lib/api";
import { clusterHealth } from "../lib/freshness";
import type {
  Cluster,
  ConfigurationRevision,
  Deployment,
  Node,
  User,
} from "../lib/types";
import { ThemeControl } from "../theme/ThemeControl";
import {
  groupForPath,
  isGroupActive,
  isLinkActive,
  isNavigationGroup,
  type NavigationGroup,
  type NavigationLink,
  PRIMARY_NAVIGATION,
  UTILITY_NAVIGATION,
} from "./navigation";
import { ScopeProvider } from "./ScopeContext";

interface ApplicationShellProps {
  user: User;
  clusters: Cluster[];
  selected?: Cluster;
  pathname: string;
  onSelectCluster: (clusterID: string) => void;
  onLogout: () => void;
  children: ReactNode;
}

const SIDEBAR_STORAGE_KEY = "atlas-dns.sidebar-collapsed";

export function ApplicationShell({
  user,
  clusters,
  selected,
  pathname,
  onSelectCluster,
  onLogout,
  children,
}: ApplicationShellProps) {
  const activeGroup = groupForPath(pathname);
  const [drawerOpen, setDrawerOpen] = useState(false);
  const [collapsed, setCollapsed] = useState(readCollapsedPreference);
  const [openGroups, setOpenGroups] = useState<Set<string>>(
    () => new Set(activeGroup ? [activeGroup.id] : []),
  );
  const [mobileGroup, setMobileGroup] = useState(activeGroup?.id);
  const [accountOpen, setAccountOpen] = useState(false);
  const [nodes, setNodes] = useState<Node[]>([]);
  const [revisions, setRevisions] = useState<ConfigurationRevision[]>([]);
  const [activeDeployment, setActiveDeployment] = useState<Deployment>();
  const [refreshedAt, setRefreshedAt] = useState<string>();
  const [contextAvailable, setContextAvailable] = useState(true);
  const [scopeNodeID, setScopeNodeID] = useState("");
  const drawerTrigger = useRef<HTMLButtonElement>(null);
  const drawerClose = useRef<HTMLButtonElement>(null);
  const accountRoot = useRef<HTMLDivElement>(null);

  const closeDrawer = useCallback(() => {
    setDrawerOpen(false);
    window.requestAnimationFrame(() => drawerTrigger.current?.focus());
  }, []);

  const loadContext = useCallback(async () => {
    if (selected === undefined) {
      setNodes([]);
      setRevisions([]);
      setActiveDeployment(undefined);
      setRefreshedAt(undefined);
      return;
    }
    try {
      const [nodeResult, revisionResult, deploymentResult] = await Promise.all([
        api.nodes(selected.id),
        api.configurationRevisions(selected.id),
        api.deployments(selected.id),
      ]);
      const active = deploymentResult.items.find((deployment) =>
        ["queued", "validating", "running", "cancelling"].includes(
          deployment.status,
        ),
      );
      const detailed = active ? await api.deployment(active.id) : undefined;
      setNodes(nodeResult.items);
      setRefreshedAt(nodeResult.refreshedAt);
      setRevisions(revisionResult.items);
      setActiveDeployment(detailed);
      setContextAvailable(true);
    } catch {
      setContextAvailable(false);
    }
  }, [selected]);

  useEffect(() => {
    setScopeNodeID("");
    void loadContext();
    const interval = window.setInterval(() => void loadContext(), 15_000);
    return () => window.clearInterval(interval);
  }, [loadContext]);

  useEffect(() => {
    const group = groupForPath(pathname);
    setMobileGroup(group?.id);
    if (group !== undefined)
      setOpenGroups((current) => new Set([...current, group.id]));
  }, [pathname]);

  useEffect(() => {
    try {
      window.localStorage.setItem(SIDEBAR_STORAGE_KEY, String(collapsed));
    } catch {
      // Browser-local presentation state must never block the shell.
    }
  }, [collapsed]);

  useEffect(() => {
    if (!drawerOpen) return;
    drawerClose.current?.focus();
    const close = (event: KeyboardEvent) => {
      if (event.key === "Escape") {
        setDrawerOpen(false);
        window.requestAnimationFrame(() => drawerTrigger.current?.focus());
      }
    };
    window.addEventListener("keydown", close);
    return () => window.removeEventListener("keydown", close);
  }, [drawerOpen]);

  useEffect(() => {
    if (!accountOpen) return;
    const close = (event: PointerEvent) => {
      if (
        event.target instanceof globalThis.Node &&
        !accountRoot.current?.contains(event.target)
      )
        setAccountOpen(false);
    };
    const closeOnEscape = (event: KeyboardEvent) => {
      if (event.key === "Escape") setAccountOpen(false);
    };
    document.addEventListener("pointerdown", close);
    window.addEventListener("keydown", closeOnEscape);
    return () => {
      document.removeEventListener("pointerdown", close);
      window.removeEventListener("keydown", closeOnEscape);
    };
  }, [accountOpen]);

  const activeRevision = revisions.find(
    (revision) => revision.active || revision.id === selected?.activeRevisionId,
  );
  const activeTask = activeDeployment?.nodes.find((node) =>
    ["validating", "applying", "verifying"].includes(node.status),
  );
  const nodeNames = useMemo(
    () => new Map(nodes.map((node) => [node.id, node.name])),
    [nodes],
  );

  const toggleGroup = (group: NavigationGroup) => {
    if (collapsed) {
      setCollapsed(false);
      setOpenGroups((current) => new Set([...current, group.id]));
      return;
    }
    setOpenGroups((current) => {
      const next = new Set(current);
      if (next.has(group.id) && !isGroupActive(group, pathname))
        next.delete(group.id);
      else next.add(group.id);
      return next;
    });
  };

  return (
    <div className="app-shell" data-sidebar-collapsed={collapsed || undefined}>
      <aside className="app-sidebar" aria-label="Application sidebar">
        <a
          className="sidebar-brand"
          href="/"
          aria-label="Atlas DNS Controller dashboard"
        >
          <AtlasBrand placement="header" />
        </a>
        <SidebarNavigation
          pathname={pathname}
          collapsed={collapsed}
          openGroups={openGroups}
          onToggleGroup={toggleGroup}
        />
        <div className="sidebar-utility">
          {UTILITY_NAVIGATION.map((item) => (
            <SidebarLink
              key={item.href}
              item={item}
              pathname={pathname}
              collapsed={collapsed}
            />
          ))}
          <button
            type="button"
            className="sidebar-collapse"
            aria-label={collapsed ? "Expand sidebar" : "Collapse sidebar"}
            title={collapsed ? "Expand sidebar" : "Collapse sidebar"}
            onClick={() => setCollapsed((current) => !current)}
          >
            <Icon name="collapse" />
            <span>{collapsed ? "Expand" : "Collapse"}</span>
          </button>
        </div>
      </aside>

      <header className="app-topbar">
        <button
          ref={drawerTrigger}
          className="drawer-toggle"
          type="button"
          aria-expanded={drawerOpen}
          aria-controls="mobile-navigation"
          aria-label="Open navigation"
          onClick={() => setDrawerOpen(true)}
        >
          <Icon name="menu" />
        </button>
        <a
          className="topbar-brand"
          href="/"
          aria-label="Atlas DNS Controller dashboard"
        >
          <AtlasBrand placement="header" />
        </a>
        <section className="topbar-context" aria-label="Controller context">
          <label className="topbar-select">
            <span>Cluster</span>
            <select
              value={selected?.id ?? ""}
              onChange={(event) => onSelectCluster(event.target.value)}
              disabled={clusters.length === 0}
            >
              {clusters.map((cluster) => (
                <option key={cluster.id} value={cluster.id}>
                  {cluster.name}
                </option>
              ))}
            </select>
          </label>
          <label className="topbar-select topbar-select--scope">
            <span>Scope</span>
            <select
              value={scopeNodeID}
              onChange={(event) => setScopeNodeID(event.target.value)}
              disabled={nodes.length === 0}
            >
              <option value="">Entire Cluster</option>
              {nodes.map((node) => (
                <option key={node.id} value={node.id}>
                  {node.name}
                </option>
              ))}
            </select>
          </label>
          <span className="topbar-fact topbar-fact--revision">
            <small>Revision</small>
            <strong>
              {contextAvailable
                ? activeRevision
                  ? `#${activeRevision.revisionNumber}`
                  : "None"
                : "Unavailable"}
            </strong>
          </span>
          <span className="topbar-fact topbar-fact--health">
            <small>Health</small>
            {contextAvailable ? (
              <StatusBadge status={clusterHealth(nodes)} />
            ) : (
              <strong>Unavailable</strong>
            )}
          </span>
          {activeDeployment && (
            <a className="topbar-deployment" href="/ha/deployments">
              {activeTask
                ? `${activeTask.status} ${nodeNames.get(activeTask.nodeId) ?? "node"}`
                : activeDeployment.status}
            </a>
          )}
        </section>
        <span className="topbar-updated" title={formatDate(refreshedAt)}>
          <Icon name="updates" />
          <span>
            {contextAvailable
              ? formatRelative(refreshedAt)
              : "Refresh unavailable"}
          </span>
        </span>
        <ThemeControl />
        <a
          className="topbar-icon-button"
          href="/ha/notifications"
          aria-label="Notifications"
          title="Notifications"
        >
          <Icon name="notifications" />
        </a>
        <div className="account-menu" ref={accountRoot}>
          <button
            type="button"
            className="account-trigger"
            aria-expanded={accountOpen}
            aria-haspopup="menu"
            aria-controls="account-menu"
            onClick={() => setAccountOpen((current) => !current)}
          >
            <span className="account-avatar" aria-hidden="true">
              {initials(user.displayName)}
            </span>
            <span className="account-identity">
              <strong>{user.displayName}</strong>
              <small>{user.role}</small>
            </span>
            <Icon name="chevron" />
          </button>
          {accountOpen && (
            <div className="account-popover" id="account-menu" role="menu">
              <p>{user.email}</p>
              <button type="button" role="menuitem" onClick={onLogout}>
                Sign Out
              </button>
            </div>
          )}
        </div>
      </header>

      {drawerOpen && (
        <>
          <button
            className="drawer-backdrop"
            type="button"
            aria-label="Close navigation"
            onClick={closeDrawer}
          />
          <aside
            className="mobile-drawer"
            id="mobile-navigation"
            role="dialog"
            aria-modal="true"
            aria-label="Navigation drawer"
          >
            <div className="drawer-heading">
              <AtlasBrand placement="header" />
              <button
                ref={drawerClose}
                className="drawer-close"
                type="button"
                aria-label="Close navigation"
                onClick={closeDrawer}
              >
                ×
              </button>
            </div>
            <nav aria-label="Mobile navigation">
              {PRIMARY_NAVIGATION.map((item) =>
                isNavigationGroup(item) ? (
                  <MobileGroup
                    key={item.id}
                    group={item}
                    pathname={pathname}
                    open={mobileGroup === item.id}
                    onToggle={() =>
                      setMobileGroup((current) =>
                        current === item.id && !isGroupActive(item, pathname)
                          ? undefined
                          : item.id,
                      )
                    }
                  />
                ) : (
                  <SidebarLink
                    key={item.href}
                    item={item}
                    pathname={pathname}
                  />
                ),
              )}
            </nav>
            <div className="drawer-utility">
              {UTILITY_NAVIGATION.map((item) => (
                <SidebarLink key={item.href} item={item} pathname={pathname} />
              ))}
              <button
                className="drawer-signout"
                type="button"
                onClick={onLogout}
              >
                Sign Out · {user.displayName}
              </button>
            </div>
          </aside>
        </>
      )}

      <ScopeProvider value={{ nodeId: scopeNodeID, nodes }}>
        <main className="content">{children}</main>
      </ScopeProvider>
    </div>
  );
}

function SidebarNavigation({
  pathname,
  collapsed,
  openGroups,
  onToggleGroup,
}: {
  pathname: string;
  collapsed: boolean;
  openGroups: ReadonlySet<string>;
  onToggleGroup: (group: NavigationGroup) => void;
}) {
  return (
    <nav className="sidebar-navigation" aria-label="Primary navigation">
      {PRIMARY_NAVIGATION.map((item) =>
        isNavigationGroup(item) ? (
          <SidebarGroup
            key={item.id}
            group={item}
            pathname={pathname}
            collapsed={collapsed}
            open={openGroups.has(item.id)}
            onToggle={() => onToggleGroup(item)}
          />
        ) : (
          <SidebarLink
            key={item.href}
            item={item}
            pathname={pathname}
            collapsed={collapsed}
          />
        ),
      )}
    </nav>
  );
}

function SidebarGroup({
  group,
  pathname,
  collapsed,
  open,
  onToggle,
}: {
  group: NavigationGroup;
  pathname: string;
  collapsed: boolean;
  open: boolean;
  onToggle: () => void;
}) {
  const active = isGroupActive(group, pathname);
  const expanded = open && !collapsed;
  const trigger = useRef<HTMLButtonElement>(null);
  const handleKeyDown = (event: ReactKeyboardEvent<HTMLButtonElement>) => {
    if (["ArrowDown", "ArrowRight"].includes(event.key)) {
      event.preventDefault();
      if (!expanded) onToggle();
      window.requestAnimationFrame(() => {
        document
          .getElementById(`sidebar-group-${group.id}`)
          ?.querySelector<HTMLElement>("a")
          ?.focus();
      });
    }
    if (event.key === "Escape" && expanded) {
      event.preventDefault();
      trigger.current?.focus();
    }
  };
  return (
    <div className="sidebar-group" data-active={active || undefined}>
      <button
        ref={trigger}
        type="button"
        className="sidebar-group__trigger"
        aria-expanded={expanded}
        aria-controls={`sidebar-group-${group.id}`}
        aria-label={collapsed ? group.label : undefined}
        title={collapsed ? group.label : undefined}
        onClick={onToggle}
        onKeyDown={handleKeyDown}
      >
        <Icon name={group.icon} />
        <span>{group.label}</span>
        <Icon className="sidebar-chevron" name="chevron" />
      </button>
      {expanded && (
        <div
          className="sidebar-group__children"
          id={`sidebar-group-${group.id}`}
        >
          {group.children.map((item) => (
            <SidebarLink key={item.href} item={item} pathname={pathname} />
          ))}
        </div>
      )}
    </div>
  );
}

function MobileGroup({
  group,
  pathname,
  open,
  onToggle,
}: {
  group: NavigationGroup;
  pathname: string;
  open: boolean;
  onToggle: () => void;
}) {
  return (
    <div className="mobile-nav-group" data-open={open || undefined}>
      <button
        type="button"
        className="sidebar-group__trigger"
        aria-expanded={open}
        aria-controls={`mobile-group-${group.id}`}
        onClick={onToggle}
      >
        <Icon name={group.icon} />
        <span>{group.label}</span>
        <Icon className="sidebar-chevron" name="chevron" />
      </button>
      {open && (
        <div
          className="sidebar-group__children"
          id={`mobile-group-${group.id}`}
        >
          {group.children.map((item) => (
            <SidebarLink key={item.href} item={item} pathname={pathname} />
          ))}
        </div>
      )}
    </div>
  );
}

function SidebarLink({
  item,
  pathname,
  collapsed = false,
}: {
  item: NavigationLink;
  pathname: string;
  collapsed?: boolean;
}) {
  const current = isLinkActive(item, pathname);
  return (
    <a
      href={item.href}
      className={
        current ? "sidebar-link sidebar-link--current" : "sidebar-link"
      }
      aria-current={current ? "page" : undefined}
      aria-label={collapsed ? item.label : undefined}
      title={collapsed ? item.label : undefined}
    >
      <Icon name={item.icon} />
      <span>{item.label}</span>
    </a>
  );
}

function readCollapsedPreference() {
  try {
    return window.localStorage.getItem(SIDEBAR_STORAGE_KEY) === "true";
  } catch {
    return false;
  }
}

function formatDate(value?: string) {
  if (!value) return "Not refreshed yet";
  const date = new Date(value);
  return Number.isNaN(date.valueOf())
    ? "Refresh time unavailable"
    : date.toLocaleString();
}

function formatRelative(value?: string) {
  if (!value) return "Waiting for refresh";
  const date = new Date(value);
  if (Number.isNaN(date.valueOf())) return "Refresh time unavailable";
  const seconds = Math.max(0, Math.round((Date.now() - date.valueOf()) / 1000));
  if (seconds < 60) return "Updated just now";
  const minutes = Math.round(seconds / 60);
  return `Updated ${minutes}m ago`;
}

function initials(name: string) {
  return (
    name
      .split(/\s+/)
      .filter(Boolean)
      .slice(0, 2)
      .map((part) => part[0]?.toUpperCase())
      .join("") || "A"
  );
}
