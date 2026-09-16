package answer

import "errors"

var (
	ErrAnswerNotFound  = errors.New("answer not found")
	ErrAlreadyAnswered = errors.New("question has already been answered")
)
