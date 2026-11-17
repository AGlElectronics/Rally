package events

import (
	"context"
	"errors"
	"regexp"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/AGlElectronics/rally/backend/internal/audit"
	"github.com/AGlElectronics/rally/backend/internal/auth"
	"github.com/AGlElectronics/rally/backend/internal/db"
	"github.com/AGlElectronics/rally/backend/internal/domain"
	"github.com/AGlElectronics/rally/backend/internal/httperror"
)

// Tag statuses.
const (
	TagProposed = "proposed"
	TagApproved = "approved"
	TagRejected = "rejected"
)

const maxTagLen = 60

// tagWhitespace collapses any run of whitespace into one space so two
// different spellings of the same tag collide on the uniqueness index.
var tagWhitespace = regexp.MustCompile(`\s+`)

// normaliseTag folds a tag into its comparison form (lower case, trimmed,
// single spaces). The display form keeps the submitter's casing.
func normaliseTag(s string) string {
	return strings.ToLower(strings.TrimSpace(tagWhitespace.ReplaceAllString(s, " ")))
}

func cleanTagDisplay(s string) string {
	return strings.TrimSpace(tagWhitespace.ReplaceAllString(s, " "))
}

// Tag is one organization-scoped tag.
type Tag struct {
	ID             uuid.UUID  `json:"id"`
	OrganizationID uuid.UUID  `json:"organization_id"`
	Name           string     `json:"name"`
	Display        string     `json:"display"`
	Status         string     `json:"status"`
	ProposedBy     string     `json:"proposed_by"`
	DecidedBy      string     `json:"decided_by,omitempty"`
	DecidedAt      *time.Time `json:"decided_at,omitempty"`
	Note           string     `json:"note,omitempty"`
	CreatedAt      time.Time  `json:"created_at"`
	UpdatedAt      time.Time  `json:"updated_at"`
}

// TagInput is the propose payload.
type TagInput struct {
	Name string `json:"name"`
}

// TagService manages the organization's proposed and approved tag pool.
type TagService struct {
	pool *pgxpool.Pool
}

// NewTagService returns a service bound to pool.
func NewTagService(pool *pgxpool.Pool) *TagService {
	return &TagService{pool: pool}
}

const tagColumns = `t.id, t.organization_id, t.name, t.display, t.status,
	COALESCE(p.display_name, ''), COALESCE(d.display_name, ''),
	t.decided_at, t.note, t.created_at, t.updated_at`

const tagFrom = `FROM tags t
	LEFT JOIN users p ON p.id = t.proposed_by
	LEFT JOIN users d ON d.id = t.decided_by`

func scanTag(row pgx.Row) (*Tag, error) {
	var t Tag
	err := row.Scan(&t.ID, &t.OrganizationID, &t.Name, &t.Display, &t.Status,
		&t.ProposedBy, &t.DecidedBy, &t.DecidedAt, &t.Note, &t.CreatedAt, &t.UpdatedAt)
	switch {
	case err == nil:
		return &t, nil
	case errors.Is(err, pgx.ErrNoRows):
		return nil, httperror.NotFound("tag")
	default:
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" {
			return nil, httperror.Conflict("this tag already exists in the organisation")
		}
		return nil, httperror.Internal(err)
	}
}

// TagFilter narrows the listing.
type TagFilter struct {
	// Status is "" (every status), or one of TagProposed/TagApproved/TagRejected.
	Status string
	// Query filters by prefix of the normalised name (autocomplete).
	Query string
	// Limit caps the result (<=0 means 200).
	Limit int
}

// List returns tags of orgID matching f. Any staff member of an edition in
// the org may list them (so the writer can pick tags).
func (s *TagService) List(ctx context.Context, orgID uuid.UUID, f TagFilter) ([]Tag, error) {
	if _, err := auth.MustFromContext(ctx); err != nil {
		return nil, httperror.New(httperror.CodeUnauthorized, "not authenticated")
	}
	limit := f.Limit
	if limit <= 0 || limit > 500 {
		limit = 200
	}
	rows, err := s.pool.Query(ctx, `SELECT `+tagColumns+` `+tagFrom+`
		WHERE t.organization_id=$1
		  AND ($2 = '' OR t.status = $2)
		  AND ($3 = '' OR t.name LIKE $3 || '%')
		ORDER BY t.status, t.name LIMIT $4`,
		orgID, f.Status, normaliseTag(f.Query), limit)
	if err != nil {
		return nil, httperror.Internal(err)
	}
	defer rows.Close()
	out := []Tag{}
	for rows.Next() {
		t, err := scanTag(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *t)
	}
	return out, rows.Err()
}

