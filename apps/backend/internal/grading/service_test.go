package grading

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Satyajeet-Das/ai-scribe/internal/audit"
	"github.com/Satyajeet-Das/ai-scribe/internal/exam"
	"github.com/Satyajeet-Das/ai-scribe/internal/model"
	platformauth "github.com/Satyajeet-Das/ai-scribe/internal/platform/auth"
	"github.com/Satyajeet-Das/ai-scribe/internal/review"
)

// fakeReader serves one paper and counts what was read, so a test can say that
// something was not read at all.
type fakeReader struct {
	session    *review.SessionInfo
	sessionErr error
	paper      *PaperRow
	marks      map[uuid.UUID]MarkRow
	questions  []QuestionInput
	calls      map[string]int

	// For writes. order is every operation in the order it happened.
	order     []string
	points    map[uuid.UUID]int
	audited   []audit.Event
	auditErr  error
	upsertErr error
	events    []GradingEvent
}

func (f *fakeReader) count(name string) {
	f.calls[name]++
	f.order = append(f.order, name)
}

func (f *fakeReader) Session(context.Context, uuid.UUID, uuid.UUID) (*review.SessionInfo, error) {
	f.count("Session")
	return f.session, f.sessionErr
}

func (f *fakeReader) Paper(context.Context, uuid.UUID) (*PaperRow, error) {
	f.count("Paper")
	return f.paper, nil
}

func (f *fakeReader) Marks(context.Context, uuid.UUID) (map[uuid.UUID]MarkRow, error) {
	f.count("Marks")
	return f.marks, nil
}

func (f *fakeReader) Questions(context.Context, uuid.UUID, uuid.UUID) ([]QuestionInput, error) {
	f.count("Questions")
	return f.questions, nil
}

type fakeRepo struct{ r *fakeReader }

func (f fakeRepo) Read(_ context.Context, fn func(Reader) error) error { return fn(f.r) }

// Write runs fn and, like a database transaction, undoes everything it changed
// if fn fails.
func (f fakeRepo) Write(_ context.Context, fn func(Writer) error) error {
	r := f.r
	savedPaper := copyPaper(r.paper)
	savedMarks := make(map[uuid.UUID]MarkRow, len(r.marks))
	for k, v := range r.marks {
		savedMarks[k] = v
	}
	savedAudit := append([]audit.Event(nil), r.audited...)
	savedEvents := append([]GradingEvent(nil), r.events...)
	if err := fn(r); err != nil {
		r.paper, r.marks, r.audited, r.events = savedPaper, savedMarks, savedAudit, savedEvents
		return err
	}
	return nil
}

func copyPaper(p *PaperRow) *PaperRow {
	if p == nil {
		return nil
	}
	c := *p
	return &c
}

func (f *fakeReader) LockPaper(_ context.Context, _, sessionID uuid.UUID) (*PaperRow, error) {
	f.count("LockPaper")
	if f.paper == nil {
		f.paper = &PaperRow{SessionID: sessionID, Status: StatusDraft}
	}
	return copyPaper(f.paper), nil
}

func (f *fakeReader) Question(_ context.Context, _, questionID uuid.UUID) (*QuestionPoints, error) {
	f.count("Question")
	if pts, ok := f.points[questionID]; ok {
		return &QuestionPoints{ID: questionID, Points: pts}, nil
	}
	return nil, nil
}

func (f *fakeReader) Mark(_ context.Context, _, questionID uuid.UUID) (*MarkRow, error) {
	f.count("Mark")
	if m, ok := f.marks[questionID]; ok {
		return &m, nil
	}
	return nil, nil
}

