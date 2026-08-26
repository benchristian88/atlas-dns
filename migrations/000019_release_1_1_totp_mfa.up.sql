-- Release 1.1 optional TOTP MFA. Plaintext TOTP and recovery material never
-- enters PostgreSQL; challenges and recovery use purpose-separated token hashes.
CREATE TABLE user_mfa (
    user_id uuid PRIMARY KEY REFERENCES users(id) ON DELETE CASCADE,
    encrypted_secret bytea NOT NULL,
    secret_nonce bytea NOT NULL,
    secret_key_version integer NOT NULL,
    secret_algorithm text NOT NULL,
    enrollment_started_at timestamptz NOT NULL,
    enrollment_expires_at timestamptz NOT NULL,
    enabled_at timestamptz,
    CHECK (enrollment_expires_at > enrollment_started_at),
    CHECK (enabled_at IS NULL OR enabled_at >= enrollment_started_at)
);

CREATE TABLE user_mfa_recovery_codes (
    id uuid PRIMARY KEY,
    user_id uuid NOT NULL REFERENCES user_mfa(user_id) ON DELETE CASCADE,
    code_hash bytea NOT NULL,
    created_at timestamptz NOT NULL,
    used_at timestamptz,
    UNIQUE (user_id, code_hash),
    CHECK (used_at IS NULL OR used_at >= created_at)
);

CREATE INDEX user_mfa_recovery_codes_unused_idx
    ON user_mfa_recovery_codes (user_id, created_at)
    WHERE used_at IS NULL;

CREATE TABLE mfa_challenges (
    id uuid PRIMARY KEY,
    user_id uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    token_hash bytea NOT NULL UNIQUE,
    created_at timestamptz NOT NULL,
    expires_at timestamptz NOT NULL,
    consumed_at timestamptz,
    failed_attempts integer NOT NULL DEFAULT 0 CHECK (failed_attempts BETWEEN 0 AND 5),
    ip_metadata text NOT NULL DEFAULT '',
    user_agent text NOT NULL DEFAULT '',
    CHECK (expires_at > created_at),
    CHECK (consumed_at IS NULL OR consumed_at >= created_at)
);

CREATE INDEX mfa_challenges_expiry_idx
    ON mfa_challenges (expires_at)
    WHERE consumed_at IS NULL;

CREATE INDEX mfa_challenges_user_idx
    ON mfa_challenges (user_id, created_at DESC);
