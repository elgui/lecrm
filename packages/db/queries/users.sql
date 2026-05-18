-- users.sql — typed queries for the (issuer, subject) identity registry.
--
-- Users are keyed on (issuer, subject) per ADR-009 §7.1; the tuple keeps
-- the v0→v1 IDP migration a mapping table, not a destructive rewrite.
-- Authentik issues UUID-format `subject`; Zitadel issues numeric strings.

-- name: GetUserByIssuerSub :one
-- Returns the user_id for the given (issuer, subject) tuple, or an error
-- when no matching row exists. Callers check for pgx.ErrNoRows to
-- distinguish "unknown identity" from other failures.
SELECT id
FROM core.users
WHERE issuer = $1 AND subject = $2;

-- name: UpsertUserIdentity :one
-- Inserts a new user identity row or, on conflict, refreshes the mutable
-- display columns and last_login_at. The (issuer, subject) tuple is
-- immutable after creation; only email, display_name, and timestamps
-- are updated. Returns the user's UUID in both paths.
INSERT INTO core.users (issuer, subject, email, display_name, last_login_at)
VALUES ($1, $2, NULLIF($3, ''), NULLIF($4, ''), now())
ON CONFLICT (issuer, subject) DO UPDATE SET
    email         = COALESCE(NULLIF(EXCLUDED.email, ''), core.users.email),
    display_name  = COALESCE(NULLIF(EXCLUDED.display_name, ''), core.users.display_name),
    last_login_at = now(),
    updated_at    = now()
RETURNING id;