func (f *fakeReader) UpsertMark(_ context.Context, _, questionID uuid.UUID, in MarkWrite) (MarkRow, error) {
	f.count("UpsertMark")
	if f.upsertErr != nil {
		return MarkRow{}, f.upsertErr
	}
	if f.marks[questionID].Version != in.ExpectedVersion {
		return MarkRow{}, ErrStaleVersion
	}
	m := MarkRow{QuestionID: questionID, MarksHalf: in.MarksHalf, MaxHalf: in.MaxHalf, Comment: in.Comment,
		Version: f.marks[questionID].Version + 1, MarkedAt: time.Now()}
	f.marks[questionID] = m
	return m, nil
}

func (f *fakeReader) BumpPaper(context.Context, uuid.UUID) (int, error) {
	f.count("BumpPaper")
	f.paper.Version++
	return f.paper.Version, nil
}

func (f *fakeReader) Audit(_ context.Context, e audit.Event) error {
	f.count("Audit")
	if f.auditErr != nil {
		return f.auditErr
	}
	f.audited = append(f.audited, e)
	return nil
}

func (f *fakeReader) SetFinal(_ context.Context, _, by uuid.UUID, snap Snapshot, total, max int) (int, error) {
	f.count("SetFinal")
	now := time.Now()
	f.paper.Status, f.paper.FinalisedBy, f.paper.FinalisedAt = StatusFinal, &by, &now
	f.paper.TotalHalf, f.paper.MaxHalf, f.paper.Snapshot = &total, &max, &snap
	f.paper.Version++
	return f.paper.Version, nil
}

func (f *fakeReader) SetDraft(context.Context, uuid.UUID) (int, error) {
	f.count("SetDraft")
	f.paper.Status, f.paper.FinalisedBy, f.paper.FinalisedAt = StatusDraft, nil, nil
	f.paper.TotalHalf, f.paper.MaxHalf, f.paper.Snapshot = nil, nil, nil
	f.paper.Version++
	return f.paper.Version, nil
}

func (f *fakeReader) AddEvent(_ context.Context, e GradingEvent) error {
	f.count("AddEvent")
	f.events = append(f.events, e)
	return nil
}

type fakeExams struct {
	owner uuid.UUID
	exam  *exam.Exam
}

func (f *fakeExams) GetExamForCaller(_ context.Context, id uuid.UUID, c exam.Caller) (*exam.Exam, error) {
	if f.exam == nil || f.exam.ID != id {
		return nil, exam.ErrExamNotFound
	}
	if !c.CanManage(f.owner) {
		return nil, exam.ErrUnauthorizedCreator
	}
	return f.exam, nil
}

type fixture struct {
	svc       Service
	reader    *fakeReader
	examID    uuid.UUID
	studentID uuid.UUID
	teacher   Caller
	other     Caller
	admin     Caller
	student   Caller
	proctor   Caller
}

func submitted() *review.SessionInfo {
	return &review.SessionInfo{ID: uuid.New(), Status: review.StatusSubmitted, StartedAt: time.Now()}
}

func newFixture() *fixture {
	teacher := Caller{ID: uuid.New(), Role: platformauth.RoleTeacher}
	examID := uuid.New()
	ex := &exam.Exam{Title: "Physics", Subject: "Science", DurationMins: 60, Status: exam.StatusPublished}
	ex.Base = model.Base{BaseWithId: model.BaseWithId{ID: examID}}
	ex.CreatedBy = teacher.ID
	reader := &fakeReader{session: submitted(), calls: map[string]int{}, marks: map[uuid.UUID]MarkRow{}, points: map[uuid.UUID]int{}}
	return &fixture{
		svc:       NewService(fakeRepo{reader}, &fakeExams{owner: teacher.ID, exam: ex}, nil),
		reader:    reader,
		examID:    examID,
		studentID: uuid.New(),
		teacher:   teacher,
		other:     Caller{ID: uuid.New(), Role: platformauth.RoleTeacher},
		admin:     Caller{ID: uuid.New(), Role: platformauth.RoleAdmin},
		student:   Caller{ID: uuid.New(), Role: platformauth.RoleStudent},
		proctor:   Caller{ID: uuid.New(), Role: platformauth.RoleProctor},
	}
}

