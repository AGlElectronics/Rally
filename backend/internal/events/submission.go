package events

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
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

const (
	maxAbstractLen    = 10000
	maxSpeakers       = 20
	maxSpeakerNameLen = 200
	maxSpeakerMailLen = 320
	maxSpeakerBioLen  = 2000
	maxNoteLen        = 5000
)

// Speaker is one presenter attached to a submission.
type Speaker struct {
	Name  string `json:"name"`
	Email string `json:"email,omitempty"`
	Bio   string `json:"bio,omitempty"`
}

// Submission is the API-shaped projection of a submissions row.
type Submission struct {
	ID              uuid.UUID              `json:"id"`
	OrganizationID  uuid.UUID              `json:"organization_id"`
	EditionID       uuid.UUID              `json:"edition_id"`
	CfPID           uuid.UUID              `json:"cfp_id"`
	SubmitterUserID uuid.UUID              `json:"submitter_user_id"`
	SubmitterName   string                 `json:"submitter_name"`
	Title           string                 `json:"title"`
	Abstract        string                 `json:"abstract"`
	Speakers        []Speaker              `json:"speakers"`
	State           domain.SubmissionState `json:"state"`
	SubmittedAt     *time.Time             `json:"submitted_at"`
	DecidedAt       *time.Time             `json:"decided_at"`
	DecisionNote    string                 `json:"decision_note"`
	CreatedAt       time.Time              `json:"created_at"`
	UpdatedAt       time.Time              `json:"updated_at"`
}

// SubmissionInput is the create payload.
type SubmissionInput struct {
	Title    string    `json:"title"`
	Abstract string    `json:"abstract"`
	Speakers []Speaker `json:"speakers"`
}

// SubmissionPatch is the partial-update payload; nil fields are unchanged.
type SubmissionPatch struct {
	Title    *string    `json:"title"`
	Abstract *string    `json:"abstract"`
	Speakers *[]Speaker `json:"speakers"`
}

// SubmissionFilter narrows List results.
type SubmissionFilter struct {
	CfPID *uuid.UUID
	State domain.SubmissionState // "" = any
	Mine  bool                   // staff only: restrict to own submissions
}

// validateDraft checks the fields required for any stored submission.
func validateDraft(title, abstract string, speakers []Speaker) error {
	issues := map[string]string{}
	switch {
	case strings.TrimSpace(title) == "":
		issues["title"] = "is required"
	case len([]rune(title)) > maxTitleLen:
		issues["title"] = "must be at most 200 characters"
	}
	if len([]rune(abstract)) > maxAbstractLen {
		issues["abstract"] = "must be at most 10000 characters"
	}
	if len(speakers) > maxSpeakers {
		issues["speakers"] = "at most 20 speakers"
	}
	for i, sp := range speakers {
		key := fmt.Sprintf("speakers[%d]", i)
		switch {
		case strings.TrimSpace(sp.Name) == "":
			issues[key+".name"] = "is required"
		case len([]rune(sp.Name)) > maxSpeakerNameLen:
			issues[key+".name"] = "must be at most 200 characters"
		}
		if len(sp.Email) > maxSpeakerMailLen {
			issues[key+".email"] = "is too long"
		}
		if len([]rune(sp.Bio)) > maxSpeakerBioLen {
			issues[key+".bio"] = "must be at most 2000 characters"
		}
	}
	if len(issues) > 0 {
		return httperror.Validation("invalid submission", issues)
	}
	return nil
}

// validateForSubmit adds the completeness rules enforced at draft→submitted.
func validateForSubmit(s *Submission) error {
	issues := map[string]string{}
	if strings.TrimSpace(s.Abstract) == "" {
		issues["abstract"] = "is required to submit"
	}
	if len(s.Speakers) == 0 {
		issues["speakers"] = "at least one speaker is required to submit"
	}
	if len(issues) > 0 {
		return httperror.Validation("submission is incomplete", issues)
	}
	return nil
}

func normalizeSpeakers(in []Speaker) []Speaker {
	out := make([]Speaker, 0, len(in))
	for _, sp := range in {
		out = append(out, Speaker{
			Name:  strings.TrimSpace(sp.Name),
			Email: strings.TrimSpace(sp.Email),
			Bio:   strings.TrimSpace(sp.Bio),
		})
	}
	return out
}

// SubmissionService manages CfP submissions and their state machine.
type SubmissionService struct {
	pool *pgxpool.Pool
	now  func() time.Time
}

// NewSubmissionService returns a service bound to pool.
func NewSubmissionService(pool *pgxpool.Pool) *SubmissionService {
	return &SubmissionService{pool: pool, now: time.Now}
}

