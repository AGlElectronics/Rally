package events

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/AGlElectronics/rally/backend/internal/httperror"
)

// KeyService manages the public schedule API keys. A request presents a
// key as "Authorization: Bearer <key>" or "?key=" (calendar subscriptions
// cannot send headers); verification hashes the presented value and looks
// the hash up, so comparisons never touch the stored secret.
type KeyService struct {
	pool *pgxpool.Pool
}

// NewKeyService wires the key service.
func NewKeyService(pool *pgxpool.Pool) *KeyService { return &KeyService{pool: pool} }

// APIKey is one key's metadata (never the key itself).
type APIKey struct {
	ID         uuid.UUID  `json:"id"`
	Name       string     `json:"name"`
	Prefix     string     `json:"prefix"`
	Builtin    bool       `json:"builtin"`
	CreatedBy  string     `json:"created_by"`
	CreatedAt  time.Time  `json:"created_at"`
	LastUsedAt *time.Time `json:"last_used_at"`
	RevokedAt  *time.Time `json:"revoked_at"`
}

func hashKey(key string) string {
	sum := sha256.Sum256([]byte(key))
	return hex.EncodeToString(sum[:])
}

func displayPrefix(key string) string {
	if len(key) > 12 {
		return key[:12] + "…"
	}
	return key
}

// Create mints a key for one consumer (the website, an info screen…) and
// returns its value exactly once.
func (s *KeyService) Create(ctx context.Context, name, createdBy string) (APIKey, string, error) {
	name = strings.TrimSpace(name)
	if name == "" || len(name) > 120 {
		return APIKey{}, "", httperror.Validation("invalid key", map[string]string{"name": "required, at most 120 characters"})
	}
	raw := make([]byte, 32)
	if _, err := rand.Read(raw); err != nil {
		return APIKey{}, "", err
	}
	token := "rsk_" + base64.RawURLEncoding.EncodeToString(raw)
	out := APIKey{Name: name, Prefix: displayPrefix(token), CreatedBy: createdBy}
	err := s.pool.QueryRow(ctx, `
		INSERT INTO public_api_keys (name, prefix, key_hash, created_by)
		VALUES ($1, $2, $3, $4) RETURNING id, created_at`,
		name, out.Prefix, hashKey(token), createdBy).Scan(&out.ID, &out.CreatedAt)
	if err != nil {
		return APIKey{}, "", err
	}
	return out, token, nil
}

// List returns every key, newest first.
func (s *KeyService) List(ctx context.Context) ([]APIKey, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT id, name, prefix, builtin, created_by, created_at, last_used_at, revoked_at
		  FROM public_api_keys ORDER BY created_at DESC, id`)
	if err != nil {
		return nil, err
	}
	out, err := pgx.CollectRows(rows, func(r pgx.CollectableRow) (APIKey, error) {
		var k APIKey
		err := r.Scan(&k.ID, &k.Name, &k.Prefix, &k.Builtin, &k.CreatedBy, &k.CreatedAt, &k.LastUsedAt, &k.RevokedAt)
		return k, err
	})
	if out == nil {
		out = []APIKey{}
	}
	return out, err
}

// Revoke turns a key off for good. The builtin key is rotated by changing
// PUBLIC_API_BUILTIN_KEY instead.
func (s *KeyService) Revoke(ctx context.Context, id uuid.UUID) error {
	tag, err := s.pool.Exec(ctx, `
		UPDATE public_api_keys SET revoked_at = now() WHERE id = $1 AND builtin = FALSE AND revoked_at IS NULL`, id)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		var builtin bool
		err := s.pool.QueryRow(ctx, `SELECT builtin FROM public_api_keys WHERE id = $1`, id).Scan(&builtin)
		if errors.Is(err, pgx.ErrNoRows) {
			return httperror.NotFound("API key")
		}
		if err != nil {
			return err
		}
		if builtin {
			return httperror.Conflict("the builtin key is rotated by changing PUBLIC_API_BUILTIN_KEY")
		}
	}
	return nil
}

// EnsureBuiltin makes PUBLIC_API_BUILTIN_KEY work as the "programme-page"
// key: the matching row is created on start, and older builtin rows are
// revoked when the value changes (that is the rotation).
func (s *KeyService) EnsureBuiltin(ctx context.Context, value string) error {
	value = strings.TrimSpace(value)
	if value == "" {
		return nil
	}
	hash := hashKey(value)
	if _, err := s.pool.Exec(ctx, `
		UPDATE public_api_keys SET revoked_at = now()
		 WHERE builtin = TRUE AND revoked_at IS NULL AND key_hash <> $1`, hash); err != nil {
		return err
	}
	_, err := s.pool.Exec(ctx, `
		INSERT INTO public_api_keys (name, prefix, key_hash, builtin, created_by)
		VALUES ('programme-page', $1, $2, TRUE, 'config')
		ON CONFLICT (key_hash) DO UPDATE SET revoked_at = NULL`,
		displayPrefix(value), hash)
	return err
}

// Verify reports whether the presented value is an active key, and stamps
// its last use.
func (s *KeyService) Verify(ctx context.Context, presented string) (bool, error) {
	if presented == "" || len(presented) > 200 {
		return false, nil
	}
	var id uuid.UUID
	err := s.pool.QueryRow(ctx, `
		SELECT id FROM public_api_keys WHERE key_hash = $1 AND revoked_at IS NULL`, hashKey(presented)).Scan(&id)
	if errors.Is(err, pgx.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	_, _ = s.pool.Exec(ctx, `UPDATE public_api_keys SET last_used_at = now() WHERE id = $1`, id)
	return true, nil
}
