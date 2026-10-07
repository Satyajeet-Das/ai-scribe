package assignment

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/rs/zerolog"

	"github.com/Satyajeet-Das/ai-scribe/internal/exam"
	"github.com/Satyajeet-Das/ai-scribe/internal/model"
	"github.com/Satyajeet-Das/ai-scribe/internal/user"
)

type ExamReader interface {
	GetExam(ctx context.Context, id uuid.UUID) (*exam.Exam, error)
}

type UserReader interface {
	GetByID(ctx context.Context, id uuid.UUID) (*user.User, error)
	GetByRollNo(ctx context.Context, rollNo string) (*user.User, error)
}

type Service interface {
	GetAssignment(ctx context.Context, id uuid.UUID) (*Assignment, error)
	GetActiveAssignment(ctx context.Context, examID, studentID uuid.UUID) (*Assignment, error)
	ListAssignments(ctx context.Context, limit, offset int, examID, studentID *uuid.UUID, status *Status, search *string, caller Caller) ([]Assignment, int, error)
	CreateAssignment(ctx context.Context, req CreateAssignmentRequest, caller Caller) (*Assignment, error)
	BulkAssign(ctx context.Context, req BulkAssignRequest, caller Caller) (*BulkAssignResponse, error)
	RevokeAssignment(ctx context.Context, id uuid.UUID, caller Caller) error
	RevokeAssignmentByExamAndStudent(ctx context.Context, examID, studentID uuid.UUID, caller Caller) error
	CheckAssignment(ctx context.Context, examID, studentID uuid.UUID, caller Caller) (*Assignment, bool, error)
	ListStudentAssignedExams(ctx context.Context, studentID uuid.UUID, limit, offset int, caller Caller) ([]StudentAssignedExam, int, error)
}

type assignmentService struct {
	repo       Repository
	examReader ExamReader
	userReader UserReader
	logger     *zerolog.Logger
}

func NewService(repo Repository, examReader ExamReader, userReader UserReader, logger *zerolog.Logger) Service {
	return &assignmentService{
		repo:       repo,
		examReader: examReader,
		userReader: userReader,
		logger:     logger,
	}
}

func (s *assignmentService) GetAssignment(ctx context.Context, id uuid.UUID) (*Assignment, error) {
	return s.repo.GetByID(ctx, id)
}

func (s *assignmentService) GetActiveAssignment(ctx context.Context, examID, studentID uuid.UUID) (*Assignment, error) {
	return s.repo.GetActiveByExamAndStudent(ctx, examID, studentID)
}

func (s *assignmentService) ListAssignments(ctx context.Context, limit, offset int, examID, studentID *uuid.UUID, status *Status, search *string, caller Caller) ([]Assignment, int, error) {
	if limit <= 0 {
		limit = 20
	}
	if limit > 100 {
		limit = 100
	}
	if offset < 0 {
		offset = 0
	}

	if caller.IsStudent() {
		// Students can only view their own assignments
		if studentID == nil || *studentID != caller.ID {
			return nil, 0, ErrUnauthorized
		}
	} else if examID != nil {
		ex, err := s.examReader.GetExam(ctx, *examID)
		if err != nil {
			return nil, 0, err
		}
		if ex == nil {
			return nil, 0, exam.ErrExamNotFound
		}
		if !s.isAuthorizedToManage(ctx, caller, ex.CreatedBy) {
			return nil, 0, ErrUnauthorized
		}
	}

	return s.repo.List(ctx, limit, offset, examID, studentID, status, search)
}

func (s *assignmentService) isAuthorizedToManage(ctx context.Context, caller Caller, ownerID uuid.UUID) bool {
	if caller.IsAdmin() {
		return true
	}
	if caller.CanManage(ownerID) {
		return true
	}
	// Fallback check if role wasn't populated on caller
	if caller.ID != uuid.Nil && s.userReader != nil {
		callerUser, err := s.userReader.GetByID(ctx, caller.ID)
		if err == nil && callerUser != nil && strings.EqualFold(callerUser.Role, "ADMIN") {
			return true
		}
	}
	return false
}

