package review

import "errors"

var (
	// ErrExamNotFound covers both a missing exam and an exam the caller does
	// not own, so the response never reveals which exams exist.
	ErrExamNotFound = errors.New("exam not found")
	// ErrForbiddenRole means the caller's role may never review responses.
	ErrForbiddenRole = errors.New("forbidden: only teachers and administrators can review responses")
	// ErrStudentNotAssigned means the student is not assigned to the exam.
	ErrStudentNotAssigned = errors.New("student is not assigned to this exam")
	// ErrRecordingNotFound covers a missing recording, one that belongs to a
	// different student or exam, and one the policy hides.
	ErrRecordingNotFound = errors.New("recording not found")
	// ErrRecordingRemoved means the audio was deleted. The transcript remains.
	ErrRecordingRemoved = errors.New("recording has been removed")
	// ErrAudioUnavailable means the recording exists but its audio cannot be
	// read right now.
	ErrAudioUnavailable = errors.New("recording audio is unavailable")
	// ErrPlaybackDisabled means this server is not set up to play recordings.
	ErrPlaybackDisabled = errors.New("recording playback is not configured")
	// ErrAuditUnavailable means the access could not be recorded, so nothing was
	// played. Playing audio without a record of it is not allowed.
	ErrAuditUnavailable = errors.New("access could not be recorded")
	// ErrInvalidQuery wraps a human-readable reason a request was rejected.
	ErrInvalidQuery = errors.New("invalid query")
)
