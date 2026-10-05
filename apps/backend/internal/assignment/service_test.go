package assignment

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/rs/zerolog"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Satyajeet-Das/ai-scribe/internal/exam"
	"github.com/Satyajeet-Das/ai-scribe/internal/user"
)

type mockAssignmentRepo struct {
	assignments map[uuid.UUID]*Assignment
}

func newMockAssignmentRepo() *mockAssignmentRepo {
	return &mockAssignmentRepo{
		assignments: make(map[uuid.UUID]*Assignment),
	}
}

func (m *mockAssignmentRepo) GetByID(ctx context.Context, id uuid.UUID) (*Assignment, error) {
	if a, ok := m.assignments[id]; ok {
		copy := *a
		return &copy, nil
	}
	return nil, nil
}

func (m *mockAssignmentRepo) GetActiveByExamAndStudent(ctx context.Context, examID, studentID uuid.UUID) (*Assignment, error) {
	for _, a := range m.assignments {
		if a.ExamID == examID && a.StudentID == studentID && a.Status == StatusAssigned {
			copy := *a
			return &copy, nil
		}
	}
	return nil, nil
}

func (m *mockAssignmentRepo) List(ctx context.Context, limit, offset int, examID, studentID *uuid.UUID, status *Status) ([]Assignment, int, error) {
	res := make([]Assignment, 0)
	for _, a := range m.assignments {
		if examID != nil && a.ExamID != *examID {
			continue
		}
		if studentID != nil && a.StudentID != *studentID {
			continue
		}
		if status != nil && a.Status != *status {
			continue
		}
		res = append(res, *a)
	}
	return res, len(res), nil
}

func (m *mockAssignmentRepo) Create(ctx context.Context, a *Assignment) error {
	m.assignments[a.ID] = a
	return nil
}

func (m *mockAssignmentRepo) Revoke(ctx context.Context, id uuid.UUID) error {
	if a, ok := m.assignments[id]; ok {
		a.Status = StatusRevoked
		return nil
	}
	return ErrAssignmentNotFound
}

type mockExamReader struct {
	exams map[uuid.UUID]*exam.Exam
}

func (m *mockExamReader) GetExam(ctx context.Context, id uuid.UUID) (*exam.Exam, error) {
	if e, ok := m.exams[id]; ok {
		return e, nil
	}
	return nil, exam.ErrExamNotFound
}

type mockUserReader struct {
	users map[uuid.UUID]*user.User
}

func (m *mockUserReader) GetByID(ctx context.Context, id uuid.UUID) (*user.User, error) {
	if u, ok := m.users[id]; ok {
		return u, nil
	}
	return nil, user.ErrUserNotFound
}

