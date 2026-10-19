package review

import (
	"time"

	"github.com/google/uuid"
)

// ExamDTO is the exam context shown with a review.
type ExamDTO struct {
	ID             uuid.UUID `json:"id"`
	Title          string    `json:"title"`
	Subject        string    `json:"subject"`
	Status         string    `json:"status"`
	DurationMins   int       `json:"durationMins"`
	TotalQuestions int       `json:"totalQuestions"`
}

// SummaryDTO counts every student assigned to the exam, regardless of the
// search or status filter applied to the list.
type SummaryDTO struct {
	Assigned   int `json:"assigned"`
	NotStarted int `json:"notStarted"`
	InProgress int `json:"inProgress"`
	Submitted  int `json:"submitted"`
	Expired    int `json:"expired"`
}

// StudentRowDTO is one row of the roster. AnsweredCount is omitted when the
// student has no session.
type StudentRowDTO struct {
	StudentID     uuid.UUID  `json:"studentId"`
	RollNo        string     `json:"rollNo"`
	Name          string     `json:"name"`
	Status        Status     `json:"status"`
	StartedAt     *time.Time `json:"startedAt,omitempty"`
	SubmittedAt   *time.Time `json:"submittedAt,omitempty"`
	AnsweredCount *int       `json:"answeredCount,omitempty"`
}

// StudentListResponse follows the list convention used elsewhere in the API.
type StudentListResponse struct {
	Exam    ExamDTO         `json:"exam"`
	Summary SummaryDTO      `json:"summary"`
	Data    []StudentRowDTO `json:"data"`
	Total   int             `json:"total"`
	Limit   int             `json:"limit"`
	Offset  int             `json:"offset"`
}

type StudentDTO struct {
	ID     uuid.UUID `json:"id"`
	RollNo string    `json:"rollNo"`
	Name   string    `json:"name"`
}

// SessionDTO is present only when the student started the exam. IsFinal is
// false while the session is still in progress.
type SessionDTO struct {
	ID          uuid.UUID  `json:"id"`
	Status      Status     `json:"status"`
	StartedAt   time.Time  `json:"startedAt"`
	SubmittedAt *time.Time `json:"submittedAt,omitempty"`
	IsFinal     bool       `json:"isFinal"`
}

type CountsDTO struct {
	Total      int `json:"total"`
	Answered   int `json:"answered"`
	Unanswered int `json:"unanswered"`
}

// OptionDTO omits IsCorrect unless the policy allows answer keys.
type OptionDTO struct {
	ID           uuid.UUID `json:"id"`
	Key          string    `json:"key"`
	Text         string    `json:"text"`
	DisplayOrder int       `json:"displayOrder"`
	IsCorrect    *bool     `json:"isCorrect,omitempty"`
}

const (
	AnswerStatusAnswered   = "ANSWERED"
	AnswerStatusUnanswered = "UNANSWERED"
)

// AnswerDTO always has a Status so an unanswered question is explicit.
// SavedAt is when the answer was last saved, absent if nothing was saved.
type AnswerDTO struct {
	Status             string     `json:"status"`
	SelectedOptionID   *uuid.UUID `json:"selectedOptionId,omitempty"`
	SelectedOptionKey  string     `json:"selectedOptionKey,omitempty"`
	SelectedOptionText string     `json:"selectedOptionText,omitempty"`
	TextAnswer         string     `json:"textAnswer,omitempty"`
	SavedAt            *time.Time `json:"savedAt,omitempty"`
	// Source says where the saved value came from: OPTION, TYPED,
	// STT_CONFIRMED (the student confirmed what the recognizer heard) or
	// STUDENT_CORRECTION. Absent for answers saved before this was recorded.
	Source string `json:"source,omitempty"`
}

