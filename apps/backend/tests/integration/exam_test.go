package integration

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Satyajeet-Das/ai-scribe/internal/assignment"
	"github.com/Satyajeet-Das/ai-scribe/internal/exam"
	platformauth "github.com/Satyajeet-Das/ai-scribe/internal/platform/auth"
	"github.com/Satyajeet-Das/ai-scribe/internal/platform/logger"
	"github.com/Satyajeet-Das/ai-scribe/internal/user"
)

func TestExamRepositoryAndService_Integration(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	testDB, cleanup := SetupTestDB(t)
	defer cleanup()

	ctx := context.Background()
	log := logger.NewLoggerService(testDB.Config.Observability)
	l := logger.NewLoggerWithService(testDB.Config.Observability, log)

	userRepo := user.NewRepository(testDB.Pool)
	examRepo := exam.NewRepository(testDB.Pool)
	assignmentRepo := assignment.NewRepository(testDB.Pool)
	examSvc := exam.NewService(examRepo, &l)

	// Seed teacher and student
	teacherClerkID := uuid.New().String()
	teacher := &user.User{
		ClerkID:      &teacherClerkID,
		Email:        "teacher_exam_int@test.com",
		PasswordHash: "hash",
		Role:         "TEACHER",
		FirstName:    "Exam",
		LastName:     "Teacher",
		IsActive:     true,
	}
	require.NoError(t, userRepo.Create(ctx, teacher))

	studentClerkID := uuid.New().String()
	student := &user.User{
		ClerkID:      &studentClerkID,
		Email:        "student_exam_int@test.com",
		PasswordHash: "hash",
		Role:         "STUDENT",
		FirstName:    "Exam",
		LastName:     "Student",
		IsActive:     true,
	}
	require.NoError(t, userRepo.Create(ctx, student))

	teacherCaller := exam.Caller{ID: teacher.ID, Role: platformauth.RoleTeacher}
	adminCaller := exam.Caller{ID: uuid.New(), Role: platformauth.RoleAdmin}

	t.Run("Create, Get, and List with search filters", func(t *testing.T) {
		created, err := examSvc.CreateExam(ctx, exam.CreateExamRequest{
			Title:        "Advanced Organic Chemistry",
			Subject:      "Chemistry",
			Description:  "Organic synthesis and reaction mechanisms",
			DurationMins: 120,
		}, teacherCaller)
		require.NoError(t, err)
		assert.Equal(t, exam.StatusDraft, created.Status)
		assert.Nil(t, created.PublishedAt)

		// Get by ID
		fetched, err := examSvc.GetExamForCaller(ctx, created.ID, teacherCaller)
		require.NoError(t, err)
		assert.Equal(t, created.Title, fetched.Title)

		// List with search
		exams, total, err := examSvc.ListExams(ctx, exam.ListExamsParams{
			Search: "Organic",
		}, teacherCaller)
		require.NoError(t, err)
		assert.Equal(t, 1, total)
		assert.Len(t, exams, 1)
		assert.Equal(t, created.ID, exams[0].ID)

		// Update draft exam
		newTitle := "Advanced Organic Chemistry (Honors)"
		updated, err := examSvc.UpdateExam(ctx, created.ID, exam.UpdateExamRequest{
			Title: &newTitle,
		}, teacherCaller)
		require.NoError(t, err)
		assert.Equal(t, newTitle, updated.Title)

		// Publish exam
		published, err := examSvc.PublishExam(ctx, created.ID, teacherCaller)
		require.NoError(t, err)
		assert.Equal(t, exam.StatusPublished, published.Status)
		assert.NotNil(t, published.PublishedAt)

		// Create assignment
		asgn := &assignment.Assignment{
			ExamID:     created.ID,
			StudentID:  student.ID,
			AssignedAt: time.Now().UTC(),
			Status:     assignment.StatusAssigned,
		}
		require.NoError(t, assignmentRepo.Create(ctx, asgn))

		// Emergency unpublish while active assignment exists -> must succeed and revert to DRAFT
		unpublished, err := examSvc.UnpublishExam(ctx, created.ID, teacherCaller)
		require.NoError(t, err)
		assert.Equal(t, exam.StatusDraft, unpublished.Status)

		// Attempt delete while active assignment exists -> must fail
		err = examSvc.DeleteExam(ctx, created.ID, teacherCaller)
		assert.ErrorIs(t, err, exam.ErrCannotDeleteActiveExam)

		// Revoke assignment
		require.NoError(t, assignmentRepo.Revoke(ctx, asgn.ID))

		// Archive exam
		archived, err := examSvc.ArchiveExam(ctx, created.ID, teacherCaller)
		require.NoError(t, err)
		assert.Equal(t, exam.StatusArchived, archived.Status)

		// Delete an exam that has no relations
		examToDelete, err := examSvc.CreateExam(ctx, exam.CreateExamRequest{
			Title:        "Disposable Exam",
			Subject:      "Test",
			DurationMins: 30,
		}, teacherCaller)
		require.NoError(t, err)

		err = examSvc.DeleteExam(ctx, examToDelete.ID, adminCaller)
		require.NoError(t, err)

		// Verify it is now deleted (not found)
		_, err = examSvc.GetExam(ctx, examToDelete.ID)
		assert.ErrorIs(t, err, exam.ErrExamNotFound)
	})
}
