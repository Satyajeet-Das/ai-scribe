package voice

import (
	"context"
	"errors"
	"sync"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Satyajeet-Das/ai-scribe/internal/answer"
	"github.com/Satyajeet-Das/ai-scribe/internal/recording"
)

// fakeRecorder stands in for the recording service.
type fakeRecorder struct {
	mu    sync.Mutex
	saves []recording.SaveInput
	err   error
	gate  chan struct{}
}

func (f *fakeRecorder) Save(ctx context.Context, in recording.SaveInput) (*recording.Saved, error) {
	if f.gate != nil {
		select {
		case <-f.gate:
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.err != nil {
		return nil, f.err
	}
	f.saves = append(f.saves, in)
	return &recording.Saved{RecordingID: uuid.New(), TranscriptionID: uuid.New(), DurationMillis: 40}, nil
}

func (f *fakeRecorder) inputs() []recording.SaveInput {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]recording.SaveInput(nil), f.saves...)
}

func recordingStatus(t *testing.T, c *client) RecordingStatusPayload {
	t.Helper()
	return decode[RecordingStatusPayload](t, c.until(TypeRecordingStatus))
}

func TestRecording_DictatedAnswerIsKeptWithItsTranscript(t *testing.T) {
	h := newHarness(t, withoutTTS(), withRecorder())
	c, ready := h.connect()
	assert.True(t, ready.Capabilities.Recording, "the client is told audio is kept")
	qid := toEssay(t, h, c)

	stream, _ := h.sayAs(c, AudioModeAnswer, "Plants turn light into energy")
	status := recordingStatus(t, c)
	assert.Equal(t, RecordingStored, status.Status)
	assert.Equal(t, stream.String(), status.StreamID)
	assert.NotEmpty(t, status.RecordingID)

	saves := h.rec.inputs()
	require.Len(t, saves, 1)
	assert.Equal(t, h.sessionID, saves[0].SessionID)
	assert.Equal(t, qid, saves[0].QuestionID)
	assert.Equal(t, h.student, saves[0].StudentID)
	assert.Equal(t, stream, saves[0].StreamID)
	assert.Equal(t, "Plants turn light into energy", saves[0].Transcript)
	assert.Equal(t, 16000, saves[0].SampleRate)
	assert.Len(t, saves[0].PCM, 2*640, "exactly the audio that was sent")
	assert.Equal(t, 2, saves[0].ChunkCount)
}

func TestRecording_ConfirmedAnswerCarriesItsProvenance(t *testing.T) {
	h := newHarness(t, withoutTTS(), withRecorder())
	c, _ := h.connect()
	qid := toEssay(t, h, c)

	h.sayAs(c, AudioModeAnswer, "Plants turn light into energy")
	status := recordingStatus(t, c)
	confirm := decode[ConfirmRequestPayload](t, c.until(TypeConfirmRequest))
	assert.Zero(t, h.exam.count("SubmitAnswer"), "keeping audio saves nothing")

	command(t, c, CommandPayload{Intent: "confirm"})
	_, saved := h.exam.saved(qid)
	require.True(t, saved)

	revs := h.exam.revisionsOf()
	require.Len(t, revs, 1)
	assert.Equal(t, answer.SourceSTTConfirmed, revs[0].Source)
	assert.Equal(t, confirm.ConfirmationID, revs[0].ConfirmationID)
	require.NotNil(t, revs[0].TranscriptionID)
	require.Len(t, revs[0].RecordingIDs, 1)
	assert.Equal(t, status.RecordingID, revs[0].RecordingIDs[0].String())
}

func TestRecording_StorageFailureNeverBlocksTheAnswer(t *testing.T) {
	h := newHarness(t, withoutTTS(), withRecorder())
	h.rec.err = errors.New("bucket unreachable")
	c, _ := h.connect()
	qid := toEssay(t, h, c)

	h.sayAs(c, AudioModeAnswer, "Plants turn light into energy")
	status := recordingStatus(t, c)
	assert.Equal(t, RecordingFailed, status.Status)
	assert.Empty(t, status.RecordingID)

	// The student is still asked to confirm, and can save the answer.
	confirm := decode[ConfirmRequestPayload](t, c.until(TypeConfirmRequest))
	assert.Equal(t, "Plants turn light into energy", confirm.Text)
	command(t, c, CommandPayload{Intent: "confirm"})
	ans, saved := h.exam.saved(qid)
	require.True(t, saved)
	assert.Equal(t, "Plants turn light into energy", ans.TextAnswer)

	revs := h.exam.revisionsOf()
	require.Len(t, revs, 1)
	assert.Empty(t, revs[0].RecordingIDs, "no recording is claimed that does not exist")
	assert.Nil(t, revs[0].TranscriptionID)
}