func (f *fixture) get(caller Caller) (*Marking, error) {
	return f.svc.GetMarking(context.Background(), f.examID, f.studentID, caller)
}

func TestGetMarking_Authorization(t *testing.T) {
	f := newFixture()

	t.Run("the exam's owner and an administrator may mark", func(t *testing.T) {
		for _, c := range []Caller{f.teacher, f.admin} {
			_, err := f.get(c)
			assert.NoError(t, err)
		}
	})
	t.Run("students and proctors may not, whatever they ask for", func(t *testing.T) {
		for _, c := range []Caller{f.student, f.proctor, {ID: uuid.New(), Role: ""}} {
			_, err := f.get(c)
			assert.ErrorIs(t, err, ErrForbiddenRole, string(c.Role))
		}
	})
	t.Run("an anonymous caller may not", func(t *testing.T) {
		_, err := f.get(Caller{Role: platformauth.RoleTeacher})
		assert.ErrorIs(t, err, ErrForbiddenRole)
	})
	t.Run("another teacher cannot tell the exam exists", func(t *testing.T) {
		_, err := f.get(f.other)
		assert.ErrorIs(t, err, ErrExamNotFound)

		_, err = f.svc.GetMarking(context.Background(), uuid.New(), f.studentID, f.teacher)
		assert.ErrorIs(t, err, ErrExamNotFound, "an exam that does not exist looks the same")
	})
	t.Run("a refused caller learns nothing and nothing is read", func(t *testing.T) {
		g := newFixture()
		_, _ = g.get(g.other)
		_, _ = g.get(g.student)
		assert.Empty(t, g.reader.calls)
	})
	t.Run("a student who is not assigned to the exam is not found", func(t *testing.T) {
		g := newFixture()
		g.reader.sessionErr = review.ErrStudentNotAssigned
		_, err := g.get(g.teacher)
		assert.ErrorIs(t, err, ErrExamNotFound)
	})
}

func TestGetMarking_NothingToMarkYet(t *testing.T) {
	t.Run("a student who has not started", func(t *testing.T) {
		f := newFixture()
		f.reader.session = nil
		m, err := f.get(f.teacher)
		require.NoError(t, err)
		assert.False(t, m.MarkingAllowed)
		assert.Equal(t, ReasonNotStarted, m.Reason)
		assert.Nil(t, m.SessionID)
		assert.Empty(t, m.Lines)
	})
	t.Run("a student still sitting the exam: no answers, no key, nothing read", func(t *testing.T) {
		f := newFixture()
		f.reader.session = &review.SessionInfo{ID: uuid.New(), Status: review.StatusInProgress, StartedAt: time.Now()}
		f.reader.questions = []QuestionInput{mcq(2, true, nil, uuid.New())}
		m, err := f.get(f.teacher)
		require.NoError(t, err)
		assert.False(t, m.MarkingAllowed)
		assert.Equal(t, ReasonSessionNotEnded, m.Reason)
		assert.Equal(t, review.StatusInProgress, m.SessionStatus)
		assert.NotNil(t, m.SessionID)
		assert.Empty(t, m.Lines)
		assert.Zero(t, f.reader.calls["Questions"], "the questions, and so the answer key, were not even loaded")
	})
	t.Run("both ways a paper can end are markable", func(t *testing.T) {
		for _, status := range []review.Status{review.StatusSubmitted, review.StatusExpired} {
			f := newFixture()
			f.reader.session = &review.SessionInfo{ID: uuid.New(), Status: status, StartedAt: time.Now()}
			m, err := f.get(f.teacher)
			require.NoError(t, err)
			assert.True(t, m.MarkingAllowed, string(status))
			assert.Empty(t, m.Reason)
		}
	})
}

