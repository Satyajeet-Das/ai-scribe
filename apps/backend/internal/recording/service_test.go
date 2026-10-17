package recording

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/binary"
	"errors"
	"io"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Satyajeet-Das/ai-scribe/internal/platform/audiostore"
)

// fakeRepo keeps recordings in memory with the same (session, stream)
// uniqueness the database enforces.
type fakeRepo struct {
	recs    map[[2]uuid.UUID]*Recording
	trs     map[uuid.UUID]*Transcription
	failErr error
}

func newFakeRepo() *fakeRepo {
	return &fakeRepo{recs: map[[2]uuid.UUID]*Recording{}, trs: map[uuid.UUID]*Transcription{}}
}

func (f *fakeRepo) Insert(_ context.Context, rec *Recording, tr *Transcription) (*Recording, uuid.UUID, bool, error) {
	if f.failErr != nil {
		return nil, uuid.Nil, false, f.failErr
	}
	k := [2]uuid.UUID{rec.SessionID, rec.StreamID}
	if ex, ok := f.recs[k]; ok {
		return ex, f.trs[ex.ID].ID, false, nil
	}
	tr.RecordingID = rec.ID
	f.recs[k], f.trs[rec.ID] = rec, tr
	return nil, uuid.Nil, true, nil
}

func input() SaveInput {
	return SaveInput{
		SessionID: uuid.New(), QuestionID: uuid.New(), StudentID: uuid.New(), StreamID: uuid.New(),
		SampleRate: 16000, ChunkCount: 2, PCM: bytes.Repeat([]byte{1, 0}, 16000), // one second
		Transcript: "plants make sugar", Provider: "test", Language: "en",
	}
}

func newService(repo Repository, store audiostore.Store, opts Options) *Service {
	return NewService(repo, store, opts, nil)
}

func TestSave_StoresAudioThenMetadataAndKeepsTheTranscriptionSeparate(t *testing.T) {
	repo, store := newFakeRepo(), audiostore.NewMemory()
	svc := newService(repo, store, Options{})
	in := input()

	saved, err := svc.Save(context.Background(), in)
	require.NoError(t, err)
	assert.False(t, saved.Duplicate)
	assert.Equal(t, 1000, saved.DurationMillis)

	rec := repo.recs[[2]uuid.UUID{in.SessionID, in.StreamID}]
	require.NotNil(t, rec)
	assert.Equal(t, in.QuestionID, rec.QuestionID)
	assert.Equal(t, in.StudentID, rec.StudentID)
	sum := sha256.Sum256(in.PCM)
	assert.Equal(t, sum[:], rec.SHA256, "the checksum is of the audio itself")
	assert.Nil(t, rec.RetentionUntil, "no retention period is invented")
	assert.Equal(t, "plants make sugar", repo.trs[rec.ID].Text)

	// The object exists, is a valid WAV, and holds exactly the received audio.
	rc, size, err := store.Open(context.Background(), rec.StorageKey)
	require.NoError(t, err)
	data, _ := io.ReadAll(rc)
	rc.Close()
	assert.Equal(t, rec.ByteSize, size)
	assert.Equal(t, "RIFF", string(data[:4]))
	assert.Equal(t, "WAVE", string(data[8:12]))
	assert.Equal(t, in.PCM, data[44:])
}

func TestSave_StorageFailureStoresNothingAndReportsIt(t *testing.T) {
	repo, store := newFakeRepo(), audiostore.NewMemory()
	store.PutErr = audiostore.ErrUnavailable
	svc := newService(repo, store, Options{})

	_, err := svc.Save(context.Background(), input())
	assert.ErrorIs(t, err, ErrStorage)
	assert.Empty(t, repo.recs, "no row may exist for audio that was not stored")
}

func TestSave_MetadataFailureRemovesTheStoredAudio(t *testing.T) {
	repo, store := newFakeRepo(), audiostore.NewMemory()
	repo.failErr = errors.New("database down")
	svc := newService(repo, store, Options{})

	_, err := svc.Save(context.Background(), input())
	assert.ErrorIs(t, err, ErrStorage)
	assert.Empty(t, store.Keys(), "audio without a row would be unreachable and unaccountable")
}

func TestSave_RetryOfTheSameStreamStoresOnce(t *testing.T) {
	repo, store := newFakeRepo(), audiostore.NewMemory()
	svc := newService(repo, store, Options{})
	in := input()

	first, err := svc.Save(context.Background(), in)
	require.NoError(t, err)
	second, err := svc.Save(context.Background(), in)
	require.NoError(t, err)

	assert.True(t, second.Duplicate)
	assert.Equal(t, first.RecordingID, second.RecordingID)
	assert.Equal(t, first.TranscriptionID, second.TranscriptionID)
	assert.Len(t, store.Keys(), 1, "the retry's copy is discarded")
	assert.Len(t, repo.recs, 1)
}

