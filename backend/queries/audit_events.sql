-- Rally audit log queries. The scaffold inserts audit rows via the
-- hand-written pgx call in internal/audit/audit.go; this file is a sqlc
-- shape ready to be wired in with the first vertical slice.

-- name: InsertAuditEvent :exec
INSERT INTO audit_events (
    organization_id, edition_id,
    actor_user_id, actor_issuer, actor_subject, actor_display_name,
    action, resource_type, resource_id,
    before_json, after_json, note
) VALUES (
    $1, $2,
    $3, $4, $5, $6,
    $7, $8, $9,
    $10, $11, $12
);

-- name: ListAuditEventsForOrganization :many
SELECT id, organization_id, edition_id,
       actor_user_id, actor_issuer, actor_subject, actor_display_name,
       action, resource_type, resource_id,
       before_json, after_json, note, occurred_at
  FROM audit_events
 WHERE organization_id = $1
 ORDER BY occurred_at DESC
 LIMIT $2;
