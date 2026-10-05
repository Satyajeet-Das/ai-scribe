package exam

import (
	"context"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/rs/zerolog"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	platformauth "github.com/Satyajeet-Das/ai-scribe/internal/platform/auth"
)

type mockExamRepo struct {
	mu               sync.Mutex
	exams            map[uuid.UUID]*Exam
	hasRelations     map[uuid.UUID]bool
	assignedStudents map[string]bool
}

func newMockExamRepo() *mockExamRepo {
	return &mockExamRepo{
		exams:            make(map[uuid.UUID]*Exam),
		hasRelations:     make(map[uuid.UUID]bool),
		assignedStudents: make(map[string]bool),
	}
}

func (m *mockExamRepo) GetByID(ctx context.Context, id uuid.UUID) (*Exam, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if e, ok := m.exams[id]; ok && e.DeletedAt == nil {
		copy := *e
		return &copy, nil
	}
	return nil, ErrExamNotFound
}

func (m *mockExamRepo) List(ctx context.Context, params ListExamsParams) ([]Exam, int, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	var matched []Exam
	for _, e := range m.exams {
		if e.DeletedAt != nil {
			continue
		}
		if params.Status != nil && e.Status != *params.Status {
			continue
		}
		if params.Subject != "" && !strings.EqualFold(e.Subject, params.Subject) {
			continue
		}
		if params.CreatedBy != nil && e.CreatedBy != *params.CreatedBy {
			continue
		}
		if params.AssignedStudentID != nil && *params.AssignedStudentID != uuid.Nil {
			key := e.ID.String() + ":" + params.AssignedStudentID.String()
			if !m.assignedStudents[key] {
				continue
			}
		}
		if params.Search != "" {
			searchLower := strings.ToLower(params.Search)
			if !strings.Contains(strings.ToLower(e.Title), searchLower) &&
				!strings.Contains(strings.ToLower(e.Subject), searchLower) &&
				!strings.Contains(strings.ToLower(e.Description), searchLower) {
				continue
			}
		}
		matched = append(matched, *e)
	}

	total := len(matched)
	offset := params.Offset
	if offset > total {
		offset = total
	}
	end := offset + params.Limit
	if end > total {
		end = total
	}

	return matched[offset:end], total, nil
}

func (m *mockExamRepo) Create(ctx context.Context, e *Exam) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	copy := *e
	m.exams[e.ID] = &copy
	return nil
}

func (m *mockExamRepo) Update(ctx context.Context, e *Exam) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if existing, ok := m.exams[e.ID]; ok && existing.DeletedAt == nil {
		copy := *e
		m.exams[e.ID] = &copy
		return nil
	}
	return ErrExamNotFound
}

func (m *mockExamRepo) Publish(ctx context.Context, id uuid.UUID, publishedAt time.Time) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if e, ok := m.exams[id]; ok && e.DeletedAt == nil {
		if e.Status == StatusPublished {
			return ErrExamAlreadyPublished
		}
		if e.Status == StatusArchived {
			return ErrExamAlreadyArchived
		}
		if e.Status != StatusDraft {
			return ErrInvalidExamState
		}
		e.Status = StatusPublished
		e.PublishedAt = &publishedAt
		return nil
	}
	return ErrExamNotFound
}

func (m *mockExamRepo) Unpublish(ctx context.Context, id uuid.UUID) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if e, ok := m.exams[id]; ok && e.DeletedAt == nil {
		if e.Status == StatusDraft {
			return ErrExamNotPublished
		}
		if e.Status == StatusArchived {
			return ErrExamAlreadyArchived
		}
		if e.Status != StatusPublished {
			return ErrInvalidExamState
		}
		e.Status = StatusDraft
		e.PublishedAt = nil
		return nil
	}
	return ErrExamNotFound
}

func (m *mockExamRepo) Archive(ctx context.Context, id uuid.UUID) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if e, ok := m.exams[id]; ok && e.DeletedAt == nil {
		if e.Status == StatusArchived {
			return ErrExamAlreadyArchived
		}
		e.Status = StatusArchived
		return nil
	}
	return ErrExamNotFound
}

func (m *mockExamRepo) Delete(ctx context.Context, id uuid.UUID) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if e, ok := m.exams[id]; ok && e.DeletedAt == nil {
		now := time.Now().UTC()
		e.DeletedAt = &now
		return nil
	}
	return ErrExamNotFound
}

func (m *mockExamRepo) HasActiveSessionsOrAssignments(ctx context.Context, examID uuid.UUID) (bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.hasRelations[examID], nil
}

