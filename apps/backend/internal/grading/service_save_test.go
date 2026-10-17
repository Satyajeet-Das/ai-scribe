package grading

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Satyajeet-Das/ai-scribe/internal/audit"
	"github.com/Satyajeet-Das/ai-scribe/internal/review"
)

func strp(s string) *string { return &s }

// savable is a fixture with one essay worth 10 marks on a submitted paper.
type savable struct {
	*fixture
	question uuid.UUID
}

func newSavable() *savable {
	f := newFixture()
	q := uuid.New()
	f.reader.points[q] = 10
	f.reader.questions = []QuestionInput{{ID: q, Number: 1, Type: TypeEssay, Points: 10, Answered: true}}
	return &savable{fixture: f, question: q}
}

func (s *savable) save(req SaveMarkRequest, caller Caller) (*SaveMarkResult, error) {
	if req.SessionID == uuid.Nil {
		req.SessionID = s.reader.session.ID
	}
	return s.svc.SaveMark(context.Background(), s.examID, s.studentID, s.question, req, caller, AccessMeta{RequestID: "req-1", IP: "203.0.113.4"})
}

func TestSaveMark_SavesAMarkAndAComment(t *testing.T) {
	s := newSavable()
	res, err := s.save(SaveMarkRequest{MarksHalf: intp(15), Comment: strp("  well argued  "), ExpectedVersion: 0}, s.teacher)
	require.NoError(t, err)

	require.NotNil(t, res.Line.EffectiveHalf)
	assert.Equal(t, 15, *res.Line.EffectiveHalf)
	assert.Equal(t, SourceTeacher, res.Line.Source)
	assert.Equal(t, 1, res.Line.Version, "a first save is version 1")
	require.NotNil(t, res.Line.Comment)
	assert.Equal(t, "well argued", *res.Line.Comment, "the comment is trimmed")

	assert.Equal(t, 15, res.Paper.TotalHalf)
	assert.Equal(t, 20, res.Paper.MaxHalf)
	assert.Equal(t, 1, res.Paper.Marked)
	assert.Equal(t, 1, res.Paper.Version, "the paper's version moves with every change")
}

func TestSaveMark_EverythingIsCheckedUnderTheLock(t *testing.T) {
	s := newSavable()
	_, err := s.save(SaveMarkRequest{MarksHalf: intp(10)}, s.teacher)
	require.NoError(t, err)

	order := s.reader.order
	idx := func(name string) int {
		for i, n := range order {
			if n == name {
				return i
			}
		}
		return -1
	}
	require.NotEqual(t, -1, idx("LockPaper"))
	assert.Less(t, idx("Session"), idx("LockPaper"))
	for _, after := range []string{"Question", "Mark", "UpsertMark", "BumpPaper", "Audit"} {
		assert.Less(t, idx("LockPaper"), idx(after),
			"%s is read or written only once the paper is locked, so nothing can change under it", after)
	}
	assert.Less(t, idx("UpsertMark"), idx("Audit"))
}

func TestSaveMark_AuditsEveryChangeInTheSameTransactionWithoutTheComment(t *testing.T) {
	s := newSavable()
	secret := "this essay copied from Wikipedia"
	_, err := s.save(SaveMarkRequest{MarksHalf: intp(15), Comment: strp(secret)}, s.teacher)
	require.NoError(t, err)

	require.Len(t, s.reader.audited, 1)
	e := s.reader.audited[0]
	assert.Equal(t, audit.ActionMarkSave, e.Action)
	assert.Equal(t, audit.ResourceAnswerMark, e.ResourceType)
	assert.Equal(t, s.question, e.ResourceID)
	assert.Equal(t, s.reader.session.ID, e.SessionID)
	assert.Equal(t, s.teacher.ID, e.ActorID)
	assert.Equal(t, "TEACHER", e.ActorRole)
	assert.Equal(t, "req-1", e.RequestID)
	assert.Equal(t, "203.0.113.4", e.IP)

	assert.Equal(t, intp(15), e.Metadata["marks_half"])
	assert.Equal(t, 20, e.Metadata["max_half"])
	assert.Equal(t, true, e.Metadata["comment_changed"])
	assert.Equal(t, false, e.Metadata["cleared"])
	assert.Equal(t, 1, e.Metadata["version"])
	assert.Equal(t, s.examID.String(), e.Metadata["exam_id"])

	for k, v := range e.Metadata {
		assert.NotContains(t, strings.ToLower(toString(v)), "wikipedia", "metadata %q must not carry the comment", k)
	}
}

