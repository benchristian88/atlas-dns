import { type FormEvent, useState } from "react";
import { Banner } from "../../components/Feedback";
import { PageHeader } from "../../components/Page";
import { Field, SettingRow, SettingsGroup } from "../../components/Settings";
import { api } from "../../lib/api";
import type { User } from "../../lib/types";
import {
  type ThemePreference,
  useTheme,
} from "../../theme/ThemeProvider";

export function MyAccountPage({ user }: { user: User }) {
  const [currentPassword, setCurrentPassword] = useState("");
  const [newPassword, setNewPassword] = useState("");
  const [confirmation, setConfirmation] = useState("");
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<unknown>();
  const [changed, setChanged] = useState(false);

  async function submit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    setChanged(false);
    if (newPassword !== confirmation) {
      setError(new Error("New password and confirmation do not match."));
      return;
    }
    setBusy(true);
    try {
      await api.changeOwnPassword(currentPassword, newPassword);
      setCurrentPassword("");
      setNewPassword("");
      setConfirmation("");
      setError(undefined);
      setChanged(true);
    } catch (caught) {
      setError(caught);
    } finally {
      setBusy(false);
    }
  }

  return (
    <div className="account-page">
      <PageHeader
        eyebrow="Account"
        title="My Account"
        description="Review your signed-in identity and manage your own credential. Administrator account management remains under Administration → Users."
      />
      {changed && (
        <Banner tone="success" title="Password changed">
          Your current session remains signed in. Other active sessions were
          revoked.
        </Banner>
      )}
      {error !== undefined && (
        <Banner tone="danger" title="Password was not changed">
          {error instanceof Error ? error.message : "Something went wrong."}
        </Banner>
      )}
      <SettingsGroup title="Identity" description="Your current Atlas account.">
        <SettingRow title="Display name" control={user.displayName} />
        <SettingRow title="Email" control={<span>{user.email}</span>} />
        <SettingRow title="Role" control={roleLabel(user.role)} />
      </SettingsGroup>
      <SettingsGroup
        title="Change password"
        description="Confirm your current password before choosing a replacement. Passwords must contain at least 12 characters."
        bodySpacing="padded"
      >
        <form className="form-stack account-password-form" onSubmit={submit}>
          <Field
            label="Current password"
            htmlFor="account-current-password"
            required
          >
            <input
              id="account-current-password"
              type="password"
              autoComplete="current-password"
              value={currentPassword}
              onChange={(event) => setCurrentPassword(event.target.value)}
              required
            />
          </Field>
          <Field label="New password" htmlFor="account-new-password" required>
            <input
              id="account-new-password"
              type="password"
              autoComplete="new-password"
              minLength={12}
              value={newPassword}
              onChange={(event) => setNewPassword(event.target.value)}
              required
            />
          </Field>
          <Field
            label="Confirm new password"
            htmlFor="account-confirm-password"
            required
          >
            <input
              id="account-confirm-password"
              type="password"
              autoComplete="new-password"
              minLength={12}
              value={confirmation}
              onChange={(event) => setConfirmation(event.target.value)}
              required
            />
          </Field>
          <button className="button" type="submit" disabled={busy}>
            {busy ? "Changing password…" : "Change password"}
          </button>
        </form>
      </SettingsGroup>
    </div>
  );
}

const appearanceOptions: readonly {
  value: ThemePreference;
  title: string;
  description: string;
}[] = [
  {
    value: "system",
    title: "System",
    description: "Follow your browser or operating-system appearance.",
  },
  {
    value: "light",
    title: "Light",
    description: "Always use Atlas light appearance.",
  },
  {
    value: "dark",
    title: "Dark",
    description: "Always use Atlas dark appearance.",
  },
];

export function PreferencesPage() {
  const { preference, setPreference } = useTheme();
  return (
    <div className="account-page">
      <PageHeader
        eyebrow="Account"
        title="Preferences"
        description="Presentation choices for this browser. Appearance does not affect controller configuration or authentication."
      />
      <SettingsGroup
        title="Appearance"
        description="Choose how Atlas resolves its light and dark surfaces."
        bodySpacing="padded"
      >
        <fieldset className="appearance-options">
          <legend className="visually-hidden">Appearance</legend>
          {appearanceOptions.map((option) => (
            <label key={option.value}>
              <input
                type="radio"
                name="appearance"
                value={option.value}
                checked={preference === option.value}
                onChange={() => setPreference(option.value)}
              />
              <span>
                <strong>{option.title}</strong>
                <small>{option.description}</small>
              </span>
            </label>
          ))}
        </fieldset>
      </SettingsGroup>
    </div>
  );
}

export function roleLabel(role: User["role"]) {
  return role === "administrator" ? "Administrator" : role;
}
