package events

import (
	"testing"
	"time"

	"github.com/google/uuid"
)

func TestSignageFromPublic(t *testing.T) {
	roomA := uuid.MustParse("11111111-1111-1111-1111-111111111111")
	roomB := uuid.MustParse("22222222-2222-2222-2222-222222222222")
	id1 := uuid.MustParse("aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa")
	id2 := uuid.MustParse("bbbbbbbb-bbbb-bbbb-bbbb-bbbbbbbbbbbb")
	id3 := uuid.MustParse("cccccccc-cccc-cccc-cccc-cccccccccccc")

	at := time.Date(2026, 11, 17, 12, 0, 0, 0, time.UTC)
	p := &PublicSchedule{
		Version: 2,
		Rooms: []SnapshotRoom{
			{ID: roomA, Name: "Main"},
			{ID: roomB, Name: "Side"},
		},
		Items: []PublicItem{
			{ID: id1, Title: "Past", RoomID: roomA, Room: "Main", StartsAt: at.Add(-2 * time.Hour), EndsAt: at.Add(-time.Hour), Status: "scheduled"},
			{ID: id2, Title: "Now", RoomID: roomA, Room: "Main", StartsAt: at.Add(-30 * time.Minute), EndsAt: at.Add(30 * time.Minute), Status: "scheduled"},
			{ID: id3, Title: "Next", RoomID: roomA, Room: "Main", StartsAt: at.Add(time.Hour), EndsAt: at.Add(2 * time.Hour), Status: "scheduled"},
		},
	}
	p.Edition.ID = uuid.MustParse("33333333-3333-3333-3333-333333333333")
	p.Edition.Slug = "con-2026"
	p.Edition.Title = "Con 2026"
	p.Edition.Timezone = "UTC"

	got := SignageFromPublic(p, at, 2)
	if got.Version != 2 || len(got.Rooms) != 2 {
		t.Fatalf("got %+v", got)
	}
	main := got.Rooms[0]
	if main.Name != "Main" || main.Current == nil || main.Current.Title != "Now" {
		t.Fatalf("main current: %+v", main)
	}
	if len(main.Next) != 1 || main.Next[0].Title != "Next" {
		t.Fatalf("main next: %+v", main.Next)
	}
	side := got.Rooms[1]
	if side.Current != nil || len(side.Next) != 0 {
		t.Fatalf("empty room: %+v", side)
	}
}

func TestParseSignageNextAndAt(t *testing.T) {
	n, err := ParseSignageNext("")
	if err != nil || n != 3 {
		t.Fatalf("default next: %d %v", n, err)
	}
	if _, err := ParseSignageNext("0"); err == nil {
		t.Fatal("expected validation error")
	}
	if _, err := ParseSignageAt("not-a-time"); err == nil {
		t.Fatal("expected at validation error")
	}
	ts, err := ParseSignageAt("2026-11-17T12:00:00Z")
	if err != nil || ts == nil || !ts.Equal(time.Date(2026, 11, 17, 12, 0, 0, 0, time.UTC)) {
		t.Fatalf("at: %v %v", ts, err)
	}
}
