package grading

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func intp(i int) *int { return &i }

func TestParseHalfMarks(t *testing.T) {
	ok := map[string]int{
		"0": 0, "0.5": 1, "1": 2, "7": 14, "7.5": 15, "10.0": 20, "10.5": 21, "  3.5 ": 7, "007": 14, "100": 200,
	}
	for in, want := range ok {
		t.Run("accepts "+in, func(t *testing.T) {
			got, err := ParseHalfMarks(in)
			require.NoError(t, err)
			assert.Equal(t, want, got)
		})
	}

	bad := []string{
		"", " ", "-1", "-0.5", "+1", "0.25", "7.25", "7.55", "7.05", "7.", ".5", "1e1", "1E1", "0x10",
		"abc", "7,5", "NaN", "Infinity", "7.5.0", "1 000", "9999999", "7.50", "٣",
	}
	for _, in := range bad {
		t.Run("rejects "+in, func(t *testing.T) {
			_, err := ParseHalfMarks(in)
			assert.ErrorIs(t, err, ErrInvalidMarks)
		})
	}
}

func TestValidateHalfMarks(t *testing.T) {
	assert.NoError(t, ValidateHalfMarks(0, 20))
	assert.NoError(t, ValidateHalfMarks(20, 20), "the maximum itself is allowed")
	assert.NoError(t, ValidateHalfMarks(0, 0), "a zero-point question takes 0")

	err := ValidateHalfMarks(21, 20)
	assert.ErrorIs(t, err, ErrInvalidMarks)
	assert.Contains(t, err.Error(), "more than 10", "the message says what the limit is")

	assert.ErrorIs(t, ValidateHalfMarks(-1, 20), ErrInvalidMarks)
	assert.ErrorIs(t, ValidateHalfMarks(1, 0), ErrInvalidMarks)
}

func TestFormatHalfAndMarks(t *testing.T) {
	for half, want := range map[int]string{0: "0", 1: "0.5", 2: "1", 14: "7", 15: "7.5", 21: "10.5"} {
		assert.Equal(t, want, FormatHalf(half))
	}
	// A multiple of 0.5 is exact as a float, so the API never rounds.
	for half := 0; half <= 2000; half++ {
		assert.Equal(t, float64(half)/2, Marks(half))
	}
	assert.Equal(t, 7.5, Marks(15))
}

func TestParseThenFormatRoundTrips(t *testing.T) {
	for half := 0; half <= 400; half++ {
		got, err := ParseHalfMarks(FormatHalf(half))
		require.NoError(t, err)
		assert.Equal(t, half, got)
	}
}

func mcq(points int, answered bool, selected *uuid.UUID, correct ...uuid.UUID) QuestionInput {
	return QuestionInput{ID: uuid.New(), Number: 1, Type: TypeMCQ, Points: points, Answered: answered,
		SelectedOptionID: selected, CorrectOptionIDs: correct}
}

func TestSuggest(t *testing.T) {
	right, wrong := uuid.New(), uuid.New()

	t.Run("a correct multiple-choice answer gets full marks", func(t *testing.T) {
		got := Suggest(mcq(5, true, &right, right))
		require.NotNil(t, got)
		assert.Equal(t, 10, *got)
	})
	t.Run("a wrong one gets nothing", func(t *testing.T) {
		got := Suggest(mcq(5, true, &wrong, right))
		require.NotNil(t, got)
		assert.Zero(t, *got)
	})
	t.Run("an answer with no selected option gets nothing", func(t *testing.T) {
		got := Suggest(mcq(5, true, nil, right))
		require.NotNil(t, got)
		assert.Zero(t, *got)
	})
	t.Run("any of several correct options counts", func(t *testing.T) {
		other := uuid.New()
		got := Suggest(mcq(2, true, &other, right, other))
		require.NotNil(t, got)
		assert.Equal(t, 4, *got)
	})
	t.Run("with no correct option on record there is nothing to compare with", func(t *testing.T) {
		assert.Nil(t, Suggest(mcq(5, true, &right)))
	})
	t.Run("a question left unanswered gets 0, whatever its type", func(t *testing.T) {
		for _, typ := range []string{TypeMCQ, TypeEssay, TypeVoice} {
			q := QuestionInput{Type: typ, Points: 10, Answered: false}
			got := Suggest(q)
			require.NotNil(t, got, typ)
			assert.Zero(t, *got, typ)
		}
	})
	t.Run("an unanswered multiple-choice question without a key still gets 0", func(t *testing.T) {
		got := Suggest(mcq(5, false, nil))
		require.NotNil(t, got)
		assert.Zero(t, *got)
	})
	t.Run("written and spoken answers are marked by hand", func(t *testing.T) {
		assert.Nil(t, Suggest(QuestionInput{Type: TypeEssay, Points: 10, Answered: true}))
		assert.Nil(t, Suggest(QuestionInput{Type: TypeVoice, Points: 10, Answered: true}))
	})
	t.Run("a zero-point question suggests 0", func(t *testing.T) {
		got := Suggest(mcq(0, true, &right, right))
		require.NotNil(t, got)
		assert.Zero(t, *got)
	})
}

