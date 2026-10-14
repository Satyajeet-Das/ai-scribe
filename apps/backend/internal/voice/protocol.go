// Package voice is the real-time voice interaction module: it terminates the
// exam WebSocket, ingests audio, runs speech recognition and intent
// classification, and turns validated intents into calls on the existing
// exam-session and answer services.
//
// The invariant the package is built around:
//
//	The LLM interprets. The backend decides. PostgreSQL persists.
package voice

import (
	"encoding/binary"
	"encoding/json"
	"errors"
	"time"

	"github.com/google/uuid"

	"github.com/Satyajeet-Das/ai-scribe/internal/platform/speech"
	"github.com/Satyajeet-Das/ai-scribe/internal/platform/ws"
	"github.com/Satyajeet-Das/ai-scribe/internal/session"
)

// ProtocolVersion is the version carried in every envelope ("v").
const ProtocolVersion = 1

const (
	// Subprotocol is negotiated on every connection.
	Subprotocol = "scribe.v1"
	// AuthSubprotocolPrefix lets browsers, which cannot set an Authorization
	// header on a WebSocket, pass the access token as a second subprotocol.
	AuthSubprotocolPrefix = "scribe.auth."
)

// Client → server message types.
const (
	TypePing        = "ping"
	TypeAuthRefresh = "auth.refresh"
	TypeSessionSync = "session.sync"
	TypeAudioStart  = "audio.start"
	TypeAudioEnd    = "audio.end"
	TypeAudioCancel = "audio.cancel"
	TypeTTSCancel   = "tts.cancel"
	TypeCommand     = "command"
)

// Server → client message types.
const (
	TypeReady          = "ready"
	TypePong           = "pong"
	TypeAck            = "ack"
	TypeError          = "error"
	TypeSessionState   = "session.state"
	TypeSessionClosed  = "session.closed"
	TypeAuthExpiring   = "auth.expiring"
	TypeSTTPartial     = "stt.partial"
	TypeSTTFinal       = "stt.final"
	TypeIntent         = "intent"
	TypeConfirmRequest = "confirm.request"
	TypeActionResult   = "action.result"
	TypeResponse       = "response"
	TypeTTSStart       = "tts.start"
	TypeTTSEnd         = "tts.end"
)

// Error codes carried in "error" messages and failed results.
const (
	CodeInvalidMessage     = "invalid_message"
	CodeUnsupportedVersion = "unsupported_version"
	CodeUnknownType        = "unknown_type"
	CodeDuplicateRequest   = "duplicate_request"
	CodeRateLimited        = "rate_limited"
	CodeUnauthorized       = "unauthorized"
	CodeForbidden          = "forbidden"

	CodeInvalidAudioFormat  = "invalid_audio_format"
	CodeStreamActive        = "stream_already_active"
	CodeUnknownStream       = "unknown_stream"
	CodeInvalidFrame        = "invalid_frame"
	CodeChunkTooLarge       = "chunk_too_large"
	CodeChunkOutOfOrder     = "chunk_out_of_order"
	CodeStreamTooLarge      = "stream_too_large"
	CodeStreamTooLong       = "stream_too_long"
	CodeChunkCountMismatch  = "chunk_count_mismatch"
	CodeEmptyStream         = "empty_stream"
	CodeBackpressure        = "audio_backpressure"
	CodeSTTUnavailable      = "stt_unavailable"
	CodeSTTFailed           = "stt_failed"
	CodeSTTTimeout          = "stt_timeout"
	CodeProviderBusy        = "provider_busy"
	CodeIntentUnavailable   = "intent_unavailable"
	CodeTTSUnavailable      = "tts_unavailable"
	CodeTTSFailed           = "tts_failed"
	CodeTurnCancelled       = "turn_cancelled"
	CodeNoSpeech            = "no_speech"
	CodeNotUnderstood       = "not_understood"
	CodeUnsupportedIntent   = "unsupported_intent"
	CodeInvalidOption       = "invalid_option"
	CodeNothingToConfirm    = "nothing_to_confirm"
	CodeConfirmationExpired = "confirmation_expired"
	CodeConfirmationUnknown = "confirmation_mismatch"
	CodeNoNextQuestion      = "no_next_question"
	CodeNoPreviousQuestion  = "no_previous_question"
	CodeNoAnswer            = "no_answer"
	CodeSessionExpired      = "session_expired"
	CodeSessionSubmitted    = "session_submitted"
	CodeSessionInactive     = "session_inactive"
	CodeExamUnavailable     = "exam_unavailable"
	CodeBusy                = "busy"
	CodeUnavailable         = "service_unavailable"
	CodeInternal            = "internal_error"
)

