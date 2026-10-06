package question

import (
	"errors"
	"time"

	"github.com/go-playground/validator/v10"
	"github.com/google/uuid"

	platformauth "github.com/Satyajeet-Das/ai-scribe/internal/platform/auth"
)

var validate = validator.New()

// Caller encapsulates the authenticated identity and permissions.
type Caller struct {
	ID   uuid.UUID
	Role platformauth.Role
}

func (c Caller) IsAdmin() bool {
	return c.Role == platformauth.RoleAdmin
}

func (c Caller) IsTeacher() bool {
	return c.Role == platformauth.RoleTeacher || c.Role == platformauth.RoleEducator
}

func (c Caller) IsStudent() bool {
	return c.Role == platformauth.RoleStudent || c.Role == platformauth.RoleCandidate
}

func (c Caller) CanManage(ownerID uuid.UUID) bool {
	if c.IsAdmin() {
		return true
	}
	return c.ID != uuid.Nil && ownerID == c.ID
}

// CreateQuestionRequest defines the payload for creating a question.
type CreateQuestionRequest struct {
	QuestionNumber int                   `json:"questionNumber" validate:"omitempty,min=1"`
	Text           string                `json:"text" validate:"required,min=2"`
	Type           Type                  `json:"type" validate:"required,oneof=MCQ ESSAY VOICE"`
	Points         int                   `json:"points" validate:"min=0"`
	Options        []CreateOptionRequest `json:"options,omitempty" validate:"omitempty,dive"`
}

func (r *CreateQuestionRequest) Validate() error {
	return validate.Struct(r)
}

// UpdateQuestionRequest defines partial updates for a question.
type UpdateQuestionRequest struct {
	QuestionNumber *int    `json:"questionNumber,omitempty" validate:"omitempty,min=1"`
	Text           *string `json:"text,omitempty" validate:"omitempty,min=2"`
	Points         *int    `json:"points,omitempty" validate:"omitempty,min=0"`
}

func (r *UpdateQuestionRequest) Validate() error {
	return validate.Struct(r)
}

// CreateOptionRequest defines the payload for creating a question option.
type CreateOptionRequest struct {
	OptionKey    string `json:"optionKey" validate:"required,min=1,max=10"`
	OptionText   string `json:"optionText" validate:"required,min=1"`
	DisplayOrder int    `json:"displayOrder" validate:"min=0"`
	IsCorrect    bool   `json:"isCorrect"`
}

func (r *CreateOptionRequest) Validate() error {
	return validate.Struct(r)
}

// UpdateOptionRequest defines partial updates for a question option.
type UpdateOptionRequest struct {
	OptionKey    *string `json:"optionKey,omitempty" validate:"omitempty,min=1,max=10"`
	OptionText   *string `json:"optionText,omitempty" validate:"omitempty,min=1"`
	DisplayOrder *int    `json:"displayOrder,omitempty" validate:"omitempty,min=0"`
	IsCorrect    *bool   `json:"isCorrect,omitempty"`
}

func (r *UpdateOptionRequest) Validate() error {
	return validate.Struct(r)
}

// QuestionOrderItem defines explicit position for reordering.
type QuestionOrderItem struct {
	ID             uuid.UUID `json:"id" validate:"required"`
	QuestionNumber int       `json:"questionNumber" validate:"required,min=1"`
}

// ReorderQuestionsRequest defines the payload for reordering questions.
type ReorderQuestionsRequest struct {
	QuestionIDs []uuid.UUID         `json:"questionIds,omitempty"`
	Orders      []QuestionOrderItem `json:"orders,omitempty" validate:"omitempty,dive"`
}

func (r *ReorderQuestionsRequest) Validate() error {
	if len(r.QuestionIDs) == 0 && len(r.Orders) == 0 {
		return errors.New("either questionIds or orders must be provided")
	}
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
	Options        []TeacherQuestionOptionResponse `json:"options"`
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
	Options        []StudentQuestionOptionResponse `json:"options"`
}

// List Responses
type QuestionListResponse struct {
	Questions []TeacherQuestionResponse `json:"questions"`
	Data      []TeacherQuestionResponse `json:"data,omitempty"`
	Total     int                       `json:"total"`
}

type StudentQuestionListResponse struct {
	Questions []StudentQuestionResponse `json:"questions"`
	Data      []StudentQuestionResponse `json:"data,omitempty"`
	Total     int                       `json:"total"`
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
	opts := make([]TeacherQuestionOptionResponse, 0, len(q.Options))
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
	opts := make([]StudentQuestionOptionResponse, 0, len(q.Options))
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

func ToTeacherQuestionListResponse(questions []Question) QuestionListResponse {
	list := ToTeacherQuestionResponseList(questions)
	return QuestionListResponse{
		Questions: list,
		Data:      list,
		Total:     len(list),
	}
}

func ToStudentQuestionListResponse(questions []Question) StudentQuestionListResponse {
	list := ToStudentQuestionResponseList(questions)
	return StudentQuestionListResponse{
		Questions: list,
		Data:      list,
		Total:     len(list),
	}
}
