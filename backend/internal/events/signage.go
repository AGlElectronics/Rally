package events

import (
	"context"
	"sort"
	"strconv"
	"time"

	"github.com/google/uuid"

	"github.com/AGlElectronics/rally/backend/internal/httperror"
)

// SignageRoom is one room's now/next view for screens.
type SignageRoom struct {
	ID       uuid.UUID    `json:"id"`
	Name     string       `json:"name"`
	Capacity *int         `json:"capacity"`
	Current  *PublicItem  `json:"current"`
	Next     []PublicItem `json:"next"`
}

// SignageSchedule is the published programme shaped for digital signage.
type SignageSchedule struct {
	Edition struct {
		ID       uuid.UUID `json:"id"`
		Slug     string    `json:"slug"`
		Title    string    `json:"title"`
		Timezone string    `json:"timezone"`
	} `json:"edition"`
	Version     int           `json:"version"`
	PublishedAt time.Time     `json:"published_at"`
	AsOf        time.Time     `json:"as_of"`
	Rooms       []SignageRoom `json:"rooms"`
}

// SignageFromPublic builds a now/next view from a public schedule.
// nextN is how many upcoming items per room (default 3, max 10).
func SignageFromPublic(p *PublicSchedule, at time.Time, nextN int) *SignageSchedule {
	if nextN <= 0 {
		nextN = 3
	}
	if nextN > 10 {
		nextN = 10
	}
	out := &SignageSchedule{
		Version:     p.Version,
		PublishedAt: p.PublishedAt,
		AsOf:        at.UTC(),
		Rooms:       make([]SignageRoom, 0, len(p.Rooms)),
	}
	out.Edition = p.Edition

	byRoom := map[uuid.UUID][]PublicItem{}
	for _, it := range p.Items {
		byRoom[it.RoomID] = append(byRoom[it.RoomID], it)
	}
	for _, room := range p.Rooms {
		items := byRoom[room.ID]
		sort.Slice(items, func(i, j int) bool {
			if !items[i].StartsAt.Equal(items[j].StartsAt) {
				return items[i].StartsAt.Before(items[j].StartsAt)
			}
			return items[i].Title < items[j].Title
		})
		sr := SignageRoom{ID: room.ID, Name: room.Name, Capacity: room.Capacity, Next: []PublicItem{}}
		for i := range items {
			it := items[i]
			if !it.StartsAt.After(at) && at.Before(it.EndsAt) {
				cp := it
				sr.Current = &cp
				break
			}
		}
		for i := range items {
			it := items[i]
			if !it.StartsAt.After(at) {
				continue
			}
			if sr.Current != nil && it.ID == sr.Current.ID {
				continue
			}
			sr.Next = append(sr.Next, it)
			if len(sr.Next) >= nextN {
				break
			}
		}
		out.Rooms = append(out.Rooms, sr)
	}
	return out
}

// Signage returns now/next for every room of a published edition.
// ref is an edition id or unique published slug. at nil means time.Now().
func (s *ScheduleService) Signage(ctx context.Context, ref string, at *time.Time, nextN int) (*SignageSchedule, error) {
	p, err := s.Public(ctx, ref)
	if err != nil {
		return nil, err
	}
	when := s.now()
	if at != nil {
		when = *at
	}
	return SignageFromPublic(p, when, nextN), nil
}

// SignageRoomView returns now/next for one room.
func (s *ScheduleService) SignageRoomView(ctx context.Context, ref, roomRef string, at *time.Time, nextN int) (*SignageRoom, error) {
	full, err := s.Signage(ctx, ref, at, nextN)
	if err != nil {
		return nil, err
	}
	roomID, err := uuid.Parse(roomRef)
	if err != nil {
		for i := range full.Rooms {
			if full.Rooms[i].Name == roomRef {
				return &full.Rooms[i], nil
			}
		}
		return nil, httperror.NotFound("room")
	}
	for i := range full.Rooms {
		if full.Rooms[i].ID == roomID {
			return &full.Rooms[i], nil
		}
	}
	return nil, httperror.NotFound("room")
}

// ParseSignageNext reads ?next= (optional).
func ParseSignageNext(raw string) (int, error) {
	if raw == "" {
		return 3, nil
	}
	n, err := strconv.Atoi(raw)
	if err != nil || n < 1 || n > 10 {
		return 0, httperror.Validation("validation failed", map[string]string{"next": "must be an integer from 1 to 10"})
	}
	return n, nil
}

// ParseSignageAt reads ?at= RFC3339 (optional).
func ParseSignageAt(raw string) (*time.Time, error) {
	if raw == "" {
		return nil, nil
	}
	t, err := time.Parse(time.RFC3339, raw)
	if err != nil {
		return nil, httperror.Validation("validation failed", map[string]string{"at": "must be an RFC3339 timestamp"})
	}
	return &t, nil
}
