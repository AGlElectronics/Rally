package events

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/AGlElectronics/rally/backend/internal/audit"
	"github.com/AGlElectronics/rally/backend/internal/auth"
	"github.com/AGlElectronics/rally/backend/internal/db"
	"github.com/AGlElectronics/rally/backend/internal/domain"
	"github.com/AGlElectronics/rally/backend/internal/httperror"
)

// CfP statuses. Only "open" CfPs accept new submissions and are visible to
// users without a staff permission.
const (
	CfPDraft  = "draft"
	CfPOpen   = "open"
	CfPClosed = "closed"
)

const (
	maxTitleLen       = 200
	maxDescriptionLen = 10000
)

// CallForPapers is the API-shaped projection of a call_for_papers row.
type CallForPapers struct {
	ID             uuid.UUID         `json:"id"`
	OrganizationID uuid.UUID         `json:"organization_id"`
	EditionID      uuid.UUID         `json:"edition_id"`
	Title          string            `json:"title"`
	Description    string            `json:"description"`
	Status         string            `json:"status"`
	OpensAt        *time.Time        `json:"opens_at"`
	ClosesAt       *time.Time        `json:"closes_at"`
	Form           Form              `json:"form"`
	ReviewMode     string            `json:"review_mode"`
	EventTypeIDs   []uuid.UUID       `json:"event_type_ids"`
	StatusLabels   map[string]string `json:"status_labels"`
	CreatedBy      *uuid.UUID        `json:"created_by,omitempty"`
	CreatedAt      time.Time         `json:"created_at"`
	UpdatedAt      time.Time         `json:"updated_at"`
}

// Review modes of a call for papers.
const (
	ReviewLead   = "lead"   // the lead decides; reviewers leave notes
	ReviewScored = "scored" // reviewers also score 1-5; the lead decides
)

// AcceptsSubmissions reports whether the CfP is open for new submissions
// at `now`: status=open and inside the optional opens_at/closes_at window.
func (c *CallForPapers) AcceptsSubmissions(now time.Time) bool {
	if c.Status != CfPOpen {
		return false
	}
	if c.OpensAt != nil && now.Before(*c.OpensAt) {
		return false
	}
	if c.ClosesAt != nil && !now.Before(*c.ClosesAt) {
		return false
	}
	return true
}

// CfPInput is the full-replace payload for create/update. The pointer fields
// are optional: omitted means "keep" on update and "default" on create.
type CfPInput struct {
	Title        string             `json:"title"`
	Description  string             `json:"description"`
	Status       string             `json:"status"`
	OpensAt      *time.Time         `json:"opens_at"`
	ClosesAt     *time.Time         `json:"closes_at"`
	Form         *Form              `json:"form,omitempty"`
	ReviewMode   *string            `json:"review_mode,omitempty"`
	EventTypeIDs *[]uuid.UUID       `json:"event_type_ids,omitempty"`
	StatusLabels *map[string]string `json:"status_labels,omitempty"`
}

// fillFrom copies the optional fields from cur where the input omits them.
func (in *CfPInput) fillFrom(cur *CallForPapers) {
	if in.Form == nil {
		f := cur.Form
		in.Form = &f
	}
	if in.ReviewMode == nil {
		in.ReviewMode = &cur.ReviewMode
	}
	if in.EventTypeIDs == nil {
		ids := cur.EventTypeIDs
		in.EventTypeIDs = &ids
	}
	if in.StatusLabels == nil {
		l := cur.StatusLabels
		in.StatusLabels = &l
	}
}

func (in *CfPInput) fillDefaults() {
	if in.Form == nil {
		f := DefaultForm()
		in.Form = &f
	}
	if in.ReviewMode == nil {
		m := ReviewLead
		in.ReviewMode = &m
	}
	if in.EventTypeIDs == nil {
		in.EventTypeIDs = &[]uuid.UUID{}
	}
	if in.StatusLabels == nil {
		in.StatusLabels = &map[string]string{}
	}
}

