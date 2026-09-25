package exam

import "errors"

var (
	ErrExamNotFound         = errors.New("exam not found")
	ErrExamAlreadyPublished = errors.New("exam is already published")
	ErrExamAlreadyArchived  = errors.New("exam is already archived")
	ErrExamNotDraft         = errors.New("cannot modify exam: only draft exams can be modified")
	ErrInvalidExamState     = errors.New("invalid exam state transition")
	ErrUnauthorizedCreator  = errors.New("unauthorized: only the exam creator can perform this action")
	ErrExamCannotPublish    = errors.New("exam cannot be published: must have valid duration and subject")
)