func (m *mockExamRepo) IsStudentAssigned(ctx context.Context, examID, studentID uuid.UUID) (bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.assignedStudents[examID.String()+":"+studentID.String()], nil
}

func TestExamService_CreateAndGet(t *testing.T) {
	repo := newMockExamRepo()
	logger := zerolog.Nop()
	svc := NewService(repo, &logger)
	ctx := context.Background()

	teacher := Caller{ID: uuid.New(), Role: platformauth.RoleTeacher}
	admin := Caller{ID: uuid.New(), Role: platformauth.RoleAdmin}
	otherTeacher := Caller{ID: uuid.New(), Role: platformauth.RoleTeacher}
	student := Caller{ID: uuid.New(), Role: platformauth.RoleStudent}

	req := CreateExamRequest{
		Title:        "Data Structures Exam",
		Subject:      "Computer Science",
		Description:  "Midterm covering trees and graphs",
		DurationMins: 90,
	}

	// Student cannot create exam
	_, err := svc.CreateExam(ctx, req, student)
	assert.ErrorIs(t, err, ErrInvalidCallerRole)

	// Teacher creates exam
	created, err := svc.CreateExam(ctx, req, teacher)
	require.NoError(t, err)
	require.NotNil(t, created)
	assert.Equal(t, "Data Structures Exam", created.Title)
	assert.Equal(t, StatusDraft, created.Status)
	assert.Equal(t, teacher.ID, created.CreatedBy)
	assert.Nil(t, created.PublishedAt)

	// Fetch internal (GetExam)
	fetched, err := svc.GetExam(ctx, created.ID)
	require.NoError(t, err)
	assert.Equal(t, created.ID, fetched.ID)

	// Fetch for owner teacher succeeds
	teacherFetched, err := svc.GetExamForCaller(ctx, created.ID, teacher)
	require.NoError(t, err)
	assert.Equal(t, created.ID, teacherFetched.ID)

	// Fetch for admin succeeds
	adminFetched, err := svc.GetExamForCaller(ctx, created.ID, admin)
	require.NoError(t, err)
	assert.Equal(t, created.ID, adminFetched.ID)

	// Fetch for other teacher fails (403 forbidden)
	_, err = svc.GetExamForCaller(ctx, created.ID, otherTeacher)
	assert.ErrorIs(t, err, ErrUnauthorizedCreator)

	// Fetch missing
	_, err = svc.GetExam(ctx, uuid.New())
	assert.ErrorIs(t, err, ErrExamNotFound)
}

func TestExamService_ListExams_AuthorizationAndFiltering(t *testing.T) {
	repo := newMockExamRepo()
	logger := zerolog.Nop()
	svc := NewService(repo, &logger)
	ctx := context.Background()

	teacher1 := Caller{ID: uuid.New(), Role: platformauth.RoleTeacher}
	teacher2 := Caller{ID: uuid.New(), Role: platformauth.RoleTeacher}
	admin := Caller{ID: uuid.New(), Role: platformauth.RoleAdmin}

	// Teacher 1 creates 2 exams
	_, err := svc.CreateExam(ctx, CreateExamRequest{
		Title:        "Physics 101",
		Subject:      "Physics",
		DurationMins: 60,
	}, teacher1)
	require.NoError(t, err)

	chemExam, err := svc.CreateExam(ctx, CreateExamRequest{
		Title:        "Chemistry 101",
		Subject:      "Chemistry",
		DurationMins: 45,
	}, teacher1)
	require.NoError(t, err)
	_, err = svc.PublishExam(ctx, chemExam.ID, teacher1)
	require.NoError(t, err)

	// Teacher 2 creates 1 exam
	_, err = svc.CreateExam(ctx, CreateExamRequest{
		Title:        "Biology 101",
		Subject:      "Biology",
		DurationMins: 90,
	}, teacher2)
	require.NoError(t, err)

	t.Run("teacher 1 only sees own exams", func(t *testing.T) {
		list, total, err := svc.ListExams(ctx, ListExamsParams{}, teacher1)
		require.NoError(t, err)
		assert.Equal(t, 2, total)
		assert.Len(t, list, 2)
		for _, e := range list {
			assert.Equal(t, teacher1.ID, e.CreatedBy)
		}
	})

	t.Run("teacher 2 only sees own exam", func(t *testing.T) {
		list, total, err := svc.ListExams(ctx, ListExamsParams{}, teacher2)
		require.NoError(t, err)
		assert.Equal(t, 1, total)
		assert.Len(t, list, 1)
		assert.Equal(t, teacher2.ID, list[0].CreatedBy)
	})

	t.Run("admin sees all exams", func(t *testing.T) {
		list, total, err := svc.ListExams(ctx, ListExamsParams{}, admin)
		require.NoError(t, err)
		assert.Equal(t, 3, total)
		assert.Len(t, list, 3)
	})

	t.Run("filter by status", func(t *testing.T) {
		pubStatus := StatusPublished
		list, total, err := svc.ListExams(ctx, ListExamsParams{Status: &pubStatus}, admin)
		require.NoError(t, err)
		assert.Equal(t, 1, total)
		assert.Len(t, list, 1)
		assert.Equal(t, "Chemistry 101", list[0].Title)
	})

	t.Run("filter by subject", func(t *testing.T) {
		list, total, err := svc.ListExams(ctx, ListExamsParams{Subject: "Physics"}, admin)
		require.NoError(t, err)
		assert.Equal(t, 1, total)
		assert.Equal(t, "Physics 101", list[0].Title)
	})

	t.Run("filter by search query", func(t *testing.T) {
		list, total, err := svc.ListExams(ctx, ListExamsParams{Search: "Bio"}, admin)
		require.NoError(t, err)
		assert.Equal(t, 1, total)
		assert.Equal(t, "Biology 101", list[0].Title)
	})

	t.Run("pagination limit and offset", func(t *testing.T) {
		list, total, err := svc.ListExams(ctx, ListExamsParams{Limit: 2, Offset: 0}, admin)
		require.NoError(t, err)
		assert.Equal(t, 3, total)
		assert.Len(t, list, 2)
	})
}