// ListForEdition is List filtered to the organisation that owns editionID.
// Staff of the edition see every status; signed-in users who can view the
// edition see only approved tags (autocomplete still works for them).
func (s *TagService) ListForEdition(ctx context.Context, editionID uuid.UUID, f TagFilter) ([]Tag, error) {
	u, err := auth.MustFromContext(ctx)
	if err != nil {
		return nil, httperror.New(httperror.CodeUnauthorized, "not authenticated")
	}
	orgID, err := editionOwner(ctx, s.pool, editionID)
	if err != nil {
		return nil, err
	}
	switch {
	case isScheduleStaff(u, orgID, editionID):
		// see every status
	case u.HasForEdition(domain.PermEditionView, orgID, editionID):
		f.Status = TagApproved
	default:
		return nil, httperror.NotFound("edition")
	}
	return s.List(ctx, orgID, f)
}

// Propose creates a tag in the "proposed" status (anyone with
// schedule.edit). An existing name is returned as-is, so proposing a tag
// that was already approved by someone else is a no-op.
func (s *TagService) Propose(ctx context.Context, editionID uuid.UUID, in TagInput) (*Tag, error) {
	u, err := auth.MustFromContext(ctx)
	if err != nil {
		return nil, httperror.New(httperror.CodeUnauthorized, "not authenticated")
	}
	orgID, err := editionOwner(ctx, s.pool, editionID)
	if err != nil {
		return nil, err
	}
	if err := auth.RequireForEdition(ctx, domain.PermScheduleEdit, orgID, editionID); err != nil {
		return nil, err
	}
	display := cleanTagDisplay(in.Name)
	name := normaliseTag(display)
	switch {
	case name == "":
		return nil, httperror.Validation("invalid tag", map[string]string{"name": "is required"})
	case len([]rune(display)) > maxTagLen:
		return nil, httperror.Validation("invalid tag", map[string]string{"name": "must be at most 60 characters"})
	}
	var out *Tag
	err = db.WithTx(ctx, s.pool, func(tx pgx.Tx) error {
		// Return the existing row when the tag already exists so proposers
		// aren't asked to deduplicate.
		existing, err := scanTag(tx.QueryRow(ctx, `SELECT `+tagColumns+` `+tagFrom+`
			WHERE t.organization_id=$1 AND t.name=$2 FOR UPDATE`, orgID, name))
		if err == nil {
			out = existing
			return nil
		}
		var he *httperror.Error
		if !errors.As(err, &he) || he.Code != httperror.CodeNotFound {
			return err
		}
		var id uuid.UUID
		if err := tx.QueryRow(ctx, `INSERT INTO tags
			(organization_id, name, display, status, proposed_by)
			VALUES ($1,$2,$3,'proposed',$4) RETURNING id`,
			orgID, name, display, u.ID).Scan(&id); err != nil {
			var pgErr *pgconn.PgError
			if errors.As(err, &pgErr) && pgErr.Code == "23505" {
				// A concurrent proposer won the race; return their row.
				row, err := scanTag(tx.QueryRow(ctx, `SELECT `+tagColumns+` `+tagFrom+`
					WHERE t.organization_id=$1 AND t.name=$2`, orgID, name))
				if err != nil {
					return err
				}
				out = row
				return nil
			}
			return httperror.Internal(err)
		}
		t, err := scanTag(tx.QueryRow(ctx, `SELECT `+tagColumns+` `+tagFrom+` WHERE t.id=$1`, id))
		if err != nil {
			return err
		}
		if err := audit.Record(ctx, tx, orgID, &editionID, "tag.propose",
			string(domain.ResourceTag), t.ID, nil, t, ""); err != nil {
			return err
		}
		out = t
		return nil
	})
	return out, wrapTxErr(err)
}

func (s *TagService) decide(ctx context.Context, id uuid.UUID, status, note string) (*Tag, error) {
	u, err := auth.MustFromContext(ctx)
	if err != nil {
		return nil, httperror.New(httperror.CodeUnauthorized, "not authenticated")
	}
	note = strings.TrimSpace(note)
	if len([]rune(note)) > 500 {
		return nil, httperror.Validation("invalid decision", map[string]string{"note": "must be at most 500 characters"})
	}
	var out *Tag
	err = db.WithTx(ctx, s.pool, func(tx pgx.Tx) error {
		before, err := scanTag(tx.QueryRow(ctx, `SELECT `+tagColumns+` `+tagFrom+`
			WHERE t.id=$1 FOR UPDATE OF t`, id))
		if err != nil {
			return err
		}
		if err := auth.RequireInOrg(ctx, domain.PermTagApprove, before.OrganizationID); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `UPDATE tags SET status=$2, decided_by=$3, decided_at=now(), note=$4,
			updated_at=now() WHERE id=$1`, id, status, u.ID, note); err != nil {
			return httperror.Internal(err)
		}
		after, err := scanTag(tx.QueryRow(ctx, `SELECT `+tagColumns+` `+tagFrom+` WHERE t.id=$1`, id))
		if err != nil {
			return err
		}
		out = after
		return audit.Record(ctx, tx, before.OrganizationID, nil, "tag."+status,
			string(domain.ResourceTag), id, before, after, note)
	})
	return out, wrapTxErr(err)
}