func (s *assignmentService) resolveStudent(ctx context.Context, studentID *uuid.UUID, rollNo *string) (*user.User, error) {
	if s.userReader == nil {
		return nil, nil
	}

	var stu *user.User
	var err error

	hasRollNo := rollNo != nil && strings.TrimSpace(*rollNo) != ""
	hasStudentID := studentID != nil && *studentID != uuid.Nil

	if hasRollNo {
		cleanRoll := strings.TrimSpace(*rollNo)
		stu, err = s.userReader.GetByRollNo(ctx, cleanRoll)
		if err != nil {
			if errors.Is(err, user.ErrUserNotFound) {
				return nil, ErrStudentNotFound
			}
			return nil, err
		}
		if stu == nil {
			return nil, ErrStudentNotFound
		}
		// If both provided, verify they refer to the same student
		if hasStudentID && stu.ID != *studentID {
			return nil, ErrMismatchedStudent
		}
	} else if hasStudentID {
		stu, err = s.userReader.GetByID(ctx, *studentID)
		if err != nil {
			if errors.Is(err, user.ErrUserNotFound) {
				return nil, ErrStudentNotFound
			}
			return nil, err
		}
		if stu == nil {
			return nil, ErrStudentNotFound
		}
	} else {
		return nil, errors.New("either studentId or rollNo is required")
	}

	if !strings.EqualFold(stu.Role, "STUDENT") {
		return nil, ErrStudentIneligible
	}
	if !stu.IsActive {
		return nil, ErrStudentIneligible
	}

	return stu, nil
}

func (s *assignmentService) CreateAssignment(ctx context.Context, req CreateAssignmentRequest, caller Caller) (*Assignment, error) {
	if err := req.Validate(); err != nil {
		return nil, err
	}

	ex, err := s.examReader.GetExam(ctx, req.ExamID)
	if err != nil {
		return nil, err
	}
	if ex == nil {
		return nil, exam.ErrExamNotFound
	}

	if !s.isAuthorizedToManage(ctx, caller, ex.CreatedBy) {
		return nil, ErrUnauthorized
	}

	if ex.Status == exam.StatusArchived {
		return nil, ErrExamArchived
	}
	if ex.Status != exam.StatusDraft && ex.Status != exam.StatusPublished {
		return nil, ErrInvalidAssignmentState
	}

	stu, err := s.resolveStudent(ctx, req.StudentID, req.RollNo)
	if err != nil {
		return nil, err
	}

	var targetStudentID uuid.UUID
	var studentName string
	var studentRollNo string

	if stu != nil {
		targetStudentID = stu.ID
		studentName = strings.TrimSpace(stu.FirstName + " " + stu.LastName)
		if stu.RollNo != nil {
			studentRollNo = *stu.RollNo
		}
	} else if req.StudentID != nil {
		targetStudentID = *req.StudentID
	}

	existing, err := s.repo.GetActiveByExamAndStudent(ctx, req.ExamID, targetStudentID)
	if err != nil {
		return nil, err
	}
	if existing != nil {
		return nil, ErrDuplicateAssignment
	}

	now := time.Now().UTC()
	a := &Assignment{
		Base: model.Base{
			BaseWithId:        model.BaseWithId{ID: uuid.New()},
			BaseWithCreatedAt: model.BaseWithCreatedAt{CreatedAt: now},
			BaseWithUpdatedAt: model.BaseWithUpdatedAt{UpdatedAt: now},
		},
		ExamID:        req.ExamID,
		StudentID:     targetStudentID,
		StudentName:   studentName,
		StudentRollNo: studentRollNo,
		AssignedAt:    now,
		Status:        StatusAssigned,
	}

	if err := s.repo.Create(ctx, a); err != nil {
		s.logger.Error().
			Err(err).
			Str("exam_id", req.ExamID.String()).
			Str("student_id", targetStudentID.String()).
			Msg("failed to create assignment")
		return nil, err
	}

	s.logger.Info().
		Str("event", "assignment.created").
		Str("assignment_id", a.ID.String()).
		Str("exam_id", req.ExamID.String()).
		Str("student_id", targetStudentID.String()).
		Msg("assignment created successfully")

	return a, nil
}