func TestExamService_Update_DraftAndPublished(t *testing.T) {
	repo := newMockExamRepo()
	logger := zerolog.Nop()
	svc := NewService(repo, &logger)
	ctx := context.Background()

	teacher := Caller{ID: uuid.New(), Role: platformauth.RoleTeacher}
	admin := Caller{ID: uuid.New(), Role: platformauth.RoleAdmin}
	otherTeacher := Caller{ID: uuid.New(), Role: platformauth.RoleTeacher}

	exam, err := svc.CreateExam(ctx, CreateExamRequest{
		Title:        "Calculus I",
		Subject:      "Mathematics",
		DurationMins: 60,
	}, teacher)
	require.NoError(t, err)

	t.Run("update by owner in DRAFT succeeds", func(t *testing.T) {
		newTitle := "Calculus I (Advanced)"
		newDuration := 75
		updated, err := svc.UpdateExam(ctx, exam.ID, UpdateExamRequest{
			Title:        &newTitle,
			DurationMins: &newDuration,
		}, teacher)
		require.NoError(t, err)
		assert.Equal(t, newTitle, updated.Title)
		assert.Equal(t, 75, updated.DurationMins)
	})

	t.Run("update by admin in DRAFT succeeds", func(t *testing.T) {
		desc := "Admin updated description"
		updated, err := svc.UpdateExam(ctx, exam.ID, UpdateExamRequest{
			Description: &desc,
		}, admin)
		require.NoError(t, err)
		assert.Equal(t, desc, updated.Description)
	})

	t.Run("update by unauthorized teacher fails", func(t *testing.T) {
		newTitle := "Hacked Title"
		_, err := svc.UpdateExam(ctx, exam.ID, UpdateExamRequest{
			Title: &newTitle,
		}, otherTeacher)
		assert.ErrorIs(t, err, ErrUnauthorizedCreator)
	})

	// Publish the exam
	_, err = svc.PublishExam(ctx, exam.ID, teacher)
	require.NoError(t, err)

	t.Run("published exam: altering duration (structural) fails", func(t *testing.T) {
		diffDuration := 90
		_, err := svc.UpdateExam(ctx, exam.ID, UpdateExamRequest{
			DurationMins: &diffDuration,
		}, teacher)
		assert.ErrorIs(t, err, ErrPublishedStructuralChange)
	})

	t.Run("published exam: non-structural update (description) succeeds", func(t *testing.T) {
		safeDesc := "Updated syllabus notes"
		updated, err := svc.UpdateExam(ctx, exam.ID, UpdateExamRequest{
			Description: &safeDesc,
		}, teacher)
		require.NoError(t, err)
		assert.Equal(t, safeDesc, updated.Description)
	})

	// Archive the exam
	_, err = svc.ArchiveExam(ctx, exam.ID, teacher)
	require.NoError(t, err)

	t.Run("archived exam: all updates fail", func(t *testing.T) {
		desc := "Will not work"
		_, err := svc.UpdateExam(ctx, exam.ID, UpdateExamRequest{
			Description: &desc,
		}, teacher)
		assert.ErrorIs(t, err, ErrExamAlreadyArchived)
	})
}

