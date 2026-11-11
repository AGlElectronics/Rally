-- +goose Up
-- denlink.SchemaUpgrade: when Den rejects the module credential the link is
-- marked revoked until an admin pairs again (see internal/denlink).
ALTER TABLE den_link ADD COLUMN IF NOT EXISTS revoked_at TIMESTAMPTZ;

-- +goose Down
ALTER TABLE den_link DROP COLUMN IF EXISTS revoked_at;
