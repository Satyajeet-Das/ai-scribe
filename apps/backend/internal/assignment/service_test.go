package assignment

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/rs/zerolog"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Satyajeet-Das/ai-scribe/internal/exam"
	platformauth "github.com/Satyajeet-Das/ai-scribe/internal/platform/auth"
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

func (m *mockAssignmentRepo) List(ctx context.Context, limit, offset int, examID, studentID *uuid.UUID, status *Status, search *string) ([]Assignment, int, error) {
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
		if search != nil && *search != "" {
			s := strings.ToLower(*search)
			if !strings.Contains(strings.ToLower(a.StudentRollNo), s) && !strings.Contains(strings.ToLower(a.StudentName), s) {
				continue
			}
		}
		res = append(res, *a)
	}
	return res, len(res), nil
}

func (m *mockAssignmentRepo) ListStudentExams(ctx context.Context, studentID uuid.UUID, limit, offset int, examStatus *string) ([]StudentAssignedExam, int, error) {
	res := make([]StudentAssignedExam, 0)
	for _, a := range m.assignments {
		if a.StudentID != studentID || a.Status != StatusAssigned {
			continue
		}
		res = append(res, StudentAssignedExam{
			AssignmentID: a.ID,
			ExamID:       a.ExamID,
			Title:        "Mock Exam Title",
			Subject:      "Mock Subject",
			DurationMins: 60,
			ExamStatus:   "PUBLISHED",
			AssignedAt:   a.AssignedAt,
			Status:       a.Status,
		})
	}
	return res, len(res), nil
}

func (m *mockAssignmentRepo) Create(ctx context.Context, a *Assignment) error {
	m.assignments[a.ID] = a
	return nil
}

func (m *mockAssignmentRepo) Revoke(ctx context.Context, id uuid.UUID) error {
	if a, ok := m.assignments[id]; ok {
		if a.Status == StatusRevoked {
			return ErrAssignmentAlreadyRevoked
		}
		a.Status = StatusRevoked
		return nil
	}
	return ErrAssignmentNotFound
}

func (m *mockAssignmentRepo) RevokeByExamAndStudent(ctx context.Context, examID, studentID uuid.UUID) error {
	for _, a := range m.assignments {
		if a.ExamID == examID && a.StudentID == studentID {
			if a.Status == StatusRevoked {
				return ErrAssignmentAlreadyRevoked
			}
			a.Status = StatusRevoked
			return nil
		}
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

func (m *mockUserReader) GetByRollNo(ctx context.Context, rollNo string) (*user.User, error) {
	for _, u := range m.users {
		if u.RollNo != nil && strings.EqualFold(*u.RollNo, strings.TrimSpace(rollNo)) {
			return u, nil
		}
	}
	return nil, user.ErrUserNotFound
}

func makeTestUser(id uuid.UUID, role string, isActive bool, firstName, lastName string, rollNo *string) *user.User {
	u := &user.User{
		Role:      role,
		IsActive:  isActive,
		FirstName: firstName,
		LastName:  lastName,
		RollNo:    rollNo,
	}
	u.ID = id
	return u
}

func setupTestService() (Service, *mockAssignmentRepo, *mockExamReader, *mockUserReader, uuid.UUID, uuid.UUID, Caller, Caller, Caller) {
	repo := newMockAssignmentRepo()
	logger := zerolog.Nop()
	creatorID := uuid.New()
	studentID := uuid.New()
	rollNo := "23CS001"

	teacherCaller := Caller{ID: creatorID, Role: platformauth.RoleTeacher}
	studentCaller := Caller{ID: studentID, Role: platformauth.RoleStudent}
	adminCaller := Caller{ID: uuid.New(), Role: platformauth.RoleAdmin}

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
			studentID: makeTestUser(studentID, "STUDENT", true, "Alice", "Smith", &rollNo),
		},
	}

	svc := NewService(repo, examReader, userReader, &logger)
	return svc, repo, examReader, userReader, publishedExamID, draftExamID, teacherCaller, studentCaller, adminCaller
}

