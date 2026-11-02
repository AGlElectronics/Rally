-- Rally (Events) — initial schema.
--
-- Scope: the foundation layer only — organizations, conventions, editions,
-- users with (issuer, subject) external identities, memberships, role
-- assignments, and an append-only audit log. CfP / submissions / schedule
-- are deliberately left for the first vertical slice migration.
--
-- Naming note: internal packages use the neutral "events" domain language
-- even though the repository folder is "Rally". These tables intentionally
-- avoid convention-specific or product-brand terms.
--
-- See docs/adr/0001-initial-scaffold.md for the invariants this schema
-- defends.

-- +goose Up
-- +goose StatementBegin
CREATE EXTENSION IF NOT EXISTS pgcrypto;
-- +goose StatementEnd

-- ----------------------------------------------------------------------------
-- ORGANIZATIONS / CONVENTIONS / EDITIONS
-- ----------------------------------------------------------------------------
-- Three-level ownership. An organization owns conventions (recurring brands);
-- a convention has editions (one actual occurrence per year, with its own
-- dates, timezone, branding and settings). Every downstream resource — users'
-- memberships, submissions, programme items, publications — hangs off one
-- organization and (where relevant) one edition.

CREATE TABLE organizations (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    slug            TEXT NOT NULL UNIQUE,
    display_name    TEXT NOT NULL,
    -- Opaque JSON blob so a convention can set its own name, colors, logo
    -- URL, locale, etc. without a schema change. Keep it small; large assets
    -- belong in object storage, referenced by URL.
    branding_config JSONB NOT NULL DEFAULT '{}'::jsonb,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    CHECK (slug = lower(slug) AND slug ~ '^[a-z0-9][a-z0-9-]*[a-z0-9]$')
);

CREATE TABLE conventions (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    organization_id UUID NOT NULL REFERENCES organizations(id) ON DELETE RESTRICT,
    slug            TEXT NOT NULL,
    name            TEXT NOT NULL,
    description     TEXT NOT NULL DEFAULT '',
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (organization_id, slug),
    -- Composite unique lets children (editions) FK on (convention_id,
    -- organization_id) to prove the two tables stay owned by the same org.
    UNIQUE (id, organization_id),
    CHECK (slug = lower(slug) AND slug ~ '^[a-z0-9][a-z0-9-]*[a-z0-9]$')
);

CREATE INDEX conventions_org_idx ON conventions (organization_id);

CREATE TABLE editions (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    organization_id UUID NOT NULL REFERENCES organizations(id) ON DELETE RESTRICT,
    convention_id   UUID NOT NULL,
    slug            TEXT NOT NULL,
    title           TEXT NOT NULL,
    -- IANA timezone (e.g. "Europe/Vienna"). All start/end timestamps are
    -- stored UTC; this is what we render the public programme in.
    timezone        TEXT NOT NULL DEFAULT 'UTC',
    start_at        TIMESTAMPTZ,
    end_at          TIMESTAMPTZ,
    status          TEXT NOT NULL DEFAULT 'draft'
                    CHECK (status IN ('draft','planning','live','archived','cancelled')),
    settings        JSONB NOT NULL DEFAULT '{}'::jsonb,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (convention_id, slug),
    CHECK (slug = lower(slug) AND slug ~ '^[a-z0-9][a-z0-9-]*[a-z0-9]$'),
    -- Allow either to be NULL (edition still being planned) but if both are
    -- set, start must precede end.
    CHECK (start_at IS NULL OR end_at IS NULL OR start_at < end_at),
    -- Enforce that convention and edition share the same organization so a
    -- client can't post an edition with a mismatched (convention_id,
    -- organization_id). The composite FK below is only valid because
    -- conventions carries the matching UNIQUE (id, organization_id).
    FOREIGN KEY (convention_id, organization_id)
        REFERENCES conventions (id, organization_id) ON DELETE RESTRICT
);