// Application close codes (4000-4999 are reserved for applications).
const (
	CloseUnauthorized ws.StatusCode = 4401
	CloseForbidden    ws.StatusCode = 4403
	CloseReplaced     ws.StatusCode = 4409
	CloseSessionEnded ws.StatusCode = 4410
	CloseSlowConsumer ws.StatusCode = 4429
)

// maxRequestIDLen bounds the client-chosen request and stream identifiers.
const maxRequestIDLen = 64

// Envelope is the frame every JSON message travels in, in both directions.
type Envelope struct {
	// V is the protocol version.
	V int `json:"v"`
	// Type selects the payload schema.
	Type string `json:"type"`
	// ID is the sender's request ID. Client IDs must be unique per connection;
	// a repeated ID is rejected as a duplicate and never processed twice.
	ID string `json:"id,omitempty"`
	// ReplyTo is the request ID a server message answers, when there is one.
	ReplyTo string `json:"replyTo,omitempty"`
	// TS is the server's send time in Unix milliseconds.
	TS      int64           `json:"ts,omitempty"`
	Payload json.RawMessage `json:"payload,omitempty"`
}

// ---------------------------------------------------------------------------
// Client payloads
// ---------------------------------------------------------------------------

type AuthRefreshPayload struct {
	Token string `json:"token"`
}

type AudioStartPayload struct {
	StreamID   string `json:"streamId"`
	Encoding   string `json:"encoding"`
	SampleRate int    `json:"sampleRate"`
	Channels   int    `json:"channels"`
}

type AudioEndPayload struct {
	StreamID string `json:"streamId"`
	// Chunks, when present, is the number of chunks the client sent; a
	// mismatch means audio was lost and the stream is rejected.
	Chunks *uint32 `json:"chunks,omitempty"`
}

type AudioCancelPayload struct {
	StreamID string `json:"streamId"`
}

type TTSCancelPayload struct {
	ResponseID string `json:"responseId,omitempty"`
}

// CommandPayload is a typed intent sent without speech (keyboard, switch
// device, or a client-side fallback). It takes exactly the same validation and
// confirmation path as a spoken intent.
type CommandPayload struct {
	Intent         string `json:"intent"`
	OptionKey      string `json:"optionKey,omitempty"`
	ConfirmationID string `json:"confirmationId,omitempty"`
}

// ---------------------------------------------------------------------------
// Server payloads
// ---------------------------------------------------------------------------

type ErrorPayload struct {
	Code      string `json:"code"`
	Message   string `json:"message"`
	Retryable bool   `json:"retryable"`
	StreamID  string `json:"streamId,omitempty"`
}

type Limits struct {
	MaxMessageBytes   int64    `json:"maxMessageBytes"`
	MaxChunkBytes     int      `json:"maxChunkBytes"`
	MaxStreamBytes    int64    `json:"maxStreamBytes"`
	MaxStreamMillis   int64    `json:"maxStreamMillis"`
	AudioEncodings    []string `json:"audioEncodings"`
	AudioSampleRates  []int    `json:"audioSampleRates"`
	AudioChannels     int      `json:"audioChannels"`
	ConfirmTTLMillis  int64    `json:"confirmTtlMillis"`
	BinaryHeaderBytes int      `json:"binaryHeaderBytes"`
}

type Capabilities struct {
	STT    bool                `json:"stt"`
	TTS    bool                `json:"tts"`
	TTSOut *speech.AudioFormat `json:"ttsFormat,omitempty"`
	Intent string              `json:"intentClassifier"`
}

type ReadyPayload struct {
	ConnectionID string       `json:"connectionId"`
	Protocol     int          `json:"protocol"`
	SessionID    uuid.UUID    `json:"sessionId"`
	Resumed      bool         `json:"resumed"`
	Limits       Limits       `json:"limits"`
	Capabilities Capabilities `json:"capabilities"`
	Session      SessionState `json:"session"`
}

// SessionState is the authoritative exam state, always read from the session
// service (and therefore PostgreSQL), never from connection memory.
type SessionState struct {
	Status           session.Status                   `json:"status"`
	RemainingSeconds *int64                           `json:"remainingSeconds,omitempty"`
	Question         *session.StudentQuestionResponse `json:"question,omitempty"`
}

type SessionClosedPayload struct {
	Reason string `json:"reason"`
}

type AuthExpiringPayload struct {
	ExpiresAt time.Time `json:"expiresAt"`
}

type TranscriptPayload struct {
	StreamID string `json:"streamId"`
	Text     string `json:"text"`
}

