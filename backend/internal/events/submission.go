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

// Speaker is one presenter attached to a submission (legacy free-text list;
// people with accounts live in submission_people).
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
	CfPTitle        string                 `json:"cfp_title"`
	EditionTitle    string                 `json:"edition_title"`
	SubmitterUserID uuid.UUID              `json:"submitter_user_id"`
	SubmitterName   string                 `json:"submitter_name"`
	Title           string                 `json:"title"`
	Abstract        string                 `json:"abstract"`
	Speakers        []Speaker              `json:"speakers"`
	Answers         map[string]any         `json:"answers"`
	EventTypeID     *uuid.UUID             `json:"event_type_id"`
	EventTypeName   string                 `json:"event_type_name"`
	DurationMinutes *int                   `json:"duration_minutes"`
	State           domain.SubmissionState `json:"state"`
	SubmittedAt     *time.Time             `json:"submitted_at"`
	DecidedAt       *time.Time             `json:"decided_at"`
	DecisionNote    string                 `json:"decision_note"`
	People          []Person               `json:"people"`
	Files           []FileInfo             `json:"files"`
	// From the call: its status names and whether it still takes changes.
	StatusLabels map[string]string `json:"status_labels"`
	CfPAccepting bool              `json:"cfp_accepting"`
	// Staff only (scored review mode).
	ScoreAvg   *float64  `json:"score_avg,omitempty"`
	ScoreCount int       `json:"score_count,omitempty"`
	CreatedAt  time.Time `json:"created_at"`
	UpdatedAt  time.Time `json:"updated_at"`
}

// SubmissionInput is the create payload.
type SubmissionInput struct {
	Title           string                     `json:"title"`
	Abstract        string                     `json:"abstract"`
	Speakers        []Speaker                  `json:"speakers"`
	Answers         map[string]json.RawMessage `json:"answers"`
	EventTypeID     *uuid.UUID                 `json:"event_type_id"`
	DurationMinutes *int                       `json:"duration_minutes"`
}

// SubmissionPatch is the partial-update payload; nil fields are unchanged.
// DurationMinutes 0 clears the requested duration.
type SubmissionPatch struct {
	Title           *string                     `json:"title"`
	Abstract        *string                     `json:"abstract"`
	Speakers        *[]Speaker                  `json:"speakers"`
	Answers         *map[string]json.RawMessage `json:"answers"`
	EventTypeID     *uuid.UUID                  `json:"event_type_id"`
	DurationMinutes *int                        `json:"duration_minutes"`
}

// SubmissionFilter narrows List results.
type SubmissionFilter struct {
	CfPID *uuid.UUID
	State domain.SubmissionState // "" = any
	Mine  bool                   // staff only: restrict to own submissions
}

