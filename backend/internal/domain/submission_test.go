package domain

import (
	"errors"
	"testing"
)

func TestTransition(t *testing.T) {
	tests := []struct {
		name    string
		from    SubmissionState
		action  SubmissionAction
		want    SubmissionState
		wantErr bool
	}{
		{"draft submit", SubmissionDraft, ActionSubmit, SubmissionSubmitted, false},
		{"submitted accept", SubmissionSubmitted, ActionAccept, SubmissionAccepted, false},
		{"submitted reject", SubmissionSubmitted, ActionReject, SubmissionRejected, false},
		{"submitted withdraw", SubmissionSubmitted, ActionWithdraw, SubmissionWithdrawn, false},
		{"accepted withdraw", SubmissionAccepted, ActionWithdraw, SubmissionWithdrawn, false},

		{"draft accept", SubmissionDraft, ActionAccept, "", true},
		{"draft reject", SubmissionDraft, ActionReject, "", true},
		{"draft withdraw", SubmissionDraft, ActionWithdraw, "", true},
		{"submitted submit", SubmissionSubmitted, ActionSubmit, "", true},
		{"accepted reject", SubmissionAccepted, ActionReject, "", true},
		{"accepted accept", SubmissionAccepted, ActionAccept, "", true},
		{"rejected accept", SubmissionRejected, ActionAccept, "", true},
		{"rejected withdraw", SubmissionRejected, ActionWithdraw, "", true},
		{"withdrawn submit", SubmissionWithdrawn, ActionSubmit, "", true},
		{"withdrawn accept", SubmissionWithdrawn, ActionAccept, "", true},
		{"unknown state", SubmissionState("bogus"), ActionSubmit, "", true},
		{"unknown action", SubmissionDraft, SubmissionAction("bogus"), "", true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := Transition(tc.from, tc.action)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("expected error, got state %q", got)
				}
				if !errors.Is(err, ErrInvalidTransition) {
					t.Fatalf("error %v does not wrap ErrInvalidTransition", err)
				}
				if got != "" {
					t.Fatalf("expected empty state on error, got %q", got)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != tc.want {
				t.Fatalf("got %q, want %q", got, tc.want)
			}
		})
	}
}

func TestTransitionExhaustive(t *testing.T) {
	actions := []SubmissionAction{ActionSubmit, ActionAccept, ActionReject, ActionWithdraw}
	legal := 0
	for _, from := range AllSubmissionStates() {
		for _, a := range actions {
			to, err := Transition(from, a)
			if err != nil {
				continue
			}
			legal++
			if !to.Valid() {
				t.Errorf("%s --%s--> %q is not a valid state", from, a, to)
			}
			if from.IsTerminal() {
				t.Errorf("terminal state %s allowed %s", from, a)
			}
		}
	}
	if legal != 5 {
		t.Fatalf("expected exactly 5 legal transitions, got %d", legal)
	}
}

func TestTerminalStates(t *testing.T) {
	for _, s := range []SubmissionState{SubmissionRejected, SubmissionWithdrawn} {
		if !s.IsTerminal() {
			t.Errorf("%s should be terminal", s)
		}
	}
	for _, s := range []SubmissionState{SubmissionDraft, SubmissionSubmitted, SubmissionAccepted} {
		if s.IsTerminal() {
			t.Errorf("%s should not be terminal", s)
		}
	}
}
