package events

import (
	"context"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/AGlElectronics/rally/backend/internal/httperror"
)

// PublicEventType is the attendee-facing part of an event type.
type PublicEventType struct {
	ID             uuid.UUID `json:"id"`
	Name           string    `json:"name"`
	Color          string    `json:"color"`
	DefaultMinutes int       `json:"default_minutes"`
}

// PublicCall is an open call for papers as the submission site shows it.
type PublicCall struct {
	ID              uuid.UUID         `json:"id"`
	EditionID       uuid.UUID         `json:"edition_id"`
	EditionTitle    string            `json:"edition_title"`
	EditionTimezone string            `json:"edition_timezone"`
	Title           string            `json:"title"`
	Description     string            `json:"description"`
	OpensAt         *time.Time        `json:"opens_at"`
	ClosesAt        *time.Time        `json:"closes_at"`
	AcceptingNow    bool              `json:"accepting_now"`
	Form            Form              `json:"form"`
	EventTypes      []PublicEventType `json:"event_types"`
	StatusLabels    map[string]string `json:"status_labels"`
}

// PublicService serves the unauthenticated parts of the submission site.
type PublicService struct {
	pool *pgxpool.Pool
	now  func() time.Time
}

// NewPublicService returns a service bound to pool.
func NewPublicService(pool *pgxpool.Pool) *PublicService {
	return &PublicService{pool: pool, now: time.Now}
}

// OpenCalls lists every call with status open whose window hasn't ended.
func (s *PublicService) OpenCalls(ctx context.Context) ([]PublicCall, error) {
	rows, err := s.pool.Query(ctx, `SELECT id FROM call_for_papers
		WHERE status='open' AND (closes_at IS NULL OR closes_at > now())
		ORDER BY closes_at NULLS LAST, created_at`)
	if err != nil {
		return nil, httperror.Internal(err)
	}
	var ids []uuid.UUID
	for rows.Next() {
		var id uuid.UUID
		if err := rows.Scan(&id); err != nil {
			rows.Close()
			return nil, httperror.Internal(err)
		}
		ids = append(ids, id)
	}
	rows.Close()
	out := make([]PublicCall, 0, len(ids))
	for _, id := range ids {
		c, err := s.Call(ctx, id)
		if err != nil {
			return nil, err
		}
		out = append(out, *c)
	}
	return out, nil
}

// Call returns one published call (404 for drafts). Closed calls stay
// readable so submitters can still see their answers in context.
func (s *PublicService) Call(ctx context.Context, id uuid.UUID) (*PublicCall, error) {
	c, err := loadCfP(ctx, s.pool, id, false)
	if err != nil {
		return nil, err
	}
	if c.Status == CfPDraft {
		return nil, httperror.NotFound("call for papers")
	}
	out := &PublicCall{
		ID: c.ID, EditionID: c.EditionID, Title: c.Title, Description: c.Description,
		OpensAt: c.OpensAt, ClosesAt: c.ClosesAt, AcceptingNow: c.AcceptsSubmissions(s.now()),
		StatusLabels: c.StatusLabels, EventTypes: []PublicEventType{},
	}
	for _, fd := range c.Form.Fields {
		if !fd.Archived {
			out.Form.Fields = append(out.Form.Fields, fd)
		}
	}
	if out.Form.Fields == nil {
		out.Form.Fields = []FormField{}
	}
	if err := s.pool.QueryRow(ctx, `SELECT title, timezone FROM editions WHERE id=$1`, c.EditionID).
		Scan(&out.EditionTitle, &out.EditionTimezone); err != nil {
		return nil, httperror.Internal(err)
	}
	types, err := allowedEventTypes(ctx, s.pool, c)
	if err != nil {
		return nil, err
	}
	for _, t := range types {
		out.EventTypes = append(out.EventTypes, PublicEventType{ID: t.ID, Name: t.Name, Color: t.Color, DefaultMinutes: t.DefaultMinutes})
	}
	return out, nil
}