// checkEventTypes verifies every referenced event type belongs to the edition.
func (in *CfPInput) checkEventTypes(ctx context.Context, q querier, editionID uuid.UUID) error {
	ids := *in.EventTypeIDs
	if len(ids) == 0 {
		return nil
	}
	if len(ids) > maxEventTypesInCfP {
		return httperror.Validation("invalid call for papers", map[string]string{"event_type_ids": "too many"})
	}
	var n int
	if err := q.QueryRow(ctx, `SELECT count(DISTINCT id) FROM event_types WHERE edition_id=$1 AND id = ANY($2)`,
		editionID, ids).Scan(&n); err != nil {
		return httperror.Internal(err)
	}
	if n != len(uniqueIDs(ids)) {
		return httperror.Validation("invalid call for papers", map[string]string{"event_type_ids": "must be event types of this edition"})
	}
	return nil
}

func uniqueIDs(ids []uuid.UUID) []uuid.UUID {
	seen := map[uuid.UUID]struct{}{}
	out := make([]uuid.UUID, 0, len(ids))
	for _, id := range ids {
		if _, ok := seen[id]; !ok {
			seen[id] = struct{}{}
			out = append(out, id)
		}
	}
	return out
}

// normalize trims, applies defaults and validates the input. Optional fields
// must already be filled (fillFrom / fillDefaults).
func (in *CfPInput) normalize() error {
	in.Title = strings.TrimSpace(in.Title)
	in.Description = strings.TrimSpace(in.Description)
	in.Status = strings.TrimSpace(in.Status)
	if in.Status == "" {
		in.Status = CfPDraft
	}

	issues := map[string]string{}
	switch {
	case in.Title == "":
		issues["title"] = "is required"
	case len([]rune(in.Title)) > maxTitleLen:
		issues["title"] = "must be at most 200 characters"
	}
	if len([]rune(in.Description)) > maxDescriptionLen {
		issues["description"] = "must be at most 10000 characters"
	}
	switch in.Status {
	case CfPDraft, CfPOpen, CfPClosed:
	default:
		issues["status"] = "must be one of draft, open, closed"
	}
	if in.OpensAt != nil && in.ClosesAt != nil && !in.OpensAt.Before(*in.ClosesAt) {
		issues["closes_at"] = "must be after opens_at"
	}
	for k, v := range normalizeForm(in.Form) {
		issues[k] = v
	}
	switch *in.ReviewMode {
	case ReviewLead, ReviewScored:
	default:
		issues["review_mode"] = "must be lead or scored"
	}
	ids := uniqueIDs(*in.EventTypeIDs)
	in.EventTypeIDs = &ids
	labels, labelIssues := normalizeStatusLabels(*in.StatusLabels)
	in.StatusLabels = &labels
	for k, v := range labelIssues {
		issues[k] = v
	}
	if len(issues) > 0 {
		return httperror.Validation("invalid call for papers", issues)
	}
	return nil
}

// CfPService manages calls for papers.
type CfPService struct {
	pool *pgxpool.Pool
}

// NewCfPService returns a service bound to pool.
func NewCfPService(pool *pgxpool.Pool) *CfPService {
	return &CfPService{pool: pool}
}

const cfpColumns = `id, organization_id, edition_id, title, description, status,
	opens_at, closes_at, form, review_mode, event_type_ids, status_labels,
	created_by, created_at, updated_at`

func scanCfP(row pgx.Row) (*CallForPapers, error) {
	var c CallForPapers
	var formJSON, labelsJSON []byte
	err := row.Scan(&c.ID, &c.OrganizationID, &c.EditionID, &c.Title, &c.Description,
		&c.Status, &c.OpensAt, &c.ClosesAt, &formJSON, &c.ReviewMode, &c.EventTypeIDs, &labelsJSON,
		&c.CreatedBy, &c.CreatedAt, &c.UpdatedAt)
	switch {
	case err == nil:
		if err := json.Unmarshal(formJSON, &c.Form); err != nil {
			return nil, httperror.Internal(err)
		}
		if c.Form.Fields == nil {
			c.Form.Fields = []FormField{}
		}
		c.StatusLabels = map[string]string{}
		if err := json.Unmarshal(labelsJSON, &c.StatusLabels); err != nil {
			return nil, httperror.Internal(err)
		}
		if c.EventTypeIDs == nil {
			c.EventTypeIDs = []uuid.UUID{}
		}
		return &c, nil
	case errors.Is(err, pgx.ErrNoRows):
		return nil, httperror.NotFound("call for papers")
	default:
		return nil, httperror.Internal(err)
	}
}

