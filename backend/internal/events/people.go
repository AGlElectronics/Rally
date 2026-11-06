package events

import (
	"context"
	"errors"
	"net/mail"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	"github.com/AGlElectronics/rally/backend/internal/audit"
	"github.com/AGlElectronics/rally/backend/internal/auth"
	"github.com/AGlElectronics/rally/backend/internal/codes"
	"github.com/AGlElectronics/rally/backend/internal/db"
	"github.com/AGlElectronics/rally/backend/internal/domain"
	"github.com/AGlElectronics/rally/backend/internal/httperror"
)

// Person roles and statuses on a submission.
const (
	PersonOwner     = "owner"
	PersonPresenter = "presenter"

	PersonInvited  = "invited"
	PersonAccepted = "accepted"

	maxPeoplePerSubmission = 20
)

// Person is someone on a submission: the owner, a linked co-presenter, or an
// open invite waiting for someone to claim it.
type Person struct {
	ID          uuid.UUID  `json:"id"`
	UserID      *uuid.UUID `json:"user_id"`
	DisplayName string     `json:"display_name"`
	Role        string     `json:"role"`
	Status      string     `json:"status"`
	InviteEmail string     `json:"invite_email,omitempty"`
	InviteCode  string     `json:"invite_code,omitempty"`
	CreatedAt   time.Time  `json:"created_at"`
}

// FileInfo describes an uploaded file attached to a submission.
type FileInfo struct {
	ID          uuid.UUID `json:"id"`
	FieldID     string    `json:"field_id"`
	Filename    string    `json:"filename"`
	ContentType string    `json:"content_type"`
	SizeBytes   int64     `json:"size_bytes"`
}

func loadPeopleAndFiles(ctx context.Context, q querier, s *Submission) error {
	rows, err := q.Query(ctx, `
		SELECT p.id, p.user_id, COALESCE(u.display_name, ''), p.role, p.status,
		       p.invite_email, COALESCE(p.invite_code, ''), p.created_at
		  FROM submission_people p
		  LEFT JOIN users u ON u.id = p.user_id
		 WHERE p.submission_id = $1
		 ORDER BY (p.role = 'owner') DESC, p.created_at, p.id`, s.ID)
	if err != nil {
		return httperror.Internal(err)
	}
	people := []Person{}
	for rows.Next() {
		var p Person
		if err := rows.Scan(&p.ID, &p.UserID, &p.DisplayName, &p.Role, &p.Status,
			&p.InviteEmail, &p.InviteCode, &p.CreatedAt); err != nil {
			rows.Close()
			return httperror.Internal(err)
		}
		people = append(people, p)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return httperror.Internal(err)
	}
	s.People = people

	frows, err := q.Query(ctx, `SELECT id, field_id, filename, content_type, size_bytes
		FROM submission_files WHERE submission_id = $1 AND status = 'uploaded'
		ORDER BY created_at, id`, s.ID)
	if err != nil {
		return httperror.Internal(err)
	}
	defer frows.Close()
	files := []FileInfo{}
	for frows.Next() {
		var f FileInfo
		if err := frows.Scan(&f.ID, &f.FieldID, &f.Filename, &f.ContentType, &f.SizeBytes); err != nil {
			return httperror.Internal(err)
		}
		files = append(files, f)
	}
	s.Files = files
	return frows.Err()
}

// RedactFor hides invite codes and emails from everyone but the owner.
func RedactFor(u *auth.User, s *Submission) {
	if s.SubmitterUserID == u.ID {
		return
	}
	for i := range s.People {
		s.People[i].InviteCode = ""
		s.People[i].InviteEmail = ""
	}
}

// AttendeeProfile is what the submission site shows about the signed-in
// attendee.
type AttendeeProfile struct {
	ID            uuid.UUID `json:"id"`
	DisplayName   string    `json:"display_name"`
	Email         string    `json:"email"`
	EmailVerified bool      `json:"email_verified"`
	PresenterCode string    `json:"presenter_code"`
}

// Profile returns the caller's attendee profile.
func (s *SubmissionService) Profile(ctx context.Context) (*AttendeeProfile, error) {
	u, err := auth.MustFromContext(ctx)
	if err != nil {
		return nil, httperror.New(httperror.CodeUnauthorized, "not authenticated")
	}
	p := AttendeeProfile{ID: u.ID, DisplayName: u.DisplayName, Email: u.Email}
	err = s.pool.QueryRow(ctx, `SELECT presenter_code, email_verified FROM attendee_profiles WHERE user_id=$1`, u.ID).
		Scan(&p.PresenterCode, &p.EmailVerified)
	if err != nil {
		return nil, httperror.Internal(err)
	}
	return &p, nil
}