func (s *assignmentService) BulkAssign(ctx context.Context, req BulkAssignRequest, caller Caller) (*BulkAssignResponse, error) {
	if err := req.Validate(); err != nil {
		return nil, err
	}

	ex, err := s.examReader.GetExam(ctx, req.ExamID)
	if err != nil {
		return nil, err
	}
	if ex == nil {
		return nil, exam.ErrExamNotFound
	}

	if !s.isAuthorizedToManage(ctx, caller, ex.CreatedBy) {
		return nil, ErrUnauthorized
	}

	if ex.Status == exam.StatusArchived {
		return nil, ErrExamArchived
	}
	if ex.Status != exam.StatusDraft && ex.Status != exam.StatusPublished {
		return nil, ErrInvalidAssignmentState
	}

	resp := &BulkAssignResponse{
		Assigned: make([]AssignmentResponse, 0),
		Failed:   make([]BulkAssignFailure, 0),
	}

	seenStudentIDs := make(map[uuid.UUID]bool)
	now := time.Now().UTC()

	// 1. Process RollNumbers
	for _, rNo := range req.RollNumbers {
		cleanRoll := strings.TrimSpace(rNo)
		if cleanRoll == "" {
			resp.Failed = append(resp.Failed, BulkAssignFailure{
				Identifier: rNo,
				Reason:     "roll number cannot be blank",
			})
			continue
		}

		stu, err := s.resolveStudent(ctx, nil, &cleanRoll)
		if err != nil {
			resp.Failed = append(resp.Failed, BulkAssignFailure{
				Identifier: cleanRoll,
				Reason:     err.Error(),
			})
			continue
		}

		if seenStudentIDs[stu.ID] {
			resp.Failed = append(resp.Failed, BulkAssignFailure{
				Identifier: cleanRoll,
				Reason:     "duplicate student in request batch",
			})
			continue
		}
		seenStudentIDs[stu.ID] = true

		existing, err := s.repo.GetActiveByExamAndStudent(ctx, req.ExamID, stu.ID)
		if err != nil {
			resp.Failed = append(resp.Failed, BulkAssignFailure{
				Identifier: cleanRoll,
				Reason:     err.Error(),
			})
			continue
		}
		if existing != nil {
			resp.Failed = append(resp.Failed, BulkAssignFailure{
				Identifier: cleanRoll,
				Reason:     ErrDuplicateAssignment.Error(),
			})
			continue
		}

		rollVal := ""
		if stu.RollNo != nil {
			rollVal = *stu.RollNo
		}

		a := &Assignment{
			Base: model.Base{
				BaseWithId:        model.BaseWithId{ID: uuid.New()},
				BaseWithCreatedAt: model.BaseWithCreatedAt{CreatedAt: now},
				BaseWithUpdatedAt: model.BaseWithUpdatedAt{UpdatedAt: now},
			},
			ExamID:        req.ExamID,
			StudentID:     stu.ID,
			StudentName:   strings.TrimSpace(stu.FirstName + " " + stu.LastName),
			StudentRollNo: rollVal,
			AssignedAt:    now,
			Status:        StatusAssigned,
		}

		if err := s.repo.Create(ctx, a); err != nil {
			resp.Failed = append(resp.Failed, BulkAssignFailure{
				Identifier: cleanRoll,
				Reason:     err.Error(),
			})
			continue
		}

		resp.Assigned = append(resp.Assigned, ToAssignmentResponse(a))
	}

	// 2. Process StudentIDs
	for _, sid := range req.StudentIDs {
		if sid == uuid.Nil {
			resp.Failed = append(resp.Failed, BulkAssignFailure{
				Identifier: sid.String(),
				Reason:     "invalid student ID",
			})
			continue
		}

		if seenStudentIDs[sid] {
			resp.Failed = append(resp.Failed, BulkAssignFailure{
				Identifier: sid.String(),
				Reason:     "duplicate student in request batch",
			})
			continue
		}

		stu, err := s.resolveStudent(ctx, &sid, nil)
		if err != nil {
			resp.Failed = append(resp.Failed, BulkAssignFailure{
				Identifier: sid.String(),
				Reason:     err.Error(),
			})
			continue
		}

		seenStudentIDs[sid] = true

		existing, err := s.repo.GetActiveByExamAndStudent(ctx, req.ExamID, sid)
		if err != nil {
			resp.Failed = append(resp.Failed, BulkAssignFailure{
				Identifier: sid.String(),
				Reason:     err.Error(),
			})
			continue
		}
		if existing != nil {
			resp.Failed = append(resp.Failed, BulkAssignFailure{
				Identifier: sid.String(),
				Reason:     ErrDuplicateAssignment.Error(),
			})
			continue
		}

		rollVal := ""
		studentName := ""
		if stu != nil {
			studentName = strings.TrimSpace(stu.FirstName + " " + stu.LastName)
			if stu.RollNo != nil {
				rollVal = *stu.RollNo
			}
		}

		a := &Assignment{
			Base: model.Base{
				BaseWithId:        model.BaseWithId{ID: uuid.New()},
				BaseWithCreatedAt: model.BaseWithCreatedAt{CreatedAt: now},
				BaseWithUpdatedAt: model.BaseWithUpdatedAt{UpdatedAt: now},
			},
			ExamID:        req.ExamID,
			StudentID:     sid,
			StudentName:   studentName,
			StudentRollNo: rollVal,
			AssignedAt:    now,
			Status:        StatusAssigned,
		}

		if err := s.repo.Create(ctx, a); err != nil {
			resp.Failed = append(resp.Failed, BulkAssignFailure{
				Identifier: sid.String(),
				Reason:     err.Error(),
			})
			continue
		}

		resp.Assigned = append(resp.Assigned, ToAssignmentResponse(a))
	}

	resp.TotalAssigned = len(resp.Assigned)
	resp.TotalFailed = len(resp.Failed)

	s.logger.Info().
		Str("event", "assignment.bulk_completed").
		Str("exam_id", req.ExamID.String()).
		Int("assigned_count", resp.TotalAssigned).
		Int("failed_count", resp.TotalFailed).
		Msg("bulk assignment process finished")

	return resp, nil
}

