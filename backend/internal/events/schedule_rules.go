package events

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/google/uuid"
)

// RuleMode says what a broken rule does: refuse the change, flag it, or
// nothing.
type RuleMode string

const (
	RuleBlock RuleMode = "block"
	RuleWarn  RuleMode = "warn"
	RuleOff   RuleMode = "off"
)

// Rule names, also the keys of schedules.rules.
const (
	RuleNoOverlap = "no_overlap"
	RuleGap       = "gap"
	RulePerson    = "person"
	RuleRoomFit   = "room_fit"
)

// RuleNames lists every rule in display order.
var RuleNames = []string{RuleNoOverlap, RuleGap, RulePerson, RuleRoomFit}

// DefaultRules: a person in two places is often planned on purpose (a host
// who pops in), so it only warns by default.
func DefaultRules() map[string]RuleMode {
	return map[string]RuleMode{
		RuleNoOverlap: RuleBlock,
		RuleGap:       RuleBlock,
		RulePerson:    RuleWarn,
		RuleRoomFit:   RuleBlock,
	}
}

// Conflict is one broken rule between an item and (for pair rules) another.
type Conflict struct {
	Rule     string     `json:"rule"`
	Severity RuleMode   `json:"severity"`
	ItemID   uuid.UUID  `json:"item_id"`
	OtherID  *uuid.UUID `json:"other_id,omitempty"`
	Message  string     `json:"message"`
}

// ruleItem is what the checker needs to know about a scheduled item.
type ruleItem struct {
	ID          uuid.UUID
	Title       string
	RoomID      uuid.UUID
	EventTypeID *uuid.UUID
	Start, End  time.Time
	Cancelled   bool
	People      []rulePerson
}

type rulePerson struct {
	UserID uuid.UUID
	Name   string
}

type ruleType struct {
	Name    string
	Gap     time.Duration
	RoomIDs []uuid.UUID
}

type ruleWorld struct {
	Rules map[string]RuleMode
	Types map[uuid.UUID]ruleType
	Rooms map[uuid.UUID]string
}

func (w ruleWorld) mode(rule string) RuleMode {
	if m, ok := w.Rules[rule]; ok {
		return m
	}
	return DefaultRules()[rule]
}

func (w ruleWorld) gapAfter(it ruleItem) time.Duration {
	if it.EventTypeID == nil {
		return 0
	}
	return w.Types[*it.EventTypeID].Gap
}

// checkConflicts returns every broken rule among items. When focus is set,
// only conflicts involving those items are returned (used to judge a single
// drop). Cancelled items never conflict.
func checkConflicts(items []ruleItem, w ruleWorld, focus map[uuid.UUID]bool) []Conflict {
	out := []Conflict{}
	involves := func(ids ...uuid.UUID) bool {
		if focus == nil {
			return true
		}
		for _, id := range ids {
			if focus[id] {
				return true
			}
		}
		return false
	}
	add := func(rule string, a ruleItem, b *ruleItem, msg string) {
		sev := w.mode(rule)
		if sev == RuleOff {
			return
		}
		c := Conflict{Rule: rule, Severity: sev, ItemID: a.ID, Message: msg}
		if b != nil {
			id := b.ID
			c.OtherID = &id
		}
		out = append(out, c)
	}

	live := make([]ruleItem, 0, len(items))
	for _, it := range items {
		if !it.Cancelled {
			live = append(live, it)
		}
	}
	sort.Slice(live, func(i, j int) bool {
		if !live[i].Start.Equal(live[j].Start) {
			return live[i].Start.Before(live[j].Start)
		}
		return live[i].ID.String() < live[j].ID.String()
	})

	for _, it := range live {
		if !involves(it.ID) || it.EventTypeID == nil {
			continue
		}
		t, ok := w.Types[*it.EventTypeID]
		if !ok || len(t.RoomIDs) == 0 {
			continue
		}
		allowed := false
		for _, r := range t.RoomIDs {
			if r == it.RoomID {
				allowed = true
				break
			}
		}
		if !allowed {
			add(RuleRoomFit, it, nil, fmt.Sprintf("%s can't take place in %s (not a room for %s)",
				quote(it.Title), w.Rooms[it.RoomID], t.Name))
		}
	}

	for i := range live {
		a := live[i]
		for j := i + 1; j < len(live); j++ {
			b := live[j]
			// Sorted by start, so b never starts before a. Once b starts
			// after a's end plus a's gap, nothing later can clash with a.
			aEnd := a.End.Add(w.gapAfter(a))
			if !b.Start.Before(aEnd) {
				break
			}
			if !involves(a.ID, b.ID) {
				continue
			}
			overlap := b.Start.Before(a.End)

			if a.RoomID == b.RoomID {
				if overlap {
					add(RuleNoOverlap, b, &a, fmt.Sprintf("%s and %s overlap in %s",
						quote(a.Title), quote(b.Title), w.Rooms[a.RoomID]))
				} else {
					add(RuleGap, b, &a, fmt.Sprintf("%s needs %d min changeover in %s before %s starts",
						quote(a.Title), int(w.gapAfter(a)/time.Minute), w.Rooms[a.RoomID], quote(b.Title)))
				}
			}

			if shared := sharedPeople(a.People, b.People); len(shared) > 0 {
				add(RulePerson, b, &a, fmt.Sprintf("%s would be in %s and %s at the same time",
					strings.Join(shared, ", "), quote(a.Title), quote(b.Title)))
			}
		}
	}
	return out
}

func sharedPeople(a, b []rulePerson) []string {
	if len(a) == 0 || len(b) == 0 {
		return nil
	}
	in := map[uuid.UUID]bool{}
	for _, p := range a {
		in[p.UserID] = true
	}
	var names []string
	for _, p := range b {
		if in[p.UserID] {
			names = append(names, p.Name)
		}
	}
	return names
}

func quote(s string) string { return "\u201c" + s + "\u201d" }

// hasBlocking reports whether any conflict refuses the change.
func hasBlocking(cs []Conflict) bool {
	for _, c := range cs {
		if c.Severity == RuleBlock {
			return true
		}
	}
	return false
}
