-- Rally (Events) — schedule builder, publishing and live day (PoC phases 2+3).
--
-- Rooms belong to an edition; event types may be limited to some rooms. The
-- draft schedule is a set of schedule_items the events lead edits freely;
-- publishing freezes a JSON snapshot in schedule_versions, which is all the
-- public API ever serves. Live-day changes patch both the draft and a new
-- published version, and draft Telegram announcements a person confirms.

-- +goose Up
CREATE TABLE rooms (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    organization_id UUID NOT NULL,
    edition_id      UUID NOT NULL,
    name            TEXT NOT NULL CHECK (length(btrim(name)) > 0),
    capacity        INT CHECK (capacity IS NULL OR capacity > 0),
    sort_order      INT NOT NULL DEFAULT 0,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (edition_id, name),
    FOREIGN KEY (edition_id, organization_id)
        REFERENCES editions (id, organization_id) ON DELETE RESTRICT
);

CREATE INDEX rooms_edition_idx ON rooms (edition_id, sort_order);

-- Empty = the event type may use every room.
ALTER TABLE event_types ADD COLUMN room_ids UUID[] NOT NULL DEFAULT '{}';

-- One row per edition, created on first use.
CREATE TABLE schedules (
    edition_id       UUID PRIMARY KEY,
    organization_id  UUID NOT NULL,
    -- Minutes after local midnight; only size the grid.
    day_start_minute INT NOT NULL DEFAULT 540  CHECK (day_start_minute BETWEEN 0 AND 1439),
    day_end_minute   INT NOT NULL DEFAULT 1380 CHECK (day_end_minute BETWEEN 1 AND 1440),
    snap_minutes     INT NOT NULL DEFAULT 15   CHECK (snap_minutes IN (5, 10, 15, 30)),
    -- {"no_overlap":"block","gap":"block","person":"warn","room_fit":"block"}
    rules            JSONB NOT NULL DEFAULT '{}'::jsonb CHECK (jsonb_typeof(rules) = 'object'),
    -- Bumped by every draft edit; compared with the last publish.
    draft_updated_at TIMESTAMPTZ,
    updated_at       TIMESTAMPTZ NOT NULL DEFAULT now(),
    CHECK (day_start_minute < day_end_minute),
    FOREIGN KEY (edition_id, organization_id)
        REFERENCES editions (id, organization_id) ON DELETE CASCADE
);

CREATE TABLE schedule_items (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    organization_id UUID NOT NULL,
    edition_id      UUID NOT NULL,
    -- NULL = staff-made item (opening ceremony, break...).
    submission_id   UUID UNIQUE REFERENCES submissions(id) ON DELETE CASCADE,
    room_id         UUID NOT NULL REFERENCES rooms(id) ON DELETE RESTRICT,
    event_type_id   UUID REFERENCES event_types(id) ON DELETE SET NULL,
    title           TEXT NOT NULL CHECK (length(btrim(title)) > 0),
    description     TEXT NOT NULL DEFAULT '',
    starts_at       TIMESTAMPTZ NOT NULL,
    ends_at         TIMESTAMPTZ NOT NULL,
    staff_only      BOOLEAN NOT NULL DEFAULT FALSE,
    status          TEXT NOT NULL DEFAULT 'scheduled' CHECK (status IN ('scheduled','cancelled')),
    created_by      UUID REFERENCES users(id) ON DELETE SET NULL,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    CHECK (starts_at < ends_at),
    FOREIGN KEY (edition_id, organization_id)
        REFERENCES editions (id, organization_id) ON DELETE CASCADE
);

CREATE INDEX schedule_items_edition_idx ON schedule_items (edition_id, starts_at);

CREATE TABLE schedule_versions (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    organization_id UUID NOT NULL,
    edition_id      UUID NOT NULL,
    version         INT NOT NULL CHECK (version > 0),
    data            JSONB NOT NULL CHECK (jsonb_typeof(data) = 'object'),
    note            TEXT NOT NULL DEFAULT '',
    -- 'publish' (from the draft), 'live' (a live-day change), 'restore'.
    kind            TEXT NOT NULL DEFAULT 'publish' CHECK (kind IN ('publish','live','restore')),
    published_by    UUID REFERENCES users(id) ON DELETE SET NULL,
    published_at    TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (edition_id, version),
    FOREIGN KEY (edition_id, organization_id)
        REFERENCES editions (id, organization_id) ON DELETE CASCADE
);

CREATE TABLE telegram_chats (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    organization_id UUID NOT NULL,
    edition_id      UUID NOT NULL,
    name            TEXT NOT NULL CHECK (length(btrim(name)) > 0),
    chat_id         TEXT NOT NULL CHECK (length(btrim(chat_id)) > 0),
    enabled         BOOLEAN NOT NULL DEFAULT TRUE,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (edition_id, chat_id),
    FOREIGN KEY (edition_id, organization_id)
        REFERENCES editions (id, organization_id) ON DELETE CASCADE
);

CREATE TABLE announcements (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    organization_id UUID NOT NULL,
    edition_id      UUID NOT NULL,
    item_id         UUID REFERENCES schedule_items(id) ON DELETE SET NULL,
    body            TEXT NOT NULL CHECK (length(btrim(body)) BETWEEN 1 AND 4000),
    status          TEXT NOT NULL DEFAULT 'draft' CHECK (status IN ('draft','sent','discarded')),
    -- Per chat: [{"chat":"...","ok":true,"error":""}]
    result          JSONB NOT NULL DEFAULT '[]'::jsonb,
    created_by      UUID REFERENCES users(id) ON DELETE SET NULL,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    sent_by         UUID REFERENCES users(id) ON DELETE SET NULL,
    sent_at         TIMESTAMPTZ,
    FOREIGN KEY (edition_id, organization_id)
        REFERENCES editions (id, organization_id) ON DELETE CASCADE
);

CREATE INDEX announcements_edition_idx ON announcements (edition_id, created_at DESC);

-- +goose Down
DROP TABLE IF EXISTS announcements;
DROP TABLE IF EXISTS telegram_chats;
DROP TABLE IF EXISTS schedule_versions;
DROP TABLE IF EXISTS schedule_items;
DROP TABLE IF EXISTS schedules;
ALTER TABLE event_types DROP COLUMN IF EXISTS room_ids;
DROP TABLE IF EXISTS rooms;
