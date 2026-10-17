package grading

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Satyajeet-Das/ai-scribe/internal/audit"
	"github.com/Satyajeet-Das/ai-scribe/internal/review"
)

func (s *savable) finalise(version int, caller Caller) (*Marking, error) {
	return s.svc.Finalise(context.Background(), s.examID, s.studentID,
		FinaliseRequest{SessionID: s.reader.session.ID, ExpectedPaperVersion: version}, caller, AccessMeta{RequestID: "req-2"})
}

func (s *savable) reopen(reason string, caller Caller) (*Marking, error) {
	return s.svc.Reopen(context.Background(), s.examID, s.studentID,
		ReopenRequest{SessionID: s.reader.session.ID, Reason: reason}, caller, AccessMeta{RequestID: "req-3"})
}

// markedAndFinal is a paper with its one question marked 7.5 of 10, then finalised.
func markedAndFinal(t *testing.T) *savable {
	s := newSavable()
	res, err := s.save(SaveMarkRequest{MarksHalf: intp(15), Comment: strp("good")}, s.teacher)
	require.NoError(t, err)
	_, err = s.finalise(res.Paper.Version, s.teacher)
	require.NoError(t, err)
	return s
}

func TestFinalise_FreezesTheMarksAndRecordsIt(t *testing.T) {
	s := newSavable()
	res, err := s.save(SaveMarkRequest{MarksHalf: intp(15), Comment: strp("good")}, s.teacher)
	require.NoError(t, err)

	m, err := s.finalise(res.Paper.Version, s.teacher)
	require.NoError(t, err)
	assert.Equal(t, StatusFinal, m.Paper.Status)
	assert.Equal(t, 15, m.Paper.TotalHalf)
	assert.Equal(t, 20, m.Paper.MaxHalf)
	assert.Equal(t, res.Paper.Version+1, m.Paper.Version)
	require.NotNil(t, m.Paper.FinalisedAt)
	require.Len(t, m.Lines, 1)
	assert.True(t, m.Lines[0].FromSnapshot, "a final paper is read from its snapshot")

	require.Len(t, s.reader.events, 1)
	assert.Equal(t, EventFinalised, s.reader.events[0].Event)
	require.NotNil(t, s.reader.events[0].Snapshot)
	assert.Equal(t, 15, s.reader.events[0].Snapshot.Lines[0].AwardedHalf)

	last := s.reader.audited[len(s.reader.audited)-1]
	assert.Equal(t, audit.ActionPaperFinalise, last.Action)
	assert.NotContains(t, strings.ToLower(toJSON(last.Metadata)), "good", "the comment never reaches the audit log")
}

func TestFinalise_RefusesAPaperWithUnmarkedQuestionsAndNamesThem(t *testing.T) {
	s := newSavable()
	_, err := s.finalise(0, s.teacher)
	var inc *IncompleteError
	require.ErrorAs(t, err, &inc)
	assert.ErrorIs(t, err, ErrPaperIncomplete)
	assert.Equal(t, []int{1}, inc.Questions)
	assert.Nil(t, s.reader.paper, "the failed attempt left no trace, not even the lock row")
	assert.Empty(t, s.reader.events)
	assert.Empty(t, s.reader.audited)
}

func TestFinalise_RefusesAMarkThatNowExceedsTheQuestion(t *testing.T) {
	s := newSavable()
	res, err := s.save(SaveMarkRequest{MarksHalf: intp(20)}, s.teacher)
	require.NoError(t, err)
	// The question's points are lowered after it was marked.
	s.reader.questions[0].Points = 5

	_, err = s.finalise(res.Paper.Version, s.teacher)
	assert.ErrorIs(t, err, ErrPaperIncomplete)
	assert.Equal(t, StatusDraft, s.reader.paper.Status)
}

func TestFinalise_RefusesAPaperThatChangedSinceTheTeacherSawIt(t *testing.T) {
	s := newSavable()
	res, err := s.save(SaveMarkRequest{MarksHalf: intp(15)}, s.teacher)
	require.NoError(t, err)

	_, err = s.finalise(res.Paper.Version-1, s.teacher)
	assert.ErrorIs(t, err, ErrStaleVersion)
	assert.Equal(t, StatusDraft, s.reader.paper.Status)
}

func TestFinalise_RepeatingItChangesNothing(t *testing.T) {
	s := markedAndFinal(t)
	before := s.reader.paper.Version
	events, audited := len(s.reader.events), len(s.reader.audited)

	m, err := s.finalise(0, s.teacher) // an old version is fine: it is already done
	require.NoError(t, err)
	assert.Equal(t, StatusFinal, m.Paper.Status)
	assert.Equal(t, before, s.reader.paper.Version)
	assert.Len(t, s.reader.events, events)
	assert.Len(t, s.reader.audited, audited)
}

func TestFinalise_FailsClosedWhenTheAuditEntryCannotBeWritten(t *testing.T) {
	s := newSavable()
	res, err := s.save(SaveMarkRequest{MarksHalf: intp(15)}, s.teacher)
	require.NoError(t, err)
	s.reader.auditErr = errors.New("audit table gone")

	_, err = s.finalise(res.Paper.Version, s.teacher)
	assert.ErrorIs(t, err, ErrAuditUnavailable)
	assert.Equal(t, StatusDraft, s.reader.paper.Status, "nothing is finalised without a record of it")
	assert.Empty(t, s.reader.events)
}