func loadCfP(ctx context.Context, q querier, id uuid.UUID, forUpdate bool) (*CallForPapers, error) {
	sql := `SELECT ` + cfpColumns + ` FROM call_for_papers WHERE id = $1`
	if forUpdate {
		sql += ` FOR UPDATE`
	}
	return scanCfP(q.QueryRow(ctx, sql, id))
}

// isStaffForEdition reports whether the caller may see non-public CfP data
// (drafts, closed calls) and every submission of the edition.
func isStaffForEdition(u *auth.User, orgID, editionID uuid.UUID) bool {
	return u.HasForEdition(domain.PermCfPManage, orgID, editionID) ||
		u.HasForEdition(domain.PermSubmissionReview, orgID, editionID)
}

// ListForEdition returns the CfPs of an edition. Staff see every status;
// everyone else sees only open CfPs.
func (s *CfPService) ListForEdition(ctx context.Context, editionID uuid.UUID) ([]CallForPapers, error) {
	user, err := auth.MustFromContext(ctx)
	if err != nil {
		return nil, httperror.New(httperror.CodeUnauthorized, "not authenticated")
	}
	orgID, err := editionOwner(ctx, s.pool, editionID)
	if err != nil {
		return nil, err
	}
	staff := isStaffForEdition(user, orgID, editionID)

	rows, err := s.pool.Query(ctx,
		`SELECT `+cfpColumns+` FROM call_for_papers
		  WHERE edition_id = $1 AND ($2 OR status = 'open')
		  ORDER BY created_at ASC, id ASC`, editionID, staff)
	if err != nil {
		return nil, httperror.Internal(err)
	}
	defer rows.Close()

	out := make([]CallForPapers, 0, 2)
	for rows.Next() {
		c, err := scanCfP(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *c)
	}
	if err := rows.Err(); err != nil {
		return nil, httperror.Internal(err)
	}
	return out, nil
}

// Get returns one CfP. Non-staff callers get a 404 for non-open CfPs.
func (s *CfPService) Get(ctx context.Context, id uuid.UUID) (*CallForPapers, error) {
	user, err := auth.MustFromContext(ctx)
	if err != nil {
		return nil, httperror.New(httperror.CodeUnauthorized, "not authenticated")
	}
	c, err := loadCfP(ctx, s.pool, id, false)
	if err != nil {
		return nil, err
	}
	if c.Status != CfPOpen && !isStaffForEdition(user, c.OrganizationID, c.EditionID) {
		return nil, httperror.NotFound("call for papers")
	}
	return c, nil
}

// Create adds a CfP to an edition. Requires cfp.manage for that edition.
func (s *CfPService) Create(ctx context.Context, editionID uuid.UUID, in CfPInput) (*CallForPapers, error) {
	user, err := auth.MustFromContext(ctx)
	if err != nil {
		return nil, httperror.New(httperror.CodeUnauthorized, "not authenticated")
	}
	orgID, err := editionOwner(ctx, s.pool, editionID)
	if err != nil {
		return nil, err
	}
	if err := auth.RequireForEdition(ctx, domain.PermCfPManage, orgID, editionID); err != nil {
		return nil, err
	}
	in.fillDefaults()
	if err := in.normalize(); err != nil {
		return nil, err
	}
	if err := in.checkEventTypes(ctx, s.pool, editionID); err != nil {
		return nil, err
	}

	var out *CallForPapers
	err = db.WithTx(ctx, s.pool, func(tx pgx.Tx) error {
		c, err := scanCfP(tx.QueryRow(ctx,
			`INSERT INTO call_for_papers
			     (organization_id, edition_id, title, description, status, opens_at, closes_at,
			      form, review_mode, event_type_ids, status_labels, created_by)
			 VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12)
			 RETURNING `+cfpColumns,
			orgID, editionID, in.Title, in.Description, in.Status, in.OpensAt, in.ClosesAt,
			in.Form, *in.ReviewMode, *in.EventTypeIDs, *in.StatusLabels, user.ID))
		if err != nil {
			return err
		}
		if err := audit.Record(ctx, tx, orgID, &editionID, "cfp.create",
			string(domain.ResourceCallForPaper), c.ID, nil, c, ""); err != nil {
			return err
		}
		out = c
		return nil
	})
	return out, wrapTxErr(err)
}

