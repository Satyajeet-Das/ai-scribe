package review

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/google/uuid"
	"github.com/labstack/echo/v4"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Satyajeet-Das/ai-scribe/internal/audit"
	"github.com/Satyajeet-Das/ai-scribe/internal/platform/audiostore"
	platformauth "github.com/Satyajeet-Das/ai-scribe/internal/platform/auth"
)

// fakeAudit records events, and can be told to fail.
type fakeAudit struct {
	mu     sync.Mutex
	events []audit.Event
	err    error
}

func (f *fakeAudit) Record(_ context.Context, e audit.Event) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.err != nil {
		return f.err
	}
	f.events = append(f.events, e)
	return nil
}

func (f *fakeAudit) all() []audit.Event {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]audit.Event(nil), f.events...)
}

// playbackFixture is a review service with one stored recording.
type playbackFixture struct {
	*fixture
	playback  PlaybackService
	store     *audiostore.Memory
	audit     *fakeAudit
	file      *RecordingFile
	studentID uuid.UUID
	audioData []byte
}

func newPlaybackFixture(t *testing.T, policy Policy) *playbackFixture {
	t.Helper()
	f := newFixture(policy)
	store, aud := audiostore.NewMemory(), &fakeAudit{}
	key := audiostore.NewKey()
	data := []byte("RIFF-pretend-wav-audio")
	require.NoError(t, store.Put(context.Background(), key, bytes.NewReader(data), int64(len(data))))

	file := &RecordingFile{
		ID: uuid.New(), SessionID: uuid.New(), SessionStatus: StatusSubmitted,
		StorageKey: key, ContentType: "audio/wav", ByteSize: int64(len(data)),
	}
	f.repo.file = file
	return &playbackFixture{
		fixture: f, playback: EnablePlayback(f.svc, store, aud), store: store, audit: aud,
		file: file, studentID: uuid.New(), audioData: data,
	}
}

func (p *playbackFixture) open(caller Caller) (*Audio, error) {
	return p.playback.OpenRecording(context.Background(), p.examID, p.studentID, p.file.ID, caller,
		AccessMeta{RequestID: "req-1", IP: "203.0.113.9"})
}

func TestPlayback_OwnerAndAdminCanPlayAndEveryAccessIsAudited(t *testing.T) {
	p := newPlaybackFixture(t, DefaultPolicy())

	for _, caller := range []Caller{p.teacher, p.admin} {
		audio, err := p.open(caller)
		require.NoError(t, err)
		got, _ := io.ReadAll(audio.Body)
		audio.Body.Close()
		assert.Equal(t, p.audioData, got)
		assert.Equal(t, "audio/wav", audio.ContentType)
	}

	events := p.audit.all()
	require.Len(t, events, 2)
	assert.Equal(t, p.teacher.ID, events[0].ActorID)
	assert.Equal(t, audit.ActionAudioPlayed, events[0].Action)
	assert.Equal(t, p.file.ID, events[0].ResourceID)
	assert.Equal(t, p.file.SessionID, events[0].SessionID)
	assert.Equal(t, "req-1", events[0].RequestID)
	assert.Equal(t, "203.0.113.9", events[0].IP)
	assert.Equal(t, p.admin.ID, events[1].ActorID)
}

func TestPlayback_RefusesEveryoneElseWithoutTouchingStorageOrTheAuditLog(t *testing.T) {
	p := newPlaybackFixture(t, DefaultPolicy())

	_, err := p.open(p.other) // another teacher: does not own the exam
	assert.ErrorIs(t, err, ErrExamNotFound, "indistinguishable from a missing exam")
	_, err = p.open(p.fixture.student) // a student, even with a valid recording ID
	assert.ErrorIs(t, err, ErrForbiddenRole)
	_, err = p.open(Caller{ID: uuid.New(), Role: platformauth.RoleProctor})
	assert.ErrorIs(t, err, ErrForbiddenRole)

	assert.Empty(t, p.audit.all(), "a refused request plays nothing, so there is no playback to record")
}

