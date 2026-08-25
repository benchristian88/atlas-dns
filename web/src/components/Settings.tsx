import { type ReactNode, useId } from "react";
import { Banner } from "./Feedback";

export function SettingRow({
  title,
  description,
  control,
  help,
  status,
  disabled = false,
}: {
  title: ReactNode;
  description?: ReactNode;
  control: ReactNode;
  help?: ReactNode;
  status?: ReactNode;
  disabled?: boolean;
}) {
  return (
    <div className="setting-row" aria-disabled={disabled || undefined}>
      <div className="setting-row__description">
        <div className="setting-row__title">{title}</div>
        {description !== undefined && <div>{description}</div>}
        {help !== undefined && <div className="field__help">{help}</div>}
      </div>
      <div className="setting-row__control">
        {control}
        {status !== undefined && (
          <div className="setting-row__status">{status}</div>
        )}
      </div>
    </div>
  );
}

export function SettingsGroup({
  title,
  description,
  actions,
  children,
  className,
  headingLevel = 2,
  disabled = false,
  bodySpacing = "rows",
}: {
  title: ReactNode;
  description?: ReactNode;
  actions?: ReactNode;
  children: ReactNode;
  className?: string;
  headingLevel?: 2 | 3;
  disabled?: boolean;
  bodySpacing?: "rows" | "padded";
}) {
  const Heading = headingLevel === 3 ? "h3" : "h2";
  return (
    <section
      className={`settings-group${className ? ` ${className}` : ""}`}
      aria-disabled={disabled || undefined}
    >
      <header className="settings-group__header">
        <div>
          <Heading>{title}</Heading>
          {description !== undefined && (
            <div className="muted">{description}</div>
          )}
        </div>
        {actions}
      </header>
      <div
        className={`settings-group__body settings-group__body--${bodySpacing}`}
      >
        {children}
      </div>
    </section>
  );
}

export function Field({
  label,
  htmlFor,
  help,
  error,
  suffix,
  scope,
  required = false,
  children,
}: {
  label: ReactNode;
  htmlFor?: string;
  help?: ReactNode;
  error?: ReactNode;
  suffix?: ReactNode;
  scope?: ReactNode;
  required?: boolean;
  children: ReactNode;
}) {
  const generatedID = useId();
  const helpID = `${generatedID}-help`;
  const errorID = `${generatedID}-error`;
  return (
    <div className="field" data-invalid={error !== undefined || undefined}>
      <div className="field__label-row">
        <label className="field__label" htmlFor={htmlFor}>
          {label}
          {required && <span aria-hidden="true"> *</span>}
        </label>
        {scope}
      </div>
      <div
        className={
          suffix === undefined
            ? "field__control"
            : "field__control field__control--suffix"
        }
      >
        {children}
        {suffix !== undefined && (
          <span className="field__suffix">{suffix}</span>
        )}
      </div>
      {help !== undefined && (
        <div className="field__help" id={helpID}>
          {help}
        </div>
      )}
      {error !== undefined && (
        <div className="field__error" id={errorID} role="alert">
          {error}
        </div>
      )}
    </div>
  );
}

export type ScopeKind = "cluster" | "node" | "observed";

export function ScopeIndicator({
  scope,
  label,
}: {
  scope: ScopeKind;
  label?: string;
}) {
  const text =
    label ??
    (scope === "cluster"
      ? "Entire Cluster"
      : scope === "node"
        ? "Node specific"
        : "Observed only");
  return (
    <span className={`scope-indicator scope-indicator--${scope}`}>{text}</span>
  );
}

export type CapabilityState = "supported" | "partial" | "unsupported" | "stale";

export function CapabilityWarning({
  state,
  title,
  children,
}: {
  state: CapabilityState;
  title?: string;
  children: ReactNode;
}) {
  if (state === "supported") return null;
  const tone = state === "unsupported" ? "danger" : "warning";
  const defaultTitle =
    state === "unsupported"
      ? "Unsupported"
      : state === "stale"
        ? "Capability data is stale"
        : "Partially supported";
  return (
    <Banner tone={tone} title={title ?? defaultTitle}>
      {children}
    </Banner>
  );
}

export function UnsavedChangesNotice({
  dirty,
  saving = false,
  saved = false,
  onSave,
  disabled = false,
}: {
  dirty: boolean;
  saving?: boolean;
  saved?: boolean;
  onSave?: () => void;
  disabled?: boolean;
}) {
  if (!dirty && !saving && !saved) return null;
  const message = saving
    ? "Saving draft…"
    : dirty
      ? "You have unsaved draft changes. Nodes are unchanged until a revision is published and deployed."
      : "Draft saved. Nodes are unchanged.";
  return (
    <Banner
      tone={dirty ? "warning" : "success"}
      title={saving ? "Saving" : dirty ? "Unsaved changes" : "Draft saved"}
      actions={
        dirty && onSave ? (
          <button
            type="button"
            className="button"
            disabled={disabled || saving}
            onClick={onSave}
          >
            Save Draft
          </button>
        ) : undefined
      }
    >
      {message}
    </Banner>
  );
}
