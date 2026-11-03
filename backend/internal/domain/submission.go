package domain

import (
	"errors"
	"fmt"
)

// SubmissionState is the lifecycle state of a CfP submission.
//
//	draft ──submit──▶ submitted ──accept──▶ accepted
//	                      │                    │
//	                      ├──reject──▶ rejected│
//	                      └──withdraw─▶ withdrawn ◀──withdraw──┘
//
// rejected and withdrawn are terminal.
type SubmissionState string

const (
	SubmissionDraft     SubmissionState = "draft"
	SubmissionSubmitted SubmissionState = "submitted"
	SubmissionAccepted  SubmissionState = "accepted"
	SubmissionRejected  SubmissionState = "rejected"
	SubmissionWithdrawn SubmissionState = "withdrawn"
)

// SubmissionAction is an event that may move a submission between states.
type SubmissionAction string

const (
	ActionSubmit   SubmissionAction = "submit"
	ActionAccept   SubmissionAction = "accept"
	ActionReject   SubmissionAction = "reject"
	ActionWithdraw SubmissionAction = "withdraw"
)

// ErrInvalidTransition is returned (wrapped) when an action is not allowed
// from the current state. Callers map it to a 409 Conflict.
var ErrInvalidTransition = errors.New("invalid submission transition")

// submissionTransitions is the single source of truth for the state machine.
var submissionTransitions = map[SubmissionState]map[SubmissionAction]SubmissionState{
	SubmissionDraft: {
		ActionSubmit: SubmissionSubmitted,
	},
	SubmissionSubmitted: {
		ActionAccept:   SubmissionAccepted,
		ActionReject:   SubmissionRejected,
		ActionWithdraw: SubmissionWithdrawn,
	},
	SubmissionAccepted: {
		// A submitter may pull out after acceptance.
		ActionWithdraw: SubmissionWithdrawn,
	},
}

// AllSubmissionStates lists every valid state (used for validation and
// query filters).
func AllSubmissionStates() []SubmissionState {
	return []SubmissionState{
		SubmissionDraft, SubmissionSubmitted, SubmissionAccepted,
		SubmissionRejected, SubmissionWithdrawn,
	}
}

// Valid reports whether s is a known state.
func (s SubmissionState) Valid() bool {
	for _, v := range AllSubmissionStates() {
		if v == s {
			return true
		}
	}
	return false
}

// IsTerminal reports whether no further transitions are possible.
func (s SubmissionState) IsTerminal() bool {
	return len(submissionTransitions[s]) == 0
}

// Transition returns the state reached by applying action to from, or an
// error wrapping ErrInvalidTransition when the move is not permitted.
func Transition(from SubmissionState, action SubmissionAction) (SubmissionState, error) {
	if !from.Valid() {
		return "", fmt.Errorf("%w: unknown state %q", ErrInvalidTransition, from)
	}
	to, ok := submissionTransitions[from][action]
	if !ok {
		return "", fmt.Errorf("%w: cannot %s a %s submission", ErrInvalidTransition, action, from)
	}
	return to, nil
}