// Approve lifts a proposed tag into the approved pool.
func (s *TagService) Approve(ctx context.Context, id uuid.UUID, note string) (*Tag, error) {
	return s.decide(ctx, id, TagApproved, note)
}

// Reject marks a proposed tag as rejected (kept for audit, stripped from
// autocomplete). Already-attached tags stay on their items until removed.
func (s *TagService) Reject(ctx context.Context, id uuid.UUID, note string) (*Tag, error) {
	return s.decide(ctx, id, TagRejected, note)
}

// Delete removes a tag for good, and with it every attachment on items and
// submissions. Requires tag.approve.
func (s *TagService) Delete(ctx context.Context, id uuid.UUID) error {
	err := db.WithTx(ctx, s.pool, func(tx pgx.Tx) error {
		before, err := scanTag(tx.QueryRow(ctx, `SELECT `+tagColumns+` `+tagFrom+`
			WHERE t.id=$1 FOR UPDATE OF t`, id))
		if err != nil {
			return err
		}
		if err := auth.RequireInOrg(ctx, domain.PermTagApprove, before.OrganizationID); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `DELETE FROM tags WHERE id=$1`, id); err != nil {
			return httperror.Internal(err)
		}
		return audit.Record(ctx, tx, before.OrganizationID, nil, "tag.delete",
			string(domain.ResourceTag), id, before, nil, "")
	})
	return wrapTxErr(err)
}

// ItemTag is one attachment on a schedule item (approved tags only on read).
type ItemTag struct {
	ID      uuid.UUID `json:"id"`
	Name    string    `json:"name"`
	Display string    `json:"display"`
	Status  string    `json:"status"`
}

// listScheduleItemTags returns the tags attached to an item in display order.
func listScheduleItemTags(ctx context.Context, q querier, itemID uuid.UUID) ([]ItemTag, error) {
	rows, err := q.Query(ctx, `SELECT t.id, t.name, t.display, t.status
		FROM schedule_item_tags st JOIN tags t ON t.id = st.tag_id
		WHERE st.schedule_item_id = $1 ORDER BY t.name`, itemID)
	if err != nil {
		return nil, httperror.Internal(err)
	}
	defer rows.Close()
	out := []ItemTag{}
	for rows.Next() {
		var t ItemTag
		if err := rows.Scan(&t.ID, &t.Name, &t.Display, &t.Status); err != nil {
			return nil, httperror.Internal(err)
		}
		out = append(out, t)
	}
	return out, rows.Err()
}

// tagsForItems batch-loads tags for a set of items.
func tagsForItems(ctx context.Context, q querier, itemIDs []uuid.UUID) (map[uuid.UUID][]ItemTag, error) {
	out := map[uuid.UUID][]ItemTag{}
	if len(itemIDs) == 0 {
		return out, nil
	}
	rows, err := q.Query(ctx, `SELECT st.schedule_item_id, t.id, t.name, t.display, t.status
		FROM schedule_item_tags st JOIN tags t ON t.id = st.tag_id
		WHERE st.schedule_item_id = ANY($1) ORDER BY t.name`, itemIDs)
	if err != nil {
		return nil, httperror.Internal(err)
	}
	defer rows.Close()
	for rows.Next() {
		var itemID uuid.UUID
		var t ItemTag
		if err := rows.Scan(&itemID, &t.ID, &t.Name, &t.Display, &t.Status); err != nil {
			return nil, httperror.Internal(err)
		}
		out[itemID] = append(out[itemID], t)
	}
	return out, rows.Err()
}

// setScheduleItemTags replaces the tag list on an item with ids. Only
// approved tags of the same organisation are accepted; unknown or proposed
// ones are refused with a validation error.
func setScheduleItemTags(ctx context.Context, tx pgx.Tx, orgID, itemID uuid.UUID, ids []uuid.UUID) error {
	seen := map[uuid.UUID]bool{}
	wanted := make([]uuid.UUID, 0, len(ids))
	for _, id := range ids {
		if seen[id] {
			continue
		}
		seen[id] = true
		wanted = append(wanted, id)
	}
	if len(wanted) > 0 {
		var ok int
		if err := tx.QueryRow(ctx, `SELECT count(*) FROM tags
			WHERE id = ANY($1) AND organization_id = $2 AND status = 'approved'`,
			wanted, orgID).Scan(&ok); err != nil {
			return httperror.Internal(err)
		}
		if ok != len(wanted) {
			return httperror.Validation("invalid item", map[string]string{
				"tag_ids": "one or more tags are not approved for this organisation",
			})
		}
	}
	if _, err := tx.Exec(ctx, `DELETE FROM schedule_item_tags WHERE schedule_item_id=$1`, itemID); err != nil {
		return httperror.Internal(err)
	}
	for _, id := range wanted {
		if _, err := tx.Exec(ctx, `INSERT INTO schedule_item_tags (schedule_item_id, tag_id)
			VALUES ($1, $2)`, itemID, id); err != nil {
			return httperror.Internal(err)
		}
	}
	return nil
}