const submissionSelect = `
	SELECT s.id, s.organization_id, s.edition_id, s.cfp_id, s.submitter_user_id,
	       u.display_name, s.title, s.abstract, s.speakers, s.state,
	       s.submitted_at, s.decided_at,
	       COALESCE((SELECT r.note FROM submission_reviews r
	                  WHERE r.submission_id = s.id
	                  ORDER BY r.created_at DESC, r.id DESC LIMIT 1), ''),
	       s.created_at, s.updated_at
	  FROM submissions s
	  JOIN users u ON u.id = s.submitter_user_id`

func scanSubmission(row pgx.Row) (*Submission, error) {
	var s Submission
	var speakersJSON []byte
	err := row.Scan(&s.ID, &s.OrganizationID, &s.EditionID, &s.CfPID, &s.SubmitterUserID,
		&s.SubmitterName, &s.Title, &s.Abstract, &speakersJSON, &s.State,
		&s.SubmittedAt, &s.DecidedAt, &s.DecisionNote, &s.CreatedAt, &s.UpdatedAt)
	switch {
	case err == nil:
	case errors.Is(err, pgx.ErrNoRows):
		return nil, httperror.NotFound("submission")
	default:
		return nil, httperror.Internal(err)
	}
	s.Speakers = []Speaker{}
	if len(speakersJSON) > 0 {
		if err := json.Unmarshal(speakersJSON, &s.Speakers); err != nil {
			return nil, httperror.Internal(err)
		}
	}
	return &s, nil
}

func loadSubmission(ctx context.Context, q querier, id uuid.UUID, forUpdate bool) (*Submission, error) {
	sql := submissionSelect + ` WHERE s.id = $1`
	if forUpdate {
		sql += ` FOR UPDATE OF s`
	}
	return scanSubmission(q.QueryRow(ctx, sql, id))
}

// canView: the owner or an edition reviewer.
func canView(u *auth.User, s *Submission) bool {
	return s.SubmitterUserID == u.ID ||
		u.HasForEdition(domain.PermSubmissionReview, s.OrganizationID, s.EditionID)
}

// loadVisible loads a submission and hides it (404) from callers who are
// neither the owner nor a reviewer of its edition.
func loadVisible(ctx context.Context, q querier, u *auth.User, id uuid.UUID, forUpdate bool) (*Submission, error) {
	s, err := loadSubmission(ctx, q, id, forUpdate)
	if err != nil {
		return nil, err
	}
	if !canView(u, s) {
		return nil, httperror.NotFound("submission")
	}
	return s, nil
}

// requireOwner returns 403 unless u authored s.
func requireOwner(u *auth.User, s *Submission) error {
	if s.SubmitterUserID != u.ID {
		return httperror.New(httperror.CodeForbidden, "only the submitter can do this")
	}
	return nil
}

// auditView is the minimal projection written to audit before/after.
func auditView(s *Submission) map[string]any {
	return map[string]any{
		"cfp_id": s.CfPID,
		"title":  s.Title,
		"state":  s.State,
	}
}

func transitionError(err error) error {
	if errors.Is(err, domain.ErrInvalidTransition) {
		return httperror.Conflict(err.Error()).WithCause(err)
	}
	return err
}

// Get returns a submission visible to the caller.
func (s *SubmissionService) Get(ctx context.Context, id uuid.UUID) (*Submission, error) {
	u, err := auth.MustFromContext(ctx)
	if err != nil {
		return nil, httperror.New(httperror.CodeUnauthorized, "not authenticated")
	}
	return loadVisible(ctx, s.pool, u, id, false)
}

// List returns the submissions of an edition. Reviewers see all (unless
// filter.Mine); everyone else sees only their own.
func (s *SubmissionService) List(ctx context.Context, editionID uuid.UUID, f SubmissionFilter) ([]Submission, error) {
	u, err := auth.MustFromContext(ctx)
	if err != nil {
		return nil, httperror.New(httperror.CodeUnauthorized, "not authenticated")
	}
	orgID, err := editionOwner(ctx, s.pool, editionID)
	if err != nil {
		return nil, err
	}
	if f.State != "" && !f.State.Valid() {
		return nil, httperror.Validation("invalid filter", map[string]string{"state": "unknown state"})
	}
	seeAll := u.HasForEdition(domain.PermSubmissionReview, orgID, editionID) && !f.Mine

	rows, err := s.pool.Query(ctx, submissionSelect+`
		 WHERE s.edition_id = $1
		   AND ($2 OR s.submitter_user_id = $3)
		   AND ($4::uuid IS NULL OR s.cfp_id = $4)
		   AND ($5 = '' OR s.state = $5)
		 ORDER BY s.created_at ASC, s.id ASC`,
		editionID, seeAll, u.ID, f.CfPID, string(f.State))
	if err != nil {
		return nil, httperror.Internal(err)
	}
	defer rows.Close()

	out := make([]Submission, 0, 8)
	for rows.Next() {
		sub, err := scanSubmission(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *sub)
	}
	if err := rows.Err(); err != nil {
		return nil, httperror.Internal(err)
	}
	return out, nil
}

