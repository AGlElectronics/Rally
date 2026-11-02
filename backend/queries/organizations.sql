-- Rally organization queries. Hand-written queries in internal/events are
-- the source of truth for the scaffold; these .sql files are kept so the
-- first vertical slice can switch to sqlc with a single `sqlc generate`.
--
-- See internal/events/events.go for the executed versions.

-- name: ListOrganizationsForUser :many
SELECT o.id,
       o.slug,
       o.display_name
  FROM organizations o
  JOIN memberships   m  ON m.organization_id = o.id
                       AND m.user_id = $1
                       AND m.status  = 'active'
 ORDER BY o.display_name ASC;

-- name: GetOrganizationForUser :one
SELECT o.id, o.slug, o.display_name
  FROM organizations o
  JOIN memberships m
    ON m.organization_id = o.id
   AND m.user_id = $2
   AND m.status  = 'active'
 WHERE o.id = $1;
