package session

import (
	"errors"
	"fmt"
)

// Event represents an action that triggers a state transition in the session.
type Event string

const (
	// EventBegin starts the exam session.
	EventBegin Event = "BEGIN"
	// EventRecordAnswer records or updates a student's answer.
	EventRecordAnswer Event = "RECORD_ANSWER"
	// EventSubmit explicitly finishes the exam by the student.
	EventSubmit Event = "SUBMIT"
	// EventSystemExpire automatically finishes the exam due to time limits.
	EventSystemExpire Event = "SYSTEM_EXPIRE"
	// EventNavigateNext navigates to the next question.
	EventNavigateNext Event = "NAVIGATE_NEXT"
	// EventNavigatePrevious navigates to the previous question.
	EventNavigatePrevious Event = "NAVIGATE_PREVIOUS"
)

var (
	// ErrInvalidTransition is returned when an event is not allowed for the current state.
	ErrInvalidTransition = errors.New("invalid state transition")
)

// Transition applies the given event to the current state and returns the resulting state.
// If the transition is not allowed, it returns ErrInvalidTransition.
func Transition(current Status, event Event) (Status, error) {
	switch current {
	case StatusPending:
		if event == EventBegin {
			return StatusInProgress, nil
		}
	case StatusInProgress:
		switch event {
		case EventRecordAnswer, EventNavigateNext, EventNavigatePrevious:
			return StatusInProgress, nil
		case EventSubmit:
			return StatusSubmitted, nil
		case EventSystemExpire:
			return StatusExpired, nil
		}
	case StatusSubmitted, StatusExpired:
		// Terminal states cannot accept any further events
		if current == StatusSubmitted && event == EventSubmit {
			return current, ErrSessionAlreadySubmitted
		}
		if current == StatusExpired {
			return current, ErrSessionExpired
		}
		return current, fmt.Errorf("%w: cannot transition from terminal state %s via event %s", ErrInvalidTransition, current, event)
	}

	if current != StatusInProgress {
		return current, ErrSessionNotInProgress
	}

	return current, fmt.Errorf("%w: cannot transition from %s via event %s", ErrInvalidTransition, current, event)
}
