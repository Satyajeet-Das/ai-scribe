package review

import (
	"context"
	"errors"
	"io"

	"github.com/google/uuid"

	"github.com/Satyajeet-Das/ai-scribe/internal/audit"
	"github.com/Satyajeet-Das/ai-scribe/internal/platform/audiostore"
)

// AccessMeta describes the request, for the audit record only.
type AccessMeta struct {
	RequestID string
	IP        string
}

// Audio is a recording opened for playback. The caller closes Body.
type Audio struct {
	Body        io.ReadCloser
	Size        int64
	ContentType string
}

// PlaybackService opens the original audio of a student's recorded answer.
type PlaybackService interface {
	OpenRecording(ctx context.Context, examID, studentID, recordingID uuid.UUID, caller Caller, meta AccessMeta) (*Audio, error)
}

// EnablePlayback lets a review service open recordings from store, writing an
// audit record of every access. Without it OpenRecording reports
// ErrPlaybackDisabled.
func EnablePlayback(s Service, store audiostore.Store, recorder audit.Recorder) PlaybackService {
	svc, ok := s.(*service)
	if !ok {
		return disabledPlayback{}
	}
	svc.store, svc.audit = store, recorder
	return svc
}

type disabledPlayback struct{}

func (disabledPlayback) OpenRecording(context.Context, uuid.UUID, uuid.UUID, uuid.UUID, Caller, AccessMeta) (*Audio, error) {
	return nil, ErrPlaybackDisabled
}

// OpenRecording checks, in order: the caller's role, ownership of the exam,
// that the recording is this student's recording for this exam, that review of
// the session is allowed, and that the audio can be read. Only then is the
// access recorded, and only if that record is written is the audio returned:
// no audio leaves the system without an audit entry.
func (s *service) OpenRecording(ctx context.Context, examID, studentID, recordingID uuid.UUID, caller Caller, meta AccessMeta) (*Audio, error) {
	if s.store == nil || s.audit == nil {
		return nil, ErrPlaybackDisabled
	}
	if _, err := s.authorize(ctx, examID, caller); err != nil {
		return nil, err
	}

	file, err := s.repo.GetRecording(ctx, examID, studentID, recordingID)
	if err != nil {
		return nil, err
	}
	// The same rule that decides whether answers are shown decides whether
	// their audio can be played.
	if file.SessionStatus == StatusInProgress && !s.policy.AllowInProgress {
		return nil, ErrRecordingNotFound
	}
	if file.Removed {
		return nil, ErrRecordingRemoved
	}

	event := audit.Event{
		ActorID: caller.ID, ActorRole: string(caller.Role),
		Action: audit.ActionAudioPlayed, ResourceType: audit.ResourceRecording,
		ResourceID: file.ID, SessionID: file.SessionID,
		Outcome: audit.OutcomeOK, RequestID: meta.RequestID, IP: meta.IP,
	}

	body, size, err := s.store.Open(ctx, file.StorageKey)
	if err != nil {
		if s.logger != nil {
			s.logger.Error().Err(err).Str("event", "review.audio_unavailable").
				Str("recording_id", file.ID.String()).Msg("recording audio could not be opened")
		}
		if !errors.Is(err, context.Canceled) {
			// Best effort: nothing was played, so a failure to note this does not matter.
			event.Action, event.Outcome = audit.ActionAudioUnavailable, audit.OutcomeUnavailable
			_ = s.audit.Record(ctx, event)
		}
		return nil, ErrAudioUnavailable
	}

	if err := s.audit.Record(ctx, event); err != nil {
		_ = body.Close()
		if s.logger != nil {
			s.logger.Error().Err(err).Str("event", "review.audit_failed").
				Str("recording_id", file.ID.String()).Msg("audio not played: access could not be recorded")
		}
		return nil, ErrAuditUnavailable
	}
	return &Audio{Body: body, Size: size, ContentType: file.ContentType}, nil
}
