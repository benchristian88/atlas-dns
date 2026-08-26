import { type FormEvent, useEffect, useRef, useState } from "react";
import { Banner } from "../../components/Feedback";
import { PageHeader } from "../../components/Page";
import { Field, SettingRow, SettingsGroup } from "../../components/Settings";
import { api } from "../../lib/api";
import type { MFAEnrollment, MFAStatus, User } from "../../lib/types";
import { type ThemePreference, useTheme } from "../../theme/ThemeProvider";

export function MyAccountPage({ user }: { user: User }) {
  const [currentPassword, setCurrentPassword] = useState("");
  const [newPassword, setNewPassword] = useState("");
  const [confirmation, setConfirmation] = useState("");
  const [passwordTOTP, setPasswordTOTP] = useState("");
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<unknown>();
  const [changed, setChanged] = useState(false);
  const [mfaStatus, setMFAStatus] = useState<MFAStatus>();
  const [mfaError, setMFAError] = useState<unknown>();
  const [mfaBusy, setMFABusy] = useState(false);
  const [mfaAction, setMFAAction] = useState<
    "none" | "enroll" | "regenerate" | "disable"
  >("none");
  const [mfaPassword, setMFAPassword] = useState("");
  const [mfaCode, setMFACode] = useState("");
  const [enrollment, setEnrollment] = useState<MFAEnrollment>();
  const [recoveryCodes, setRecoveryCodes] = useState<string[]>();
  const enrollmentCodeInput = useRef<HTMLInputElement>(null);

  useEffect(() => {
    let active = true;
    void api
      .mfaStatus()
      .then((status) => {
        if (active) setMFAStatus(status);
      })
      .catch((caught) => {
        if (active) setMFAError(caught);
      });
    return () => {
      active = false;
    };
  }, []);

  useEffect(() => {
    if (enrollment !== undefined) enrollmentCodeInput.current?.focus();
  }, [enrollment]);

  async function submit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    setChanged(false);
    if (newPassword !== confirmation) {
      setError(new Error("New password and confirmation do not match."));
      return;
    }
    setBusy(true);
    try {
      await api.changeOwnPassword(currentPassword, newPassword, passwordTOTP);
      setCurrentPassword("");
      setNewPassword("");
      setConfirmation("");
      setPasswordTOTP("");
      setError(undefined);
      setChanged(true);
    } catch (caught) {
      setError(caught);
    } finally {
      setBusy(false);
    }
  }

  function resetMFAAction() {
    setMFAAction("none");
    setMFAPassword("");
    setMFACode("");
    setEnrollment(undefined);
    setMFAError(undefined);
  }

  async function startEnrollment(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    setMFABusy(true);
    setMFAError(undefined);
    try {
      setEnrollment(await api.startMFAEnrollment(mfaPassword));
      setMFAPassword("");
    } catch (caught) {
      setMFAError(caught);
    } finally {
      setMFABusy(false);
    }
  }

  async function verifyEnrollment(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    setMFABusy(true);
    setMFAError(undefined);
    try {
      const result = await api.verifyMFAEnrollment(mfaCode);
      setRecoveryCodes(result.recoveryCodes);
      setMFAStatus({
        enabled: true,
        recoveryCodesRemaining: result.recoveryCodesRemaining,
      });
      setEnrollment(undefined);
      setMFACode("");
      setMFAAction("none");
    } catch (caught) {
      setMFAError(caught);
    } finally {
      setMFABusy(false);
    }
  }

  async function manageMFA(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    setMFABusy(true);
    setMFAError(undefined);
    try {
      if (mfaAction === "regenerate") {
        const result = await api.regenerateMFARecoveryCodes(
          mfaPassword,
          mfaCode,
        );
        setRecoveryCodes(result.recoveryCodes);
        setMFAStatus({
          enabled: true,
          recoveryCodesRemaining: result.recoveryCodesRemaining,
        });
      } else if (mfaAction === "disable") {
        await api.disableMFA(mfaPassword, mfaCode);
        setMFAStatus({ enabled: false, recoveryCodesRemaining: 0 });
      }
      resetMFAAction();
    } catch (caught) {
      setMFAError(caught);
    } finally {
      setMFABusy(false);
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
        title="Two-factor authentication"
        description="Protect your local Atlas account with a standards-compatible authenticator app."
        bodySpacing="padded"
      >
        {mfaError !== undefined && (
          <Banner
            tone="danger"
            title="Two-factor authentication was not updated"
          >
            {mfaError instanceof Error
              ? mfaError.message
              : "Something went wrong."}
          </Banner>
        )}
        {recoveryCodes !== undefined ? (
          <section className="mfa-recovery-display" aria-live="polite">
            <h3>Save your recovery codes</h3>
            <p>
              Each code works once. These codes will not be shown again after
              you leave this display.
            </p>
            <ul className="mfa-recovery-codes">
              {recoveryCodes.map((code) => (
                <li key={code}>
                  <code>{code}</code>
                </li>
              ))}
            </ul>
            <button
              className="button"
              type="button"
              onClick={() => setRecoveryCodes(undefined)}
            >
              I have saved these codes
            </button>
          </section>
        ) : enrollment !== undefined ? (
          <section className="mfa-enrollment">
            <h3>Scan the QR code</h3>
            <p>
              Add <strong>{enrollment.accountLabel}</strong> under issuer{" "}
              <strong>{enrollment.issuer}</strong>, then enter the current code.
            </p>
            <img
              className="mfa-qr-code"
              src={enrollment.qrCodeDataUrl}
              alt="QR code for this active two-factor enrollment"
            />
            <p>
              Manual secret:{" "}
              <code className="mfa-manual-secret">{enrollment.secret}</code>
            </p>
            <form className="form-stack mfa-form" onSubmit={verifyEnrollment}>
              <Field
                label="Authenticator code"
                htmlFor="mfa-enrollment-code"
                required
              >
                <input
                  id="mfa-enrollment-code"
                  ref={enrollmentCodeInput}
                  inputMode="numeric"
                  autoComplete="one-time-code"
                  pattern="[0-9]{6}"
                  maxLength={6}
                  value={mfaCode}
                  onChange={(event) => setMFACode(event.target.value)}
                  required
                />
              </Field>
              <div className="mfa-actions">
                <button className="button" type="submit" disabled={mfaBusy}>
                  {mfaBusy ? "Verifying…" : "Verify and enable"}
                </button>
                <button
                  className="button button--secondary"
                  type="button"
                  onClick={resetMFAAction}
                >
                  Cancel
                </button>
              </div>
            </form>
          </section>
        ) : mfaAction === "enroll" ? (
          <form className="form-stack mfa-form" onSubmit={startEnrollment}>
            <p>Confirm your current password to begin enrollment.</p>
            <Field
              label="Current password"
              htmlFor="mfa-enroll-password"
              required
            >
              <input
                id="mfa-enroll-password"
                type="password"
                autoComplete="current-password"
                value={mfaPassword}
                onChange={(event) => setMFAPassword(event.target.value)}
                required
              />
            </Field>
            <div className="mfa-actions">
              <button className="button" type="submit" disabled={mfaBusy}>
                {mfaBusy ? "Starting…" : "Continue"}
              </button>
              <button
                className="button button--secondary"
                type="button"
                onClick={resetMFAAction}
              >
                Cancel
              </button>
            </div>
          </form>
        ) : mfaAction === "regenerate" || mfaAction === "disable" ? (
          <form className="form-stack mfa-form" onSubmit={manageMFA}>
            <p>
              Enter your current password and authenticator code to{" "}
              {mfaAction === "disable"
                ? "disable two-factor authentication"
                : "replace every unused recovery code"}
              .
            </p>
            <Field
              label="Current password"
              htmlFor="mfa-manage-password"
              required
            >
              <input
                id="mfa-manage-password"
                type="password"
                autoComplete="current-password"
                value={mfaPassword}
                onChange={(event) => setMFAPassword(event.target.value)}
                required
              />
            </Field>
            <Field
              label="Authenticator code"
              htmlFor="mfa-manage-code"
              required
            >
              <input
                id="mfa-manage-code"
                inputMode="numeric"
                autoComplete="one-time-code"
                pattern="[0-9]{6}"
                maxLength={6}
                value={mfaCode}
                onChange={(event) => setMFACode(event.target.value)}
                required
              />
            </Field>
            <div className="mfa-actions">
              <button
                className={
                  mfaAction === "disable" ? "button button--danger" : "button"
                }
                type="submit"
                disabled={mfaBusy}
              >
                {mfaBusy
                  ? "Verifying…"
                  : mfaAction === "disable"
                    ? "Disable two-factor authentication"
                    : "Regenerate recovery codes"}
              </button>
              <button
                className="button button--secondary"
                type="button"
                onClick={resetMFAAction}
              >
                Cancel
              </button>
            </div>
          </form>
        ) : mfaStatus === undefined ? (
          <p className="muted">Loading two-factor status…</p>
        ) : mfaStatus.enabled ? (
          <div className="mfa-status">
            <p>
              <strong>Enabled</strong>
            </p>
            <p>Recovery codes remaining: {mfaStatus.recoveryCodesRemaining}</p>
            <div className="mfa-actions">
              <button
                className="button button--secondary"
                type="button"
                onClick={() => setMFAAction("regenerate")}
              >
                Regenerate recovery codes
              </button>
              <button
                className="button button--danger"
                type="button"
                onClick={() => setMFAAction("disable")}
              >
                Disable two-factor authentication
              </button>
            </div>
          </div>
        ) : (
          <div className="mfa-status">
            <p>
              <strong>Not enabled</strong>
            </p>
            <p>Protect your account with an authenticator app.</p>
            <button
              className="button"
              type="button"
              onClick={() => setMFAAction("enroll")}
            >
              Enable two-factor authentication
            </button>
          </div>
        )}
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
          {mfaStatus?.enabled && (
            <Field
              label="Authenticator code"
              htmlFor="account-password-totp"
              required
            >
              <input
                id="account-password-totp"
                inputMode="numeric"
                autoComplete="one-time-code"
                pattern="[0-9]{6}"
                maxLength={6}
                value={passwordTOTP}
                onChange={(event) => setPasswordTOTP(event.target.value)}
                required
              />
            </Field>
          )}
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