// RecordingDTO describes one kept audio segment. It never contains the audio
// or where it is stored; the audio is fetched from the playback endpoint,
// which checks access and writes an audit record every time.
type RecordingDTO struct {
	ID           uuid.UUID `json:"id"`
	SegmentIndex int       `json:"segmentIndex"`
	// Status is AVAILABLE, or REMOVED when the audio has been deleted (the
	// transcript stays).
	Status     string    `json:"status"`
	DurationMs int       `json:"durationMs"`
	RecordedAt time.Time `json:"recordedAt"`
	// Transcript is the recognizer's unedited output. It can differ from the
	// answer text, which is what the student confirmed or corrected.
	Transcript string `json:"transcript"`
	// Confirmed is true when the student confirmed an answer using this audio.
	Confirmed bool `json:"confirmed"`
	// SupportsCurrentAnswer is true when the saved answer is still the one
	// confirmed with this audio, and false once it was typed or dictated over.
	SupportsCurrentAnswer bool `json:"supportsCurrentAnswer"`
}

// RevisionDTO is one value an answer held.
type RevisionDTO struct {
	RevisionNo int `json:"revisionNo"`
	// Source is OPTION, TYPED, STT_CONFIRMED or STUDENT_CORRECTION.
	Source            string    `json:"source"`
	TextAnswer        string    `json:"textAnswer,omitempty"`
	SelectedOptionKey string    `json:"selectedOptionKey,omitempty"`
	CreatedAt         time.Time `json:"createdAt"`
	// Transcript is what the speech recognizer heard, for a dictated value.
	Transcript *string `json:"transcript,omitempty"`
	// Corrected is true when the student changed the recognized words before
	// confirming them.
	Corrected bool `json:"corrected"`
	// RecordingIDs are the recordings confirmed for this value.
	RecordingIDs []uuid.UUID `json:"recordingIds"`
}

// RevisionHistoryResponse is the history of one answer, oldest value first.
// Revisions is always an array; it is empty when Available is false, in
// which case UnavailableReason says why.
type RevisionHistoryResponse struct {
	Available         bool          `json:"available"`
	UnavailableReason string        `json:"unavailableReason,omitempty"`
	Revisions         []RevisionDTO `json:"revisions"`
}

func ToRevisionHistoryResponse(h *RevisionHistory) RevisionHistoryResponse {
	out := RevisionHistoryResponse{
		Available: h.Available, UnavailableReason: h.UnavailableReason,
		Revisions: make([]RevisionDTO, 0, len(h.Revisions)),
	}
	for _, r := range h.Revisions {
		ids := r.RecordingIDs
		if ids == nil {
			ids = []uuid.UUID{}
		}
		out.Revisions = append(out.Revisions, RevisionDTO{
			RevisionNo: r.RevisionNo, Source: r.Source, TextAnswer: r.TextAnswer,
			SelectedOptionKey: r.OptionKey, CreatedAt: r.CreatedAt, Transcript: r.Transcript,
			Corrected: r.Source == "STUDENT_CORRECTION", RecordingIDs: ids,
		})
	}
	return out
}

type QuestionDTO struct {
	ID             uuid.UUID   `json:"id"`
	QuestionNumber int         `json:"questionNumber"`
	Text           string      `json:"text"`
	Type           string      `json:"type"`
	MaxMarks       int         `json:"maxMarks"`
	Options        []OptionDTO `json:"options,omitempty"`
	Answer         AnswerDTO   `json:"answer"`
	// Recordings is always present; it is empty when no audio was kept, which
	// is the normal case.
	Recordings []RecordingDTO `json:"recordings"`
}

// StudentResponsesResponse is one student's review. Questions is always an
// array; it is empty when ResponseAvailable is false, in which case
// UnavailableReason says why.
type StudentResponsesResponse struct {
	Exam              ExamDTO       `json:"exam"`
	Student           StudentDTO    `json:"student"`
	Status            Status        `json:"status"`
	Session           *SessionDTO   `json:"session,omitempty"`
	ResponseAvailable bool          `json:"responseAvailable"`
	UnavailableReason string        `json:"unavailableReason,omitempty"`
	Counts            *CountsDTO    `json:"counts,omitempty"`
	Questions         []QuestionDTO `json:"questions"`
}

func toExamDTO(e ExamInfo) ExamDTO {
	return ExamDTO{
		ID: e.ID, Title: e.Title, Subject: e.Subject, Status: e.Status,
		DurationMins: e.DurationMins, TotalQuestions: e.TotalQuestions,
	}
}

