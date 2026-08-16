-- Orgs/Workspaces service schema (orgs_db): organizations, workspaces,
-- memberships (denormalized user identity snapshot), invites, and the
-- signup-request projections (join + create modes) consumed from Auth.
CREATE TABLE IF NOT EXISTS organizations (
    id         UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    owner_id   UUID        NOT NULL,
    name       TEXT        NOT NULL,
    subdomain  TEXT        NOT NULL DEFAULT '',
    plan       TEXT        NOT NULL DEFAULT 'free',    -- free | team | pro | enterprise
    seats_total INTEGER    NOT NULL DEFAULT 5,
    status     TEXT        NOT NULL DEFAULT 'active', -- active | trial | suspended
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS organizations_owner_idx ON organizations (owner_id);

CREATE TABLE IF NOT EXISTS workspaces (
    id              UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    organization_id UUID        NOT NULL REFERENCES organizations(id),
    name            TEXT        NOT NULL,
    repo_source     TEXT        NOT NULL DEFAULT '',
    default_branch  TEXT        NOT NULL DEFAULT 'main',
    glyph           TEXT        NOT NULL DEFAULT '',
    description     TEXT        NOT NULL DEFAULT '',
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS workspaces_org_idx ON workspaces (organization_id);

CREATE TABLE IF NOT EXISTS memberships (
    id                UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    workspace_id      UUID        NOT NULL REFERENCES workspaces(id) ON DELETE CASCADE,
    user_id           UUID        NOT NULL,
    user_name         TEXT        NOT NULL DEFAULT '',
    user_email        TEXT        NOT NULL DEFAULT '',
    role              TEXT        NOT NULL DEFAULT 'member', -- owner | admin | member
    status            TEXT        NOT NULL DEFAULT 'active', -- active | invited | suspended
    last_active_at    TIMESTAMPTZ,
    is_service_account BOOLEAN    NOT NULL DEFAULT false,
    created_at        TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (workspace_id, user_id)
);

CREATE INDEX IF NOT EXISTS memberships_user_idx ON memberships (user_id);

CREATE TABLE IF NOT EXISTS invites (
    id          UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    workspace_id UUID       NOT NULL REFERENCES workspaces(id) ON DELETE CASCADE,
    email       TEXT        NOT NULL,
    name        TEXT        NOT NULL DEFAULT '',
    role        TEXT        NOT NULL DEFAULT 'member',
    invite_code TEXT        NOT NULL UNIQUE,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS invites_workspace_idx ON invites (workspace_id);

-- Signup-request projections (from auth signup.requested events).
CREATE TABLE IF NOT EXISTS join_requests (
    request_id     UUID        PRIMARY KEY,
    user_id        UUID        NOT NULL,
    name           TEXT        NOT NULL,
    email          TEXT        NOT NULL,
    workspace_id   UUID        NOT NULL,
    requested_role TEXT        NOT NULL DEFAULT 'member',
    status         TEXT        NOT NULL DEFAULT 'pending', -- pending | approved | declined
    created_at     TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS join_requests_ws_idx ON join_requests (workspace_id, status);

CREATE TABLE IF NOT EXISTS org_requests (
    request_id       UUID        PRIMARY KEY,
    user_id          UUID        NOT NULL,
    name             TEXT        NOT NULL,
    email            TEXT        NOT NULL,
    organization_name TEXT       NOT NULL DEFAULT '',
    requested_role   TEXT        NOT NULL DEFAULT 'owner',
    status           TEXT        NOT NULL DEFAULT 'pending',
    created_at       TIMESTAMPTZ NOT NULL DEFAULT now()
);
