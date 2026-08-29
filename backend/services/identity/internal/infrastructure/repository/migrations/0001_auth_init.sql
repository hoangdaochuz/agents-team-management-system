-- Auth service schema (auth_db): users, sessions, signup requests, and the
-- invite-code projection used to resolve join-mode signups locally.
CREATE TABLE IF NOT EXISTS users (
    id             UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    name           TEXT        NOT NULL,
    email          TEXT        NOT NULL UNIQUE,
    password_hash  TEXT        NOT NULL,
    is_active      BOOLEAN     NOT NULL DEFAULT false,
    is_superadmin  BOOLEAN     NOT NULL DEFAULT false,
    created_at     TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS sessions (
    id         UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    token      TEXT        NOT NULL UNIQUE,
    user_id    UUID        NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    expires_at TIMESTAMPTZ NOT NULL
);

CREATE INDEX IF NOT EXISTS sessions_token_idx ON sessions (token);
CREATE INDEX IF NOT EXISTS sessions_user_idx  ON sessions (user_id);

CREATE TABLE IF NOT EXISTS signup_requests (
    id               UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id          UUID        NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    email            TEXT        NOT NULL,
    mode             TEXT        NOT NULL,             -- join | create
    invite_code      TEXT        NOT NULL DEFAULT '',
    workspace_id     UUID,
    workspace_name   TEXT        NOT NULL DEFAULT '',
    organization_name TEXT       NOT NULL DEFAULT '',
    requested_role   TEXT        NOT NULL DEFAULT 'member',
    status           TEXT        NOT NULL DEFAULT 'pending', -- pending | approved | declined
    created_at       TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS signup_requests_email_idx ON signup_requests (email);

-- Invite-code projection (populated from orgs invite.created events).
CREATE TABLE IF NOT EXISTS invite_codes (
    invite_code    TEXT        PRIMARY KEY,
    email          TEXT        NOT NULL DEFAULT '',
    role           TEXT        NOT NULL DEFAULT 'member',
    workspace_id   UUID        NOT NULL,
    workspace_name TEXT        NOT NULL DEFAULT '',
    created_at     TIMESTAMPTZ NOT NULL DEFAULT now()
);
