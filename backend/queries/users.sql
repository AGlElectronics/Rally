-- Rally user/identity queries. See internal/auth/resolver.go for the
-- hand-written pgx versions executed today.

-- name: GetUserByExternalIdentity :one
SELECT u.id, u.display_name, u.email
  FROM users u
  JOIN external_identities e ON e.user_id = u.id
 WHERE e.issuer = $1 AND e.subject = $2;

-- name: InsertUser :one
INSERT INTO users (display_name, email, last_login_at)
VALUES ($1, $2, now())
RETURNING id;

-- name: UpsertExternalIdentity :exec
INSERT INTO external_identities (user_id, issuer, subject)
VALUES ($1, $2, $3)
ON CONFLICT (issuer, subject) DO NOTHING;

-- name: TouchUserLogin :exec
UPDATE users
   SET display_name  = $2,
       email         = COALESCE($3, email),
       last_login_at = now(),
       updated_at    = now()
 WHERE id = $1;