// listSubmissionTags returns the tags attached to a submission.
func listSubmissionTags(ctx context.Context, q querier, submissionID uuid.UUID) ([]ItemTag, error) {
	rows, err := q.Query(ctx, `SELECT t.id, t.name, t.display, t.status
		FROM submission_tags st JOIN tags t ON t.id = st.tag_id
		WHERE st.submission_id = $1 ORDER BY t.name`, submissionID)
	if err != nil {
		return nil, httperror.Internal(err)
	}
	defer rows.Close()
	out := []ItemTag{}
	for rows.Next() {
		var t ItemTag
		if err := rows.Scan(&t.ID, &t.Name, &t.Display, &t.Status); err != nil {
			return nil, httperror.Internal(err)
		}
		out = append(out, t)
	}
	return out, rows.Err()
}

// SubmissionTagsInput replaces the tag set on one submission.
type SubmissionTagsInput struct {
	TagIDs []uuid.UUID `json:"tag_ids"`
}

// SetSubmissionTags replaces the tags on a submission. Staff with
// submission.review may change any submission's tags; the submitter can
// change their own while the call still takes submissions.
func (s *TagService) SetSubmissionTags(ctx context.Context, submissionID uuid.UUID, in SubmissionTagsInput) ([]ItemTag, error) {
	u, err := auth.MustFromContext(ctx)
	if err != nil {
		return nil, httperror.New(httperror.CodeUnauthorized, "not authenticated")
	}
	var out []ItemTag
	err = db.WithTx(ctx, s.pool, func(tx pgx.Tx) error {
		var orgID, editionID, cfpID, submitter uuid.UUID
		err := tx.QueryRow(ctx, `SELECT organization_id, edition_id, cfp_id, submitter_user_id
			FROM submissions WHERE id=$1`, submissionID).Scan(&orgID, &editionID, &cfpID, &submitter)
		if errors.Is(err, pgx.ErrNoRows) {
			return httperror.NotFound("submission")
		}
		if err != nil {
			return httperror.Internal(err)
		}
		staff := u.HasForEdition(domain.PermSubmissionReview, orgID, editionID)
		if !staff && submitter != u.ID {
			return httperror.New(httperror.CodeForbidden, "only the submitter or review staff can change tags")
		}
		seen := map[uuid.UUID]bool{}
		wanted := make([]uuid.UUID, 0, len(in.TagIDs))
		for _, id := range in.TagIDs {
			if seen[id] {
				continue
			}
			seen[id] = true
			wanted = append(wanted, id)
		}
		if len(wanted) > 0 {
			var ok int
			if err := tx.QueryRow(ctx, `SELECT count(*) FROM tags
				WHERE id = ANY($1) AND organization_id = $2 AND status='approved'`,
				wanted, orgID).Scan(&ok); err != nil {
				return httperror.Internal(err)
			}
			if ok != len(wanted) {
				return httperror.Validation("invalid tags", map[string]string{
					"tag_ids": "one or more tags are not approved for this organisation",
				})
			}
		}
		if _, err := tx.Exec(ctx, `DELETE FROM submission_tags WHERE submission_id=$1`, submissionID); err != nil {
			return httperror.Internal(err)
		}
		for _, id := range wanted {
			if _, err := tx.Exec(ctx, `INSERT INTO submission_tags (submission_id, tag_id)
				VALUES ($1, $2)`, submissionID, id); err != nil {
				return httperror.Internal(err)
			}
		}
		tags, err := listSubmissionTags(ctx, tx, submissionID)
		if err != nil {
			return err
		}
		out = tags
		return audit.Record(ctx, tx, orgID, &editionID, "submission.tags",
			string(domain.ResourceSubmission), submissionID, nil, map[string]any{"tag_ids": wanted}, "")
	})
	return out, wrapTxErr(err)
}

// SubmissionTags reads the tags attached to a submission.
func (s *TagService) SubmissionTags(ctx context.Context, submissionID uuid.UUID) ([]ItemTag, error) {
	if _, err := auth.MustFromContext(ctx); err != nil {
		return nil, httperror.New(httperror.CodeUnauthorized, "not authenticated")
	}
	return listSubmissionTags(ctx, s.pool, submissionID)
}