// Create starts a draft submission in an open CfP, owned by the caller.
func (s *SubmissionService) Create(ctx context.Context, cfpID uuid.UUID, in SubmissionInput) (*Submission, error) {
	u, err := auth.MustFromContext(ctx)
	if err != nil {
		return nil, httperror.New(httperror.CodeUnauthorized, "not authenticated")
	}
	in.Title = strings.TrimSpace(in.Title)
	in.Abstract = strings.TrimSpace(in.Abstract)
	in.Speakers = normalizeSpeakers(in.Speakers)
	if err := validateDraft(in.Title, in.Abstract, in.Speakers); err != nil {
		return nil, err
	}
	speakersJSON, err := json.Marshal(in.Speakers)
	if err != nil {
		return nil, httperror.Internal(err)
	}

	var out *Submission
	err = db.WithTx(ctx, s.pool, func(tx pgx.Tx) error {
		cfp, err := loadCfP(ctx, tx, cfpID, false)
		if err != nil {
			return err
		}
		if cfp.Status != CfPOpen && !isStaffForEdition(u, cfp.OrganizationID, cfp.EditionID) {
			return httperror.NotFound("call for papers")
		}
		if !cfp.AcceptsSubmissions(s.now()) {
			return httperror.Conflict("call for papers is not accepting submissions")
		}
		var id uuid.UUID
		if err := tx.QueryRow(ctx,
			`INSERT INTO submissions
			     (organization_id, edition_id, cfp_id, submitter_user_id, title, abstract, speakers)
			 VALUES ($1,$2,$3,$4,$5,$6,$7)
			 RETURNING id`,
			cfp.OrganizationID, cfp.EditionID, cfp.ID, u.ID, in.Title, in.Abstract, speakersJSON,
		).Scan(&id); err != nil {
			return httperror.Internal(err)
		}
		sub, err := loadSubmission(ctx, tx, id, false)
		if err != nil {
			return err
		}
		if err := audit.Record(ctx, tx, sub.OrganizationID, &sub.EditionID, "submission.create",
			string(domain.ResourceSubmission), sub.ID, nil, auditView(sub), ""); err != nil {
			return err
		}
		out = sub
		return nil
	})
	return out, wrapTxErr(err)
}

// Update edits a draft submission. Only the submitter may edit, and only
// while the submission is still a draft.
func (s *SubmissionService) Update(ctx context.Context, id uuid.UUID, p SubmissionPatch) (*Submission, error) {
	u, err := auth.MustFromContext(ctx)
	if err != nil {
		return nil, httperror.New(httperror.CodeUnauthorized, "not authenticated")
	}
	var out *Submission
	err = db.WithTx(ctx, s.pool, func(tx pgx.Tx) error {
		cur, err := loadVisible(ctx, tx, u, id, true)
		if err != nil {
			return err
		}
		if err := requireOwner(u, cur); err != nil {
			return err
		}
		if cur.State != domain.SubmissionDraft {
			return httperror.Conflict("only draft submissions can be edited")
		}
		before := auditView(cur)

		next := *cur
		if p.Title != nil {
			next.Title = strings.TrimSpace(*p.Title)
		}
		if p.Abstract != nil {
			next.Abstract = strings.TrimSpace(*p.Abstract)
		}
		if p.Speakers != nil {
			next.Speakers = normalizeSpeakers(*p.Speakers)
		}
		if err := validateDraft(next.Title, next.Abstract, next.Speakers); err != nil {
			return err
		}
		speakersJSON, err := json.Marshal(next.Speakers)
		if err != nil {
			return httperror.Internal(err)
		}
		if _, err := tx.Exec(ctx,
			`UPDATE submissions
			    SET title=$2, abstract=$3, speakers=$4, updated_at=now()
			  WHERE id=$1`,
			id, next.Title, next.Abstract, speakersJSON); err != nil {
			return httperror.Internal(err)
		}
		after, err := loadSubmission(ctx, tx, id, false)
		if err != nil {
			return err
		}
		if err := audit.Record(ctx, tx, after.OrganizationID, &after.EditionID, "submission.update",
			string(domain.ResourceSubmission), after.ID, before, auditView(after), ""); err != nil {
			return err
		}
		out = after
		return nil
	})
	return out, wrapTxErr(err)
}

