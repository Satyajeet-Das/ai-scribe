package recording

import (
	"bytes"
	"context"
	"crypto/sha256"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/rs/zerolog"

	"github.com/Satyajeet-Das/ai-scribe/internal/platform/audiostore"
)

const contentTypeWAV = "audio/wav"

// Options configure a Service.
type Options struct {
	// RetentionDays sets retention_until on new recordings. Zero keeps them
	// until an administrator removes them: nothing is deleted automatically.
	RetentionDays int
	// SaveTimeout bounds one Save, so a slow store cannot hold up the student.
	SaveTimeout time.Duration
}

// Service stores answer audio and its transcription.
type Service struct {
	repo   Repository
	store  audiostore.Store
	opts   Options
	logger *zerolog.Logger
	now    func() time.Time
}

func NewService(repo Repository, store audiostore.Store, opts Options, logger *zerolog.Logger) *Service {
	if opts.SaveTimeout <= 0 {
		opts.SaveTimeout = 5 * time.Second
	}
	return &Service{repo: repo, store: store, opts: opts, logger: logger, now: time.Now}
}

// SaveInput is one finished utterance.
type SaveInput struct {
	SessionID  uuid.UUID
	QuestionID uuid.UUID
	StudentID  uuid.UUID
	// StreamID is the client's stream identifier. Saving the same stream twice
	// stores it once.
	StreamID   uuid.UUID
	SampleRate int
	ChunkCount int
	// PCM is mono PCM16LE audio exactly as received.
	PCM        []byte
	Transcript string
	Provider   string
	Language   string
}

// Timeout is how long the caller should allow Save.
func (s *Service) Timeout() time.Duration { return s.opts.SaveTimeout }

// Save stores the audio, then records its metadata and transcription. The
// order matters: a metadata row must never point at audio that is not there.
// If the metadata write fails the stored object is removed again, so a failure
// leaves nothing behind. Saving a stream that was already saved is a no-op.
func (s *Service) Save(ctx context.Context, in SaveInput) (*Saved, error) {
	if len(in.PCM) == 0 || len(in.PCM)%2 != 0 || in.SampleRate <= 0 || in.ChunkCount <= 0 ||
		in.SessionID == uuid.Nil || in.QuestionID == uuid.Nil || in.StudentID == uuid.Nil || in.StreamID == uuid.Nil {
		return nil, ErrInvalidAudio
	}
	ctx, cancel := context.WithTimeout(ctx, s.opts.SaveTimeout)
	defer cancel()

	sum := sha256.Sum256(in.PCM)
	wav := EncodeWAV(in.PCM, in.SampleRate)
	now := s.now().UTC()

	rec := &Recording{
		ID: uuid.New(), SessionID: in.SessionID, QuestionID: in.QuestionID, StudentID: in.StudentID,
		StreamID: in.StreamID, ContentType: contentTypeWAV, SampleRate: in.SampleRate,
		DurationMillis: DurationMillis(len(in.PCM), in.SampleRate), ByteSize: int64(len(wav)),
		ChunkCount: in.ChunkCount, SHA256: sum[:], StorageKey: audiostore.NewKey(), RecordedAt: now,
	}
	if s.opts.RetentionDays > 0 {
		until := now.AddDate(0, 0, s.opts.RetentionDays)
		rec.RetentionUntil = &until
	}
	tr := &Transcription{ID: uuid.New(), Text: in.Transcript, Provider: in.Provider, Language: in.Language, CreatedAt: now}

	if err := s.store.Put(ctx, rec.StorageKey, bytes.NewReader(wav), int64(len(wav))); err != nil {
		return nil, fmt.Errorf("%w: %v", ErrStorage, err)
	}

	existing, existingTr, created, err := s.repo.Insert(ctx, rec, tr)
	if err != nil {
		s.discard(rec.StorageKey)
		return nil, fmt.Errorf("%w: %v", ErrStorage, err)
	}
	if !created {
		s.discard(rec.StorageKey)
		// A retry of the same utterance is answered with the recording that is
		// already there. A stream ID that belongs to something else is a mistake
		// or an attack, and is refused rather than answered with someone else's
		// recording.
		if existing.QuestionID != in.QuestionID || existing.StudentID != in.StudentID {
			return nil, ErrStreamReused
		}
		return &Saved{
			RecordingID: existing.ID, TranscriptionID: existingTr,
			DurationMillis: existing.DurationMillis, Duplicate: true,
		}, nil
	}
	return &Saved{RecordingID: rec.ID, TranscriptionID: tr.ID, DurationMillis: rec.DurationMillis}, nil
}

// discard removes an object that no row refers to. It uses its own context
// because the caller's may already be cancelled, which is exactly when
// cleanup matters.
func (s *Service) discard(key string) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := s.store.Delete(ctx, key); err != nil && s.logger != nil {
		s.logger.Error().Err(err).Str("event", "recording.orphan").Str("storage_key", key).
			Msg("could not remove audio that has no metadata row")
	}
}
