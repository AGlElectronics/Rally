-- Rally (Events) — Call for Papers (first vertical slice).
--
-- Adds call_for_papers (edition-scoped), submissions (with a state machine
-- enforced in internal/domain.Transition and mirrored by a CHECK here), and
-- submission_reviews (one row per accept/reject decision).
--
-- Tenancy: every row carries organization_id AND edition_id, tied together
-- with composite foreign keys so a row can never reference an edition from a
-- different organization, or a submission from a different CfP's edition.

-- +goose Up
ALTER TABLE editions
    ADD CONSTRAINT editions_id_organization_key UNIQUE (id, organization_id);

CREATE TABLE call_for_papers (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    organization_id UUID NOT NULL,
    edition_id      UUID NOT NULL,
    title           TEXT NOT NULL CHECK (length(btrim(title)) > 0),
    description     TEXT NOT NULL DEFAULT '',
    status          TEXT NOT NULL DEFAULT 'draft'
                    CHECK (status IN ('draft','open','closed')),
    opens_at        TIMESTAMPTZ,
    closes_at       TIMESTAMPTZ,
    created_by      UUID REFERENCES users(id) ON DELETE SET NULL,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    CHECK (opens_at IS NULL OR closes_at IS NULL OR opens_at < closes_at),
    -- Lets submissions prove (cfp, edition, organization) agree.
    UNIQUE (id, edition_id, organization_id),
    FOREIGN KEY (edition_id, organization_id)
        REFERENCES editions (id, organization_id) ON DELETE RESTRICT
);

CREATE INDEX call_for_papers_edition_idx ON call_for_papers (edition_id);
CREATE INDEX call_for_papers_org_idx     ON call_for_papers (organization_id);

CREATE TABLE submissions (
    id                UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    organization_id   UUID NOT NULL,
    edition_id        UUID NOT NULL,
    cfp_id            UUID NOT NULL,
    submitter_user_id UUID NOT NULL REFERENCES users(id) ON DELETE RESTRICT,
    title             TEXT NOT NULL CHECK (length(btrim(title)) > 0),
    abstract          TEXT NOT NULL DEFAULT '',
    -- Array of {"name","email","bio"} objects. Kept as JSONB: speakers are
    -- not yet first-class entities (no cross-submission identity).
    speakers          JSONB NOT NULL DEFAULT '[]'::jsonb
                      CHECK (jsonb_typeof(speakers) = 'array'),
    state             TEXT NOT NULL DEFAULT 'draft'
                      CHECK (state IN ('draft','submitted','accepted','rejected','withdrawn')),
    submitted_at      TIMESTAMPTZ,
    decided_at        TIMESTAMPTZ,
    decided_by        UUID REFERENCES users(id) ON DELETE SET NULL,
    created_at        TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at        TIMESTAMPTZ NOT NULL DEFAULT now(),
    FOREIGN KEY (cfp_id, edition_id, organization_id)
        REFERENCES call_for_papers (id, edition_id, organization_id) ON DELETE RESTRICT
);

CREATE INDEX submissions_edition_state_idx ON submissions (edition_id, state);
CREATE INDEX submissions_cfp_idx           ON submissions (cfp_id);
CREATE INDEX submissions_submitter_idx     ON submissions (submitter_user_id);

CREATE TABLE submission_reviews (
    id               UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    submission_id    UUID NOT NULL REFERENCES submissions(id) ON DELETE CASCADE,
    organization_id  UUID NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
    reviewer_user_id UUID REFERENCES users(id) ON DELETE SET NULL,
    decision         TEXT NOT NULL CHECK (decision IN ('accept','reject')),
    note             TEXT NOT NULL DEFAULT '',
    created_at       TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX submission_reviews_submission_idx ON submission_reviews (submission_id, created_at DESC);

-- +goose Down
DROP TABLE IF EXISTS submission_reviews;
DROP TABLE IF EXISTS submissions;
DROP TABLE IF EXISTS call_for_papers;
ALTER TABLE editions DROP CONSTRAINT IF EXISTS editions_id_organization_key;
