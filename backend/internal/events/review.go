package events

import (
	"context"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/AGlElectronics/rally/backend/internal/audit"
	"github.com/AGlElectronics/rally/backend/internal/auth"
	"github.com/AGlElectronics/rally/backend/internal/db"
	"github.com/AGlElectronics/rally/backend/internal/domain"
	"github.com/AGlElectronics/rally/backend/internal/httperror"
)

// Note is an internal review comment; never shown to submitters.
type Note struct {
	ID         uuid.UUID  `json:"id"`
	AuthorID   *uuid.UUID `json:"author_id"`
	AuthorName string     `json:"author_name"`
	Body       string     `json:"body"`
	CreatedAt  time.Time  `json:"created_at"`
}

// Score is one reviewer's 1-5 score.
type Score struct {
	ReviewerID   uuid.UUID `json:"reviewer_id"`
	ReviewerName string    `json:"reviewer_name"`
	Score        int       `json:"score"`
	Note         string    `json:"note"`
	UpdatedAt    time.Time `json:"updated_at"`
}

// ScoreSummary is the scores of a submission plus the caller's own.
type ScoreSummary struct {
	Mode    string   `json:"review_mode"`
	Average *float64 `json:"average"`
	Scores  []Score  `json:"scores"`
	Mine    *Score   `json:"mine"`
}

// loadForReview loads a submission for a reviewer of its edition.
func loadForReview(ctx context.Context, q querier, id uuid.UUID) (*Submission, error) {
	u, err := auth.MustFromContext(ctx)
	if err != nil {
		return nil, httperror.New(httperror.CodeUnauthorized, "not authenticated")
	}
	sub, err := loadSubmission(ctx, q, id, false)
	if err != nil {
		return nil, err
	}
	if !u.HasForEdition(domain.PermSubmissionReview, sub.OrganizationID, sub.EditionID) {
		return nil, httperror.NotFound("submission")
	}
	return sub, nil
}

// Notes lists the internal notes on a submission (reviewers only).
func (s *SubmissionService) Notes(ctx context.Context, id uuid.UUID) ([]Note, error) {
	if _, err := loadForReview(ctx, s.pool, id); err != nil {
		return nil, err
	}
	rows, err := s.pool.Query(ctx, `SELECT n.id, n.author_user_id, COALESCE(u.display_name, 'Former member'), n.body, n.created_at
		FROM submission_notes n LEFT JOIN users u ON u.id = n.author_user_id
		WHERE n.submission_id=$1 ORDER BY n.created_at, n.id`, id)
	if err != nil {
		return nil, httperror.Internal(err)
	}
	defer rows.Close()
	out := []Note{}
	for rows.Next() {
		var n Note
		if err := rows.Scan(&n.ID, &n.AuthorID, &n.AuthorName, &n.Body, &n.CreatedAt); err != nil {
			return nil, httperror.Internal(err)
		}
		out = append(out, n)
	}
	return out, rows.Err()
}

// AddNote appends an internal note (reviewers only).
func (s *SubmissionService) AddNote(ctx context.Context, id uuid.UUID, body string) (*Note, error) {
	body = strings.TrimSpace(body)
	if body == "" || len([]rune(body)) > maxNoteLen {
		return nil, httperror.Validation("invalid note", map[string]string{"body": "must be 1 to 5000 characters"})
	}
	u, _ := auth.FromContext(ctx)
	var out Note
	err := db.WithTx(ctx, s.pool, func(tx pgx.Tx) error {
		sub, err := loadForReview(ctx, tx, id)
		if err != nil {
			return err
		}
		if err := tx.QueryRow(ctx, `INSERT INTO submission_notes (submission_id, author_user_id, body)
			VALUES ($1,$2,$3) RETURNING id, created_at`, id, u.ID, body).Scan(&out.ID, &out.CreatedAt); err != nil {
			return httperror.Internal(err)
		}
		out.AuthorID, out.AuthorName, out.Body = &u.ID, u.DisplayName, body
		return audit.Record(ctx, tx, sub.OrganizationID, &sub.EditionID, "submission.note",
			string(domain.ResourceSubmission), sub.ID, nil, nil, "")
	})
	if err != nil {
		return nil, wrapTxErr(err)
	}
	return &out, nil
}

