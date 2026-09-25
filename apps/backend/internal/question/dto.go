package question

import (
	"time"

	"github.com/go-playground/validator/v10"
	"github.com/google/uuid"
)

var validate = validator.New()

type CreateQuestionRequest struct {
	QuestionNumber int    `json:"questionNumber" validate:"required,min=1"`
	Text           string `json:"text" validate:"required,min=2"`
	Type           Type   `json:"type" validate:"required,oneof=MCQ ESSAY VOICE"`
	Points         int    `json:"points" validate:"min=0"`
}

func (r *CreateQuestionRequest) Validate() error {
	return validate.Struct(r)
}

type UpdateQuestionRequest struct {
	QuestionNumber *int    `json:"questionNumber,omitempty" validate:"omitempty,min=1"`
	Text           *string `json:"text,omitempty" validate:"omitempty,min=2"`
	Points         *int    `json:"points,omitempty" validate:"omitempty,min=0"`
}

func (r *UpdateQuestionRequest) Validate() error {
	return validate.Struct(r)
}

type CreateOptionRequest struct {
	OptionKey    string `json:"optionKey" validate:"required,min=1,max=10"`
	OptionText   string `json:"optionText" validate:"required,min=1"`
	DisplayOrder int    `json:"displayOrder" validate:"min=0"`
	IsCorrect    bool   `json:"isCorrect"`
}

func (r *CreateOptionRequest) Validate() error {
	return validate.Struct(r)
}

// Option DTOs
type TeacherQuestionOptionResponse struct {
	ID           uuid.UUID `json:"id"`
	QuestionID   uuid.UUID `json:"questionId"`
	OptionKey    string    `json:"optionKey"`
	OptionText   string    `json:"optionText"`
	DisplayOrder int       `json:"displayOrder"`
	IsCorrect    bool      `json:"isCorrect"`
}

type StudentQuestionOptionResponse struct {
	ID           uuid.UUID `json:"id"`
	QuestionID   uuid.UUID `json:"questionId"`
	OptionKey    string    `json:"optionKey"`
	OptionText   string    `json:"optionText"`
	DisplayOrder int       `json:"displayOrder"`
}

// Question DTOs
type TeacherQuestionResponse struct {
	ID             uuid.UUID                       `json:"id"`
	ExamID         uuid.UUID                       `json:"examId"`
	QuestionNumber int                             `json:"questionNumber"`
	Text           string                          `json:"text"`
	Type           Type                            `json:"type"`
	Points         int                             `json:"points"`
	Options        []TeacherQuestionOptionResponse `json:"options,omitempty"`
	CreatedAt      time.Time                       `json:"createdAt"`
	UpdatedAt      time.Time                       `json:"updatedAt"`
}

type StudentQuestionResponse struct {
	ID             uuid.UUID                       `json:"id"`
	ExamID         uuid.UUID                       `json:"examId"`
	QuestionNumber int                             `json:"questionNumber"`
	Text           string                          `json:"text"`
	Type           Type                            `json:"type"`
	Points         int                             `json:"points"`
	Options        []StudentQuestionOptionResponse `json:"options,omitempty"`
}

func ToTeacherOptionResponse(o *QuestionOption) TeacherQuestionOptionResponse {
	return TeacherQuestionOptionResponse{
		ID:           o.ID,
		QuestionID:   o.QuestionID,
		OptionKey:    o.OptionKey,
		OptionText:   o.OptionText,
		DisplayOrder: o.DisplayOrder,
		IsCorrect:    o.IsCorrect,
	}
}

func ToStudentOptionResponse(o *QuestionOption) StudentQuestionOptionResponse {
	return StudentQuestionOptionResponse{
		ID:           o.ID,
		QuestionID:   o.QuestionID,
		OptionKey:    o.OptionKey,
		OptionText:   o.OptionText,
		DisplayOrder: o.DisplayOrder,
	}
}

func ToTeacherQuestionResponse(q *Question) TeacherQuestionResponse {
	var opts []TeacherQuestionOptionResponse
	for _, o := range q.Options {
		opts = append(opts, ToTeacherOptionResponse(&o))
	}
	return TeacherQuestionResponse{
		ID:             q.ID,
		ExamID:         q.ExamID,
		QuestionNumber: q.QuestionNumber,
		Text:           q.Text,
		Type:           q.Type,
		Points:         q.Points,
		Options:        opts,
		CreatedAt:      q.CreatedAt,
		UpdatedAt:      q.UpdatedAt,
	}
}

func ToStudentQuestionResponse(q *Question) StudentQuestionResponse {
	var opts []StudentQuestionOptionResponse
	for _, o := range q.Options {
		opts = append(opts, ToStudentOptionResponse(&o))
	}
	return StudentQuestionResponse{
		ID:             q.ID,
		ExamID:         q.ExamID,
		QuestionNumber: q.QuestionNumber,
		Text:           q.Text,
		Type:           q.Type,
		Points:         q.Points,
		Options:        opts,
	}
}

func ToTeacherQuestionResponseList(questions []Question) []TeacherQuestionResponse {
	res := make([]TeacherQuestionResponse, len(questions))
	for i := range questions {
		res[i] = ToTeacherQuestionResponse(&questions[i])
	}
	return res
}

func ToStudentQuestionResponseList(questions []Question) []StudentQuestionResponse {
	res := make([]StudentQuestionResponse, len(questions))
	for i := range questions {
		res[i] = ToStudentQuestionResponse(&questions[i])
	}
	return res
}
