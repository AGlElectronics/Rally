// Package codes generates short human-friendly codes (invite and presenter
// codes) that people read out, type or paste.
package codes

import (
	"crypto/rand"
	"strings"
)

// alphabet has no 0/O, 1/I/L or U so codes survive being read aloud.
const alphabet = "23456789ABCDEFGHJKMNPQRSTVWXYZ"

// New returns a random code like "K7QM-3XPA" (8 symbols, ~39 bits).
func New() string {
	var b [8]byte
	if _, err := rand.Read(b[:]); err != nil {
		panic("codes: crypto/rand failed: " + err.Error())
	}
	var sb strings.Builder
	for i, v := range b {
		if i == 4 {
			sb.WriteByte('-')
		}
		sb.WriteByte(alphabet[int(v)%len(alphabet)])
	}
	return sb.String()
}

// Normalize uppercases, drops spaces and re-inserts the dash so "k7qm3xpa"
// and "K7QM-3XPA" match.
func Normalize(s string) string {
	s = strings.ToUpper(strings.NewReplacer(" ", "", "-", "").Replace(strings.TrimSpace(s)))
	if len(s) != 8 {
		return s
	}
	return s[:4] + "-" + s[4:]
}