CREATE INDEX editions_org_idx         ON editions (organization_id);
CREATE INDEX editions_convention_idx  ON editions (convention_id);
CREATE INDEX editions_status_idx      ON editions (status);
CREATE INDEX editions_start_idx       ON editions (start_at);

-- ----------------------------------------------------------------------------
-- USERS & EXTERNAL IDENTITIES
-- ----------------------------------------------------------------------------
-- The users row is the stable internal identity. Email and display_name are
-- freshness-of-metadata only; they MUST NOT be used as join keys or identity
-- comparators. The real identity key is the (issuer, subject) pair in
-- external_identities (architecture.md §4.4).

CREATE TABLE users (
    id             UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    display_name   TEXT NOT NULL,
    email          TEXT,
    is_disabled    BOOLEAN NOT NULL DEFAULT FALSE,
    last_login_at  TIMESTAMPTZ,
    created_at     TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at     TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- Email is a convenience lookup only; it is case-insensitive and MAY be
-- duplicated across accounts (two humans sharing an alias). It is NEVER the
-- identity primary key.
CREATE INDEX users_email_lower_idx ON users (lower(email));

CREATE TABLE external_identities (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id     UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    -- OIDC issuer URL as advertised by discovery; "dev" is reserved for the
    -- dev-mode bypass used during local scaffolding (AUTH_MODE=dev).
    issuer      TEXT NOT NULL,
    -- OIDC `sub` claim; opaque, stable, provider-assigned.
    subject     TEXT NOT NULL,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (issuer, subject)
);

CREATE INDEX external_identities_user_idx ON external_identities (user_id);

-- ----------------------------------------------------------------------------
-- MEMBERSHIPS & ROLE ASSIGNMENTS
-- ----------------------------------------------------------------------------
-- Membership = "this user is on the roster for this organization" (a
-- precondition for any role assignment). role_assignments carry the actual
-- authority and can be scoped either to the whole organization or to one
-- edition. Role names come from the access config (TOML, loaded at startup),
-- not from the DB, so adding a new role does not require a migration.

CREATE TABLE memberships (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    organization_id UUID NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
    user_id         UUID NOT NULL REFERENCES users(id)         ON DELETE CASCADE,
    status          TEXT NOT NULL DEFAULT 'active'
                    CHECK (status IN ('invited','active','suspended','removed')),
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (organization_id, user_id)
);

CREATE INDEX memberships_user_idx ON memberships (user_id);

CREATE TABLE role_assignments (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    organization_id UUID NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
    -- NULL = org-scoped; set = edition-scoped. We don't FK (edition_id,
    -- organization_id) back to editions because deleting an edition should
    -- cascade the assignments away; the composite check lives in the
    -- service layer when the assignment is created.
    edition_id      UUID REFERENCES editions(id) ON DELETE CASCADE,
    user_id         UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    role            TEXT NOT NULL,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    created_by      UUID REFERENCES users(id)
);

CREATE INDEX role_assignments_org_user_idx ON role_assignments (organization_id, user_id);
CREATE INDEX role_assignments_user_idx     ON role_assignments (user_id);
CREATE INDEX role_assignments_edition_idx  ON role_assignments (edition_id);

-- Prevent duplicate (user, role, scope) rows. Separate unique indexes so one
-- user can hold the same role at both the org level and inside one edition
-- without collision.
CREATE UNIQUE INDEX role_assignments_org_unique
    ON role_assignments (organization_id, user_id, role)
    WHERE edition_id IS NULL;

CREATE UNIQUE INDEX role_assignments_edition_unique
    ON role_assignments (edition_id, user_id, role)
    WHERE edition_id IS NOT NULL;

-- ----------------------------------------------------------------------------
-- AUDIT EVENTS
-- ----------------------------------------------------------------------------
-- Append-only forensic trail for privileged state changes. Written inside
-- the same transaction as the change. Deliberately does NOT accept raw
-- tokens or free-form PII dumps — before/after should be the smallest
-- business-relevant projection.