func TestGetMarking_ADraftPaper(t *testing.T) {
	f := newFixture()
	right, wrong := uuid.New(), uuid.New()

	q1 := QuestionInput{ID: uuid.New(), Number: 1, Type: TypeMCQ, Points: 2, Answered: true,
		SelectedOptionID: &right, CorrectOptionIDs: []uuid.UUID{right}}
	q2 := QuestionInput{ID: uuid.New(), Number: 2, Type: TypeEssay, Points: 10, Answered: true}
	q3 := QuestionInput{ID: uuid.New(), Number: 3, Type: TypeEssay, Points: 5, Answered: true}
	q4 := QuestionInput{ID: uuid.New(), Number: 4, Type: TypeMCQ, Points: 2, Answered: true,
		SelectedOptionID: &wrong, CorrectOptionIDs: []uuid.UUID{right}}
	f.reader.questions = []QuestionInput{q1, q2, q3, q4}
	comment := "clear and complete"
	f.reader.marks[q2.ID] = MarkRow{QuestionID: q2.ID, MarksHalf: intp(15), MaxHalf: 20, Comment: &comment, Version: 3, MarkedAt: time.Now()}
	// A mark for a question that has since been removed from the exam is ignored.
	f.reader.marks[uuid.New()] = MarkRow{MarksHalf: intp(4), MaxHalf: 4, Version: 1}
	f.reader.paper = &PaperRow{SessionID: uuid.New(), Status: StatusDraft, Version: 7}

	m, err := f.get(f.teacher)
	require.NoError(t, err)

	assert.True(t, m.MarkingAllowed)
	require.Len(t, m.Lines, 4, "one line per question of the exam, in order")
	for i, l := range m.Lines {
		assert.Equal(t, i+1, l.QuestionNumber)
	}

	assert.Equal(t, SourceAuto, m.Lines[0].Source)
	assert.Equal(t, 4, *m.Lines[0].EffectiveHalf)
	assert.Equal(t, SourceTeacher, m.Lines[1].Source)
	assert.Equal(t, 15, *m.Lines[1].EffectiveHalf)
	assert.Equal(t, 3, m.Lines[1].Version)
	assert.Equal(t, SourceNone, m.Lines[2].Source, "an unmarked essay")
	assert.Equal(t, 0, *m.Lines[3].EffectiveHalf, "a wrong answer")

	assert.Equal(t, StatusDraft, m.Paper.Status)
	assert.Equal(t, 7, m.Paper.Version)
	assert.Equal(t, 4+15+0, m.Paper.TotalHalf)
	assert.Equal(t, 4+20+10+4, m.Paper.MaxHalf)
	assert.Equal(t, 3, m.Paper.Marked)
	assert.Equal(t, 4, m.Paper.Questions)

	t.Run("the teacher is shown the correct option while marking", func(t *testing.T) {
		require.NotNil(t, m.Lines[0].CorrectOptionID)
		assert.Equal(t, right, *m.Lines[0].CorrectOptionID)
		assert.Nil(t, m.Lines[1].CorrectOptionID, "an essay has none")
	})
	t.Run("a paper with no grading record yet is a draft at version 0", func(t *testing.T) {
		g := newFixture()
		g.reader.questions = []QuestionInput{q2}
		out, err := g.get(g.teacher)
		require.NoError(t, err)
		assert.Equal(t, StatusDraft, out.Paper.Status)
		assert.Zero(t, out.Paper.Version)
	})
	t.Run("a mark over the question's current maximum is flagged and not counted", func(t *testing.T) {
		g := newFixture()
		small := QuestionInput{ID: uuid.New(), Number: 1, Type: TypeEssay, Points: 3, Answered: true} // was 10
		g.reader.questions = []QuestionInput{small}
		g.reader.marks[small.ID] = MarkRow{QuestionID: small.ID, MarksHalf: intp(18), MaxHalf: 20, Version: 2}
		out, err := g.get(g.teacher)
		require.NoError(t, err)
		assert.True(t, out.Lines[0].ExceedsMax)
		assert.Zero(t, out.Paper.TotalHalf)
		assert.Zero(t, out.Paper.Marked)
	})
}

