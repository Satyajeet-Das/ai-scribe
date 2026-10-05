package exam

import (
	"time"

	"github.com/go-playground/validator/v10"
	"github.com/google/uuid"

	platformauth "github.com/Satyajeet-Das/ai-scribe/internal/platform/auth"
)

var validate = validator.New()

type Caller struct {
	ID   uuid.UUID
	Role platformauth.Role
}

func (c Caller) IsAdmin() bool {
	return c.Role == platformauth.RoleAdmin
}

func (c Caller) IsTeacher() bool {
	return c.Role == platformauth.RoleTeacher
}

func (c Caller) CanManage(ownerID uuid.UUID) bool {
	if c.IsAdmin() {
		return true
	}
	return c.ID != uuid.Nil && ownerID == c.ID
}

type CreateExamRequest struct {
	Title        string `json:"title" validate:"required,min=3,max=255"`
	Subject      string `json:"subject" validate:"required,min=2,max=100"`
	Description  string `json:"description" validate:"max=2000"`
	DurationMins int    `json:"durationMins" validate:"required,min=1,max=600"`
}

func (r *CreateExamRequest) Validate() error {
	return validate.Struct(r)
}

type UpdateExamRequest struct {
	Title        *string `json:"title,omitempty" validate:"omitempty,min=3,max=255"`
	Subject      *string `json:"subject,omitempty" validate:"omitempty,min=2,max=100"`
	Description  *string `json:"description,omitempty" validate:"omitempty,max=2000"`
	DurationMins *int    `json:"durationMins,omitempty" validate:"omitempty,min=1,max=600"`
}

func (r *UpdateExamRequest) Validate() error {
	return validate.Struct(r)
}

type ListExamsParams struct {
	Limit     int        `query:"limit" json:"limit"`
	Offset    int        `query:"offset" json:"offset"`
	Status    *Status    `query:"status" json:"status,omitempty"`
	Subject   string     `query:"subject" json:"subject,omitempty"`
	Search    string     `query:"search" json:"search,omitempty"`
	CreatedBy *uuid.UUID `query:"createdBy" json:"createdBy,omitempty"`
}

func (p *ListExamsParams) Defaults() {
	if p.Limit <= 0 {
		p.Limit = 20
	}
	if p.Limit > 100 {
		p.Limit = 100
	}
	if p.Offset < 0 {
		p.Offset = 0
	}
}

type ExamResponse struct {
	ID           uuid.UUID  `json:"id"`
	Title        string     `json:"title"`
	Subject      string     `json:"subject"`
	Description  string     `json:"description"`
	DurationMins int        `json:"durationMins"`
	Status       Status     `json:"status"`
	CreatedBy    uuid.UUID  `json:"createdBy"`
	PublishedAt  *time.Time `json:"publishedAt,omitempty"`
	CreatedAt    time.Time  `json:"createdAt"`
	UpdatedAt    time.Time  `json:"updatedAt"`
}

type ExamListResponse struct {
	Exams  []ExamResponse `json:"exams"`
	Data   []ExamResponse `json:"data"` // Backward compatibility
	Total  int            `json:"total"`
	Limit  int            `json:"limit"`
	Offset int            `json:"offset"`
}

func ToExamResponse(e *Exam) ExamResponse {
	return ExamResponse{
		ID:           e.ID,
		Title:        e.Title,
		Subject:      e.Subject,
		Description:  e.Description,
		DurationMins: e.DurationMins,
		Status:       e.Status,
		CreatedBy:    e.CreatedBy,
		PublishedAt:  e.PublishedAt,
		CreatedAt:    e.CreatedAt,
		UpdatedAt:    e.UpdatedAt,
	}
}

func ToExamResponseList(exams []Exam) []ExamResponse {
	resp := make([]ExamResponse, len(exams))
	for i := range exams {
		resp[i] = ToExamResponse(&exams[i])
	}
	return resp
}

func ToExamListResponse(exams []Exam, total, limit, offset int) ExamListResponse {
	list := ToExamResponseList(exams)
	return ExamListResponse{
		Exams:  list,
		Data:   list,
		Total:  total,
		Limit:  limit,
		Offset: offset,
	}
}
