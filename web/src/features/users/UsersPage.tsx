import { type FormEvent, useCallback, useEffect, useState } from "react";
import { Banner, ErrorState, Loading } from "../../components/Feedback";
import { PageContainer, PageHeader } from "../../components/Page";
import { Field, SettingsGroup } from "../../components/Settings";
import { api } from "../../lib/api";
import type { AdminUser, User } from "../../lib/types";

export function UsersPage({ currentUser }: { currentUser: User }) {
  const [users, setUsers] = useState<AdminUser[]>();
  const [error, setError] = useState<unknown>();
  const [notice, setNotice] = useState("");
  const [email, setEmail] = useState("");
  const [displayName, setDisplayName] = useState("");
  const [password, setPassword] = useState("");
  const load = useCallback(async () => {
    try {
      setUsers((await api.users()).items);
      setError(undefined);
    } catch (caught) {
      setError(caught);
    }
  }, []);
  useEffect(() => {
    void load();
  }, [load]);

  async function create(event: FormEvent) {
    event.preventDefault();
    setNotice("");
    try {
      await api.createUser({ email, displayName, password });
      setEmail("");
      setDisplayName("");
      setPassword("");
      setNotice("Administrator created.");
      await load();
    } catch (caught) {
      setError(caught);
    }
  }

  async function toggle(user: AdminUser) {
    setNotice("");
    try {
      await api.updateUser(user, !user.enabled);
      setNotice(
        `${user.displayName} ${user.enabled ? "disabled" : "enabled"}.`,
      );
      await load();
    } catch (caught) {
      setError(caught);
    }
  }

  async function reset(event: FormEvent<HTMLFormElement>, user: AdminUser) {
    event.preventDefault();
    const form = new FormData(event.currentTarget);
    try {
      await api.resetUserPassword(user.id, String(form.get("password") ?? ""));
      setNotice(`Password reset and sessions revoked for ${user.displayName}.`);
      event.currentTarget.reset();
    } catch (caught) {
      setError(caught);
    }
  }

  return (
    <PageContainer size="wide">
      <PageHeader
        eyebrow="System"
        title="Users"
        description="Local administrator accounts. Atlas DNS Controller 1.0 preserves the established administrator-only permission model."
      />
      {notice && (
        <Banner tone="success" title="User administration updated">
          {notice}
        </Banner>
      )}
      {error !== undefined && (
        <ErrorState error={error} retry={() => void load()} />
      )}
      <SettingsGroup
        title="Create administrator"
        description="The password is hashed immediately and is never returned, logged, or shown again."
      >
        <form
          className="form-stack settings-group-content"
          onSubmit={(event) => void create(event)}
        >
          <Field label="Display name" htmlFor="new-user-name" required>
            <input
              id="new-user-name"
              value={displayName}
              onChange={(event) => setDisplayName(event.target.value)}
              required
              maxLength={120}
            />
          </Field>
          <Field label="Email" htmlFor="new-user-email" required>
            <input
              id="new-user-email"
              type="email"
              value={email}
              onChange={(event) => setEmail(event.target.value)}
              required
            />
          </Field>
          <Field
            label="Initial password"
            htmlFor="new-user-password"
            help="At least 12 characters."
            required
          >
            <input
              id="new-user-password"
              type="password"
              value={password}
              onChange={(event) => setPassword(event.target.value)}
              required
              minLength={12}
              autoComplete="new-password"
            />
          </Field>
          <button className="button" type="submit">
            Create administrator
          </button>
        </form>
      </SettingsGroup>
      <SettingsGroup
        title="Administrators"
        description="Disabling an account immediately prevents authentication and revokes its active sessions. The final enabled administrator is protected server-side."
      >
        {users === undefined && !error ? (
          <Loading label="Loading users…" />
        ) : (
          <div className="user-list">
            {users?.map((user) => (
              <article className="user-card" key={user.id}>
                <header className="user-card__summary">
                  <div>
                    <h3>{user.displayName}</h3>
                    <p className="muted">{user.email} · Administrator</p>
                  </div>
                  <span
                    className={`status ${
                      user.enabled ? "status--healthy" : ""
                    }`}
                  >
                    {user.enabled ? "Enabled" : "Disabled"}
                  </span>
                </header>
                <div className="user-card__details">
                  <p className="user-card__metadata">
                    Created {new Date(user.createdAt).toLocaleString()} · Last
                    login{" "}
                    {user.lastLoginAt
                      ? new Date(user.lastLoginAt).toLocaleString()
                      : "Never"}
                  </p>
                  <button
                    className={
                      user.enabled
                        ? "button button--danger"
                        : "button button--secondary"
                    }
                    type="button"
                    disabled={user.id === currentUser.id && user.enabled}
                    onClick={() => void toggle(user)}
                  >
                    {user.enabled ? "Disable" : "Enable"}
                  </button>
                </div>
                {user.id === currentUser.id && user.enabled && (
                  <p className="field__help user-card__help">
                    Sign in as another administrator before disabling this
                    account.
                  </p>
                )}
                <details className="user-card__password">
                  <summary>Reset password</summary>
                  <form
                    className="form-stack"
                    onSubmit={(event) => void reset(event, user)}
                  >
                    <Field label="New password" required>
                      <input
                        name="password"
                        type="password"
                        minLength={12}
                        required
                        autoComplete="new-password"
                      />
                    </Field>
                    <button className="button button--secondary" type="submit">
                      Reset password and revoke sessions
                    </button>
                  </form>
                </details>
              </article>
            ))}
          </div>
        )}
      </SettingsGroup>
    </PageContainer>
  );
}