func TestAssignmentService_CreateAssignment_ByID(t *testing.T) {
	svc, _, _, _, publishedExamID, _, teacherCaller, _, _ := setupTestService()
	ctx := context.Background()

	studentID := uuid.New()
	rNo := "23CS002"
	svc.(*assignmentService).userReader.(*mockUserReader).users[studentID] = makeTestUser(studentID, "STUDENT", true, "Bob", "Jones", &rNo)

	req := CreateAssignmentRequest{
		ExamID:    publishedExamID,
		StudentID: &studentID,
	}

	assigned, err := svc.CreateAssignment(ctx, req, teacherCaller)
	require.NoError(t, err)
	assert.Equal(t, StatusAssigned, assigned.Status)
	assert.Equal(t, studentID, assigned.StudentID)
	assert.Equal(t, "Bob Jones", assigned.StudentName)
	assert.Equal(t, "23CS002", assigned.StudentRollNo)
}

func TestAssignmentService_CreateAssignment_ByRollNo(t *testing.T) {
	svc, _, _, _, publishedExamID, _, teacherCaller, _, _ := setupTestService()
	ctx := context.Background()

	roll := "23cs001" // case-insensitive lookup
	req := CreateAssignmentRequest{
		ExamID: publishedExamID,
		RollNo: &roll,
	}

	assigned, err := svc.CreateAssignment(ctx, req, teacherCaller)
	require.NoError(t, err)
	assert.Equal(t, StatusAssigned, assigned.Status)
	assert.Equal(t, "Alice Smith", assigned.StudentName)
	assert.Equal(t, "23CS001", assigned.StudentRollNo)
}

func TestAssignmentService_CreateAssignment_Rules(t *testing.T) {
	svc, _, examReader, userReader, publishedExamID, draftExamID, teacherCaller, _, adminCaller := setupTestService()
	ctx := context.Background()
	studentID := uuid.New()
	roll := "23CS005"
	userReader.users[studentID] = makeTestUser(studentID, "STUDENT", true, "Charlie", "", &roll)

	// 1. Assign to draft exam succeeds (allows assigning before publishing)
	draftAsgn, err := svc.CreateAssignment(ctx, CreateAssignmentRequest{
		ExamID:    draftExamID,
		StudentID: &studentID,
	}, teacherCaller)
	require.NoError(t, err)
	assert.Equal(t, StatusAssigned, draftAsgn.Status)

	// 2. Assign to archived exam fails
	var archivedExamID uuid.UUID
	for id, ex := range examReader.exams {
		if ex.Status == exam.StatusArchived {
			archivedExamID = id
			break
		}
	}
	_, err = svc.CreateAssignment(ctx, CreateAssignmentRequest{
		ExamID:    archivedExamID,
		StudentID: &studentID,
	}, teacherCaller)
	assert.ErrorIs(t, err, ErrExamArchived)

	// 3. Duplicate assignment fails
	_, err = svc.CreateAssignment(ctx, CreateAssignmentRequest{
		ExamID:    draftExamID,
		StudentID: &studentID,
	}, teacherCaller)
	assert.ErrorIs(t, err, ErrDuplicateAssignment)

	// 4. Non-existent student roll number fails
	invalidRoll := "NONEXISTENT"
	_, err = svc.CreateAssignment(ctx, CreateAssignmentRequest{
		ExamID: publishedExamID,
		RollNo: &invalidRoll,
	}, teacherCaller)
	assert.ErrorIs(t, err, ErrStudentNotFound)

	// 5. Inactive student fails
	inactiveID := uuid.New()
	inactiveRoll := "INACTIVE01"
	userReader.users[inactiveID] = makeTestUser(inactiveID, "STUDENT", false, "", "", &inactiveRoll)
	_, err = svc.CreateAssignment(ctx, CreateAssignmentRequest{
		ExamID: publishedExamID,
		RollNo: &inactiveRoll,
	}, teacherCaller)
	assert.ErrorIs(t, err, ErrStudentIneligible)

	// 6. Non-student user fails
	teacherUser := uuid.New()
	userReader.users[teacherUser] = makeTestUser(teacherUser, "TEACHER", true, "", "", nil)
	_, err = svc.CreateAssignment(ctx, CreateAssignmentRequest{
		ExamID:    publishedExamID,
		StudentID: &teacherUser,
	}, teacherCaller)
	assert.ErrorIs(t, err, ErrStudentIneligible)

	// 7. Unauthorized caller fails
	otherTeacher := Caller{ID: uuid.New(), Role: platformauth.RoleTeacher}
	_, err = svc.CreateAssignment(ctx, CreateAssignmentRequest{
		ExamID:    publishedExamID,
		StudentID: &studentID,
	}, otherTeacher)
	assert.ErrorIs(t, err, ErrUnauthorized)

	// 8. Admin caller succeeds even for other teacher's exam
	adminAsgn, err := svc.CreateAssignment(ctx, CreateAssignmentRequest{
		ExamID:    publishedExamID,
		StudentID: &studentID,
	}, adminCaller)
	require.NoError(t, err)
	assert.Equal(t, StatusAssigned, adminAsgn.Status)
}