func toString(v any) string {
	switch x := v.(type) {
	case string:
		return x
	case *string:
		if x != nil {
			return *x
		}
	}
	return ""
}

func TestSaveMark_AFailedAuditTakesTheMarkWithIt(t *testing.T) {
	s := newSavable()
	s.reader.auditErr = errors.New("audit database down")

	_, err := s.save(SaveMarkRequest{MarksHalf: intp(15)}, s.teacher)
	assert.ErrorIs(t, err, ErrAuditUnavailable)

	assert.Empty(t, s.reader.marks, "the mark was undone: no change without its record")
	assert.Nil(t, s.reader.paper, "the paper's grading record, created by the same transaction, is gone too")
	assert.Empty(t, s.reader.audited)

	// Once the audit log is back the same request goes through.
	s.reader.auditErr = nil
	res, err := s.save(SaveMarkRequest{MarksHalf: intp(15)}, s.teacher)
	require.NoError(t, err)
	assert.Equal(t, 1, res.Line.Version)
}

func TestSaveMark_ValidatesMarks(t *testing.T) {
	cases := map[string]struct {
		marks *int
		ok    bool
		text  string
	}{
		"zero":             {intp(0), true, ""},
		"half a mark":      {intp(1), true, ""},
		"the maximum":      {intp(20), true, ""},
		"half a mark over": {intp(21), false, "more than 10"},
		"far over":         {intp(2000), false, "more than 10"},
		"negative":         {intp(-1), false, "negative"},
		"no mark (clears)": {nil, true, ""},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			s := newSavable()
			_, err := s.save(SaveMarkRequest{MarksHalf: tc.marks}, s.teacher)
			if tc.ok {
				assert.NoError(t, err)
				return
			}
			require.Error(t, err)
			assert.ErrorIs(t, err, ErrInvalidRequest)
			var ve *ValidationError
			require.True(t, errors.As(err, &ve))
			assert.Equal(t, "marks", ve.Field)
			assert.Contains(t, ve.Message, tc.text)
			assert.Zero(t, s.reader.calls["UpsertMark"], "nothing was written")
			assert.Empty(t, s.reader.audited)
		})
	}

	t.Run("a zero-point question takes only 0", func(t *testing.T) {
		s := newSavable()
		s.reader.points[s.question] = 0
		_, err := s.save(SaveMarkRequest{MarksHalf: intp(0)}, s.teacher)
		assert.NoError(t, err)
		_, err = s.save(SaveMarkRequest{MarksHalf: intp(1), ExpectedVersion: 1}, s.teacher)
		assert.ErrorIs(t, err, ErrInvalidRequest)
	})
	t.Run("the maximum is the question's points now", func(t *testing.T) {
		s := newSavable()
		s.reader.points[s.question] = 3 // lowered since the paper was last looked at
		_, err := s.save(SaveMarkRequest{MarksHalf: intp(7)}, s.teacher)
		assert.ErrorIs(t, err, ErrInvalidRequest)
		_, err = s.save(SaveMarkRequest{MarksHalf: intp(6)}, s.teacher)
		assert.NoError(t, err)
	})
}

func TestSaveMark_ValidatesComments(t *testing.T) {
	s := newSavable()

	long := strings.Repeat("é", MaxCommentRunes) // 2000 characters, 4000 bytes
	_, err := s.save(SaveMarkRequest{Comment: &long}, s.teacher)
	assert.NoError(t, err, "the limit counts characters, not bytes")

	tooLong := long + "x"
	_, err = s.save(SaveMarkRequest{Comment: &tooLong, ExpectedVersion: 1}, s.teacher)
	var ve *ValidationError
	require.True(t, errors.As(err, &ve))
	assert.Equal(t, "comment", ve.Field)

	nul := "a\x00b"
	_, err = s.save(SaveMarkRequest{Comment: &nul, ExpectedVersion: 1}, s.teacher)
	require.True(t, errors.As(err, &ve), "text a database cannot store is refused with a reason")
	assert.Contains(t, ve.Message, "cannot be saved")

	_, err = s.save(SaveMarkRequest{MarksHalf: intp(1), ExpectedVersion: -1}, s.teacher)
	require.True(t, errors.As(err, &ve))
	assert.Equal(t, "expectedVersion", ve.Field)

	t.Run("a blank comment is no comment", func(t *testing.T) {
		g := newSavable()
		res, err := g.save(SaveMarkRequest{MarksHalf: intp(4), Comment: strp("   \n ")}, g.teacher)
		require.NoError(t, err)
		assert.Nil(t, res.Line.Comment)
	})
}

