package exam

import "errors"

var (
	ErrExamNotFound       = errors.New("exam not found")
	ErrExamAlreadyStarted = errors.New("exam has already started")
	ErrExamClosed         = errors.New("exam is closed")
	ErrInvalidExamState   = errors.New("invalid exam state transition")
)