func (s *assignmentService) RevokeAssignment(ctx context.Context, id uuid.UUID, caller Caller) error {
	a, err := s.repo.GetByID(ctx, id)
	if err != nil {
		return err
	}
	if a == nil {
		return ErrAssignmentNotFound
	}

	if a.Status == StatusRevoked {
		return ErrAssignmentAlreadyRevoked
	}

	ex, err := s.examReader.GetExam(ctx, a.ExamID)
	if err != nil {
		return err
	}
	if ex == nil {
		return exam.ErrExamNotFound
	}

	if !s.isAuthorizedToManage(ctx, caller, ex.CreatedBy) {
		return ErrUnauthorized
	}

	if err := s.repo.Revoke(ctx, id); err != nil {
		s.logger.Error().Err(err).Str("assignment_id", id.String()).Msg("failed to revoke assignment")
		return err
	}

	s.logger.Info().
		Str("event", "assignment.revoked").
		Str("assignment_id", id.String()).
		Msg("assignment revoked successfully")

	return nil
}

func (s *assignmentService) RevokeAssignmentByExamAndStudent(ctx context.Context, examID, studentID uuid.UUID, caller Caller) error {
	ex, err := s.examReader.GetExam(ctx, examID)
	if err != nil {
		return err
	}
	if ex == nil {
		return exam.ErrExamNotFound
	}

	if !s.isAuthorizedToManage(ctx, caller, ex.CreatedBy) {
		return ErrUnauthorized
	}

	if err := s.repo.RevokeByExamAndStudent(ctx, examID, studentID); err != nil {
		s.logger.Error().Err(err).
			Str("exam_id", examID.String()).
			Str("student_id", studentID.String()).
			Msg("failed to revoke assignment by exam and student")
		return err
	}

	s.logger.Info().
		Str("event", "assignment.revoked").
		Str("exam_id", examID.String()).
		Str("student_id", studentID.String()).
		Msg("assignment revoked successfully by exam and student")

	return nil
}

func (s *assignmentService) CheckAssignment(ctx context.Context, examID, studentID uuid.UUID, caller Caller) (*Assignment, bool, error) {
	if caller.IsStudent() && caller.ID != studentID {
		return nil, false, ErrUnauthorized
	}

	ex, err := s.examReader.GetExam(ctx, examID)
	if err != nil {
		return nil, false, err
	}
	if ex == nil {
		return nil, false, exam.ErrExamNotFound
	}

	if !caller.IsStudent() && !s.isAuthorizedToManage(ctx, caller, ex.CreatedBy) {
		return nil, false, ErrUnauthorized
	}

	a, err := s.repo.GetActiveByExamAndStudent(ctx, examID, studentID)
	if err != nil {
		return nil, false, err
	}
	if a == nil {
		return nil, false, nil
	}
	return a, true, nil
}

func (s *assignmentService) ListStudentAssignedExams(ctx context.Context, studentID uuid.UUID, limit, offset int, caller Caller) ([]StudentAssignedExam, int, error) {
	if caller.IsStudent() && caller.ID != studentID {
		return nil, 0, ErrUnauthorized
	}

	if limit <= 0 {
		limit = 20
	}
	if limit > 100 {
		limit = 100
	}
	if offset < 0 {
		offset = 0
	}

	var statusFilter *string
	if caller.IsStudent() {
		pub := string(exam.StatusPublished)
		statusFilter = &pub
	}

	return s.repo.ListStudentExams(ctx, studentID, limit, offset, statusFilter)
}