// Scores returns every reviewer score and the caller's own.
func (s *SubmissionService) Scores(ctx context.Context, id uuid.UUID) (*ScoreSummary, error) {
	sub, err := loadForReview(ctx, s.pool, id)
	if err != nil {
		return nil, err
	}
	u, _ := auth.FromContext(ctx)
	cfp, err := loadCfP(ctx, s.pool, sub.CfPID, false)
	if err != nil {
		return nil, err
	}
	rows, err := s.pool.Query(ctx, `SELECT sc.reviewer_user_id, u.display_name, sc.score, sc.note, sc.updated_at
		FROM submission_scores sc JOIN users u ON u.id = sc.reviewer_user_id
		WHERE sc.submission_id=$1 ORDER BY sc.updated_at`, id)
	if err != nil {
		return nil, httperror.Internal(err)
	}
	defer rows.Close()
	out := &ScoreSummary{Mode: cfp.ReviewMode, Scores: []Score{}}
	total := 0
	for rows.Next() {
		var sc Score
		if err := rows.Scan(&sc.ReviewerID, &sc.ReviewerName, &sc.Score, &sc.Note, &sc.UpdatedAt); err != nil {
			return nil, httperror.Internal(err)
		}
		total += sc.Score
		out.Scores = append(out.Scores, sc)
		if sc.ReviewerID == u.ID {
			mine := sc
			out.Mine = &mine
		}
	}
	if n := len(out.Scores); n > 0 {
		avg := float64(total) / float64(n)
		out.Average = &avg
	}
	return out, rows.Err()
}

// ScoreInput is a reviewer's score.
type ScoreInput struct {
	Score int    `json:"score"`
	Note  string `json:"note"`
}

// SetScore records or replaces the caller's score. Only in "scored" review
// mode, only for submitted submissions, never on your own.
func (s *SubmissionService) SetScore(ctx context.Context, id uuid.UUID, in ScoreInput) (*ScoreSummary, error) {
	in.Note = strings.TrimSpace(in.Note)
	issues := map[string]string{}
	if in.Score < 1 || in.Score > 5 {
		issues["score"] = "must be 1 to 5"
	}
	if len([]rune(in.Note)) > maxNoteLen {
		issues["note"] = "must be at most 5000 characters"
	}
	if err := validationIfAny("invalid score", issues); err != nil {
		return nil, err
	}
	u, _ := auth.FromContext(ctx)
	err := db.WithTx(ctx, s.pool, func(tx pgx.Tx) error {
		sub, err := loadForReview(ctx, tx, id)
		if err != nil {
			return err
		}
		cfp, err := loadCfP(ctx, tx, sub.CfPID, false)
		if err != nil {
			return err
		}
		if cfp.ReviewMode != ReviewScored {
			return httperror.Conflict("this call doesn't use scores; leave a note instead")
		}
		if sub.SubmitterUserID == u.ID {
			return httperror.New(httperror.CodeForbidden, "you cannot score your own submission")
		}
		if sub.State != domain.SubmissionSubmitted {
			return httperror.Conflict("only submissions waiting for a decision can be scored")
		}
		if _, err := tx.Exec(ctx, `INSERT INTO submission_scores (submission_id, reviewer_user_id, score, note)
			VALUES ($1,$2,$3,$4)
			ON CONFLICT (submission_id, reviewer_user_id) DO UPDATE SET score=EXCLUDED.score, note=EXCLUDED.note, updated_at=now()`,
			id, u.ID, in.Score, in.Note); err != nil {
			return httperror.Internal(err)
		}
		return audit.Record(ctx, tx, sub.OrganizationID, &sub.EditionID, "submission.score",
			string(domain.ResourceSubmission), sub.ID, nil, map[string]any{"score": in.Score}, "")
	})
	if err != nil {
		return nil, wrapTxErr(err)
	}
	return s.Scores(ctx, id)
}
