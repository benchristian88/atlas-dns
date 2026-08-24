import type { ReactNode, SVGProps } from "react";

export type IconName =
  | "activity"
  | "admin"
  | "attention"
  | "audit"
  | "backups"
  | "block"
  | "chevron"
  | "clients"
  | "collapse"
  | "dashboard"
  | "deployments"
  | "dhcp"
  | "dns"
  | "drift"
  | "encryption"
  | "filters"
  | "general"
  | "ha"
  | "help"
  | "menu"
  | "nodes"
  | "notifications"
  | "revisions"
  | "rewrites"
  | "settings"
  | "statistics"
  | "system"
  | "updates"
  | "users";

const paths: Record<IconName, ReactNode> = {
  activity: <path d="M4 12h3l2-6 4 12 2-6h5" />,
  admin: (
    <>
      <circle cx="12" cy="8" r="3" />
      <path d="M5.5 20a6.5 6.5 0 0 1 13 0" />
    </>
  ),
  attention: (
    <>
      <path d="m12 3 9 17H3L12 3Z" />
      <path d="M12 9v4m0 3h.01" />
    </>
  ),
  audit: (
    <>
      <path d="M5 4h14v16H5z" />
      <path d="M8 8h8M8 12h8M8 16h5" />
    </>
  ),
  backups: (
    <>
      <path d="M4 7h16v13H4z" />
      <path d="M8 7V4h8v3m-7 6h6" />
    </>
  ),
  block: (
    <>
      <circle cx="12" cy="12" r="8" />
      <path d="m6.5 6.5 11 11" />
    </>
  ),
  chevron: <path d="m9 7 5 5-5 5" />,
  clients: (
    <>
      <circle cx="9" cy="9" r="3" />
      <circle cx="17" cy="10" r="2" />
      <path d="M3.5 20a5.5 5.5 0 0 1 11 0m0-4a4 4 0 0 1 6 4" />
    </>
  ),
  collapse: (
    <>
      <path d="M5 4h14v16H5z" />
      <path d="M10 4v16m5-5-3-3 3-3" />
    </>
  ),
  dashboard: (
    <>
      <rect x="4" y="4" width="6" height="6" rx="1" />
      <rect x="14" y="4" width="6" height="6" rx="1" />
      <rect x="4" y="14" width="6" height="6" rx="1" />
      <rect x="14" y="14" width="6" height="6" rx="1" />
    </>
  ),
  deployments: (
    <>
      <path d="M12 3v12m0 0-4-4m4 4 4-4" />
      <path d="M5 19h14" />
    </>
  ),
  dhcp: (
    <>
      <circle cx="6" cy="12" r="2" />
      <circle cx="18" cy="7" r="2" />
      <circle cx="18" cy="17" r="2" />
      <path d="M8 12h4m0 0 4-5m-4 5 4 5" />
    </>
  ),
  dns: (
    <>
      <circle cx="12" cy="12" r="9" />
      <path d="M3 12h18M12 3c3 3 3 15 0 18M12 3c-3 3-3 15 0 18" />
    </>
  ),
  drift: (
    <>
      <path d="M4 7h8a4 4 0 0 1 4 4v6" />
      <path d="m13 14 3 3 3-3" />
    </>
  ),
  encryption: (
    <>
      <rect x="5" y="10" width="14" height="10" rx="2" />
      <path d="M8 10V7a4 4 0 0 1 8 0v3" />
    </>
  ),
  filters: (
    <>
      <path d="M4 5h16l-6 7v6l-4 2v-8L4 5Z" />
    </>
  ),
  general: (
    <>
      <path d="M4 7h10M18 7h2M4 12h2m4 0h10M4 17h7m4 0h5" />
      <circle cx="16" cy="7" r="2" />
      <circle cx="8" cy="12" r="2" />
      <circle cx="13" cy="17" r="2" />
    </>
  ),
  ha: (
    <>
      <path d="M7 5h10v5H7zM7 14h10v5H7z" />
      <path d="M4 7.5h3m10 9H20" />
    </>
  ),
  help: (
    <>
      <circle cx="12" cy="12" r="9" />
      <path d="M9.5 9a2.6 2.6 0 1 1 4 2.2c-1 .6-1.5 1.1-1.5 2.3M12 17h.01" />
    </>
  ),
  menu: <path d="M4 7h16M4 12h16M4 17h16" />,
  nodes: (
    <>
      <rect x="4" y="4" width="16" height="6" rx="2" />
      <rect x="4" y="14" width="16" height="6" rx="2" />
      <path d="M8 7h.01M8 17h.01" />
    </>
  ),
  notifications: (
    <>
      <path d="M6 16h12l-1.5-2V10a4.5 4.5 0 0 0-9 0v4L6 16Z" />
      <path d="M10 19h4" />
    </>
  ),
  revisions: (
    <>
      <path d="M6 3h9l3 3v15H6z" />
      <path d="M14 3v4h4M9 11h6M9 15h6" />
    </>
  ),
  rewrites: (
    <>
      <path d="M4 8h12m0 0-3-3m3 3-3 3M20 16H8m0 0 3-3m-3 3 3 3" />
    </>
  ),
  settings: (
    <>
      <circle cx="12" cy="12" r="3" />
      <path d="M12 3v2m0 14v2M3 12h2m14 0h2M5.6 5.6 7 7m10 10 1.4 1.4M18.4 5.6 17 7M7 17l-1.4 1.4" />
    </>
  ),
  statistics: (
    <>
      <path d="M5 19V9m7 10V4m7 15v-7" />
      <path d="M3 19h18" />
    </>
  ),
  system: (
    <>
      <circle cx="12" cy="12" r="3" />
      <path
        d="m19 13.5 2-1.5-2-1.5-.5-1.3.4-2.5-2.5-.9-1.6 2-1.3-.3L9 3H6l-.5 2.5-1.3.5-2-1.4-1.5 2.5 2 1.6-.2 1.3L0 12l2.5 2 .3 1.3-2 1.6 1.5 2.5 2-1.4 1.2.5L6 21h3l1.5-2.5 1.3-.3 1.6 2 2.5-1.5-1.4-2 .5-1.2 2-.5Z"
        transform="scale(.85) translate(2.1 2.1)"
      />
    </>
  ),
  updates: (
    <>
      <path d="M12 20V7m0 0-4 4m4-4 4 4" />
      <path d="M5 4h14" />
    </>
  ),
  users: (
    <>
      <circle cx="9" cy="8" r="3" />
      <path d="M3 20a6 6 0 0 1 12 0m1-11h5m-2.5-2.5v5" />
    </>
  ),
};

export function Icon({
  name,
  ...props
}: { name: IconName } & SVGProps<SVGSVGElement>) {
  return (
    <svg
      viewBox="0 0 24 24"
      fill="none"
      stroke="currentColor"
      strokeWidth="1.7"
      strokeLinecap="round"
      strokeLinejoin="round"
      aria-hidden="true"
      focusable="false"
      {...props}
    >
      {paths[name]}
    </svg>
  );
}
