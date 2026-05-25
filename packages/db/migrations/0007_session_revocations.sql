-- 0007_session_revocations.sql
--
-- Per-session and per-user revocation tables for fine-grained session
-- invalidation without rotating the global session secret.
-- Council architecture review 2026-05-24, Approach B (Postgres + bloom filter).

CREATE TABLE core.session_revocations (
    jti        UUID        PRIMARY KEY,
    user_id    UUID        NOT NULL,
    revoked_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    expires_at TIMESTAMPTZ NOT NULL
);

COMMENT ON TABLE core.session_revocations IS
    'Per-session revocation entries. Checked via in-memory bloom filter; '
    'DB hit only on bloom-positive. Rows auto-expire and are cleaned by a periodic job.';

CREATE INDEX idx_session_revocations_expires
    ON core.session_revocations (expires_at);

CREATE TABLE core.user_revocations (
    user_id    UUID        PRIMARY KEY,
    revoked_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

COMMENT ON TABLE core.user_revocations IS
    'User-level revocation: all sessions issued before revoked_at are invalid. '
    'Used for account compromise scenario (revoke-all).';
