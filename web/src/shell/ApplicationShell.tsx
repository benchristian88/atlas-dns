import {
  type KeyboardEvent as ReactKeyboardEvent,
  type ReactNode,
  useCallback,
  useEffect,
  useRef,
  useState,
} from "react";
import { AtlasBrand } from "../components/Brand";
import { Icon } from "../components/Icon";
import type { User } from "../lib/types";
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

interface ApplicationShellProps {
  user: User;
  pathname: string;
  onLogout: () => void;
  children: ReactNode;
}

const SIDEBAR_STORAGE_KEY = "atlas-dns.sidebar-collapsed";

export function ApplicationShell({
  user,
  pathname,
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
  const drawerTrigger = useRef<HTMLButtonElement>(null);
  const drawerClose = useRef<HTMLButtonElement>(null);
  const drawer = useRef<HTMLElement>(null);

  const closeDrawer = useCallback(() => {
    setDrawerOpen(false);
    window.requestAnimationFrame(() => drawerTrigger.current?.focus());
  }, []);

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
    const handleKeyDown = (event: KeyboardEvent) => {
      if (event.key === "Escape") {
        closeDrawer();
        return;
      }
      if (event.key !== "Tab") return;
      const focusable = Array.from(
        drawer.current?.querySelectorAll<HTMLElement>(
          'a[href], button:not([disabled]), input:not([disabled]), select:not([disabled]), textarea:not([disabled]), [tabindex]:not([tabindex="-1"])',
        ) ?? [],
      );
      const first = focusable[0];
      const last = focusable.at(-1);
      if (first === undefined || last === undefined) return;
      if (event.shiftKey && document.activeElement === first) {
        event.preventDefault();
        last.focus();
      } else if (!event.shiftKey && document.activeElement === last) {
        event.preventDefault();
        first.focus();
      }
    };
    window.addEventListener("keydown", handleKeyDown);
    return () => window.removeEventListener("keydown", handleKeyDown);
  }, [closeDrawer, drawerOpen]);

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
        <ShellBrandLink className="sidebar-brand" />
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
          <AccountMenu
            id="sidebar-account-menu"
            user={user}
            collapsed={collapsed}
            onLogout={onLogout}
          />
        </div>
      </aside>

      <header className="mobile-shell-header">
        <ShellBrandLink className="mobile-shell-brand" />
        <button
          ref={drawerTrigger}
          className="drawer-toggle shell-drawer-toggle"
          type="button"
          aria-expanded={drawerOpen}
          aria-controls="mobile-navigation"
          aria-label="Open navigation"
          onClick={() => setDrawerOpen(true)}
        >
          <Icon name="menu" />
        </button>
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
            ref={drawer}
            className="mobile-drawer"
            id="mobile-navigation"
            role="dialog"
            aria-modal="true"
            aria-label="Navigation drawer"
          >
            <div className="drawer-heading">
              <ShellBrandLink className="drawer-brand" />
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
              <AccountMenu
                id="drawer-account-menu"
                user={user}
                onLogout={onLogout}
              />
            </div>
          </aside>
        </>
      )}

      <main className="content">{children}</main>
    </div>
  );
}

function ShellBrandLink({ className }: { className: string }) {
  return (
    <a
      className={className}
      href="/"
      aria-label="Atlas DNS Controller dashboard"
    >
      <AtlasBrand placement="header" />
    </a>
  );
}

function AccountMenu({
  id,
  user,
  collapsed = false,
  onLogout,
}: {
  id: string;
  user: User;
  collapsed?: boolean;
  onLogout: () => void;
}) {
  const [open, setOpen] = useState(false);
  const root = useRef<HTMLDivElement>(null);
  const trigger = useRef<HTMLButtonElement>(null);

  useEffect(() => {
    if (!open) return;
    const closeOutside = (event: PointerEvent) => {
      if (
        event.target instanceof globalThis.Node &&
        !root.current?.contains(event.target)
      )
        setOpen(false);
    };
    const closeOnEscape = (event: KeyboardEvent) => {
      if (event.key === "Escape") {
        setOpen(false);
        window.requestAnimationFrame(() => trigger.current?.focus());
      }
    };
    document.addEventListener("pointerdown", closeOutside);
    window.addEventListener("keydown", closeOnEscape);
    return () => {
      document.removeEventListener("pointerdown", closeOutside);
      window.removeEventListener("keydown", closeOnEscape);
    };
  }, [open]);

  return (
    <div className="account-menu" ref={root}>
      <button
        ref={trigger}
        type="button"
        className="account-trigger"
        aria-expanded={open}
        aria-haspopup="menu"
        aria-controls={id}
        aria-label={
          collapsed ? `Account menu for ${user.displayName}` : undefined
        }
        title={collapsed ? user.displayName : undefined}
        onClick={() => setOpen((current) => !current)}
      >
        <span className="account-avatar" aria-hidden="true">
          {initials(user.displayName)}
        </span>
        <span className="account-identity">
          <strong>{user.displayName}</strong>
          <small>{roleLabel(user.role)}</small>
        </span>
        <Icon name="chevron" />
      </button>
      {open && (
        <div className="account-popover" id={id} role="menu">
          <div className="account-popover__identity">
            <strong>{user.displayName}</strong>
            <span>{user.email}</span>
          </div>
          <a role="menuitem" href="/account">
            My Account
          </a>
          <a role="menuitem" href="/account/preferences">
            Preferences
          </a>
          <button type="button" role="menuitem" onClick={onLogout}>
            Sign out
          </button>
        </div>
      )}
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

function roleLabel(role: User["role"]) {
  return role === "administrator" ? "Administrator" : role;
}