// AddPersonInput adds a co-presenter by email or by their presenter code.
type AddPersonInput struct {
	Email         string `json:"email"`
	PresenterCode string `json:"presenter_code"`
}

// AddPerson adds a co-presenter (owner only, while editable):
//   - a presenter code links that attendee straight away;
//   - an email matching exactly one verified attendee links them too;
//   - otherwise an invite code is created for the person to claim.
//
// Linked co-presenters can leave at any time.
func (s *SubmissionService) AddPerson(ctx context.Context, submissionID uuid.UUID, in AddPersonInput) (*Submission, error) {
	u, err := auth.MustFromContext(ctx)
	if err != nil {
		return nil, httperror.New(httperror.CodeUnauthorized, "not authenticated")
	}
	email := strings.TrimSpace(in.Email)
	code := codes.Normalize(in.PresenterCode)
	if (email == "") == (code == "") {
		return nil, httperror.Validation("invalid person", map[string]string{"email": "give an email or a presenter code"})
	}
	if email != "" {
		if a, err := mail.ParseAddress(email); err != nil || a.Address != email || len(email) > maxSpeakerMailLen {
			return nil, httperror.Validation("invalid person", map[string]string{"email": "must be an email address"})
		}
	}
	return s.mutatePeople(ctx, u, submissionID, "submission.person_add", func(tx pgx.Tx, cur *Submission) error {
		if len(cur.People) >= maxPeoplePerSubmission {
			return httperror.Validation("invalid person", map[string]string{"email": "at most 20 people"})
		}
		var linkUser *uuid.UUID
		if code != "" {
			var id uuid.UUID
			err := tx.QueryRow(ctx, `SELECT user_id FROM attendee_profiles WHERE presenter_code=$1`, code).Scan(&id)
			if errors.Is(err, pgx.ErrNoRows) {
				return httperror.Validation("invalid person", map[string]string{"presenter_code": "no one has this presenter code"})
			}
			if err != nil {
				return httperror.Internal(err)
			}
			linkUser = &id
		} else {
			rows, err := tx.Query(ctx, `SELECT u.id FROM users u JOIN attendee_profiles a ON a.user_id = u.id
				WHERE lower(u.email) = lower($1) AND a.email_verified LIMIT 2`, email)
			if err != nil {
				return httperror.Internal(err)
			}
			var ids []uuid.UUID
			for rows.Next() {
				var id uuid.UUID
				if err := rows.Scan(&id); err != nil {
					rows.Close()
					return httperror.Internal(err)
				}
				ids = append(ids, id)
			}
			rows.Close()
			if len(ids) == 1 {
				linkUser = &ids[0]
			}
		}
		if linkUser != nil {
			for _, p := range cur.People {
				if p.UserID != nil && *p.UserID == *linkUser {
					return httperror.Conflict("this person is already on the submission")
				}
			}
			_, err := tx.Exec(ctx, `INSERT INTO submission_people
				(submission_id, user_id, role, status, invite_email, invited_by, responded_at)
				VALUES ($1, $2, 'presenter', 'accepted', $3, $4, now())`,
				submissionID, *linkUser, email, u.ID)
			return mapPeopleErr(err)
		}
		for _, p := range cur.People {
			if p.Status == PersonInvited && strings.EqualFold(p.InviteEmail, email) {
				return httperror.Conflict("this email already has an open invite")
			}
		}
		return insertInvite(ctx, tx, submissionID, email, u.ID)
	})
}

func insertInvite(ctx context.Context, tx pgx.Tx, submissionID uuid.UUID, email string, by uuid.UUID) error {
	for attempt := 0; attempt < 5; attempt++ {
		_, err := tx.Exec(ctx, `INSERT INTO submission_people
			(submission_id, role, status, invite_email, invite_code, invited_by)
			VALUES ($1, 'presenter', 'invited', $2, $3, $4)`, submissionID, email, codes.New(), by)
		if isInviteCodeClash(err) {
			continue
		}
		return mapPeopleErr(err)
	}
	return httperror.Internal(errors.New("could not allocate an invite code"))
}

func isInviteCodeClash(err error) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == "23505" && pgErr.ConstraintName == "submission_people_invite_code_key"
}

func mapPeopleErr(err error) error {
	if err == nil {
		return nil
	}
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && pgErr.Code == "23505" {
		return httperror.Conflict("this person is already on the submission")
	}
	return httperror.Internal(err)
}