func TestSaveMark_RefusesAStaleVersion(t *testing.T) {
	s := newSavable()
	_, err := s.save(SaveMarkRequest{MarksHalf: intp(10), ExpectedVersion: 0}, s.teacher)
	require.NoError(t, err) // now at version 1

	t.Run("someone else saved first", func(t *testing.T) {
		_, err := s.save(SaveMarkRequest{MarksHalf: intp(12), ExpectedVersion: 0}, s.teacher)
		assert.ErrorIs(t, err, ErrStaleVersion)
		assert.Equal(t, 10, *s.reader.marks[s.question].MarksHalf, "their mark is untouched")
		assert.Len(t, s.reader.audited, 1, "and a refused save is not recorded as a change")
	})
	t.Run("the current version is accepted", func(t *testing.T) {
		res, err := s.save(SaveMarkRequest{MarksHalf: intp(12), ExpectedVersion: 1}, s.teacher)
		require.NoError(t, err)
		assert.Equal(t, 2, res.Line.Version)
	})
	t.Run("a version from the future is refused too", func(t *testing.T) {
		_, err := s.save(SaveMarkRequest{MarksHalf: intp(14), ExpectedVersion: 99}, s.teacher)
		assert.ErrorIs(t, err, ErrStaleVersion)
	})
}

func TestSaveMark_ARepeatedRequestChangesNothing(t *testing.T) {
	s := newSavable()
	req := SaveMarkRequest{MarksHalf: intp(15), Comment: strp("good"), ExpectedVersion: 0}
	first, err := s.save(req, s.teacher)
	require.NoError(t, err)

	// The reply was lost, so the client sends exactly the same thing again, still
	// carrying the version it had before the first attempt.
	again, err := s.save(req, s.teacher)
	require.NoError(t, err, "a retry must not be reported as a conflict")

	assert.Equal(t, first.Line.Version, again.Line.Version, "no new version")
	assert.Equal(t, first.Paper.Version, again.Paper.Version)
	assert.Len(t, s.reader.audited, 1, "and no second audit entry")
	assert.Equal(t, 1, s.reader.calls["UpsertMark"])

	t.Run("saving nothing for an answer with nothing saved is also a no-op", func(t *testing.T) {
		g := newSavable()
		res, err := g.save(SaveMarkRequest{}, g.teacher)
		require.NoError(t, err)
		assert.Zero(t, res.Line.Version)
		assert.Zero(t, g.reader.calls["UpsertMark"])
		assert.Zero(t, res.Paper.Version)
	})
}

func TestSaveMark_ClearingAndCommentOnlySaves(t *testing.T) {
	right := uuid.New()
	s := newSavable()
	auto := uuid.New()
	s.reader.points[auto] = 2
	s.reader.questions = append(s.reader.questions, QuestionInput{ID: auto, Number: 2, Type: TypeMCQ, Points: 2,
		Answered: true, SelectedOptionID: &right, CorrectOptionIDs: []uuid.UUID{right}})
	saveAuto := func(req SaveMarkRequest) (*SaveMarkResult, error) {
		req.SessionID = s.reader.session.ID
		return s.svc.SaveMark(context.Background(), s.examID, s.studentID, auto, req, s.teacher, AccessMeta{})
	}

	over, err := saveAuto(SaveMarkRequest{MarksHalf: intp(1), ExpectedVersion: 0}) // 0.5, a deduction
	require.NoError(t, err)
	assert.Equal(t, 1, *over.Line.EffectiveHalf)
	assert.Equal(t, SourceTeacher, over.Line.Source)

	cleared, err := saveAuto(SaveMarkRequest{ExpectedVersion: 1})
	require.NoError(t, err)
	assert.Equal(t, SourceAuto, cleared.Line.Source, "the automatic mark applies again")
	assert.Equal(t, 4, *cleared.Line.EffectiveHalf)
	assert.Nil(t, cleared.Line.TeacherHalf)
	assert.Equal(t, true, s.reader.audited[len(s.reader.audited)-1].Metadata["cleared"])

	commented, err := saveAuto(SaveMarkRequest{Comment: strp("check the units"), ExpectedVersion: 2})
	require.NoError(t, err)
	assert.Equal(t, SourceAuto, commented.Line.Source, "a comment alone is not a mark")
	require.NotNil(t, commented.Line.Comment)
}

