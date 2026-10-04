package user

import "errors"

var (
	ErrUserNotFound        = errors.New("user not found")
	ErrUserAlreadyExists   = errors.New("user with this email already exists")
	ErrRollNoAlreadyExists = errors.New("student with this roll number already exists")
	ErrRollNoRequired      = errors.New("roll number is required for students")
	ErrStudentNotFound     = errors.New("student not found")
	ErrStudentIneligible   = errors.New("student is not eligible for assignment")
)