type IntentPayload struct {
	StreamID   string  `json:"streamId,omitempty"`
	Intent     string  `json:"intent"`
	OptionKey  string  `json:"optionKey,omitempty"`
	Confidence float64 `json:"confidence"`
	Classifier string  `json:"classifier"`
}

type ConfirmRequestPayload struct {
	ConfirmationID string    `json:"confirmationId"`
	Action         string    `json:"action"`
	QuestionID     uuid.UUID `json:"questionId"`
	OptionKey      string    `json:"optionKey,omitempty"`
	OptionText     string    `json:"optionText,omitempty"`
	Text           string    `json:"text,omitempty"`
	ExpiresAt      time.Time `json:"expiresAt"`
}

// ActionResultPayload reports the outcome of a state-changing action.
// Saved is true only after the answer service has durably persisted the answer.
type ActionResultPayload struct {
	Action         string     `json:"action"`
	Status         string     `json:"status"`
	Code           string     `json:"code,omitempty"`
	Retryable      bool       `json:"retryable,omitempty"`
	Saved          bool       `json:"saved"`
	Duplicate      bool       `json:"duplicate,omitempty"`
	ConfirmationID string     `json:"confirmationId,omitempty"`
	QuestionID     *uuid.UUID `json:"questionId,omitempty"`
	OptionKey      string     `json:"optionKey,omitempty"`
	AnswerID       *uuid.UUID `json:"answerId,omitempty"`
	SavedAt        *time.Time `json:"savedAt,omitempty"`
	Direction      string     `json:"direction,omitempty"`
}

const (
	ActionAnswer   = "answer"
	ActionNavigate = "navigate"

	StatusOK        = "ok"
	StatusFailed    = "failed"
	StatusRejected  = "rejected"
	StatusCancelled = "cancelled"
)

// ResponsePayload is what the assistant says, as text. It is always sent, so
// a client can show or locally speak it even when server TTS is unavailable.
type ResponsePayload struct {
	ResponseID string `json:"responseId"`
	StreamID   string `json:"streamId,omitempty"`
	Kind       string `json:"kind"`
	Code       string `json:"code,omitempty"`
	Text       string `json:"text"`
	// Audio is true when a tts.start for this response will follow.
	Audio bool `json:"audio"`
}

type TTSStartPayload struct {
	ResponseID string             `json:"responseId"`
	Format     speech.AudioFormat `json:"format"`
}

type TTSEndPayload struct {
	ResponseID string `json:"responseId"`
	// Reason is completed, barge_in, cancelled or error.
	Reason string `json:"reason"`
	Code   string `json:"code,omitempty"`
	Chunks uint32 `json:"chunks"`
}

// ---------------------------------------------------------------------------
// Binary audio frames
// ---------------------------------------------------------------------------

// Binary frames, in both directions, are:
//
//	byte  0      frame version (1)
//	bytes 1-16   stream ID (client audio) or response ID (server audio), UUID
//	bytes 17-20  sequence number, big-endian uint32, starting at 0
//	bytes 21-    PCM16LE audio
const (
	frameVersion    byte = 1
	FrameHeaderSize      = 21
)

var errInvalidFrame = errors.New("voice: invalid binary frame")

type audioFrame struct {
	ID    uuid.UUID
	Seq   uint32
	Audio []byte
}

func decodeFrame(data []byte) (audioFrame, error) {
	if len(data) < FrameHeaderSize || data[0] != frameVersion {
		return audioFrame{}, errInvalidFrame
	}
	var f audioFrame
	copy(f.ID[:], data[1:17])
	f.Seq = binary.BigEndian.Uint32(data[17:21])
	f.Audio = data[FrameHeaderSize:]
	return f, nil
}

func encodeFrame(id uuid.UUID, seq uint32, audio []byte) []byte {
	out := make([]byte, FrameHeaderSize+len(audio))
	out[0] = frameVersion
	copy(out[1:17], id[:])
	binary.BigEndian.PutUint32(out[17:21], seq)
	copy(out[FrameHeaderSize:], audio)
	return out
}

// validID reports whether a client-chosen identifier is acceptable.
func validID(id string) bool {
	if id == "" || len(id) > maxRequestIDLen {
		return false
	}
	for _, r := range id {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9', r == '-', r == '_', r == '.', r == ':':
		default:
			return false
		}
	}
	return true
}

// allowedSampleRates are the PCM sample rates accepted from clients.
var allowedSampleRates = []int{8000, 16000, 24000, 48000}

func validSampleRate(rate int) bool {
	for _, r := range allowedSampleRates {
		if r == rate {
			return true
		}
	}
	return false
}