func TestExamService_PublishAndUnpublish_LifecycleRules(t *testing.T) {
	repo := newMockExamRepo()
	logger := zerolog.Nop()
	svc := NewService(repo, &logger)
	ctx := context.Background()

	teacher := Caller{ID: uuid.New(), Role: platformauth.RoleTeacher}
	admin := Caller{ID: uuid.New(), Role: platformauth.RoleAdmin}
	otherTeacher := Caller{ID: uuid.New(), Role: platformauth.RoleTeacher}

	exam, err := svc.CreateExam(ctx, CreateExamRequest{
		Title:        "World History",
		Subject:      "History",
		DurationMins: 60,
	}, teacher)
	require.NoError(t, err)

	t.Run("cannot unpublish a draft exam", func(t *testing.T) {
		_, err := svc.UnpublishExam(ctx, exam.ID, teacher)
		assert.ErrorIs(t, err, ErrExamNotPublished)
	})

	t.Run("unauthorized user cannot publish", func(t *testing.T) {
		_, err := svc.PublishExam(ctx, exam.ID, otherTeacher)
		assert.ErrorIs(t, err, ErrUnauthorizedCreator)
	})

	t.Run("owner publishes exam", func(t *testing.T) {
		published, err := svc.PublishExam(ctx, exam.ID, teacher)
		require.NoError(t, err)
		assert.Equal(t, StatusPublished, published.Status)
		assert.NotNil(t, published.PublishedAt)
	})

	t.Run("cannot publish already published exam", func(t *testing.T) {
		_, err := svc.PublishExam(ctx, exam.ID, teacher)
		assert.ErrorIs(t, err, ErrExamAlreadyPublished)
	})

	t.Run("emergency unpublish with active sessions succeeds and reverts to DRAFT", func(t *testing.T) {
		repo.hasRelations[exam.ID] = true
		reverted, err := svc.UnpublishExam(ctx, exam.ID, teacher)
		require.NoError(t, err)
		assert.Equal(t, StatusDraft, reverted.Status)
		assert.Nil(t, reverted.PublishedAt)
	})

	t.Run("admin can publish exam", func(t *testing.T) {
		published, err := svc.PublishExam(ctx, exam.ID, admin)
		require.NoError(t, err)
		assert.Equal(t, StatusPublished, published.Status)
	})

	t.Run("admin can unpublish exam", func(t *testing.T) {
		reverted, err := svc.UnpublishExam(ctx, exam.ID, admin)
		require.NoError(t, err)
		assert.Equal(t, StatusDraft, reverted.Status)
	})

	t.Run("archive exam transitions to ARCHIVED", func(t *testing.T) {
		archived, err := svc.ArchiveExam(ctx, exam.ID, teacher)
		require.NoError(t, err)
		assert.Equal(t, StatusArchived, archived.Status)
	})

	t.Run("cannot publish or unpublish archived exam", func(t *testing.T) {
		_, err := svc.PublishExam(ctx, exam.ID, teacher)
		assert.ErrorIs(t, err, ErrExamAlreadyArchived)

		_, err = svc.UnpublishExam(ctx, exam.ID, teacher)
		assert.ErrorIs(t, err, ErrExamAlreadyArchived)

		_, err = svc.ArchiveExam(ctx, exam.ID, teacher)
		assert.ErrorIs(t, err, ErrExamAlreadyArchived)
	})
}

