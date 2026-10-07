package assignment

import (
	"errors"
	"strings"
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

// CreateAssignmentRequest defines the payload for creating a single assignment.
// Either StudentID or RollNo must be provided.
type CreateAssignmentRequest struct {
	ExamID    uuid.UUID  `json:"examId" validate:"required"`
	StudentID *uuid.UUID `json:"studentId,omitempty"`
	RollNo    *string    `json:"rollNo,omitempty"`
}

func (r *CreateAssignmentRequest) Validate() error {
	if r.ExamID == uuid.Nil {
		return errors.New("examId is required")
	}
	hasStudentID := r.StudentID != nil && *r.StudentID != uuid.Nil
	hasRollNo := r.RollNo != nil && strings.TrimSpace(*r.RollNo) != ""
	if !hasStudentID && !hasRollNo {
		return errors.New("either studentId or rollNo is required")
	}
	return nil
}

// BulkAssignRequest defines the payload for assigning an exam to multiple students.
type BulkAssignRequest struct {
	ExamID      uuid.UUID   `json:"examId" validate:"required"`
	StudentIDs  []uuid.UUID `json:"studentIds,omitempty"`
	RollNumbers []string    `json:"rollNumbers,omitempty"`
}

func (r *BulkAssignRequest) Validate() error {
	if r.ExamID == uuid.Nil {
		return errors.New("examId is required")
	}
	if len(r.StudentIDs) == 0 && len(r.RollNumbers) == 0 {
		return ErrNoStudentsProvided
	}
	if len(r.StudentIDs)+len(r.RollNumbers) > 500 {
		return ErrBatchTooLarge
	}
	return nil
}

// BulkAssignFailure documents an individual student assignment failure.
type BulkAssignFailure struct {
	Identifier string `json:"identifier"`
	Reason     string `json:"reason"`
}

// BulkAssignResponse returns summary of successful and failed assignments.
type BulkAssignResponse struct {
	Assigned      []AssignmentResponse `json:"assigned"`
	Failed        []BulkAssignFailure  `json:"failed"`
	TotalAssigned int                  `json:"totalAssigned"`
	TotalFailed   int                  `json:"totalFailed"`
}

// CheckAssignmentResponse represents the result of an assignment check.
type CheckAssignmentResponse struct {
	Assigned     bool                `json:"assigned"`
	AssignmentID *uuid.UUID          `json:"assignmentId,omitempty"`
	Status       *Status             `json:"status,omitempty"`
	AssignedAt   *time.Time          `json:"assignedAt,omitempty"`
	Assignment   *AssignmentResponse `json:"assignment,omitempty"`
}

// StudentAssignedExamResponse provides enriched exam details for a student.
type StudentAssignedExamResponse struct {
	AssignmentID uuid.UUID `json:"assignmentId"`
	ExamID       uuid.UUID `json:"examId"`
	Title        string    `json:"title"`
	Subject      string    `json:"subject"`
	Description  string    `json:"description,omitempty"`
	DurationMins int       `json:"durationMins"`
	ExamStatus   string    `json:"examStatus"`
	AssignedAt   time.Time `json:"assignedAt"`
	Status       Status    `json:"status"`
}

// StudentAssignedExamsListResponse wraps the paginated student exams list.
type StudentAssignedExamsListResponse struct {
	Data   []StudentAssignedExamResponse `json:"data"`
	Total  int                           `json:"total"`
	Limit  int                           `json:"limit"`
	Offset int                           `json:"offset"`
}

// AssignmentResponse represents an assignment payload returned by the API.
type AssignmentResponse struct {
	ID            uuid.UUID `json:"id"`
	ExamID        uuid.UUID `json:"examId"`
	StudentID     uuid.UUID `json:"studentId"`
	StudentName   string    `json:"studentName,omitempty"`
	StudentRollNo string    `json:"studentRollNo,omitempty"`
	AssignedAt    time.Time `json:"assignedAt"`
	Status        Status    `json:"status"`
	CreatedAt     time.Time `json:"createdAt"`
	UpdatedAt     time.Time `json:"updatedAt"`
}

// AssignmentListResponse wraps the paginated assignments list.
type AssignmentListResponse struct {
	Data        []AssignmentResponse `json:"data"`
	Assignments []AssignmentResponse `json:"assignments"`
	Total       int                  `json:"total"`
	Limit       int                  `json:"limit"`
	Offset      int                  `json:"offset"`
}

func ToAssignmentResponse(a *Assignment) AssignmentResponse {
	return AssignmentResponse{
		ID:            a.ID,
		ExamID:        a.ExamID,
		StudentID:     a.StudentID,
		StudentName:   a.StudentName,
		StudentRollNo: a.StudentRollNo,
		AssignedAt:    a.AssignedAt,
		Status:        a.Status,
		CreatedAt:     a.CreatedAt,
		UpdatedAt:     a.UpdatedAt,
	}
}

func ToAssignmentResponseList(assignments []Assignment) []AssignmentResponse {
	res := make([]AssignmentResponse, len(assignments))
	for i := range assignments {
		res[i] = ToAssignmentResponse(&assignments[i])
	}
	return res
}

func ToStudentAssignedExamResponse(e *StudentAssignedExam) StudentAssignedExamResponse {
	return StudentAssignedExamResponse{
		AssignmentID: e.AssignmentID,
		ExamID:       e.ExamID,
		Title:        e.Title,
		Subject:      e.Subject,
		Description:  e.Description,
		DurationMins: e.DurationMins,
		ExamStatus:   e.ExamStatus,
		AssignedAt:   e.AssignedAt,
		Status:       e.Status,
	}
}

func ToStudentAssignedExamResponseList(exams []StudentAssignedExam) []StudentAssignedExamResponse {
	res := make([]StudentAssignedExamResponse, len(exams))
	for i := range exams {
		res[i] = ToStudentAssignedExamResponse(&exams[i])
	}
	return res
}
