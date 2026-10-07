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
	ErrStudentNotFound          = errors.New("student not found")
	ErrStudentIneligible        = errors.New("student is not eligible for assignment")
	ErrExamArchived             = errors.New("cannot assign exam: archived exams cannot be assigned")
	ErrNoStudentsProvided       = errors.New("at least one student ID or roll number must be provided")
	ErrBatchTooLarge            = errors.New("batch size exceeds maximum allowed limit of 500")
	ErrMismatchedStudent        = errors.New("studentId and rollNo refer to different students")
)