func TestExamService_DeleteExam(t *testing.T) {
	repo := newMockExamRepo()
	logger := zerolog.Nop()
	svc := NewService(repo, &logger)
	ctx := context.Background()

	teacher := Caller{ID: uuid.New(), Role: platformauth.RoleTeacher}
	otherTeacher := Caller{ID: uuid.New(), Role: platformauth.RoleTeacher}

	exam, err := svc.CreateExam(ctx, CreateExamRequest{
		Title:        "Art History",
		Subject:      "Art",
		DurationMins: 45,
	}, teacher)
	require.NoError(t, err)

	t.Run("unauthorized delete fails", func(t *testing.T) {
		err := svc.DeleteExam(ctx, exam.ID, otherTeacher)
		assert.ErrorIs(t, err, ErrUnauthorizedCreator)
	})

	t.Run("delete with relations fails", func(t *testing.T) {
		repo.hasRelations[exam.ID] = true
		err := svc.DeleteExam(ctx, exam.ID, teacher)
		assert.ErrorIs(t, err, ErrCannotDeleteActiveExam)
	})

	t.Run("delete without relations succeeds", func(t *testing.T) {
		repo.hasRelations[exam.ID] = false
		err := svc.DeleteExam(ctx, exam.ID, teacher)
		require.NoError(t, err)

		// Exam should now return not found
		_, err = svc.GetExam(ctx, exam.ID)
		assert.ErrorIs(t, err, ErrExamNotFound)
	})
}

func TestExamService_ConcurrentUpdates(t *testing.T) {
	repo := newMockExamRepo()
	logger := zerolog.Nop()
	svc := NewService(repo, &logger)
	ctx := context.Background()

	teacher := Caller{ID: uuid.New(), Role: platformauth.RoleTeacher}

	exam, err := svc.CreateExam(ctx, CreateExamRequest{
		Title:        "Concurrent Test",
		Subject:      "CS",
		DurationMins: 60,
	}, teacher)
	require.NoError(t, err)

	const goroutines = 10
	var wg sync.WaitGroup
	wg.Add(goroutines)

	for i := 0; i < goroutines; i++ {
		go func(idx int) {
			defer wg.Done()
			title := "Updated Title"
			_, _ = svc.UpdateExam(ctx, exam.ID, UpdateExamRequest{Title: &title}, teacher)
		}(i)
	}

	wg.Wait()

	fetched, err := svc.GetExam(ctx, exam.ID)
	require.NoError(t, err)
	assert.Equal(t, "Updated Title", fetched.Title)
}

func TestExamService_StudentAccess(t *testing.T) {
	repo := newMockExamRepo()
	logger := zerolog.Nop()
	svc := NewService(repo, &logger)
	ctx := context.Background()

	teacher := Caller{ID: uuid.New(), Role: platformauth.RoleTeacher}
	student := Caller{ID: uuid.New(), Role: platformauth.RoleStudent}
	otherStudent := Caller{ID: uuid.New(), Role: platformauth.RoleStudent}

	// Create 2 exams: one draft, one published
	draftExam, err := svc.CreateExam(ctx, CreateExamRequest{
		Title:        "Draft Math Exam",
		Subject:      "Math",
		DurationMins: 45,
	}, teacher)
	require.NoError(t, err)

	publishedExam, err := svc.CreateExam(ctx, CreateExamRequest{
		Title:        "Published Science Exam",
		Subject:      "Science",
		DurationMins: 60,
	}, teacher)
	require.NoError(t, err)

	_, err = svc.PublishExam(ctx, publishedExam.ID, teacher)
	require.NoError(t, err)

	// Assign student to both exams (in mock repo)
	repo.assignedStudents[draftExam.ID.String()+":"+student.ID.String()] = true
	repo.assignedStudents[publishedExam.ID.String()+":"+student.ID.String()] = true

	t.Run("student sees only assigned published exams in ListExams", func(t *testing.T) {
		exams, total, err := svc.ListExams(ctx, ListExamsParams{}, student)
		require.NoError(t, err)
		assert.Equal(t, 1, total)
		assert.Equal(t, 1, len(exams))
		assert.Equal(t, publishedExam.ID, exams[0].ID)
	})

	t.Run("unassigned student sees no exams", func(t *testing.T) {
		exams, total, err := svc.ListExams(ctx, ListExamsParams{}, otherStudent)
		require.NoError(t, err)
		assert.Equal(t, 0, total)
		assert.Equal(t, 0, len(exams))
	})

	t.Run("student can get assigned published exam", func(t *testing.T) {
		e, err := svc.GetExamForCaller(ctx, publishedExam.ID, student)
		require.NoError(t, err)
		assert.Equal(t, publishedExam.ID, e.ID)
	})

	t.Run("student cannot get draft exam even if assigned", func(t *testing.T) {
		_, err := svc.GetExamForCaller(ctx, draftExam.ID, student)
		assert.ErrorIs(t, err, ErrExamNotFound)
	})

	t.Run("student cannot get exam if not assigned", func(t *testing.T) {
		_, err := svc.GetExamForCaller(ctx, publishedExam.ID, otherStudent)
		assert.ErrorIs(t, err, ErrUnauthorizedCreator)
	})
}