// RemovePerson removes a co-presenter or open invite (owner only).
func (s *SubmissionService) RemovePerson(ctx context.Context, submissionID, personID uuid.UUID) (*Submission, error) {
	u, err := auth.MustFromContext(ctx)
	if err != nil {
		return nil, httperror.New(httperror.CodeUnauthorized, "not authenticated")
	}
	return s.mutatePeople(ctx, u, submissionID, "submission.person_remove", func(tx pgx.Tx, _ *Submission) error {
		tag, err := tx.Exec(ctx, `DELETE FROM submission_people WHERE id=$1 AND submission_id=$2 AND role <> 'owner'`,
			personID, submissionID)
		if err != nil {
			return httperror.Internal(err)
		}
		if tag.RowsAffected() == 0 {
			return httperror.NotFound("person")
		}
		return nil
	})
}

// ReissueInvite replaces an open invite's code (the old link stops working).
func (s *SubmissionService) ReissueInvite(ctx context.Context, submissionID, personID uuid.UUID) (*Submission, error) {
	u, err := auth.MustFromContext(ctx)
	if err != nil {
		return nil, httperror.New(httperror.CodeUnauthorized, "not authenticated")
	}
	return s.mutatePeople(ctx, u, submissionID, "submission.invite_reissue", func(tx pgx.Tx, _ *Submission) error {
		for attempt := 0; attempt < 5; attempt++ {
			tag, err := tx.Exec(ctx, `UPDATE submission_people SET invite_code=$3
				WHERE id=$1 AND submission_id=$2 AND status='invited'`, personID, submissionID, codes.New())
			if isInviteCodeClash(err) {
				continue
			}
			if err != nil {
				return httperror.Internal(err)
			}
			if tag.RowsAffected() == 0 {
				return httperror.NotFound("invite")
			}
			return nil
		}
		return httperror.Internal(errors.New("could not allocate an invite code"))
	})
}

// mutatePeople runs fn for the owner of an editable submission, then audits
// and returns the fresh submission.
func (s *SubmissionService) mutatePeople(ctx context.Context, u *auth.User, submissionID uuid.UUID, action string,
	fn func(tx pgx.Tx, cur *Submission) error) (*Submission, error) {
	var out *Submission
	err := db.WithTx(ctx, s.pool, func(tx pgx.Tx) error {
		cur, err := loadVisible(ctx, tx, u, submissionID, true)
		if err != nil {
			return err
		}
		if err := requireOwner(u, cur); err != nil {
			return err
		}
		cfp, err := loadCfP(ctx, tx, cur.CfPID, false)
		if err != nil {
			return err
		}
		if err := s.editable(cur, cfp); err != nil {
			return err
		}
		if err := fn(tx, cur); err != nil {
			return err
		}
		after, err := loadSubmission(ctx, tx, submissionID, false)
		if err != nil {
			return err
		}
		if err := audit.Record(ctx, tx, after.OrganizationID, &after.EditionID, action,
			string(domain.ResourceSubmission), after.ID, len(cur.People), len(after.People), ""); err != nil {
			return err
		}
		out = after
		return nil
	})
	return out, wrapTxErr(err)
}

// InvitePreview is what someone opening an invite link sees before accepting.
type InvitePreview struct {
	Code           string `json:"code"`
	SubmissionID   string `json:"submission_id"`
	Title          string `json:"title"`
	CfPTitle       string `json:"cfp_title"`
	EditionTitle   string `json:"edition_title"`
	InvitedBy      string `json:"invited_by"`
	AlreadyOnIt    bool   `json:"already_on_it"`
	SubmissionOpen bool   `json:"submission_open"`
}

func (s *SubmissionService) findInvite(ctx context.Context, q querier, code string, forUpdate bool) (personID uuid.UUID, sub *Submission, err error) {
	code = codes.Normalize(code)
	var subID uuid.UUID
	sql := `SELECT id, submission_id FROM submission_people WHERE invite_code=$1 AND status='invited'`
	if forUpdate {
		sql += ` FOR UPDATE`
	}
	err = q.QueryRow(ctx, sql, code).Scan(&personID, &subID)
	if errors.Is(err, pgx.ErrNoRows) {
		return uuid.Nil, nil, httperror.NotFound("invite")
	}
	if err != nil {
		return uuid.Nil, nil, httperror.Internal(err)
	}
	sub, err = loadSubmission(ctx, q, subID, forUpdate)
	return personID, sub, err
}

