package question

import "errors"

var (
	ErrQuestionNotFound     = errors.New("question not found")
	ErrOptionNotFound       = errors.New("question option not found")
	ErrExamNotDraft         = errors.New("cannot modify questions: exam is already published or completed")
	ErrExamArchived         = errors.New("cannot add or modify questions: exam is archived")
	ErrInvalidQuestionType  = errors.New("invalid question type: must be MCQ, ESSAY, or VOICE")
	ErrOptionKeyDuplicate   = errors.New("option key already exists for this question")
	ErrOptionsOnlyForMCQ    = errors.New("options can only be added to multiple choice questions")
	ErrUnauthorizedCreator  = errors.New("unauthorized: only the exam creator can modify questions")
)