func TestAssignmentService_BulkAssign(t *testing.T) {
	svc, _, _, userReader, publishedExamID, _, teacherCaller, _, _ := setupTestService()
	ctx := context.Background()

	// Setup multiple students
	stu1ID := uuid.New()
	roll1 := "ROLL001"
	userReader.users[stu1ID] = makeTestUser(stu1ID, "STUDENT", true, "One", "", &roll1)

	stu2ID := uuid.New()
	roll2 := "ROLL002"
	userReader.users[stu2ID] = makeTestUser(stu2ID, "STUDENT", true, "Two", "", &roll2)

	// First assign stu1 to test duplicate handling in bulk
	_, err := svc.CreateAssignment(ctx, CreateAssignmentRequest{
		ExamID:    publishedExamID,
		StudentID: &stu1ID,
	}, teacherCaller)
	require.NoError(t, err)

	// Now run bulk assign with:
	// - stu1 (already assigned -> will fail with duplicate)
	// - roll2 (new valid -> will succeed)
	// - "INVALID_ROLL" (nonexistent -> will fail with not found)
	// - duplicate entry of roll2 in same batch -> will fail with duplicate in batch
	req := BulkAssignRequest{
		ExamID:      publishedExamID,
		StudentIDs:  []uuid.UUID{stu1ID},
		RollNumbers: []string{roll2, "INVALID_ROLL", roll2},
	}

	resp, err := svc.BulkAssign(ctx, req, teacherCaller)
	require.NoError(t, err)
	require.NotNil(t, resp)

	assert.Equal(t, 1, resp.TotalAssigned)
	assert.Len(t, resp.Assigned, 1)
	assert.Equal(t, "ROLL002", resp.Assigned[0].StudentRollNo)

	assert.Equal(t, 3, resp.TotalFailed)
	assert.Len(t, resp.Failed, 3)

	failedMap := make(map[string]string)
	for _, f := range resp.Failed {
		failedMap[f.Identifier] = f.Reason
	}
	assert.Contains(t, failedMap["INVALID_ROLL"], "not found")
	assert.Contains(t, failedMap[stu1ID.String()], "already has an active assignment")
}

