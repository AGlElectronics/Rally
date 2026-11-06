-- +goose Up
-- API keys for the public schedule API (EVT: "this should have auth").
-- Only the SHA-256 of a key is stored; the key itself is shown once on
-- creation. The builtin "programme-page" key is the exception: its value
-- comes from PUBLIC_API_BUILTIN_KEY so the submission site can use it (it
-- ships to every browser, so it buys rotation and tracking, not secrecy).
CREATE TABLE public_api_keys (
    id           UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    name         TEXT NOT NULL,
    -- prefix is the display handle ("rsk_ab12cd34…"), never checked.
    prefix       TEXT NOT NULL,
    key_hash     TEXT NOT NULL UNIQUE,
    builtin      BOOLEAN NOT NULL DEFAULT FALSE,
    created_by   TEXT NOT NULL DEFAULT '',
    created_at   TIMESTAMPTZ NOT NULL DEFAULT now(),
    last_used_at TIMESTAMPTZ,
    revoked_at   TIMESTAMPTZ
);

-- +goose Down
DROP TABLE public_api_keys;