func TestSaveMark_OnlyForAPaperThatHasEndedAndIsStillADraft(t *testing.T) {
	req := SaveMarkRequest{MarksHalf: intp(4)}

	t.Run("still being sat", func(t *testing.T) {
		s := newSavable()
		s.reader.session = &review.SessionInfo{ID: uuid.New(), Status: review.StatusInProgress, StartedAt: time.Now()}
		_, err := s.save(req, s.teacher)
		assert.ErrorIs(t, err, ErrSessionNotEnded)
		assert.Zero(t, s.reader.calls["LockPaper"])
	})
	t.Run("never started", func(t *testing.T) {
		s := newSavable()
		s.reader.session = nil
		_, err := s.save(SaveMarkRequest{SessionID: uuid.New(), MarksHalf: intp(4)}, s.teacher)
		assert.ErrorIs(t, err, ErrSessionNotEnded)
	})
	t.Run("expired is fine", func(t *testing.T) {
		s := newSavable()
		s.reader.session.Status = review.StatusExpired
		_, err := s.save(req, s.teacher)
		assert.NoError(t, err)
	})
	t.Run("a different attempt is now the one shown", func(t *testing.T) {
		s := newSavable()
		_, err := s.save(SaveMarkRequest{SessionID: uuid.New(), MarksHalf: intp(4)}, s.teacher)
		assert.ErrorIs(t, err, ErrSessionChanged)
		assert.Zero(t, s.reader.calls["UpsertMark"], "the other attempt's paper was not marked")
	})
	t.Run("finalised", func(t *testing.T) {
		s := newSavable()
		s.reader.paper = &PaperRow{SessionID: s.reader.session.ID, Status: StatusFinal, Version: 4}
		_, err := s.save(req, s.teacher)
		assert.ErrorIs(t, err, ErrPaperFinalised)
		assert.Zero(t, s.reader.calls["UpsertMark"])
		assert.Equal(t, 4, s.reader.paper.Version)
	})
	t.Run("a question that is not in this exam", func(t *testing.T) {
		s := newSavable()
		delete(s.reader.points, s.question)
		_, err := s.save(req, s.teacher)
		assert.ErrorIs(t, err, ErrExamNotFound)
	})
	t.Run("a student who is not assigned", func(t *testing.T) {
		s := newSavable()
		s.reader.sessionErr = review.ErrStudentNotAssigned
		_, err := s.save(req, s.teacher)
		assert.ErrorIs(t, err, ErrExamNotFound)
	})
}

func TestSaveMark_OnlyTheOwnerOrAnAdministrator(t *testing.T) {
	req := SaveMarkRequest{MarksHalf: intp(4)}

	t.Run("an administrator may", func(t *testing.T) {
		s := newSavable()
		_, err := s.save(req, s.admin)
		assert.NoError(t, err)
		assert.Equal(t, "ADMIN", s.reader.audited[0].ActorRole)
	})
	t.Run("nobody else may, and nothing is even opened for them", func(t *testing.T) {
		for name, who := range map[string]func(*savable) Caller{
			"another teacher": func(s *savable) Caller { return s.other },
			"a student":       func(s *savable) Caller { return s.student },
			"a proctor":       func(s *savable) Caller { return s.proctor },
		} {
			s := newSavable()
			_, err := s.save(req, who(s))
			assert.Error(t, err, name)
			assert.Empty(t, s.reader.calls, name)
			assert.Empty(t, s.reader.marks, name)
		}
	})
}

func TestSaveMark_AStorageErrorIsNotHidden(t *testing.T) {
	s := newSavable()
	boom := errors.New("disk full")
	s.reader.upsertErr = boom
	_, err := s.save(SaveMarkRequest{MarksHalf: intp(4)}, s.teacher)
	assert.ErrorIs(t, err, boom)
	assert.Empty(t, s.reader.audited)
}