CREATE TABLE audit_events (
    id                  UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    organization_id     UUID NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
    edition_id          UUID REFERENCES editions(id) ON DELETE SET NULL,
    actor_user_id       UUID REFERENCES users(id)    ON DELETE SET NULL,
    -- Snapshot the actor's external identity at write-time so the audit row
    -- survives even if the user is later deleted or re-linked. Null in
    -- unauthenticated contexts (e.g. public programme reads — which should
    -- not generate audit rows at all).
    actor_issuer        TEXT,
    actor_subject       TEXT,
    actor_display_name  TEXT,
    action              TEXT NOT NULL,
    resource_type       TEXT NOT NULL,
    resource_id         UUID NOT NULL,
    before_json         JSONB,
    after_json          JSONB,
    note                TEXT NOT NULL DEFAULT '',
    occurred_at         TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX audit_events_org_time_idx    ON audit_events (organization_id, occurred_at DESC);
CREATE INDEX audit_events_edition_idx     ON audit_events (edition_id, occurred_at DESC);
CREATE INDEX audit_events_resource_idx    ON audit_events (resource_type, resource_id);
CREATE INDEX audit_events_actor_idx       ON audit_events (actor_user_id);

-- ----------------------------------------------------------------------------
-- DEV MODE SEED
-- ----------------------------------------------------------------------------
-- Only loaded when AUTH_MODE=dev is used — see internal/auth/resolver.go,
-- which looks up this subject on first dev request. The organization and
-- membership below exist so the SPA can show something useful on page one
-- without a wizard. Delete via a later migration before production.

INSERT INTO organizations (id, slug, display_name)
VALUES ('00000000-0000-0000-0000-000000000010', 'demo-org', 'Demo Organization')
ON CONFLICT (slug) DO NOTHING;

INSERT INTO conventions (id, organization_id, slug, name, description)
VALUES (
    '00000000-0000-0000-0000-000000000020',
    '00000000-0000-0000-0000-000000000010',
    'demo-con',
    'Demo Convention',
    'Placeholder convention used by the Rally scaffold dev seed.'
) ON CONFLICT (organization_id, slug) DO NOTHING;

INSERT INTO editions (
    id, organization_id, convention_id, slug, title, timezone, status
)
VALUES (
    '00000000-0000-0000-0000-000000000030',
    '00000000-0000-0000-0000-000000000010',
    '00000000-0000-0000-0000-000000000020',
    'next',
    'Demo Convention — Next Edition',
    'UTC',
    'planning'
) ON CONFLICT (convention_id, slug) DO NOTHING;

INSERT INTO users (id, display_name, email)
VALUES ('00000000-0000-0000-0000-000000000001', 'Dev Admin', 'dev-admin@rally.local')
ON CONFLICT (id) DO NOTHING;

INSERT INTO external_identities (user_id, issuer, subject)
VALUES ('00000000-0000-0000-0000-000000000001', 'dev', 'dev-admin')
ON CONFLICT (issuer, subject) DO NOTHING;

INSERT INTO memberships (organization_id, user_id, status)
VALUES (
    '00000000-0000-0000-0000-000000000010',
    '00000000-0000-0000-0000-000000000001',
    'active'
) ON CONFLICT (organization_id, user_id) DO NOTHING;

INSERT INTO role_assignments (organization_id, edition_id, user_id, role)
VALUES (
    '00000000-0000-0000-0000-000000000010',
    NULL,
    '00000000-0000-0000-0000-000000000001',
    'organization_admin'
) ON CONFLICT DO NOTHING;


-- +goose Down
DROP TABLE IF EXISTS audit_events;
DROP TABLE IF EXISTS role_assignments;
DROP TABLE IF EXISTS memberships;
DROP TABLE IF EXISTS external_identities;
DROP TABLE IF EXISTS users;
DROP TABLE IF EXISTS editions;
DROP TABLE IF EXISTS conventions;
DROP TABLE IF EXISTS organizations;