func TestSave_AStreamIdCannotBeReusedForAnotherQuestionOrStudent(t *testing.T) {
	repo, store := newFakeRepo(), audiostore.NewMemory()
	svc := newService(repo, store, Options{})
	first := input()
	saved, err := svc.Save(context.Background(), first)
	require.NoError(t, err)

	t.Run("another question in the same session", func(t *testing.T) {
		reuse := first
		reuse.QuestionID = uuid.New()
		reuse.PCM = bytes.Repeat([]byte{9, 0}, 1600)
		reuse.Transcript = "a different answer"

		_, err := svc.Save(context.Background(), reuse)
		assert.ErrorIs(t, err, ErrStreamReused, "not answered with the first question's recording")
		assert.Len(t, store.Keys(), 1, "the new audio is not kept, and the first is untouched")
		assert.Equal(t, saved.RecordingID, repo.recs[[2]uuid.UUID{first.SessionID, first.StreamID}].ID)
	})
	t.Run("another student", func(t *testing.T) {
		reuse := first
		reuse.StudentID = uuid.New()
		_, err := svc.Save(context.Background(), reuse)
		assert.ErrorIs(t, err, ErrStreamReused)
	})
	t.Run("a genuine retry of the same utterance is still accepted", func(t *testing.T) {
		again, err := svc.Save(context.Background(), first)
		require.NoError(t, err)
		assert.True(t, again.Duplicate)
		assert.Equal(t, saved.RecordingID, again.RecordingID)
	})
}

func TestSave_RejectsAudioThatCannotBeARecording(t *testing.T) {
	svc := newService(newFakeRepo(), audiostore.NewMemory(), Options{})
	bad := map[string]func(*SaveInput){
		"empty":          func(i *SaveInput) { i.PCM = nil },
		"odd length":     func(i *SaveInput) { i.PCM = []byte{1, 2, 3} },
		"no sample rate": func(i *SaveInput) { i.SampleRate = 0 },
		"no chunks":      func(i *SaveInput) { i.ChunkCount = 0 },
		"no session":     func(i *SaveInput) { i.SessionID = uuid.Nil },
		"no stream":      func(i *SaveInput) { i.StreamID = uuid.Nil },
	}
	for name, mutate := range bad {
		t.Run(name, func(t *testing.T) {
			in := input()
			mutate(&in)
			_, err := svc.Save(context.Background(), in)
			assert.ErrorIs(t, err, ErrInvalidAudio)
		})
	}
}

func TestSave_RetentionIsSetOnlyWhenConfigured(t *testing.T) {
	repo := newFakeRepo()
	svc := newService(repo, audiostore.NewMemory(), Options{RetentionDays: 30})
	svc.now = func() time.Time { return time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC) }
	in := input()

	_, err := svc.Save(context.Background(), in)
	require.NoError(t, err)
	rec := repo.recs[[2]uuid.UUID{in.SessionID, in.StreamID}]
	require.NotNil(t, rec.RetentionUntil)
	assert.Equal(t, time.Date(2026, 1, 31, 0, 0, 0, 0, time.UTC), *rec.RetentionUntil)
}

func TestEncodeWAV_HeaderDescribesTheAudio(t *testing.T) {
	pcm := make([]byte, 3200) // 100 ms at 16 kHz
	wav := EncodeWAV(pcm, 16000)

	assert.Len(t, wav, 44+len(pcm))
	assert.Equal(t, uint32(36+len(pcm)), binary.LittleEndian.Uint32(wav[4:8]))
	assert.Equal(t, uint16(1), binary.LittleEndian.Uint16(wav[20:22]), "PCM")
	assert.Equal(t, uint16(1), binary.LittleEndian.Uint16(wav[22:24]), "mono")
	assert.Equal(t, uint32(16000), binary.LittleEndian.Uint32(wav[24:28]))
	assert.Equal(t, uint32(32000), binary.LittleEndian.Uint32(wav[28:32]), "byte rate")
	assert.Equal(t, uint16(16), binary.LittleEndian.Uint16(wav[34:36]))
	assert.Equal(t, uint32(len(pcm)), binary.LittleEndian.Uint32(wav[40:44]))
	assert.Equal(t, 100, DurationMillis(len(pcm), 16000))
}