func TestPlayback_ARecordingOfAnotherStudentOrExamIsNotFound(t *testing.T) {
	p := newPlaybackFixture(t, DefaultPolicy())
	p.repo.file = nil
	p.repo.err = ErrRecordingNotFound // what the repository returns for a mismatched student or exam

	_, err := p.open(p.teacher)
	assert.ErrorIs(t, err, ErrRecordingNotFound)
	assert.Empty(t, p.audit.all())
}

func TestPlayback_NothingIsPlayedWhenTheAccessCannotBeRecorded(t *testing.T) {
	p := newPlaybackFixture(t, DefaultPolicy())
	p.audit.err = errors.New("audit database down")

	audio, err := p.open(p.teacher)
	assert.ErrorIs(t, err, ErrAuditUnavailable)
	assert.Nil(t, audio, "audio must not be returned without an audit entry")
}

func TestPlayback_RemovedAndMissingAudioAreDistinctStates(t *testing.T) {
	t.Run("removed", func(t *testing.T) {
		p := newPlaybackFixture(t, DefaultPolicy())
		p.file.Removed = true
		_, err := p.open(p.teacher)
		assert.ErrorIs(t, err, ErrRecordingRemoved)
		assert.Empty(t, p.audit.all())
	})
	t.Run("the file is gone from storage", func(t *testing.T) {
		p := newPlaybackFixture(t, DefaultPolicy())
		require.NoError(t, p.store.Delete(context.Background(), p.file.StorageKey))
		_, err := p.open(p.teacher)
		assert.ErrorIs(t, err, ErrAudioUnavailable)
		events := p.audit.all()
		require.Len(t, events, 1)
		assert.Equal(t, audit.ActionAudioUnavailable, events[0].Action, "the attempt is noted, but not as a playback")
	})
}

// unreadableStore opens objects whose content then fails to read.
type unreadableStore struct{ *audiostore.Memory }

type failingReader struct{}

func (failingReader) Read([]byte) (int, error) { return 0, errors.New("connection reset mid-object") }

func (s unreadableStore) Open(context.Context, string) (io.ReadCloser, int64, error) {
	return io.NopCloser(failingReader{}), 100, nil
}

func TestPlayback_AudioThatCannotBeReadIsNeverRecordedAsPlayed(t *testing.T) {
	p := newPlaybackFixture(t, DefaultPolicy())
	p.playback = EnablePlayback(p.svc, unreadableStore{p.store}, p.audit)

	_, err := p.open(p.teacher)
	assert.ErrorIs(t, err, ErrAudioUnavailable)
	events := p.audit.all()
	require.Len(t, events, 1)
	assert.Equal(t, audit.ActionAudioUnavailable, events[0].Action, "noted as unavailable, not as a playback")
}

func TestPlayback_TheDownloadNameFollowsTheContentType(t *testing.T) {
	assert.Equal(t, ".wav", audioExtension("audio/wav"))
	assert.Equal(t, ".webm", audioExtension("audio/webm; codecs=opus"))
	assert.Equal(t, "", audioExtension("application/octet-stream"))
}

func TestPlayback_FollowsTheSamePolicyAsTheAnswers(t *testing.T) {
	p := newPlaybackFixture(t, Policy{AllowInProgress: false})
	p.file.SessionStatus = StatusInProgress

	_, err := p.open(p.teacher)
	assert.ErrorIs(t, err, ErrRecordingNotFound, "audio of an unfinished session is hidden when its answers are")

	p.file.SessionStatus = StatusSubmitted
	_, err = p.open(p.teacher)
	assert.NoError(t, err)
}

func TestPlayback_DisabledUntilConfigured(t *testing.T) {
	f := newFixture(DefaultPolicy())
	_, err := f.svc.(*service).OpenRecording(context.Background(), f.examID, uuid.New(), uuid.New(), f.teacher, AccessMeta{})
	assert.ErrorIs(t, err, ErrPlaybackDisabled)
}

