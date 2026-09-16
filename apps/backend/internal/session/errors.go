package session

import "errors"

var (
	ErrSessionNotFound   = errors.New("session not found")
	ErrSessionExpired    = errors.New("session time has expired")
	ErrSessionTerminated = errors.New("session was terminated")
	ErrSessionNotInProg  = errors.New("session is not currently in progress")
)
