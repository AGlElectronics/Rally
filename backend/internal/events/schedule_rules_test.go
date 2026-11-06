package events

import (
	"testing"
	"time"

	"github.com/google/uuid"
)

func TestCheckConflicts(t *testing.T) {
	base := time.Date(2026, 6, 6, 10, 0, 0, 0, time.UTC)
	at := func(min int) time.Time { return base.Add(time.Duration(min) * time.Minute) }
	stage, side := uuid.New(), uuid.New()
	show, panel := uuid.New(), uuid.New()
	ada := rulePerson{UserID: uuid.New(), Name: "Ada"}
	w := ruleWorld{
		Rules: DefaultRules(),
		Types: map[uuid.UUID]ruleType{
			show:  {Name: "Stage show", Gap: 30 * time.Minute, RoomIDs: []uuid.UUID{stage}},
			panel: {Name: "Panel"},
		},
		Rooms: map[uuid.UUID]string{stage: "Main Stage", side: "Room B"},
	}
	item := func(room uuid.UUID, typ uuid.UUID, from, to int, people ...rulePerson) ruleItem {
		return ruleItem{ID: uuid.New(), Title: "x", RoomID: room, EventTypeID: &typ, Start: at(from), End: at(to), People: people}
	}
	rules := func(cs []Conflict) map[string]int {
		m := map[string]int{}
		for _, c := range cs {
			m[c.Rule]++
		}
		return m
	}

	t.Run("overlap in one room", func(t *testing.T) {
		got := rules(checkConflicts([]ruleItem{item(side, panel, 0, 60), item(side, panel, 30, 90)}, w, nil))
		if got[RuleNoOverlap] != 1 || len(got) != 1 {
			t.Fatalf("got %v", got)
		}
	})
	t.Run("back to back without gap is fine", func(t *testing.T) {
		if cs := checkConflicts([]ruleItem{item(side, panel, 0, 60), item(side, panel, 60, 120)}, w, nil); len(cs) != 0 {
			t.Fatalf("got %v", cs)
		}
	})
	t.Run("changeover gap after a show", func(t *testing.T) {
		got := rules(checkConflicts([]ruleItem{item(stage, show, 0, 60), item(stage, panel, 75, 120)}, w, nil))
		if got[RuleGap] != 1 {
			t.Fatalf("got %v", got)
		}
		if cs := checkConflicts([]ruleItem{item(stage, show, 0, 60), item(stage, panel, 90, 120)}, w, nil); len(cs) != 0 {
			t.Fatalf("gap respected but got %v", cs)
		}
	})
	t.Run("same person in two rooms", func(t *testing.T) {
		cs := checkConflicts([]ruleItem{item(side, panel, 0, 60, ada), item(stage, panel, 30, 90, ada)}, w, nil)
		if len(cs) != 1 || cs[0].Rule != RulePerson || cs[0].Severity != RuleWarn {
			t.Fatalf("got %v", cs)
		}
	})
	t.Run("room fit", func(t *testing.T) {
		got := rules(checkConflicts([]ruleItem{item(side, show, 0, 60)}, w, nil))
		if got[RuleRoomFit] != 1 {
			t.Fatalf("got %v", got)
		}
	})
	t.Run("cancelled items and off rules are ignored", func(t *testing.T) {
		a, b := item(side, panel, 0, 60), item(side, panel, 30, 90)
		b.Cancelled = true
		if cs := checkConflicts([]ruleItem{a, b}, w, nil); len(cs) != 0 {
			t.Fatalf("got %v", cs)
		}
		off := w
		off.Rules = map[string]RuleMode{RuleNoOverlap: RuleOff}
		if cs := checkConflicts([]ruleItem{item(side, panel, 0, 60), item(side, panel, 30, 90)}, off, nil); len(cs) != 0 {
			t.Fatalf("got %v", cs)
		}
	})
	t.Run("focus limits results", func(t *testing.T) {
		a, b, c := item(side, panel, 0, 60), item(side, panel, 30, 90), item(stage, panel, 0, 60)
		if cs := checkConflicts([]ruleItem{a, b, c}, w, map[uuid.UUID]bool{c.ID: true}); len(cs) != 0 {
			t.Fatalf("got %v", cs)
		}
	})
}