func TestAssignmentService_CreateAssignment(t *testing.T) {
	repo := newMockAssignmentRepo()
	logger := zerolog.Nop()
	creatorID := uuid.New()
	studentID := uuid.New()
	inactiveStudentID := uuid.New()
	teacherID := uuid.New()
	publishedExamID := uuid.New()
	draftExamID := uuid.New()
	archivedExamID := uuid.New()

	examReader := &mockExamReader{
		exams: map[uuid.UUID]*exam.Exam{
			publishedExamID: {
				Title:     "Published Math Exam",
				Status:    exam.StatusPublished,
				CreatedBy: creatorID,
			},
			draftExamID: {
				Title:     "Draft History Exam",
				Status:    exam.StatusDraft,
				CreatedBy: creatorID,
			},
			archivedExamID: {
				Title:     "Archived Exam",
				Status:    exam.StatusArchived,
				CreatedBy: creatorID,
			},
		},
	}

	userReader := &mockUserReader{
		users: map[uuid.UUID]*user.User{
			studentID: {
				Role:     "STUDENT",
				IsActive: true,
			},
			inactiveStudentID: {
				Role:     "STUDENT",
				IsActive: false,
			},
			teacherID: {
				Role:     "TEACHER",
				IsActive: true,
			},
		},
	}

	svc := NewService(repo, examReader, userReader, &logger)
	ctx := context.Background()

	// 1. Assign published exam succeeds for active student
	req := CreateAssignmentRequest{
		ExamID:    publishedExamID,
		StudentID: studentID,
	}
	assigned, err := svc.CreateAssignment(ctx, req, creatorID)
	require.NoError(t, err)
	assert.Equal(t, StatusAssigned, assigned.Status)
	assert.Equal(t, studentID, assigned.StudentID)

	// 2. Non-existent student fails with ErrStudentNotFound
	_, err = svc.CreateAssignment(ctx, CreateAssignmentRequest{
		ExamID:    publishedExamID,
		StudentID: uuid.New(),
	}, creatorID)
	assert.ErrorIs(t, err, ErrStudentNotFound)

	// 3. Inactive student fails with ErrStudentIneligible
	_, err = svc.CreateAssignment(ctx, CreateAssignmentRequest{
		ExamID:    publishedExamID,
		StudentID: inactiveStudentID,
	}, creatorID)
	assert.ErrorIs(t, err, ErrStudentIneligible)

	// 4. Non-student user fails with ErrStudentIneligible
	_, err = svc.CreateAssignment(ctx, CreateAssignmentRequest{
		ExamID:    publishedExamID,
		StudentID: teacherID,
	}, creatorID)
	assert.ErrorIs(t, err, ErrStudentIneligible)

	// 5. Assigning draft exam succeeds (allows assigning students before publishing)
	draftAsgn, err := svc.CreateAssignment(ctx, CreateAssignmentRequest{
		ExamID:    draftExamID,
		StudentID: studentID,
	}, creatorID)
	require.NoError(t, err)
	assert.Equal(t, StatusAssigned, draftAsgn.Status)

	// 6. Assigning archived exam fails
	_, err = svc.CreateAssignment(ctx, CreateAssignmentRequest{
		ExamID:    archivedExamID,
		StudentID: studentID,
	}, creatorID)
	assert.ErrorIs(t, err, ErrExamArchived)

	// 7. Duplicate active assignment fails
	_, err = svc.CreateAssignment(ctx, req, creatorID)
	assert.ErrorIs(t, err, ErrDuplicateAssignment)

	// 8. Unauthorized creator fails
	otherUser := uuid.New()
	validOtherStudent := uuid.New()
	userReader.users[validOtherStudent] = &user.User{Role: "STUDENT", IsActive: true}
	_, err = svc.CreateAssignment(ctx, CreateAssignmentRequest{
		ExamID:    publishedExamID,
		StudentID: validOtherStudent,
	}, otherUser)
	assert.ErrorIs(t, err, ErrUnauthorized)

	// 9. Admin can assign to someone else's exam
	adminUser := uuid.New()
	userReader.users[adminUser] = &user.User{Role: "ADMIN", IsActive: true}
	adminAsgn, err := svc.CreateAssignment(ctx, CreateAssignmentRequest{
		ExamID:    publishedExamID,
		StudentID: validOtherStudent,
	}, adminUser)
	require.NoError(t, err)
	assert.Equal(t, StatusAssigned, adminAsgn.Status)
}

func TestAssignmentService_RevokeAssignment(t *testing.T) {
	repo := newMockAssignmentRepo()
	logger := zerolog.Nop()
	creatorID := uuid.New()
	examID := uuid.New()
	assignmentID := uuid.New()

	examReader := &mockExamReader{
		exams: map[uuid.UUID]*exam.Exam{
			examID: {
				Title:     "Science Exam",
				Status:    exam.StatusPublished,
				CreatedBy: creatorID,
			},
		},
	}

	repo.assignments[assignmentID] = &Assignment{
		ExamID:    examID,
		StudentID: uuid.New(),
		Status:    StatusAssigned,
	}

	svc := NewService(repo, examReader, nil, &logger)
	ctx := context.Background()

	// 1. Revoke succeeds
	err := svc.RevokeAssignment(ctx, assignmentID, creatorID)
	require.NoError(t, err)

	// 2. Revoking already revoked fails
	err = svc.RevokeAssignment(ctx, assignmentID, creatorID)
	assert.ErrorIs(t, err, ErrAssignmentAlreadyRevoked)
}
