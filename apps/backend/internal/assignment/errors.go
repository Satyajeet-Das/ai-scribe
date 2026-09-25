package assignment

import "errors"

var (
	ErrAssignmentNotFound       = errors.New("assignment not found")
	ErrExamNotPublished         = errors.New("cannot assign exam: only published exams can be assigned to students")
	ErrDuplicateAssignment      = errors.New("student already has an active assignment for this exam")
	ErrAssignmentRevoked        = errors.New("assignment has been revoked")
	ErrAssignmentAlreadyRevoked = errors.New("assignment is already revoked")
	ErrInvalidAssignmentState   = errors.New("invalid assignment state transition")
	ErrUnauthorized             = errors.New("unauthorized to manage this assignment")
)
