import { Fragment, type ReactNode } from "react";
import { EmptyState, ErrorState, LoadingSkeleton } from "./Feedback";
import { Icon, type IconName } from "./Icon";
import { StatusBadge, type StatusKind } from "./StatusBadge";

export function MetricCard({
  label,
  value,
  detail,
  valueClassName,
}: {
  label: ReactNode;
  value: ReactNode;
  detail?: ReactNode;
  valueClassName?: string;
}) {
  return (
    <article className="metric-card">
      <span>{label}</span>
      <strong className={valueClassName}>{value}</strong>
      {detail !== undefined && <small>{detail}</small>}
    </article>
  );
}

export function HealthSummaryCard({
  icon,
  label,
  value,
  status,
  statusLabel,
  detail,
  valueClassName,
}: {
  icon: IconName;
  label: string;
  value: ReactNode;
  status: StatusKind;
  statusLabel?: ReactNode;
  detail: string;
  valueClassName?: string;
}) {
  return (
    <article className="health-summary-card" aria-label={label}>
      <span className={`health-summary-card__icon status-tone--${status}`}>
        <Icon name={icon} />
      </span>
      <span className="health-summary-card__body">
        <small>{label}</small>
        <strong className={valueClassName}>{value}</strong>
        <span>
          <StatusBadge status={status} label={statusLabel} />
          <em title={detail}>{detail}</em>
        </span>
      </span>
    </article>
  );
}

export interface SummaryTile {
  id: string;
  label: ReactNode;
  value: ReactNode;
  detail?: ReactNode;
  valueClassName?: string;
}

export function SummaryTileGrid({
  items,
  className = "",
  label,
}: {
  items: readonly SummaryTile[];
  className?: string;
  label?: string;
}) {
  return (
    <dl className={`summary-tile-grid ${className}`.trim()} aria-label={label}>
      {items.map((item) => (
        <div key={item.id}>
          <dt>{item.label}</dt>
          <dd className={item.valueClassName}>
            <span className="summary-tile-grid__value">{item.value}</span>
            {item.detail !== undefined && <small>{item.detail}</small>}
          </dd>
        </div>
      ))}
    </dl>
  );
}

export interface DataTableColumn<Row> {
  id: string;
  header: ReactNode;
  render: (row: Row) => ReactNode;
  align?: "left" | "center" | "right";
  className?: string;
}

export function DataTable<Row>({
  columns,
  rows,
  rowKey,
  caption,
  loading = false,
  loadingLabel = "Loading table…",
  error,
  retry,
  stale = false,
  emptyTitle = "No items",
  emptyDescription,
  filteredEmpty = false,
  pagination,
  expandedRowKey,
  renderExpandedRow,
  expandedRowId,
}: {
  columns: readonly DataTableColumn<Row>[];
  rows: readonly Row[];
  rowKey: (row: Row) => string;
  caption?: string;
  loading?: boolean;
  loadingLabel?: string;
  error?: unknown;
  retry?: () => void;
  stale?: boolean;
  emptyTitle?: string;
  emptyDescription?: ReactNode;
  filteredEmpty?: boolean;
  pagination?: ReactNode;
  expandedRowKey?: string;
  renderExpandedRow?: (row: Row) => ReactNode;
  expandedRowId?: (row: Row) => string;
}) {
  if (loading) return <LoadingSkeleton label={loadingLabel} rows={4} />;
  if (error !== undefined) return <ErrorState error={error} retry={retry} />;
  if (rows.length === 0)
    return (
      <EmptyState title={emptyTitle} filtered={filteredEmpty}>
        {emptyDescription}
      </EmptyState>
    );
  return (
    <div className="data-table" data-stale={stale || undefined}>
      {stale && (
        <div className="data-table__state" role="status">
          Showing stale data
        </div>
      )}
      <div className="table-wrap">
        <table>
          {caption !== undefined && <caption>{caption}</caption>}
          <thead>
            <tr>
              {columns.map((column) => (
                <th
                  key={column.id}
                  scope="col"
                  className={column.className}
                  data-align={column.align}
                >
                  {column.header}
                </th>
              ))}
            </tr>
          </thead>
          <tbody>
            {rows.map((row) => {
              const key = rowKey(row);
              const expanded =
                renderExpandedRow !== undefined && key === expandedRowKey;
              return (
                <Fragment key={key}>
                  <tr
                    className={
                      expanded ? "table-summary-row--expanded" : undefined
                    }
                    data-expanded={expanded || undefined}
                  >
                    {columns.map((column) => (
                      <td
                        key={column.id}
                        className={column.className}
                        data-align={column.align}
                      >
                        {column.render(row)}
                      </td>
                    ))}
                  </tr>
                  {expanded && (
                    <tr className="table-inline-detail-row">
                      <td colSpan={columns.length}>
                        <div
                          className="table-inline-detail"
                          id={expandedRowId?.(row)}
                        >
                          {renderExpandedRow(row)}
                        </div>
                      </td>
                    </tr>
                  )}
                </Fragment>
              );
            })}
          </tbody>
        </table>
      </div>
      {pagination}
    </div>
  );
}

export function Pagination({
  page,
  pageCount,
  hasPrevious = page > 1,
  hasNext = pageCount === undefined ? false : page < pageCount,
  onPrevious,
  onNext,
  disabled = false,
  label = "Table pagination",
}: {
  page: number;
  pageCount?: number;
  hasPrevious?: boolean;
  hasNext?: boolean;
  onPrevious: () => void;
  onNext: () => void;
  disabled?: boolean;
  label?: string;
}) {
  return (
    <nav className="pagination" aria-label={label}>
      <button
        type="button"
        className="button button--secondary"
        disabled={disabled || !hasPrevious}
        onClick={onPrevious}
      >
        Previous
      </button>
      <span>
        Page {page}
        {pageCount === undefined ? "" : ` of ${pageCount}`}
      </span>
      <button
        type="button"
        className="button button--secondary"
        disabled={disabled || !hasNext}
        onClick={onNext}
      >
        Next
      </button>
    </nav>
  );
}