func ToStudentListResponse(l *StudentList) StudentListResponse {
	rows := make([]StudentRowDTO, len(l.Rows))
	for i, r := range l.Rows {
		rows[i] = StudentRowDTO{
			StudentID: r.StudentID, RollNo: r.RollNo, Name: r.Name, Status: r.Status,
			StartedAt: r.StartedAt, SubmittedAt: r.SubmittedAt, AnsweredCount: r.AnsweredCount,
		}
	}
	return StudentListResponse{
		Exam: toExamDTO(l.Exam),
		Summary: SummaryDTO{
			Assigned: l.Counts.Assigned, NotStarted: l.Counts.NotStarted,
			InProgress: l.Counts.InProgress, Submitted: l.Counts.Submitted, Expired: l.Counts.Expired,
		},
		Data: rows, Total: l.Total, Limit: l.Limit, Offset: l.Offset,
	}
}

func ToStudentResponsesResponse(r *StudentResponses) StudentResponsesResponse {
	out := StudentResponsesResponse{
		Exam:              toExamDTO(r.Exam),
		Student:           StudentDTO{ID: r.StudentID, RollNo: r.RollNo, Name: r.Name},
		Status:            r.Status,
		ResponseAvailable: r.ResponseAvailable,
		UnavailableReason: r.UnavailableReason,
		Questions:         make([]QuestionDTO, 0, len(r.Questions)),
	}
	if r.Session != nil {
		out.Session = &SessionDTO{
			ID: r.Session.ID, Status: r.Session.Status, StartedAt: r.Session.StartedAt,
			SubmittedAt: r.Session.SubmittedAt, IsFinal: r.Session.Status != StatusInProgress,
		}
	}
	if r.Counts != nil {
		out.Counts = &CountsDTO{Total: r.Counts.Total, Answered: r.Counts.Answered, Unanswered: r.Counts.Unanswered}
	}
	for _, q := range r.Questions {
		out.Questions = append(out.Questions, toQuestionDTO(q, r.ShowCorrect))
	}
	return out
}

func toQuestionDTO(q QuestionRecord, showCorrect bool) QuestionDTO {
	dto := QuestionDTO{
		ID: q.ID, QuestionNumber: q.QuestionNumber, Text: q.Text, Type: q.Type, MaxMarks: q.Points,
		Answer:     AnswerDTO{Status: AnswerStatusUnanswered},
		Recordings: make([]RecordingDTO, 0, len(q.Recordings)),
	}
	for _, r := range q.Recordings {
		dto.Recordings = append(dto.Recordings, RecordingDTO{
			ID: r.ID, SegmentIndex: r.SegmentIndex, Status: r.Status, DurationMs: r.DurationMillis,
			RecordedAt: r.RecordedAt, Transcript: r.Transcript, Confirmed: r.Confirmed,
			SupportsCurrentAnswer: r.SupportsCurrentAnswer,
		})
	}
	for _, o := range q.Options {
		od := OptionDTO{ID: o.ID, Key: o.Key, Text: o.Text, DisplayOrder: o.DisplayOrder}
		if showCorrect {
			c := o.IsCorrect
			od.IsCorrect = &c
		}
		dto.Options = append(dto.Options, od)
	}
	if q.Answer == nil {
		return dto
	}
	saved := q.Answer.UpdatedAt
	dto.Answer.SavedAt = &saved
	if !IsAnswered(q.Answer) {
		return dto
	}
	dto.Answer.Status = AnswerStatusAnswered
	dto.Answer.Source = q.Answer.Source
	dto.Answer.TextAnswer = q.Answer.TextAnswer
	if id := q.Answer.SelectedOptionID; id != nil {
		dto.Answer.SelectedOptionID = id
		for _, o := range q.Options {
			if o.ID == *id {
				dto.Answer.SelectedOptionKey = o.Key
				dto.Answer.SelectedOptionText = o.Text
				break
			}
		}
	}
	return dto
}
