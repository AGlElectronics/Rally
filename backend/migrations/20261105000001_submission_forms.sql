-- Rally (Events) — public submissions (PoC phase 1).
--
-- Adds event types (edition-scoped), configurable forms and review settings
-- on calls for papers, form answers on submissions, the people on a
-- submission (owner + co-presenters with invite codes), attendee profiles
-- (personal presenter code), uploaded files (bytes live in object storage),
-- internal review notes and reviewer scores.

-- +goose Up
CREATE TABLE event_types (
    id                UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    organization_id   UUID NOT NULL,
    edition_id        UUID NOT NULL,
    name              TEXT NOT NULL CHECK (length(btrim(name)) > 0),
    color             TEXT NOT NULL DEFAULT '#6366f1',
    default_minutes   INT  NOT NULL DEFAULT 60 CHECK (default_minutes BETWEEN 5 AND 1440),
    gap_after_minutes INT  NOT NULL DEFAULT 0  CHECK (gap_after_minutes BETWEEN 0 AND 720),
    sort_order        INT  NOT NULL DEFAULT 0,
    created_at        TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at        TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (edition_id, name),
    FOREIGN KEY (edition_id, organization_id)
        REFERENCES editions (id, organization_id) ON DELETE RESTRICT
);

CREATE INDEX event_types_edition_idx ON event_types (edition_id, sort_order);

ALTER TABLE call_for_papers
    ADD COLUMN form           JSONB  NOT NULL DEFAULT '{"fields":[]}'::jsonb
                              CHECK (jsonb_typeof(form) = 'object'),
    ADD COLUMN review_mode    TEXT   NOT NULL DEFAULT 'lead'
                              CHECK (review_mode IN ('lead','scored')),
    -- Empty = every event type of the edition is allowed.
    ADD COLUMN event_type_ids UUID[] NOT NULL DEFAULT '{}',
    -- Attendee-facing wording per submission state, e.g. {"submitted":"In review"}.
    ADD COLUMN status_labels  JSONB  NOT NULL DEFAULT '{}'::jsonb
                              CHECK (jsonb_typeof(status_labels) = 'object');

ALTER TABLE submissions
    ADD COLUMN answers          JSONB NOT NULL DEFAULT '{}'::jsonb
                                CHECK (jsonb_typeof(answers) = 'object'),
    ADD COLUMN event_type_id    UUID REFERENCES event_types(id) ON DELETE SET NULL,
    ADD COLUMN duration_minutes INT CHECK (duration_minutes IS NULL OR duration_minutes BETWEEN 5 AND 1440);

-- One row per attendee-realm user. Staff users never get one.
CREATE TABLE attendee_profiles (
    user_id        UUID PRIMARY KEY REFERENCES users(id) ON DELETE CASCADE,
    presenter_code TEXT NOT NULL UNIQUE,
    email_verified BOOLEAN NOT NULL DEFAULT FALSE,
    created_at     TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE submission_people (
    id            UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    submission_id UUID NOT NULL REFERENCES submissions(id) ON DELETE CASCADE,
    user_id       UUID REFERENCES users(id) ON DELETE CASCADE,
    role          TEXT NOT NULL CHECK (role IN ('owner','presenter')),
    status        TEXT NOT NULL CHECK (status IN ('invited','accepted')),
    invite_email  TEXT NOT NULL DEFAULT '',
    invite_code   TEXT UNIQUE,
    invited_by    UUID REFERENCES users(id) ON DELETE SET NULL,
    created_at    TIMESTAMPTZ NOT NULL DEFAULT now(),
    responded_at  TIMESTAMPTZ,
    UNIQUE (submission_id, user_id),
    CHECK (status = 'invited' OR user_id IS NOT NULL)
);

CREATE INDEX submission_people_user_idx ON submission_people (user_id) WHERE user_id IS NOT NULL;

INSERT INTO submission_people (submission_id, user_id, role, status, responded_at)
SELECT id, submitter_user_id, 'owner', 'accepted', created_at FROM submissions;

CREATE TABLE submission_files (
    id               UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    organization_id  UUID NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
    cfp_id           UUID NOT NULL REFERENCES call_for_papers(id) ON DELETE CASCADE,
    submission_id    UUID REFERENCES submissions(id) ON DELETE CASCADE,
    uploader_user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    field_id         TEXT NOT NULL,
    object_key       TEXT NOT NULL UNIQUE,
    filename         TEXT NOT NULL,
    content_type     TEXT NOT NULL,
    size_bytes       BIGINT NOT NULL CHECK (size_bytes > 0),
    status           TEXT NOT NULL DEFAULT 'pending' CHECK (status IN ('pending','uploaded')),
    created_at       TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX submission_files_submission_idx ON submission_files (submission_id);
CREATE INDEX submission_files_orphan_idx     ON submission_files (created_at) WHERE submission_id IS NULL;

CREATE TABLE submission_notes (
    id             UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    submission_id  UUID NOT NULL REFERENCES submissions(id) ON DELETE CASCADE,
    author_user_id UUID REFERENCES users(id) ON DELETE SET NULL,
    body           TEXT NOT NULL CHECK (length(btrim(body)) BETWEEN 1 AND 5000),
    created_at     TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX submission_notes_submission_idx ON submission_notes (submission_id, created_at);

CREATE TABLE submission_scores (
    submission_id    UUID NOT NULL REFERENCES submissions(id) ON DELETE CASCADE,
    reviewer_user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    score            SMALLINT NOT NULL CHECK (score BETWEEN 1 AND 5),
    note             TEXT NOT NULL DEFAULT '',
    updated_at       TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (submission_id, reviewer_user_id)
);

-- +goose Down
DROP TABLE IF EXISTS submission_scores;
DROP TABLE IF EXISTS submission_notes;
DROP TABLE IF EXISTS submission_files;
DROP TABLE IF EXISTS submission_people;
DROP TABLE IF EXISTS attendee_profiles;
ALTER TABLE submissions
    DROP COLUMN IF EXISTS duration_minutes,
    DROP COLUMN IF EXISTS event_type_id,
    DROP COLUMN IF EXISTS answers;
ALTER TABLE call_for_papers
    DROP COLUMN IF EXISTS status_labels,
    DROP COLUMN IF EXISTS event_type_ids,
    DROP COLUMN IF EXISTS review_mode,
    DROP COLUMN IF EXISTS form;
DROP TABLE IF EXISTS event_types;