export function NodeBadge({
  name,
  status,
}: {
  name: string;
  status?: StatusKind;
}) {
  return (
    <span className="entity-badge entity-badge--node">
      <span aria-hidden="true">●</span>
      {name}
      {status !== undefined && <StatusBadge status={status} />}
    </span>
  );
}

export function RevisionBadge({
  number,
  active = false,
}: {
  number: number;
  active?: boolean;
}) {
  return (
    <span
      className={`entity-badge entity-badge--revision${active ? " entity-badge--active" : ""}`}
    >
      Revision #{number}
      {active ? " · Active" : ""}
    </span>
  );
}

export interface ConvergenceCounts {
  converged: number;
  pending: number;
  drifted: number;
  failed: number;
  maintenance?: number;
}

export function ConvergenceSummary({
  counts,
  label = "Node convergence",
}: {
  counts: ConvergenceCounts;
  label?: string;
}) {
  const total =
    counts.converged +
    counts.pending +
    counts.drifted +
    counts.failed +
    (counts.maintenance ?? 0);
  const status: StatusKind =
    counts.failed > 0
      ? "failed"
      : counts.drifted > 0
        ? "drifted"
        : counts.pending > 0
          ? "pending"
          : "converged";
  return (
    <section className="convergence-summary" aria-label={label}>
      <div>
        <StatusBadge status={status} />
        <strong>
          {counts.converged} of {total} nodes converged
        </strong>
      </div>
      <dl>
        <div>
          <dt>Pending</dt>
          <dd>{counts.pending}</dd>
        </div>
        <div>
          <dt>Drifted</dt>
          <dd>{counts.drifted}</dd>
        </div>
        <div>
          <dt>Failed</dt>
          <dd>{counts.failed}</dd>
        </div>
        <div>
          <dt>Maintenance</dt>
          <dd>{counts.maintenance ?? 0}</dd>
        </div>
      </dl>
    </section>
  );
}

export interface StructuredDifference {
  id: string;
  section: string;
  field: string;
  before: unknown;
  after: unknown;
  summary?: string;
}

export function StructuredDiff({
  differences,
  beforeLabel = "Before",
  afterLabel = "After",
}: {
  differences: readonly StructuredDifference[];
  beforeLabel?: string;
  afterLabel?: string;
}) {
  if (differences.length === 0)
    return (
      <EmptyState title="No differences">
        <p>These values match.</p>
      </EmptyState>
    );
  return (
    <div className="structured-diff">
      {differences.map((difference) => (
        <article key={difference.id} className="structured-diff__row">
          <header>
            <strong>
              {difference.section} / {difference.field}
            </strong>
            {difference.summary !== undefined && (
              <span>{difference.summary}</span>
            )}
          </header>
          <dl>
            <div>
              <dt>{beforeLabel}</dt>
              <dd>
                <code>{formatDiffValue(difference.before)}</code>
              </dd>
            </div>
            <div>
              <dt>{afterLabel}</dt>
              <dd>
                <code>{formatDiffValue(difference.after)}</code>
              </dd>
            </div>
          </dl>
        </article>
      ))}
    </div>
  );
}

export interface TimelineStep {
  id: string;
  label: string;
  status: StatusKind;
  description?: ReactNode;
  time?: ReactNode;
}

export function ProgressTimeline({
  steps,
  label = "Operation progress",
}: {
  steps: readonly TimelineStep[];
  label?: string;
}) {
  return (
    <ol className="progress-timeline" aria-label={label}>
      {steps.map((step) => (
        <li
          key={step.id}
          className={`progress-timeline__step progress-timeline__step--${step.status}`}
        >
          <div>
            <strong>{step.label}</strong>
            {step.description !== undefined && <span>{step.description}</span>}
          </div>
          <div>
            {step.time} <StatusBadge status={step.status} />
          </div>
        </li>
      ))}
    </ol>
  );
}

export interface OperationResult {
  id: string;
  label: string;
  status: "success" | "failed" | "pending";
  message?: ReactNode;
}

export function PartialSuccessPanel({
  title = "Operation partially completed",
  results,
  detailsHref,
}: {
  title?: string;
  results: readonly OperationResult[];
  detailsHref?: string;
}) {
  const succeeded = results.filter(
    (result) => result.status === "success",
  ).length;
  return (
    <section className="partial-success-panel" role="status">
      <header>
        <div>
          <StatusBadge status="warning" label="Partial success" />
          <h2>{title}</h2>
          <p>
            {succeeded} of {results.length} targets succeeded.
          </p>
        </div>
        {detailsHref !== undefined && (
          <a href={detailsHref}>View persistent details</a>
        )}
      </header>
      <ul>
        {results.map((result) => (
          <li key={result.id}>
            <StatusBadge status={result.status} />
            <span>
              <strong>{result.label}</strong>
              {result.message !== undefined && <small>{result.message}</small>}
            </span>
          </li>
        ))}
      </ul>
    </section>
  );
}

function formatDiffValue(value: unknown): string {
  if (value === undefined) return "Not set";
  if (typeof value === "string") return value;
  try {
    return JSON.stringify(value, null, 2);
  } catch {
    return String(value);
  }
}
