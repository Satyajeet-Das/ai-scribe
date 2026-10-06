package question

import "errors"

var (
	ErrQuestionNotFound              = errors.New("question not found")
	ErrOptionNotFound                = errors.New("question option not found")
	ErrExamNotDraft                  = errors.New("cannot modify questions: exam is already published or completed")
	ErrExamArchived                  = errors.New("cannot add or modify questions: exam is archived")
	ErrInvalidQuestionType           = errors.New("invalid question type: must be MCQ, ESSAY, or VOICE")
	ErrOptionKeyDuplicate            = errors.New("option key already exists for this question")
	ErrOptionsOnlyForMCQ             = errors.New("options can only be added to multiple choice questions")
	ErrUnauthorizedCreator           = errors.New("unauthorized: only the exam creator or admin can modify questions")
	ErrQuestionDoesNotBelongToExam   = errors.New("question does not belong to the specified exam")
	ErrOptionDoesNotBelongToQuestion = errors.New("option does not belong to the specified question")
	ErrInvalidQuestionOrder          = errors.New("invalid question ordering")
	ErrDuplicateQuestionNumbers      = errors.New("duplicate question numbers provided")
	ErrAtLeastOneCorrectOption       = errors.New("MCQ must have at least one correct option")
	ErrMultipleCorrectOptions        = errors.New("MCQ cannot have multiple correct options")
)
