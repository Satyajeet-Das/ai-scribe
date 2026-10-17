package grading

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

// halfMarksPattern is the only form a mark may take: whole marks, or whole
// marks and a half. No sign, exponent, or other fraction.
var halfMarksPattern = regexp.MustCompile(`^\d{1,6}(\.[05])?$`)

// ParseHalfMarks turns text such as "7", "7.5" or "10.0" into half-marks.
// Anything else ("7.25", "-1", "1e1", ".5", "") is refused.
func ParseHalfMarks(raw string) (int, error) {
	raw = strings.TrimSpace(raw)
	if !halfMarksPattern.MatchString(raw) {
		return 0, fmt.Errorf("%w: marks must be a whole number or end in .5", ErrInvalidMarks)
	}
	whole, frac, _ := strings.Cut(raw, ".")
	n, err := strconv.Atoi(whole)
	if err != nil {
		return 0, fmt.Errorf("%w: marks must be a whole number or end in .5", ErrInvalidMarks)
	}
	half := n * 2
	if frac == "5" {
		half++
	}
	return half, nil
}

// ValidateHalfMarks checks a mark against the question's maximum.
func ValidateHalfMarks(half, maxHalf int) error {
	if half < 0 {
		return fmt.Errorf("%w: marks cannot be negative", ErrInvalidMarks)
	}
	if half > maxHalf {
		return fmt.Errorf("%w: marks cannot be more than %s", ErrInvalidMarks, FormatHalf(maxHalf))
	}
	return nil
}

// Marks converts half-marks to marks. A multiple of 0.5 is exact in a float64.
func Marks(half int) float64 { return float64(half) / 2 }

// FormatHalf writes half-marks as marks: 15 is "7.5", 14 is "7".
func FormatHalf(half int) string {
	if half%2 == 0 {
		return strconv.Itoa(half / 2)
	}
	return strconv.Itoa(half/2) + ".5"
}

func ptr(i int) *int { return &i }

// Suggest is the automatic mark for a question, or nil when only the teacher
// can decide.
//
//   - Not answered: 0, whatever the type.
//   - Multiple choice with a correct option on record: full marks if the student
//     chose a correct option, otherwise 0.
//   - Multiple choice with no correct option on record: nil (nothing to compare with).
//   - Essay and voice answers: nil. They are marked by hand.
func Suggest(q QuestionInput) *int {
	if !q.Answered {
		return ptr(0)
	}
	if q.Type != TypeMCQ || len(q.CorrectOptionIDs) == 0 {
		return nil
	}
	if q.SelectedOptionID != nil {
		for _, id := range q.CorrectOptionIDs {
			if id == *q.SelectedOptionID {
				return ptr(q.MaxHalf())
			}
		}
	}
	return ptr(0)
}

// BuildLine works out a question's marking state from the question and what the
// teacher has saved for it (nil if nothing).
func BuildLine(q QuestionInput, saved *TeacherMark) Line {
	line := Line{
		QuestionID:     q.ID,
		QuestionNumber: q.Number,
		Type:           q.Type,
		MaxHalf:        q.MaxHalf(),
		Answered:       q.Answered,
		SuggestedHalf:  Suggest(q),
		Source:         SourceNone,
	}
	if q.Type == TypeMCQ && len(q.CorrectOptionIDs) > 0 {
		id := q.CorrectOptionIDs[0]
		line.CorrectOptionID = &id
	}

	if saved != nil {
		line.Version = saved.Version
		line.Comment = saved.Comment
		if !saved.MarkedAt.IsZero() {
			t := saved.MarkedAt
			line.MarkedAt = &t
		}
		if saved.MarksHalf != nil {
			if *saved.MarksHalf > line.MaxHalf {
				// The question's points were lowered after this was marked.
				// It must not count until the teacher looks at it again.
				line.ExceedsMax = true
				line.TeacherHalf = ptr(*saved.MarksHalf)
				return line
			}
			line.TeacherHalf = ptr(*saved.MarksHalf)
			line.EffectiveHalf = ptr(*saved.MarksHalf)
			line.Source = SourceTeacher
			return line
		}
	}

	if line.SuggestedHalf != nil {
		line.EffectiveHalf = ptr(*line.SuggestedHalf)
		line.Source = SourceAuto
	}
	return line
}

// Summarise totals a paper's lines.
func Summarise(lines []Line) Totals {
	t := Totals{Questions: len(lines)}
	var unmarked []int
	for _, l := range lines {
		t.MaxHalf += l.MaxHalf
		if l.EffectiveHalf == nil {
			unmarked = append(unmarked, l.QuestionNumber)
			continue
		}
		t.Marked++
		t.TotalHalf += *l.EffectiveHalf
	}
	t.Unmarked = sortedNumbers(unmarked)
	return t
}

// BuildSnapshot freezes a paper's marks. It refuses a paper with any question
// still unmarked, so a snapshot is always complete.
func BuildSnapshot(lines []Line) (Snapshot, Totals, error) {
	totals := Summarise(lines)
	if !totals.Complete() {
		return Snapshot{}, totals, &IncompleteError{Questions: totals.Unmarked}
	}
	snap := Snapshot{Version: SnapshotVersion, Lines: make([]SnapshotLine, 0, len(lines))}
	for _, l := range lines {
		snap.Lines = append(snap.Lines, SnapshotLine{
			QuestionID:     l.QuestionID,
			QuestionNumber: l.QuestionNumber,
			Type:           l.Type,
			MaxHalf:        l.MaxHalf,
			AwardedHalf:    *l.EffectiveHalf,
			Source:         l.Source,
			Comment:        l.Comment,
		})
	}
	return snap, totals, nil
}
