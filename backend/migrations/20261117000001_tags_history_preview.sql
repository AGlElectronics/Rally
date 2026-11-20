-- Rally — staff-approved tags, per-item revision history, and a read-only
-- schedule preview window configured by Rally admins/leads.
--
-- Tags are organization-scoped: anyone who can edit the schedule may
-- propose one; staff with tag.approve lift it to "approved" once and it
-- then shows in autocomplete everywhere in the organisation. The join
-- tables schedule_item_tags and submission_tags attach approved tags to
-- the two things that carry them.
--
-- schedule_item_revisions records every edit to a draft schedule item
-- (create, update, delete, live-day change, restore) separately from the
-- edition-wide schedule_versions history. The schedule_item_id is a plain
-- UUID (no FK) so a revision outlives the item.
--
-- The preview columns on schedules let admins open the draft for read-only
-- viewing during a configured window, with an optional preview_public_url
-- that Den quick-links can point at.

-- +goose Up
CREATE TABLE tags (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    organization_id UUID NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
    -- Normalised (lower case, single spaces) for the uniqueness check.
    name            TEXT NOT NULL CHECK (length(btrim(name)) BETWEEN 1 AND 60),
    -- The form the submitter typed, shown to humans.
    display         TEXT NOT NULL CHECK (length(btrim(display)) BETWEEN 1 AND 60),
    status          TEXT NOT NULL DEFAULT 'proposed'
                    CHECK (status IN ('proposed','approved','rejected')),
    proposed_by     UUID REFERENCES users(id) ON DELETE SET NULL,
    decided_by      UUID REFERENCES users(id) ON DELETE SET NULL,
    decided_at      TIMESTAMPTZ,
    note            TEXT NOT NULL DEFAULT '',
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (organization_id, name)
);
CREATE INDEX tags_org_status_idx ON tags (organization_id, status, name);

CREATE TABLE schedule_item_tags (
    schedule_item_id UUID NOT NULL REFERENCES schedule_items(id) ON DELETE CASCADE,
    tag_id           UUID NOT NULL REFERENCES tags(id) ON DELETE CASCADE,
    created_at       TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (schedule_item_id, tag_id)
);
CREATE INDEX schedule_item_tags_tag_idx ON schedule_item_tags (tag_id);

CREATE TABLE submission_tags (
    submission_id UUID NOT NULL REFERENCES submissions(id) ON DELETE CASCADE,
    tag_id        UUID NOT NULL REFERENCES tags(id) ON DELETE CASCADE,
    created_at    TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (submission_id, tag_id)
);
CREATE INDEX submission_tags_tag_idx ON submission_tags (tag_id);

CREATE TABLE schedule_item_revisions (
    id               UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    organization_id  UUID NOT NULL,
    edition_id       UUID NOT NULL,
    schedule_item_id UUID NOT NULL,
    action           TEXT NOT NULL CHECK (action IN ('create','update','delete','live','restore')),
    note             TEXT NOT NULL DEFAULT '',
    before_data      JSONB,
    after_data       JSONB,
    actor_user_id    UUID REFERENCES users(id) ON DELETE SET NULL,
    actor_display    TEXT NOT NULL DEFAULT '',
    created_at       TIMESTAMPTZ NOT NULL DEFAULT now(),
    FOREIGN KEY (edition_id, organization_id)
        REFERENCES editions (id, organization_id) ON DELETE CASCADE
);
CREATE INDEX schedule_item_revisions_item_idx
    ON schedule_item_revisions (schedule_item_id, created_at DESC);
CREATE INDEX schedule_item_revisions_edition_idx
    ON schedule_item_revisions (edition_id, created_at DESC);

-- Preview window. Both nullable; preview_opens_at NULL = preview never
-- opens automatically. preview_public_url is the URL Den quick-links use.
ALTER TABLE schedules
    ADD COLUMN preview_opens_at   TIMESTAMPTZ,
    ADD COLUMN preview_closes_at  TIMESTAMPTZ,
    ADD COLUMN preview_public_url TEXT NOT NULL DEFAULT '',
    ADD CONSTRAINT schedules_preview_window_chk
        CHECK (preview_opens_at IS NULL OR preview_closes_at IS NULL
               OR preview_opens_at < preview_closes_at);

-- +goose Down
ALTER TABLE schedules DROP CONSTRAINT IF EXISTS schedules_preview_window_chk;
ALTER TABLE schedules DROP COLUMN IF EXISTS preview_public_url;
ALTER TABLE schedules DROP COLUMN IF EXISTS preview_closes_at;
ALTER TABLE schedules DROP COLUMN IF EXISTS preview_opens_at;
DROP TABLE IF EXISTS schedule_item_revisions;
DROP TABLE IF EXISTS submission_tags;
DROP TABLE IF EXISTS schedule_item_tags;
DROP TABLE IF EXISTS tags;