func essay(n, points int, answered bool) QuestionInput {
	return QuestionInput{ID: uuid.New(), Number: n, Type: TypeEssay, Points: points, Answered: answered}
}

func TestBuildLine(t *testing.T) {
	right, wrong := uuid.New(), uuid.New()
	when := time.Date(2026, 3, 1, 10, 0, 0, 0, time.UTC)
	comment := "good argument"

	t.Run("an unmarked answered essay has no mark", func(t *testing.T) {
		l := BuildLine(essay(2, 10, true), nil)
		assert.Nil(t, l.EffectiveHalf)
		assert.Nil(t, l.SuggestedHalf)
		assert.Equal(t, SourceNone, l.Source)
		assert.Equal(t, 20, l.MaxHalf)
		assert.Zero(t, l.Version)
	})
	t.Run("a multiple-choice answer is auto-marked", func(t *testing.T) {
		q := mcq(2, true, &right, right)
		l := BuildLine(q, nil)
		require.NotNil(t, l.EffectiveHalf)
		assert.Equal(t, 4, *l.EffectiveHalf)
		assert.Equal(t, SourceAuto, l.Source)
		assert.Nil(t, l.TeacherHalf)
		require.NotNil(t, l.CorrectOptionID)
		assert.Equal(t, right, *l.CorrectOptionID)
	})
	t.Run("the teacher's mark beats the automatic one", func(t *testing.T) {
		q := mcq(2, true, &wrong, right)
		l := BuildLine(q, &TeacherMark{MarksHalf: intp(2), Version: 3, MarkedAt: when})
		require.NotNil(t, l.EffectiveHalf)
		assert.Equal(t, 2, *l.EffectiveHalf, "the teacher gave a mark for a partly right answer")
		assert.Equal(t, SourceTeacher, l.Source)
		require.NotNil(t, l.SuggestedHalf)
		assert.Zero(t, *l.SuggestedHalf, "the suggestion is still shown")
		assert.Equal(t, 3, l.Version)
		require.NotNil(t, l.MarkedAt)
		assert.Equal(t, when, *l.MarkedAt)
	})
	t.Run("a teacher may give marks for a question left blank", func(t *testing.T) {
		l := BuildLine(essay(1, 10, false), &TeacherMark{MarksHalf: intp(3)})
		require.NotNil(t, l.EffectiveHalf)
		assert.Equal(t, 3, *l.EffectiveHalf)
	})
	t.Run("a comment alone leaves the mark to the automatic one", func(t *testing.T) {
		q := mcq(2, true, &right, right)
		l := BuildLine(q, &TeacherMark{Comment: &comment, Version: 1})
		require.NotNil(t, l.EffectiveHalf)
		assert.Equal(t, 4, *l.EffectiveHalf)
		assert.Equal(t, SourceAuto, l.Source)
		require.NotNil(t, l.Comment)
		assert.Equal(t, comment, *l.Comment)

		e := BuildLine(essay(2, 10, true), &TeacherMark{Comment: &comment, Version: 1})
		assert.Nil(t, e.EffectiveHalf, "a comment on an essay is not a mark")
	})
	t.Run("a teacher mark of zero counts as a mark", func(t *testing.T) {
		l := BuildLine(essay(2, 10, true), &TeacherMark{MarksHalf: intp(0)})
		require.NotNil(t, l.EffectiveHalf)
		assert.Zero(t, *l.EffectiveHalf)
		assert.Equal(t, SourceTeacher, l.Source)
	})
	t.Run("a mark higher than a lowered maximum does not count until it is looked at again", func(t *testing.T) {
		q := essay(2, 5, true) // points were 10 when this was marked
		l := BuildLine(q, &TeacherMark{MarksHalf: intp(16), Version: 2})
		assert.True(t, l.ExceedsMax)
		assert.Nil(t, l.EffectiveHalf)
		assert.Equal(t, SourceNone, l.Source)
		require.NotNil(t, l.TeacherHalf, "what was entered is still shown, so the teacher can see it")
		assert.Equal(t, 16, *l.TeacherHalf)
		assert.Equal(t, 2, l.Version)
	})
	t.Run("a mark that is exactly the maximum is fine", func(t *testing.T) {
		l := BuildLine(essay(2, 5, true), &TeacherMark{MarksHalf: intp(10)})
		assert.False(t, l.ExceedsMax)
		require.NotNil(t, l.EffectiveHalf)
		assert.Equal(t, 10, *l.EffectiveHalf)
	})
	t.Run("a raised maximum keeps an earlier mark valid", func(t *testing.T) {
		l := BuildLine(essay(2, 20, true), &TeacherMark{MarksHalf: intp(16)})
		assert.False(t, l.ExceedsMax)
		assert.Equal(t, 40, l.MaxHalf)
	})
	t.Run("multiple choice without a key is left for the teacher", func(t *testing.T) {
		l := BuildLine(mcq(2, true, &right), nil)
		assert.Nil(t, l.EffectiveHalf)
		assert.Nil(t, l.CorrectOptionID)
	})
}

func lines(qs ...Line) []Line { return qs }