func TestFinalise_PreconditionsAndAccess(t *testing.T) {
	t.Run("a paper still being sat cannot be finalised", func(t *testing.T) {
		s := newSavable()
		s.reader.session.Status = review.StatusInProgress
		_, err := s.finalise(0, s.teacher)
		assert.ErrorIs(t, err, ErrSessionNotEnded)
	})
	t.Run("a different attempt is now the one shown", func(t *testing.T) {
		s := newSavable()
		_, err := s.svc.Finalise(context.Background(), s.examID, s.studentID,
			FinaliseRequest{SessionID: uuid.New()}, s.teacher, AccessMeta{})
		assert.ErrorIs(t, err, ErrSessionChanged)
	})
	t.Run("students, proctors and other teachers cannot", func(t *testing.T) {
		s := newSavable()
		_, err := s.finalise(0, s.student)
		assert.ErrorIs(t, err, ErrForbiddenRole)
		_, err = s.finalise(0, s.proctor)
		assert.ErrorIs(t, err, ErrForbiddenRole)
		_, err = s.finalise(0, s.other)
		assert.ErrorIs(t, err, ErrExamNotFound)
	})
	t.Run("a marked paper cannot be changed once final", func(t *testing.T) {
		s := markedAndFinal(t)
		_, err := s.save(SaveMarkRequest{MarksHalf: intp(2)}, s.teacher)
		assert.ErrorIs(t, err, ErrPaperFinalised)
	})
}

func TestReopen_ReturnsThePaperToADraftAndKeepsTheHistory(t *testing.T) {
	s := markedAndFinal(t)
	finalVersion := s.reader.paper.Version

	m, err := s.reopen("  mis-keyed question 3  ", s.teacher)
	require.NoError(t, err)
	assert.Equal(t, StatusDraft, m.Paper.Status)
	assert.Equal(t, finalVersion+1, m.Paper.Version)
	assert.False(t, m.Lines[0].FromSnapshot, "marking is live again")
	assert.Nil(t, s.reader.paper.Snapshot)

	require.Len(t, s.reader.events, 2)
	ev := s.reader.events[1]
	assert.Equal(t, EventReopened, ev.Event)
	require.NotNil(t, ev.Reason)
	assert.Equal(t, "mis-keyed question 3", *ev.Reason)
	require.NotNil(t, ev.Snapshot, "what had been final is kept")
	assert.Equal(t, 15, ev.Snapshot.Lines[0].AwardedHalf)

	last := s.reader.audited[len(s.reader.audited)-1]
	assert.Equal(t, audit.ActionPaperReopen, last.Action)
	assert.NotContains(t, toJSON(last.Metadata), "mis-keyed", "the reason stays in the history table")

	// And it can be marked again.
	_, err = s.save(SaveMarkRequest{MarksHalf: intp(18), ExpectedVersion: 1}, s.teacher)
	assert.NoError(t, err)
}

func TestReopen_NeedsAReasonOfSensibleLength(t *testing.T) {
	s := markedAndFinal(t)
	for _, reason := range []string{"", "   ", "ab", strings.Repeat("x", MaxReasonRunes+1), "bad\x00reason"} {
		_, err := s.reopen(reason, s.teacher)
		assert.ErrorIs(t, err, ErrInvalidRequest, "%q", reason)
	}
	assert.Equal(t, StatusFinal, s.reader.paper.Status)
	_, err := s.reopen(strings.Repeat("é", MaxReasonRunes), s.teacher)
	assert.NoError(t, err, "the limit counts characters, not bytes")
}

func TestReopen_AnAdminMayAndRepeatingItChangesNothing(t *testing.T) {
	s := markedAndFinal(t)
	_, err := s.reopen("appeal upheld", s.admin)
	require.NoError(t, err)
	events, version := len(s.reader.events), s.reader.paper.Version

	m, err := s.reopen("appeal upheld", s.admin)
	require.NoError(t, err)
	assert.Equal(t, StatusDraft, m.Paper.Status)
	assert.Len(t, s.reader.events, events)
	assert.Equal(t, version, s.reader.paper.Version)
}

func TestReopen_Access(t *testing.T) {
	s := markedAndFinal(t)
	_, err := s.reopen("not allowed", s.student)
	assert.ErrorIs(t, err, ErrForbiddenRole)
	_, err = s.reopen("not allowed", s.other)
	assert.ErrorIs(t, err, ErrExamNotFound)
	assert.Equal(t, StatusFinal, s.reader.paper.Status)
}

func TestReopen_FailsClosedWhenTheAuditEntryCannotBeWritten(t *testing.T) {
	s := markedAndFinal(t)
	s.reader.auditErr = errors.New("audit table gone")
	_, err := s.reopen("appeal upheld", s.teacher)
	assert.ErrorIs(t, err, ErrAuditUnavailable)
	assert.Equal(t, StatusFinal, s.reader.paper.Status)
	assert.Len(t, s.reader.events, 1, "no reopen event without its audit entry")
}

func toJSON(v any) string {
	b, _ := json.Marshal(v)
	return string(b)
}