func TestRecording_CommandsAreNeverRetained(t *testing.T) {
	h := newHarness(t, withoutTTS(), withRecorder())
	c, _ := h.connect()
	toEssay(t, h, c)

	h.sayAs(c, AudioModeCommand, "read the question")
	c.until(TypeResponse)
	h.sayAs(c, "", "how much time is left")
	c.until(TypeResponse)

	assert.Empty(t, h.rec.inputs())
	assert.Zero(t, countOf(c, TypeRecordingStatus))
}

func TestRecording_MultipleChoiceUtterancesAreNotRetained(t *testing.T) {
	h := newHarness(t, withoutTTS(), withRecorder())
	c, _ := h.connect() // question 1 is multiple choice

	h.sayAs(c, AudioModeAnswer, "I think it is Paris")
	c.until(TypeResponse)
	assert.Empty(t, h.rec.inputs(), "audio that cannot become an answer is not kept")
}

func TestRecording_SilenceIsNotRetained(t *testing.T) {
	h := newHarness(t, withoutTTS(), withRecorder())
	c, _ := h.connect()
	toEssay(t, h, c)

	h.sayAs(c, AudioModeAnswer, "")
	c.until(TypeResponse)
	assert.Empty(t, h.rec.inputs())
}

func TestRecording_OffByDefaultKeepsNothingAndSaysSo(t *testing.T) {
	h := newHarness(t, withoutTTS())
	c, ready := h.connect()
	assert.False(t, ready.Capabilities.Recording)
	toEssay(t, h, c)

	h.sayAs(c, AudioModeAnswer, "Plants turn light into energy")
	c.until(TypeConfirmRequest)
	assert.Zero(t, countOf(c, TypeRecordingStatus))
}

func TestRecording_InterruptedUtteranceIsNotRetained(t *testing.T) {
	h := newHarness(t, withoutTTS(), withRecorder())
	h.rec.gate = make(chan struct{}) // storing blocks, so the turn is still in progress
	c, _ := h.connect()
	toEssay(t, h, c)

	h.sayAs(c, AudioModeAnswer, "first attempt")
	// The student starts again before the first utterance finished storing.
	h.sayAs(c, AudioModeCommand, "how much time is left")
	close(h.rec.gate)
	c.until(TypeResponse)

	for _, in := range h.rec.inputs() {
		assert.NotEqual(t, "first attempt", in.Transcript)
	}
}

// countOf counts messages of a type already received.
func countOf(c *client, typ string) int {
	n := 0
	for _, f := range c.seen {
		if f.env != nil && f.env.Type == typ {
			n++
		}
	}
	return n
}

func TestDictationBuffer_IsReleasedWhichEverWayTheTurnEnds(t *testing.T) {
	var tn turn
	tn.addPCM([]byte{1, 2, 3, 4})
	tn.addPCM([]byte{5, 6})
	assert.Equal(t, []byte{1, 2, 3, 4, 5, 6}, tn.takePCM())
	assert.Nil(t, tn.takePCM(), "once handed over it is no longer held")
	assert.Nil(t, tn.pcm)
}

func TestDictationBuffer_IsSafeWhileAudioStillArrives(t *testing.T) {
	var tn turn
	done := make(chan struct{})
	go func() {
		defer close(done)
		for i := 0; i < 1000; i++ {
			tn.addPCM([]byte{1, 2})
		}
	}()
	for i := 0; i < 100; i++ {
		_ = tn.takePCM() // the turn ending while a late chunk arrives
	}
	<-done
	tn.takePCM()
}

func TestRecording_AnInterruptedDictationLeavesNoAudioBehind(t *testing.T) {
	h := newHarness(t, withoutTTS(), withRecorder())
	c, _ := h.connect()
	toEssay(t, h, c)

	// Start dictating, then give up (cancel) instead of ending the stream.
	stream := uuid.New()
	c.send(TypeAudioStart, AudioStartPayload{StreamID: stream.String(), Encoding: "pcm_s16le", SampleRate: 16000, Channels: 1, Mode: AudioModeAnswer})
	c.chunk(stream, 0, 640)
	c.send(TypeAudioCancel, AudioCancelPayload{StreamID: stream.String()})
	c.until(TypeAck)

	h.sayAs(c, AudioModeCommand, "how much time is left")
	c.until(TypeResponse)
	assert.Empty(t, h.rec.inputs())
	conv := h.onlyConversation(t)
	conv.mu.Lock()
	cur := conv.turn
	conv.mu.Unlock()
	assert.Nil(t, cur, "no turn is left holding audio")
}