func TestAssignmentService_CheckAssignment(t *testing.T) {
	svc, _, _, userReader, publishedExamID, _, teacherCaller, _, _ := setupTestService()
	ctx := context.Background()

	studentID := uuid.New()
	userReader.users[studentID] = makeTestUser(studentID, "STUDENT", true, "Student", "", nil)
	studentCaller := Caller{ID: studentID, Role: platformauth.RoleStudent}

	// 1. Before assignment -> returns false
	a, isAssigned, err := svc.CheckAssignment(ctx, publishedExamID, studentID, teacherCaller)
	require.NoError(t, err)
	assert.False(t, isAssigned)
	assert.Nil(t, a)

	// 2. Student checking for themself -> allowed
	_, isAssigned, err = svc.CheckAssignment(ctx, publishedExamID, studentID, studentCaller)
	require.NoError(t, err)
	assert.False(t, isAssigned)

	// 3. Student checking for another student -> unauthorized
	otherStudentID := uuid.New()
	_, _, err = svc.CheckAssignment(ctx, publishedExamID, otherStudentID, studentCaller)
	assert.ErrorIs(t, err, ErrUnauthorized)

	// 4. Create assignment and check again
	created, err := svc.CreateAssignment(ctx, CreateAssignmentRequest{
		ExamID:    publishedExamID,
		StudentID: &studentID,
	}, teacherCaller)
	require.NoError(t, err)

	a, isAssigned, err = svc.CheckAssignment(ctx, publishedExamID, studentID, studentCaller)
	require.NoError(t, err)
	assert.True(t, isAssigned)
	assert.Equal(t, created.ID, a.ID)
}

func TestAssignmentService_ListStudentAssignedExams(t *testing.T) {
	svc, repo, _, _, publishedExamID, _, teacherCaller, _, _ := setupTestService()
	ctx := context.Background()

	studentID := uuid.New()
	studentCaller := Caller{ID: studentID, Role: platformauth.RoleStudent}

	asgnID := uuid.New()
	repo.assignments[asgnID] = &Assignment{
		ExamID:     publishedExamID,
		StudentID:  studentID,
		Status:     StatusAssigned,
		AssignedAt: time.Now().UTC(),
	}

	// 1. Student lists their own exams -> succeeds
	exams, total, err := svc.ListStudentAssignedExams(ctx, studentID, 10, 0, studentCaller)
	require.NoError(t, err)
	assert.Equal(t, 1, total)
	assert.Len(t, exams, 1)

	// 2. Student attempts to list another student's exams -> unauthorized
	otherStudent := uuid.New()
	_, _, err = svc.ListStudentAssignedExams(ctx, otherStudent, 10, 0, studentCaller)
	assert.ErrorIs(t, err, ErrUnauthorized)

	// 3. Teacher can list student's exams
	examsTeacher, totalTeacher, err := svc.ListStudentAssignedExams(ctx, studentID, 10, 0, teacherCaller)
	require.NoError(t, err)
	assert.Equal(t, 1, totalTeacher)
	assert.Len(t, examsTeacher, 1)
}

func TestAssignmentService_RevokeAssignment(t *testing.T) {
	svc, repo, _, _, publishedExamID, _, teacherCaller, _, _ := setupTestService()
	ctx := context.Background()

	assignmentID := uuid.New()
	studentID := uuid.New()
	repo.assignments[assignmentID] = &Assignment{
		ExamID:    publishedExamID,
		StudentID: studentID,
		Status:    StatusAssigned,
	}

	// 1. Revoke by ID succeeds
	err := svc.RevokeAssignment(ctx, assignmentID, teacherCaller)
	require.NoError(t, err)

	// 2. Revoking already revoked fails
	err = svc.RevokeAssignment(ctx, assignmentID, teacherCaller)
	assert.ErrorIs(t, err, ErrAssignmentAlreadyRevoked)

	// 3. Revoke by Exam & Student
	assignment2ID := uuid.New()
	student2ID := uuid.New()
	repo.assignments[assignment2ID] = &Assignment{
		ExamID:    publishedExamID,
		StudentID: student2ID,
		Status:    StatusAssigned,
	}
	err = svc.RevokeAssignmentByExamAndStudent(ctx, publishedExamID, student2ID, teacherCaller)
	require.NoError(t, err)
	assert.Equal(t, StatusRevoked, repo.assignments[assignment2ID].Status)
}