// Submit moves a draft to submitted (submitter only, CfP must be open).
func (s *SubmissionService) Submit(ctx context.Context, id uuid.UUID) (*Submission, error) {
	return s.apply(ctx, id, domain.ActionSubmit, "")
}

// Withdraw lets the submitter pull a submitted or accepted submission.
func (s *SubmissionService) Withdraw(ctx context.Context, id uuid.UUID) (*Submission, error) {
	return s.apply(ctx, id, domain.ActionWithdraw, "")
}

// Accept records an accept decision (requires submission.review).
func (s *SubmissionService) Accept(ctx context.Context, id uuid.UUID, note string) (*Submission, error) {
	return s.apply(ctx, id, domain.ActionAccept, note)
}

// Reject records a reject decision (requires submission.review).
func (s *SubmissionService) Reject(ctx context.Context, id uuid.UUID, note string) (*Submission, error) {
	return s.apply(ctx, id, domain.ActionReject, note)
}

// apply runs one state-machine action inside a transaction: lock the row,
// authorize per action, validate the transition, persist, write the review
// row for decisions, and audit.
func (s *SubmissionService) apply(ctx context.Context, id uuid.UUID, action domain.SubmissionAction, note string) (*Submission, error) {
	u, err := auth.MustFromContext(ctx)
	if err != nil {
		return nil, httperror.New(httperror.CodeUnauthorized, "not authenticated")
	}
	note = strings.TrimSpace(note)
	if len([]rune(note)) > maxNoteLen {
		return nil, httperror.Validation("invalid decision", map[string]string{"note": "must be at most 5000 characters"})
	}
	isDecision := action == domain.ActionAccept || action == domain.ActionReject

	var out *Submission
	err = db.WithTx(ctx, s.pool, func(tx pgx.Tx) error {
		cur, err := loadVisible(ctx, tx, u, id, true)
		if err != nil {
			return err
		}

		// Authorization first, so an unauthorized caller learns nothing
		// about whether the transition would have been legal.
		if isDecision {
			if err := auth.RequireForEdition(ctx, domain.PermSubmissionReview, cur.OrganizationID, cur.EditionID); err != nil {
				return err
			}
			if cur.SubmitterUserID == u.ID {
				return httperror.New(httperror.CodeForbidden, "you cannot decide your own submission")
			}
		} else if err := requireOwner(u, cur); err != nil {
			return err
		}

		to, err := domain.Transition(cur.State, action)
		if err != nil {
			return transitionError(err)
		}

		if action == domain.ActionSubmit {
			if err := validateForSubmit(cur); err != nil {
				return err
			}
			cfp, err := loadCfP(ctx, tx, cur.CfPID, false)
			if err != nil {
				return err
			}
			if !cfp.AcceptsSubmissions(s.now()) {
				return httperror.Conflict("call for papers is not accepting submissions")
			}
		}

		switch {
		case action == domain.ActionSubmit:
			_, err = tx.Exec(ctx,
				`UPDATE submissions SET state=$2, submitted_at=now(), updated_at=now() WHERE id=$1`, id, to)
		case isDecision:
			_, err = tx.Exec(ctx,
				`UPDATE submissions SET state=$2, decided_at=now(), decided_by=$3, updated_at=now() WHERE id=$1`,
				id, to, u.ID)
		default:
			_, err = tx.Exec(ctx,
				`UPDATE submissions SET state=$2, updated_at=now() WHERE id=$1`, id, to)
		}
		if err != nil {
			return httperror.Internal(err)
		}

		if isDecision {
			if _, err := tx.Exec(ctx,
				`INSERT INTO submission_reviews (submission_id, organization_id, reviewer_user_id, decision, note)
				 VALUES ($1,$2,$3,$4,$5)`,
				id, cur.OrganizationID, u.ID, string(action), note); err != nil {
				return httperror.Internal(err)
			}
		}

		after, err := loadSubmission(ctx, tx, id, false)
		if err != nil {
			return err
		}
		if err := audit.Record(ctx, tx, after.OrganizationID, &after.EditionID,
			"submission."+string(action), string(domain.ResourceSubmission), after.ID,
			auditView(cur), auditView(after), note); err != nil {
			return err
		}
		out = after
		return nil
	})
	return out, wrapTxErr(err)
}
