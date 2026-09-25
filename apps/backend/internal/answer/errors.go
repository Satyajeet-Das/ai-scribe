package answer

import "errors"

var (
	ErrAnswerNotFound        = errors.New("answer not found")
	ErrSessionNotActive      = errors.New("cannot submit answer: session is not in progress or has expired")
	ErrQuestionNotForExam    = errors.New("question does not belong to this session's exam")
	ErrOptionNotForQuestion  = errors.New("selected option does not belong to the specified question")
	ErrUnauthorizedStudent   = errors.New("unauthorized: student does not own this session")
	ErrInvalidAnswer         = errors.New("invalid answer: either selected option or text answer must be provided")
)
