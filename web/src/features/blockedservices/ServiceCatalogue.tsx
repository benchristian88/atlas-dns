import { useMemo, useState } from "react";
import { EmptyState } from "../../components/Feedback";
import type {
  BlockedServiceCatalogueService,
  BlockedServicesCatalogue,
} from "../../lib/types";

function groupLabel(id: string) {
  if (id === "") return "Other services";
  return id
    .replace(/[-_]+/g, " ")
    .replace(/\b\w/g, (letter) => letter.toUpperCase());
}

function normaliseSelection(ids: Iterable<string>) {
  return [...new Set([...ids].map((id) => id.trim()).filter(Boolean))].sort();
}

export function ServiceCatalogue({
  catalogue,
  selectedIDs,
  onChange,
}: {
  catalogue: BlockedServicesCatalogue;
  selectedIDs: string[];
  onChange: (ids: string[]) => void;
}) {
  const [search, setSearch] = useState("");
  const [group, setGroup] = useState("all");
  const selected = useMemo(() => new Set(selectedIDs), [selectedIDs]);
  const servicesByID = useMemo(
    () => new Map(catalogue.services.map((service) => [service.id, service])),
    [catalogue.services],
  );
  const unknown = selectedIDs.filter((id) => !servicesByID.has(id));
  const unsupportedSelected = catalogue.services.filter(
    (service) =>
      selected.has(service.id) && service.unsupportedNodeIds.length > 0,
  );
  const query = search.trim().toLocaleLowerCase();
  const filtered = catalogue.services.filter((service) => {
    const selectedGroup = group === "other" ? "" : group;
    if (group !== "all" && (service.groupId ?? "") !== selectedGroup)
      return false;
    if (query === "") return true;
    return [service.name, service.id, groupLabel(service.groupId ?? "")].some(
      (value) => value.toLocaleLowerCase().includes(query),
    );
  });
  const grouped = new Map<string, BlockedServiceCatalogueService[]>();
  for (const service of filtered) {
    const id = service.groupId ?? "";
    grouped.set(id, [...(grouped.get(id) ?? []), service]);
  }
  const groupIDs = [
    ...new Set([
      ...catalogue.groups.map((item) => item.id),
      ...catalogue.services.map((service) => service.groupId ?? ""),
    ]),
  ].sort((left, right) => groupLabel(left).localeCompare(groupLabel(right)));

  const toggle = (id: string, checked: boolean) => {
    const next = new Set(selected);
    if (checked) next.add(id);
    else next.delete(id);
    onChange(normaliseSelection(next));
  };
  const setGroupSelection = (groupID: string, checked: boolean) => {
    const next = new Set(selected);
    for (const service of catalogue.services) {
      if ((service.groupId ?? "") !== groupID) continue;
      if (checked) next.add(service.id);
      else next.delete(service.id);
    }
    onChange(normaliseSelection(next));
  };

  return (
    <div className="service-catalogue">
      <div className="service-catalogue__toolbar">
        <label>
          Search services
          <input
            type="search"
            value={search}
            placeholder="Search by service or group"
            onChange={(event) => setSearch(event.target.value)}
          />
        </label>
        <label>
          Group
          <select
            value={group}
            onChange={(event) => setGroup(event.target.value)}
          >
            <option value="all">All groups</option>
            {groupIDs.map((id) => (
              <option key={id || "other"} value={id || "other"}>
                {groupLabel(id)}
              </option>
            ))}
          </select>
        </label>
        <div className="service-catalogue__count" aria-live="polite">
          <strong>{selected.size}</strong>
          <span>selected</span>
        </div>
      </div>

      {(unknown.length > 0 || unsupportedSelected.length > 0) && (
        <section
          className="service-exceptions"
          aria-labelledby="service-exceptions-title"
        >
          <h3 id="service-exceptions-title">Unknown or unsupported IDs</h3>
          <p className="muted">
            These values are retained in the draft. Publication will identify
            every node that cannot apply them.
          </p>
          <div className="service-exceptions__items">
            {unsupportedSelected.map((service) => (
              <ServiceToggle
                key={service.id}
                service={service}
                checked
                onChange={(checked) => toggle(service.id, checked)}
                exception
                nodeNames={nodeNames(catalogue, service.unsupportedNodeIds)}
              />
            ))}
            {unknown.map((id) => (
              <label
                className="service-toggle service-toggle--exception"
                key={id}
              >
                <input
                  type="checkbox"
                  checked
                  aria-label={`Retain unknown blocked service ${id}`}
                  onChange={(event) => toggle(id, event.target.checked)}
                />
                <span>
                  <strong>{id}</strong>
                  <small>Unknown to every available node catalogue</small>
                </span>
              </label>
            ))}
          </div>
        </section>
      )}

      {filtered.length === 0 ? (
        <EmptyState
          title={
            catalogue.services.length === 0
              ? "No catalogue services are available"
              : "No services match this search"
          }
          filtered={catalogue.services.length > 0}
        >
          <p>
            {catalogue.services.length === 0
              ? "Legacy IDs above remain in the draft until you remove them."
              : "Try another service name or choose a different group."}
          </p>
        </EmptyState>
      ) : (
        <div className="service-groups">
          {[...grouped.entries()]
            .sort(([left], [right]) =>
              groupLabel(left).localeCompare(groupLabel(right)),
            )
            .map(([groupID, services]) => {
              const allSelected = catalogue.services
                .filter((service) => (service.groupId ?? "") === groupID)
                .every((service) => selected.has(service.id));
              return (
                <section className="service-group" key={groupID || "other"}>
                  <header>
                    <div>
                      <h3>{groupLabel(groupID)}</h3>
                      <small>{services.length} services</small>
                    </div>
                    <button
                      type="button"
                      className="button button--secondary"
                      onClick={() => setGroupSelection(groupID, !allSelected)}
                    >
                      {allSelected ? "Clear group" : "Select all"}
                    </button>
                  </header>
                  <div className="service-group__items">
                    {services.map((service) => (
                      <ServiceToggle
                        key={service.id}
                        service={service}
                        checked={selected.has(service.id)}
                        onChange={(checked) => toggle(service.id, checked)}
                        nodeNames={nodeNames(
                          catalogue,
                          service.unsupportedNodeIds,
                        )}
                      />
                    ))}
                  </div>
                </section>
              );
            })}
        </div>
      )}
    </div>
  );
}

function nodeNames(catalogue: BlockedServicesCatalogue, ids: string[]) {
  const names = new Map(
    catalogue.nodes.map((node) => [node.nodeId, node.nodeName]),
  );
  return ids.map((id) => names.get(id) ?? id);
}

function ServiceToggle({
  service,
  checked,
  onChange,
  nodeNames: unsupportedNodes,
  exception = false,
}: {
  service: BlockedServiceCatalogueService;
  checked: boolean;
  onChange: (checked: boolean) => void;
  nodeNames: string[];
  exception?: boolean;
}) {
  return (
    <label
      className={`service-toggle${exception ? " service-toggle--exception" : ""}`}
      data-selected={checked || undefined}
    >
      <input
        type="checkbox"
        checked={checked}
        aria-label={`Block ${service.name}`}
        onChange={(event) => onChange(event.target.checked)}
      />
      <span>
        <strong>{service.name}</strong>
        <small className="monospace">{service.id}</small>
        {unsupportedNodes.length > 0 && (
          <small className="service-toggle__warning">
            Unsupported by {unsupportedNodes.join(", ")}
          </small>
        )}
      </span>
    </label>
  );
}
