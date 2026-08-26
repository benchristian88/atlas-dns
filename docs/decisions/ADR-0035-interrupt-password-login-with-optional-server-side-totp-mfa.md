# ADR-0035: Interrupt password login with optional server-side TOTP MFA

Status: Accepted

Date: 2026-08-26

## Context

Atlas local administrators need optional authenticator-app protection without
changing the controller's server-side session, CSRF, encryption, audit, or
offline recovery boundaries. Creating a normal session after password success
and merely hiding the UI would let that session authorize the API before the
second factor. TOTP seeds and recovery material also require different storage
lifecycles from passwords and sessions.

## Decision

For an MFA-enabled user, valid password verification creates a cryptographically
random, opaque, server-persisted challenge instead of a session. The challenge
expires after five minutes, permits at most five failed factor attempts, is
single-use, and contains only a purpose-separated HMAC hash in PostgreSQL. TOTP
or recovery verification consumes the challenge transactionally before calling
the existing canonical session creation path.

Atlas uses `github.com/pquerna/otp` for RFC 6238 rather than implementing TOTP.
Parameters are issuer `Atlas DNS`, SHA-1, six digits, 30-second period, server
UTC, and ±1 time step. One optional `user_mfa` row stores an AES-256-GCM
envelope under the existing credential key with user-specific associated data.
Ten 80-bit recovery codes are shown once and stored only as purpose-separated
SHA-256 hashes in individual rows; their random entropy makes offline guessing
infeasible without coupling durable recovery state to `SESSION_SECRET`.
Transactionally marking `used_at`
prevents concurrent reuse.

Enrollment begins only after current-password verification and is not enabled
until a current TOTP succeeds. Regeneration, disable, and self-service password
change require current password plus TOTP. Enabling, regeneration, disable, and
password change retain the current session and revoke other sessions. Disable
deletes the seed/codes and consumes outstanding challenges. Losing all factors
has no browser bypass: `atlas-dns-admin reset-mfa --email` requires local host or
container access, deletes factor state, revokes every user session, and audits
the action.

MFA state is part of the control-plane database backup except for transient
challenges. The established format-v1 backup contract from ADR-0031 retains the
credential key only inside the passphrase-encrypted authenticated payload, so
the restored encrypted seed remains usable while plaintext factor material is
never added to a dump, manifest, log, or audit event.

## Consequences

- Password success cannot authorize an MFA-enabled account by itself.
- Existing non-MFA login and secure cookie/CSRF behavior remain unchanged.
- Server clock synchronization is an authentication dependency; Atlas does not
  widen the skew to conceal clock drift.
- The in-process limiter and persisted five-attempt bound protect factor entry;
  distributed throttling remains out of scope with controller HA.
- MFA remains optional per user. Mandatory policy, trusted devices, web admin
  reset, WebAuthn, external identity providers, SMS/email OTP, and push factors
  remain deferred.