// Update replaces the mutable fields of a CfP. Requires cfp.manage.
func (s *CfPService) Update(ctx context.Context, id uuid.UUID, in CfPInput) (*CallForPapers, error) {
	if _, err := auth.MustFromContext(ctx); err != nil {
		return nil, httperror.New(httperror.CodeUnauthorized, "not authenticated")
	}

	var out *CallForPapers
	err := db.WithTx(ctx, s.pool, func(tx pgx.Tx) error {
		before, err := loadCfP(ctx, tx, id, true)
		if err != nil {
			return err
		}
		if err := auth.RequireForEdition(ctx, domain.PermCfPManage, before.OrganizationID, before.EditionID); err != nil {
			return err
		}
		in.fillFrom(before)
		if err := in.normalize(); err != nil {
			return err
		}
		if err := in.checkEventTypes(ctx, tx, before.EditionID); err != nil {
			return err
		}
		after, err := scanCfP(tx.QueryRow(ctx,
			`UPDATE call_for_papers
			    SET title=$2, description=$3, status=$4, opens_at=$5, closes_at=$6,
			        form=$7, review_mode=$8, event_type_ids=$9, status_labels=$10, updated_at=now()
			  WHERE id=$1
			  RETURNING `+cfpColumns,
			id, in.Title, in.Description, in.Status, in.OpensAt, in.ClosesAt,
			in.Form, *in.ReviewMode, *in.EventTypeIDs, *in.StatusLabels))
		if err != nil {
			return err
		}
		if err := audit.Record(ctx, tx, after.OrganizationID, &after.EditionID, "cfp.update",
			string(domain.ResourceCallForPaper), after.ID, before, after, ""); err != nil {
			return err
		}
		out = after
		return nil
	})
	return out, wrapTxErr(err)
}

// Delete removes a CfP that has no submissions. Requires cfp.manage.
func (s *CfPService) Delete(ctx context.Context, id uuid.UUID) error {
	if _, err := auth.MustFromContext(ctx); err != nil {
		return httperror.New(httperror.CodeUnauthorized, "not authenticated")
	}
	err := db.WithTx(ctx, s.pool, func(tx pgx.Tx) error {
		before, err := loadCfP(ctx, tx, id, true)
		if err != nil {
			return err
		}
		if err := auth.RequireForEdition(ctx, domain.PermCfPManage, before.OrganizationID, before.EditionID); err != nil {
			return err
		}
		var n int
		if err := tx.QueryRow(ctx, `SELECT count(*) FROM submissions WHERE cfp_id = $1`, id).Scan(&n); err != nil {
			return httperror.Internal(err)
		}
		if n > 0 {
			return httperror.Conflict("call for papers has submissions; close it instead of deleting")
		}
		if _, err := tx.Exec(ctx, `DELETE FROM call_for_papers WHERE id = $1`, id); err != nil {
			return httperror.Internal(err)
		}
		return audit.Record(ctx, tx, before.OrganizationID, &before.EditionID, "cfp.delete",
			string(domain.ResourceCallForPaper), before.ID, before, nil, "")
	})
	return wrapTxErr(err)
}

// wrapTxErr leaves typed httperror values untouched and wraps anything else
// as an opaque internal error. db.WithTx returns the callback error as-is,
// so typed errors survive the transaction boundary.
func wrapTxErr(err error) error {
	if err == nil {
		return nil
	}
	var he *httperror.Error
	if errors.As(err, &he) {
		return err
	}
	return httperror.Internal(err)
}