func TestPlaybackHandler_ServesAudioSafelyWithRangeSupport(t *testing.T) {
	p := newPlaybackFixture(t, DefaultPolicy())
	h := NewHandler(p.svc).WithPlayback(p.playback)

	e := echo.New()
	request := func(rangeHeader string) *httptest.ResponseRecorder {
		req := httptest.NewRequest(http.MethodGet, "/", nil)
		if rangeHeader != "" {
			req.Header.Set("Range", rangeHeader)
		}
		rec := httptest.NewRecorder()
		c := e.NewContext(req, rec)
		c.SetParamNames("exam_id", "student_id", "recording_id")
		c.SetParamValues(p.examID.String(), p.studentID.String(), p.file.ID.String())
		c.Set("user_id", p.teacher.ID.String())
		c.Set("user_role", "teacher")
		require.NoError(t, h.GetRecordingAudio(c))
		return rec
	}

	full := request("")
	assert.Equal(t, http.StatusOK, full.Code)
	assert.Equal(t, string(p.audioData), full.Body.String())
	assert.Equal(t, "audio/wav", full.Header().Get("Content-Type"))
	assert.Contains(t, full.Header().Get("Cache-Control"), "no-store")
	assert.Equal(t, "nosniff", full.Header().Get("X-Content-Type-Options"))
	assert.Equal(t, `inline; filename="response.wav"`, full.Header().Get("Content-Disposition"),
		"the file name reveals nothing about the student")
	assert.Equal(t, "bytes", full.Header().Get("Accept-Ranges"))

	part := request("bytes=5-9")
	assert.Equal(t, http.StatusPartialContent, part.Code, "the player can seek")
	assert.Equal(t, string(p.audioData[5:10]), part.Body.String())
}

func TestPlaybackHandler_MapsFailuresToDistinctStatuses(t *testing.T) {
	cases := map[string]struct {
		err    error
		status int
	}{
		"not found":         {ErrRecordingNotFound, http.StatusNotFound},
		"wrong exam":        {ErrExamNotFound, http.StatusNotFound},
		"not a reviewer":    {ErrForbiddenRole, http.StatusForbidden},
		"removed":           {ErrRecordingRemoved, http.StatusGone},
		"unavailable":       {ErrAudioUnavailable, http.StatusServiceUnavailable},
		"audit unavailable": {ErrAuditUnavailable, http.StatusServiceUnavailable},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			h := NewHandler(&stubService{}).WithPlayback(failingPlayback{tc.err})
			_, err := call(t, h.GetRecordingAudio, "/", map[string]string{
				"exam_id": uuid.NewString(), "student_id": uuid.NewString(), "recording_id": uuid.NewString(),
			})
			assert.Equal(t, tc.status, statusOf(t, err))
		})
	}
}

func TestPlaybackHandler_AuditFailureDoesNotRevealWhy(t *testing.T) {
	h := NewHandler(&stubService{}).WithPlayback(failingPlayback{ErrAuditUnavailable})
	_, err := call(t, h.GetRecordingAudio, "/", map[string]string{
		"exam_id": uuid.NewString(), "student_id": uuid.NewString(), "recording_id": uuid.NewString(),
	})
	require.Error(t, err)
	assert.False(t, strings.Contains(strings.ToLower(err.Error()), "audit"))
}

func TestPlaybackRoute_OnlyExistsWhenPlaybackIsConfigured(t *testing.T) {
	routes := func(h *Handler) []string {
		e := echo.New()
		h.RegisterRoutes(e.Group("/api/v1"), nil, nil)
		var out []string
		for _, r := range e.Routes() {
			out = append(out, r.Method+" "+r.Path)
		}
		return out
	}
	without := routes(NewHandler(&stubService{}))
	with := routes(NewHandler(&stubService{}).WithPlayback(failingPlayback{}))

	audio := "GET /api/v1/exams/:exam_id/responses/:student_id/recordings/:recording_id/audio"
	assert.NotContains(t, without, audio)
	assert.Contains(t, with, audio)
	for _, r := range with {
		assert.True(t, strings.HasPrefix(r, "GET "), "playback adds no route that can change anything: %s", r)
	}
}

type failingPlayback struct{ err error }

func (f failingPlayback) OpenRecording(context.Context, uuid.UUID, uuid.UUID, uuid.UUID, Caller, AccessMeta) (*Audio, error) {
	return nil, f.err
}