// validateDraft checks the fields required for any stored submission.
func validateDraft(title, abstract string, speakers []Speaker, duration *int) map[string]string {
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
	if duration != nil && (*duration < 5 || *duration > 1440) {
		issues["duration_minutes"] = "must be between 5 and 1440"
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
	return issues
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
	SELECT s.id, s.organization_id, s.edition_id, s.cfp_id, c.title, e.title, s.submitter_user_id,
	       u.display_name, s.title, s.abstract, s.speakers, s.answers,
	       s.event_type_id, COALESCE(et.name, ''), s.duration_minutes, s.state,
	       s.submitted_at, s.decided_at,
	       COALESCE((SELECT r.note FROM submission_reviews r
	                  WHERE r.submission_id = s.id
	                  ORDER BY r.created_at DESC, r.id DESC LIMIT 1), ''),
	       s.created_at, s.updated_at, c.status_labels,
	       (c.status = 'open' AND (c.opens_at IS NULL OR c.opens_at <= now())
	                          AND (c.closes_at IS NULL OR c.closes_at > now()))
	  FROM submissions s
	  JOIN users u ON u.id = s.submitter_user_id
	  JOIN call_for_papers c ON c.id = s.cfp_id
	  JOIN editions e ON e.id = s.edition_id
	  LEFT JOIN event_types et ON et.id = s.event_type_id`

func scanSubmission(row pgx.Row) (*Submission, error) {
	var s Submission
	var speakersJSON, answersJSON, labelsJSON []byte
	err := row.Scan(&s.ID, &s.OrganizationID, &s.EditionID, &s.CfPID, &s.CfPTitle, &s.EditionTitle,
		&s.SubmitterUserID, &s.SubmitterName, &s.Title, &s.Abstract, &speakersJSON, &answersJSON,
		&s.EventTypeID, &s.EventTypeName, &s.DurationMinutes, &s.State,
		&s.SubmittedAt, &s.DecidedAt, &s.DecisionNote, &s.CreatedAt, &s.UpdatedAt,
		&labelsJSON, &s.CfPAccepting)
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
	s.Answers = map[string]any{}
	if len(answersJSON) > 0 {
		if err := json.Unmarshal(answersJSON, &s.Answers); err != nil {
			return nil, httperror.Internal(err)
		}
	}
	s.StatusLabels = map[string]string{}
	if len(labelsJSON) > 0 {
		if err := json.Unmarshal(labelsJSON, &s.StatusLabels); err != nil {
			return nil, httperror.Internal(err)
		}
	}
	s.People = []Person{}
	s.Files = []FileInfo{}
	return &s, nil
}

func loadSubmission(ctx context.Context, q querier, id uuid.UUID, forUpdate bool) (*Submission, error) {
	sql := submissionSelect + ` WHERE s.id = $1`
	if forUpdate {
		sql += ` FOR UPDATE OF s`
	}
	s, err := scanSubmission(q.QueryRow(ctx, sql, id))
	if err != nil {
		return nil, err
	}
	if err := loadPeopleAndFiles(ctx, q, s); err != nil {
		return nil, err
	}
	return s, nil
}

// canView: the owner, an accepted co-presenter, or an edition reviewer.
func canView(u *auth.User, s *Submission) bool {
	if s.SubmitterUserID == u.ID {
		return true
	}
	for _, p := range s.People {
		if p.UserID != nil && *p.UserID == u.ID && p.Status == PersonAccepted {
			return true
		}
	}
	return u.HasForEdition(domain.PermSubmissionReview, s.OrganizationID, s.EditionID)
}

// loadVisible loads a submission and hides it (404) from callers who may not
// see it.
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
	sub, err := loadVisible(ctx, s.pool, u, id, false)
	if err != nil {
		return nil, err
	}
	if u.HasForEdition(domain.PermSubmissionReview, sub.OrganizationID, sub.EditionID) {
		if err := s.fillScores(ctx, []*Submission{sub}); err != nil {
			return nil, err
		}
	}
	return sub, nil
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
	reviewer := u.HasForEdition(domain.PermSubmissionReview, orgID, editionID)
	seeAll := reviewer && !f.Mine

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
	out, err := collectSubmissions(rows)
	if err != nil {
		return nil, err
	}
	if reviewer {
		ptrs := make([]*Submission, len(out))
		for i := range out {
			ptrs[i] = &out[i]
		}
		if err := s.fillScores(ctx, ptrs); err != nil {
			return nil, err
		}
	}
	return out, nil
}

// ListMine returns every submission the caller owns or co-presents (accepted),
// across all calls. Used by the public submission site.
func (s *SubmissionService) ListMine(ctx context.Context) ([]Submission, error) {
	u, err := auth.MustFromContext(ctx)
	if err != nil {
		return nil, httperror.New(httperror.CodeUnauthorized, "not authenticated")
	}
	rows, err := s.pool.Query(ctx, submissionSelect+`
		 WHERE s.submitter_user_id = $1
		    OR EXISTS (SELECT 1 FROM submission_people p
		                WHERE p.submission_id = s.id AND p.user_id = $1 AND p.status = 'accepted')
		 ORDER BY s.updated_at DESC, s.id`, u.ID)
	if err != nil {
		return nil, httperror.Internal(err)
	}
	out, err := collectSubmissions(rows)
	if err != nil {
		return nil, err
	}
	for i := range out {
		if err := loadPeopleAndFiles(ctx, s.pool, &out[i]); err != nil {
			return nil, err
		}
	}
	return out, nil
}

func collectSubmissions(rows pgx.Rows) ([]Submission, error) {
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

// fillScores adds the average reviewer score (staff views only).
func (s *SubmissionService) fillScores(ctx context.Context, subs []*Submission) error {
	if len(subs) == 0 {
		return nil
	}
	ids := make([]uuid.UUID, len(subs))
	byID := map[uuid.UUID]*Submission{}
	for i, sub := range subs {
		ids[i] = sub.ID
		byID[sub.ID] = sub
	}
	rows, err := s.pool.Query(ctx, `SELECT submission_id, avg(score)::float8, count(*)
		FROM submission_scores WHERE submission_id = ANY($1) GROUP BY submission_id`, ids)
	if err != nil {
		return httperror.Internal(err)
	}
	defer rows.Close()
	for rows.Next() {
		var id uuid.UUID
		var avg float64
		var n int
		if err := rows.Scan(&id, &avg, &n); err != nil {
			return httperror.Internal(err)
		}
		if sub := byID[id]; sub != nil {
			sub.ScoreAvg, sub.ScoreCount = &avg, n
		}
	}
	return rows.Err()
}

// formState bundles what saving answers needs to know about the CfP.
type formState struct {
	cfp        *CallForPapers
	eventTypes []EventType
}

func loadFormState(ctx context.Context, q querier, cfpID uuid.UUID) (*formState, error) {
	cfp, err := loadCfP(ctx, q, cfpID, false)
	if err != nil {
		return nil, err
	}
	types, err := allowedEventTypes(ctx, q, cfp)
	if err != nil {
		return nil, err
	}
	return &formState{cfp: cfp, eventTypes: types}, nil
}

func (fs *formState) checkEventType(id *uuid.UUID, issues map[string]string) {
	if id == nil {
		return
	}
	for _, t := range fs.eventTypes {
		if t.ID == *id {
			return
		}
	}
	issues["event_type_id"] = "is not offered in this call"
}

// mergeAnswers keeps answers of archived/removed fields so they stay readable
// after the form changes.
func mergeAnswers(f Form, old, cleaned map[string]any) map[string]any {
	out := map[string]any{}
	for k, v := range old {
		if _, active := f.Field(k); !active {
			out[k] = v
		}
	}
	for k, v := range cleaned {
		out[k] = v
	}
	return out
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
	issues := validateDraft(in.Title, in.Abstract, in.Speakers, in.DurationMinutes)
	speakersJSON, err := json.Marshal(in.Speakers)
	if err != nil {
		return nil, httperror.Internal(err)
	}

	var out *Submission
	err = db.WithTx(ctx, s.pool, func(tx pgx.Tx) error {
		fs, err := loadFormState(ctx, tx, cfpID)
		if err != nil {
			return err
		}
		cfp := fs.cfp
		if cfp.Status != CfPOpen && !isStaffForEdition(u, cfp.OrganizationID, cfp.EditionID) {
			return httperror.NotFound("call for papers")
		}
		if !cfp.AcceptsSubmissions(s.now()) {
			return httperror.Conflict("call for papers is not accepting submissions")
		}
		check := checkAnswers(cfp.Form, in.Answers, false)
		for k, v := range check.issues {
			issues[k] = v
		}
		fs.checkEventType(in.EventTypeID, issues)
		if err := validationIfAny("invalid submission", issues); err != nil {
			return err
		}
		var id uuid.UUID
		if err := tx.QueryRow(ctx,
			`INSERT INTO submissions
			     (organization_id, edition_id, cfp_id, submitter_user_id, title, abstract, speakers,
			      answers, event_type_id, duration_minutes)
			 VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10)
			 RETURNING id`,
			cfp.OrganizationID, cfp.EditionID, cfp.ID, u.ID, in.Title, in.Abstract, speakersJSON,
			check.answers, in.EventTypeID, in.DurationMinutes,
		).Scan(&id); err != nil {
			return httperror.Internal(err)
		}
		if _, err := tx.Exec(ctx, `INSERT INTO submission_people (submission_id, user_id, role, status, responded_at)
			VALUES ($1, $2, 'owner', 'accepted', now())`, id, u.ID); err != nil {
			return httperror.Internal(err)
		}
		if err := attachFiles(ctx, tx, id, cfp.ID, u.ID, check.files); err != nil {
			return err
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

// editable reports whether the owner may still change a submission: drafts
// always (while the call takes submissions), submitted ones until the call
// closes.
func (s *SubmissionService) editable(sub *Submission, cfp *CallForPapers) error {
	switch sub.State {
	case domain.SubmissionDraft, domain.SubmissionSubmitted:
	default:
		return httperror.Conflict("this submission can no longer be edited")
	}
	if !cfp.AcceptsSubmissions(s.now()) {
		return httperror.Conflict("the call for papers is closed; changes are no longer possible")
	}
	return nil
}

// Update edits a submission. Only the submitter may edit, while it is a
// draft or submitted and the call is still open. Submitted submissions must
// stay complete.
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
		fs, err := loadFormState(ctx, tx, cur.CfPID)
		if err != nil {
			return err
		}
		if err := s.editable(cur, fs.cfp); err != nil {
			return err
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
		if p.EventTypeID != nil {
			next.EventTypeID = p.EventTypeID
		}
		if p.DurationMinutes != nil {
			if *p.DurationMinutes == 0 {
				next.DurationMinutes = nil
			} else {
				next.DurationMinutes = p.DurationMinutes
			}
		}
		final := cur.State == domain.SubmissionSubmitted
		issues := validateDraft(next.Title, next.Abstract, next.Speakers, next.DurationMinutes)
		fs.checkEventType(next.EventTypeID, issues)

		files := map[string][]uuid.UUID{}
		if p.Answers != nil {
			check := checkAnswers(fs.cfp.Form, *p.Answers, final)
			for k, v := range check.issues {
				issues[k] = v
			}
			next.Answers = mergeAnswers(fs.cfp.Form, cur.Answers, check.answers)
			files = check.files
		}
		if final {
			for k, v := range completenessIssues(fs, &next) {
				issues[k] = v
			}
		}
		if err := validationIfAny("invalid submission", issues); err != nil {
			return err
		}
		speakersJSON, err := json.Marshal(next.Speakers)
		if err != nil {
			return httperror.Internal(err)
		}
		if _, err := tx.Exec(ctx,
			`UPDATE submissions
			    SET title=$2, abstract=$3, speakers=$4, answers=$5, event_type_id=$6,
			        duration_minutes=$7, updated_at=now()
			  WHERE id=$1`,
			id, next.Title, next.Abstract, speakersJSON, next.Answers, next.EventTypeID,
			next.DurationMinutes); err != nil {
			return httperror.Internal(err)
		}
		if p.Answers != nil {
			if err := syncFiles(ctx, tx, fs.cfp.Form, id, cur.CfPID, u.ID, files); err != nil {
				return err
			}
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

// completenessIssues are the rules for draft→submitted (and for editing a
// submitted submission): required answers and an event type when the call
// offers any.
func completenessIssues(fs *formState, sub *Submission) map[string]string {
	issues := map[string]string{}
	raw := map[string]json.RawMessage{}
	for k, v := range sub.Answers {
		b, _ := json.Marshal(v)
		raw[k] = b
	}
	for k, v := range checkAnswers(fs.cfp.Form, raw, true).issues {
		issues[k] = v
	}
	if len(fs.eventTypes) > 0 && sub.EventTypeID == nil {
		issues["event_type_id"] = "pick what kind of event this is"
	}
	return issues
}

// Submit moves a draft to submitted (submitter only, CfP must be open).
func (s *SubmissionService) Submit(ctx context.Context, id uuid.UUID) (*Submission, error) {
	return s.apply(ctx, id, domain.ActionSubmit, "")
}

// Withdraw lets the submitter pull a submitted or accepted submission.
func (s *SubmissionService) Withdraw(ctx context.Context, id uuid.UUID) (*Submission, error) {
	return s.apply(ctx, id, domain.ActionWithdraw, "")
}

// Accept records an accept decision (requires submission.decide).
func (s *SubmissionService) Accept(ctx context.Context, id uuid.UUID, note string) (*Submission, error) {
	return s.apply(ctx, id, domain.ActionAccept, note)
}

// Reject records a reject decision (requires submission.decide).
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
			if err := auth.RequireForEdition(ctx, domain.PermSubmissionDecide, cur.OrganizationID, cur.EditionID); err != nil {
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
			fs, err := loadFormState(ctx, tx, cur.CfPID)
			if err != nil {
				return err
			}
			if !fs.cfp.AcceptsSubmissions(s.now()) {
				return httperror.Conflict("call for papers is not accepting submissions")
			}
			if err := validationIfAny("submission is incomplete", completenessIssues(fs, cur)); err != nil {
				return err
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
