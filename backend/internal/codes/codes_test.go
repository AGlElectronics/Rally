package codes

import (
	"regexp"
	"testing"
)

func TestNewAndNormalize(t *testing.T) {
	shape := regexp.MustCompile(`^[2-9A-HJKMNP-TV-Z]{4}-[2-9A-HJKMNP-TV-Z]{4}$`)
	seen := map[string]struct{}{}
	for i := 0; i < 1000; i++ {
		c := New()
		if !shape.MatchString(c) {
			t.Fatalf("bad code %q", c)
		}
		seen[c] = struct{}{}
	}
	if len(seen) < 990 {
		t.Fatalf("too many collisions: %d unique of 1000", len(seen))
	}
	if got := Normalize(" k7qm 3xpa "); got != "K7QM-3XPA" {
		t.Fatalf("normalize: %q", got)
	}
}
