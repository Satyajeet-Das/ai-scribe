package exam

import "errors"

var (
	ErrExamNotFound              = errors.New("exam not found")
	ErrExamAlreadyPublished      = errors.New("exam is already published")
	ErrExamAlreadyArchived       = errors.New("exam is already archived")
	ErrExamNotDraft              = errors.New("cannot modify exam: only draft exams can be modified")
	ErrExamNotPublished          = errors.New("cannot unpublish exam: only published exams can be unpublished")
	ErrInvalidExamState          = errors.New("invalid exam state transition")
	ErrUnauthorizedCreator       = errors.New("unauthorized: only the exam creator or administrator can perform this action")
	ErrExamCannotPublish         = errors.New("exam cannot be published: must have valid duration, title, and subject")
	ErrPublishedStructuralChange = errors.New("unsafe modification: duration and structural parameters of a published exam cannot be altered")
	ErrCannotUnpublishActiveExam = errors.New("cannot unpublish exam: student assignments or exam sessions already exist")
	ErrCannotDeleteActiveExam   = errors.New("cannot delete exam: active assignments or sessions exist, please archive instead")
	ErrInvalidCallerRole         = errors.New("unauthorized: caller does not have permission to manage exams")
)
