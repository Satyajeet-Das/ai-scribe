package session

import "errors"

var (
	ErrSessionNotFound           = errors.New("session not found")
	ErrSessionExpired            = errors.New("session time has expired")
	ErrSessionAlreadySubmitted   = errors.New("session has already been submitted")
	ErrSessionNotInProgress      = errors.New("session is not currently in progress")
	ErrInvalidAssignment         = errors.New("invalid assignment: assignment not found")
	ErrAssignmentRevoked         = errors.New("cannot start session: assignment has been revoked")
	ErrExamArchived              = errors.New("cannot start session: exam is archived")
	ErrExamNotPublished          = errors.New("cannot start session: exam is not published")
	ErrActiveSessionAlreadyExists = errors.New("an active session is already in progress for this assignment")
	ErrUnauthorizedStudent       = errors.New("unauthorized: student does not own this session or assignment")
)
