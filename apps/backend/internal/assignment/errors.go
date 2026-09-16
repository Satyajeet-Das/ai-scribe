package assignment

import "errors"

var (
	ErrAssignmentNotFound      = errors.New("assignment not found")
	ErrAssignmentAlreadyExists = errors.New("candidate already assigned to this exam")
)