// PreviewInvite shows an invite to the signed-in attendee.
func (s *SubmissionService) PreviewInvite(ctx context.Context, code string) (*InvitePreview, error) {
	u, err := auth.MustFromContext(ctx)
	if err != nil {
		return nil, httperror.New(httperror.CodeUnauthorized, "not authenticated")
	}
	_, sub, err := s.findInvite(ctx, s.pool, code, false)
	if err != nil {
		return nil, err
	}
	p := &InvitePreview{
		Code: codes.Normalize(code), SubmissionID: sub.ID.String(), Title: sub.Title,
		CfPTitle: sub.CfPTitle, EditionTitle: sub.EditionTitle, InvitedBy: sub.SubmitterName,
		SubmissionOpen: sub.State == domain.SubmissionDraft || sub.State == domain.SubmissionSubmitted || sub.State == domain.SubmissionAccepted,
	}
	for _, pp := range sub.People {
		if pp.UserID != nil && *pp.UserID == u.ID {
			p.AlreadyOnIt = true
		}
	}
	return p, nil
}

// AcceptInvite links the caller to the invite's submission.
func (s *SubmissionService) AcceptInvite(ctx context.Context, code string) (*Submission, error) {
	u, err := auth.MustFromContext(ctx)
	if err != nil {
		return nil, httperror.New(httperror.CodeUnauthorized, "not authenticated")
	}
	var out *Submission
	err = db.WithTx(ctx, s.pool, func(tx pgx.Tx) error {
		personID, sub, err := s.findInvite(ctx, tx, code, true)
		if err != nil {
			return err
		}
		if sub.State == domain.SubmissionRejected || sub.State == domain.SubmissionWithdrawn {
			return httperror.Conflict("this submission is no longer active")
		}
		for _, p := range sub.People {
			if p.UserID != nil && *p.UserID == u.ID {
				return httperror.Conflict("you are already on this submission")
			}
		}
		if _, err := tx.Exec(ctx, `UPDATE submission_people
			SET user_id=$2, status='accepted', invite_code=NULL, responded_at=now() WHERE id=$1`,
			personID, u.ID); err != nil {
			return mapPeopleErr(err)
		}
		after, err := loadSubmission(ctx, tx, sub.ID, false)
		if err != nil {
			return err
		}
		if err := audit.Record(ctx, tx, after.OrganizationID, &after.EditionID, "submission.invite_accept",
			string(domain.ResourceSubmission), after.ID, nil, nil, ""); err != nil {
			return err
		}
		out = after
		return nil
	})
	if out != nil {
		RedactFor(u, out)
	}
	return out, wrapTxErr(err)
}

// DeclineInvite removes the invite so the link stops working.
func (s *SubmissionService) DeclineInvite(ctx context.Context, code string) error {
	if _, err := auth.MustFromContext(ctx); err != nil {
		return httperror.New(httperror.CodeUnauthorized, "not authenticated")
	}
	return wrapTxErr(db.WithTx(ctx, s.pool, func(tx pgx.Tx) error {
		personID, sub, err := s.findInvite(ctx, tx, code, true)
		if err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `DELETE FROM submission_people WHERE id=$1`, personID); err != nil {
			return httperror.Internal(err)
		}
		return audit.Record(ctx, tx, sub.OrganizationID, &sub.EditionID, "submission.invite_decline",
			string(domain.ResourceSubmission), sub.ID, nil, nil, "")
	}))
}

// Leave removes the caller from a submission they co-present.
func (s *SubmissionService) Leave(ctx context.Context, submissionID uuid.UUID) error {
	u, err := auth.MustFromContext(ctx)
	if err != nil {
		return httperror.New(httperror.CodeUnauthorized, "not authenticated")
	}
	return wrapTxErr(db.WithTx(ctx, s.pool, func(tx pgx.Tx) error {
		sub, err := loadVisible(ctx, tx, u, submissionID, true)
		if err != nil {
			return err
		}
		if sub.SubmitterUserID == u.ID {
			return httperror.Conflict("the owner can't leave; withdraw the submission instead")
		}
		tag, err := tx.Exec(ctx, `DELETE FROM submission_people WHERE submission_id=$1 AND user_id=$2 AND role='presenter'`,
			submissionID, u.ID)
		if err != nil {
			return httperror.Internal(err)
		}
		if tag.RowsAffected() == 0 {
			return httperror.NotFound("submission")
		}
		return audit.Record(ctx, tx, sub.OrganizationID, &sub.EditionID, "submission.person_leave",
			string(domain.ResourceSubmission), sub.ID, nil, nil, "")
	}))
}