func TestSummarise(t *testing.T) {
	right := uuid.New()
	l1 := BuildLine(QuestionInput{ID: uuid.New(), Number: 1, Type: TypeMCQ, Points: 2, Answered: true,
		SelectedOptionID: &right, CorrectOptionIDs: []uuid.UUID{right}}, nil) // auto 2
	l2 := BuildLine(essay(2, 10, true), &TeacherMark{MarksHalf: intp(15)}) // 7.5
	l3 := BuildLine(essay(3, 5, true), nil)                                // unmarked
	l4 := BuildLine(essay(4, 5, false), nil)                               // blank: auto 0

	t.Run("totals, maximum and counts", func(t *testing.T) {
		tot := Summarise(lines(l1, l2, l3, l4))
		assert.Equal(t, 4+15+0, tot.TotalHalf)
		assert.Equal(t, 4+20+10+10, tot.MaxHalf)
		assert.Equal(t, 3, tot.Marked)
		assert.Equal(t, 4, tot.Questions)
		assert.Equal(t, []int{3}, tot.Unmarked)
		assert.False(t, tot.Complete())
	})
	t.Run("complete once everything has a mark", func(t *testing.T) {
		l3.EffectiveHalf = intp(4)
		tot := Summarise(lines(l1, l2, l3, l4))
		assert.True(t, tot.Complete())
		assert.Equal(t, 4+15+4+0, tot.TotalHalf)
		assert.Equal(t, 4, tot.Marked)
	})
	t.Run("a zero-point question needs no marking and adds nothing", func(t *testing.T) {
		z := BuildLine(QuestionInput{ID: uuid.New(), Number: 9, Type: TypeEssay, Points: 0, Answered: false}, nil)
		tot := Summarise(lines(z))
		assert.True(t, tot.Complete())
		assert.Zero(t, tot.MaxHalf)
	})
	t.Run("an empty paper is complete and zero", func(t *testing.T) {
		tot := Summarise(nil)
		assert.True(t, tot.Complete())
		assert.Zero(t, tot.TotalHalf+tot.MaxHalf+tot.Questions)
	})
	t.Run("a mark above the maximum leaves its question unmarked", func(t *testing.T) {
		bad := BuildLine(essay(7, 5, true), &TeacherMark{MarksHalf: intp(99)})
		tot := Summarise(lines(bad))
		assert.Equal(t, []int{7}, tot.Unmarked)
	})
	t.Run("unmarked question numbers are listed in order", func(t *testing.T) {
		tot := Summarise(lines(BuildLine(essay(9, 1, true), nil), BuildLine(essay(2, 1, true), nil), BuildLine(essay(5, 1, true), nil)))
		assert.Equal(t, []int{2, 5, 9}, tot.Unmarked)
	})
}

func TestBuildSnapshot(t *testing.T) {
	right := uuid.New()
	comment := "well argued"
	auto := BuildLine(QuestionInput{ID: uuid.New(), Number: 1, Type: TypeMCQ, Points: 2, Answered: true,
		SelectedOptionID: &right, CorrectOptionIDs: []uuid.UUID{right}}, nil)
	byHand := BuildLine(essay(2, 10, true), &TeacherMark{MarksHalf: intp(15), Comment: &comment})

	t.Run("a complete paper is frozen line by line", func(t *testing.T) {
		snap, tot, err := BuildSnapshot(lines(auto, byHand))
		require.NoError(t, err)
		assert.Equal(t, SnapshotVersion, snap.Version)
		assert.Equal(t, 19, tot.TotalHalf)
		assert.Equal(t, 24, tot.MaxHalf)
		require.Len(t, snap.Lines, 2)

		assert.Equal(t, SnapshotLine{QuestionID: auto.QuestionID, QuestionNumber: 1, Type: TypeMCQ,
			MaxHalf: 4, AwardedHalf: 4, Source: SourceAuto}, snap.Lines[0])
		assert.Equal(t, 15, snap.Lines[1].AwardedHalf)
		assert.Equal(t, SourceTeacher, snap.Lines[1].Source)
		require.NotNil(t, snap.Lines[1].Comment)
		assert.Equal(t, comment, *snap.Lines[1].Comment)
	})
	t.Run("it refuses a paper with an unmarked question and says which", func(t *testing.T) {
		_, _, err := BuildSnapshot(lines(auto, BuildLine(essay(3, 5, true), nil), BuildLine(essay(4, 5, true), nil)))
		require.Error(t, err)
		assert.ErrorIs(t, err, ErrPaperIncomplete)
		var inc *IncompleteError
		require.True(t, errors.As(err, &inc))
		assert.Equal(t, []int{3, 4}, inc.Questions)
		assert.Contains(t, err.Error(), "3, 4")
	})
	t.Run("the answer key is not part of what is frozen", func(t *testing.T) {
		snap, _, err := BuildSnapshot(lines(auto))
		require.NoError(t, err)
		assert.NotContains(t, snapshotJSON(t, snap), "correct")
	})
}

func snapshotJSON(t *testing.T, s Snapshot) string {
	t.Helper()
	b, err := json.Marshal(s)
	require.NoError(t, err)
	return strings.ToLower(string(b))
}
