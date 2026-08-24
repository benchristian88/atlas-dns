import { type FormEvent, useState } from "react";
import { ErrorState } from "../../components/Feedback";
import { api } from "../../lib/api";
import type { Cluster } from "../../lib/types";

export function ClusterCreate({
  onCreated,
}: {
  onCreated: (cluster: Cluster) => void;
}) {
  const [name, setName] = useState("");
  const [description, setDescription] = useState("");
  const [error, setError] = useState<unknown>();
  const [submitting, setSubmitting] = useState(false);

  async function submit(event: FormEvent) {
    event.preventDefault();
    setSubmitting(true);
    setError(undefined);
    try {
      const cluster = await api.createCluster({ name, description });
      onCreated(cluster);
      setName("");
      setDescription("");
    } catch (caught) {
      setError(caught);
    } finally {
      setSubmitting(false);
    }
  }

  return (
    <form
      className="card form-stack compact-form"
      onSubmit={(event) => void submit(event)}
    >
      <h2>Create a cluster</h2>
      <label>
        Name
        <input
          value={name}
          onChange={(event) => setName(event.target.value)}
          required
          maxLength={120}
          placeholder="Home DNS"
        />
      </label>
      <label>
        Description
        <textarea
          value={description}
          onChange={(event) => setDescription(event.target.value)}
          maxLength={2000}
          rows={3}
        />
      </label>
      {error !== undefined && <ErrorState error={error} />}
      <button type="submit" className="button" disabled={submitting}>
        {submitting ? "Creating…" : "Create cluster"}
      </button>
    </form>
  );
}