func TestGetMarking_AFinalPaperIsShownAsItWasFrozen(t *testing.T) {
	f := newFixture()
	finalAt := time.Date(2026, 3, 2, 9, 0, 0, 0, time.UTC)
	by := uuid.New()
	q := uuid.New()
	comment := "frozen comment"
	f.reader.paper = &PaperRow{
		SessionID: uuid.New(), Status: StatusFinal, Version: 12, FinalisedAt: &finalAt, FinalisedBy: &by,
		TotalHalf: intp(17), MaxHalf: intp(24),
		Snapshot: &Snapshot{Version: 1, Lines: []SnapshotLine{
			{QuestionID: q, QuestionNumber: 1, Type: TypeMCQ, MaxHalf: 4, AwardedHalf: 4, Source: SourceAuto},
			{QuestionID: uuid.New(), QuestionNumber: 2, Type: TypeEssay, MaxHalf: 20, AwardedHalf: 13, Source: SourceTeacher, Comment: &comment},
		}},
	}
	// The questions as they are now would give different marks.
	f.reader.questions = []QuestionInput{{ID: q, Number: 1, Type: TypeMCQ, Points: 99}}

	m, err := f.get(f.teacher)
	require.NoError(t, err)

	assert.Zero(t, f.reader.calls["Questions"], "a final paper never looks at the live questions")
	assert.Zero(t, f.reader.calls["Marks"])
	assert.Equal(t, StatusFinal, m.Paper.Status)
	assert.Equal(t, 12, m.Paper.Version)
	assert.Equal(t, 17, m.Paper.TotalHalf)
	assert.Equal(t, 24, m.Paper.MaxHalf)
	assert.Equal(t, 2, m.Paper.Marked)
	require.NotNil(t, m.Paper.FinalisedAt)
	assert.Equal(t, finalAt, *m.Paper.FinalisedAt)
	require.NotNil(t, m.Paper.FinalisedBy)
	assert.Equal(t, by, *m.Paper.FinalisedBy)

	require.Len(t, m.Lines, 2)
	assert.Equal(t, 4, m.Lines[0].MaxHalf, "the maximum from the snapshot, not the 99 points it has now")
	assert.Equal(t, 13, *m.Lines[1].EffectiveHalf)
	assert.Equal(t, SourceTeacher, m.Lines[1].Source)
	require.NotNil(t, m.Lines[1].Comment)
	assert.Equal(t, comment, *m.Lines[1].Comment)
	for _, l := range m.Lines {
		assert.True(t, l.FromSnapshot)
		assert.Nil(t, l.CorrectOptionID, "no answer key for a final paper")
		assert.Nil(t, l.SuggestedHalf)
	}
	assert.True(t, m.MarkingAllowed, "it can still be reopened")
}

func TestGetMarking_AFinalRowWithoutASnapshotIsNotTrusted(t *testing.T) {
	f := newFixture()
	f.reader.paper = &PaperRow{SessionID: uuid.New(), Status: StatusFinal, Version: 3}
	f.reader.questions = []QuestionInput{{ID: uuid.New(), Number: 1, Type: TypeEssay, Points: 5, Answered: true}}
	m, err := f.get(f.teacher)
	require.NoError(t, err)
	assert.Equal(t, StatusDraft, m.Paper.Status, "the database forbids this; if it ever happened it must not look final")
	assert.Len(t, m.Lines, 1)
}

func TestGetMarking_ReadErrorsAreNotHidden(t *testing.T) {
	f := newFixture()
	boom := errors.New("connection lost")
	f.reader.sessionErr = boom
	_, err := f.get(f.teacher)
	assert.ErrorIs(t, err, boom)
}
