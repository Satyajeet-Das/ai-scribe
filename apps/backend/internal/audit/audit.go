// Package audit keeps an append-only record of who accessed sensitive data. It
// never stores audio or transcript content, only that an access happened, by
// whom, to which record, and with what outcome.
package audit

import (
	"context"
	"encoding/json"
	"net/netip"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Actions.
const (
	ActionAudioPlayed = "audio.played"
	// ActionAudioUnavailable records an authorized attempt to play a recording
	// whose audio could not be read.
	ActionAudioUnavailable = "audio.unavailable"
	// ActionAudioPurged records the deletion of a recording's audio because its
	// retention period ended. The actor is the system, not a person.
	ActionAudioPurged = "audio.purged"
	// ActionOrphanRemoved records the deletion of stored audio that no recording
	// referred to.
	ActionOrphanRemoved = "audio.orphan_removed"
)

// Resource types.
const (
	ResourceRecording = "audio_recording"
	// ResourceObject is a stored object that has no recording row.
	ResourceObject = "audio_object"
)

// Outcomes.
const (
	OutcomeOK          = "ok"
	OutcomeUnavailable = "unavailable"
)

// Event is one thing that happened.
type Event struct {
	ActorID      uuid.UUID
	ActorRole    string
	Action       string
	ResourceType string
	ResourceID   uuid.UUID
	SessionID    uuid.UUID
	Outcome      string
	RequestID    string
	// IP is the client address as the server saw it. Empty or unparsable
	// values are stored as NULL.
	IP string
	// Metadata is small structured context. It must not contain audio,
	// transcripts or answers.
	Metadata map[string]any
}

// Recorder appends events. Callers that must not proceed without a record
// (playing a recording) treat an error as a reason to stop.
type Recorder interface {
	Record(ctx context.Context, e Event) error
}

type pgRecorder struct {
	pool *pgxpool.Pool
}

// NewRecorder writes events to the audit_events table.
func NewRecorder(pool *pgxpool.Pool) Recorder { return &pgRecorder{pool: pool} }

func (r *pgRecorder) Record(ctx context.Context, e Event) error {
	var ip *netip.Addr
	if a, err := netip.ParseAddr(e.IP); err == nil {
		ip = &a
	}
	meta := e.Metadata
	if meta == nil {
		meta = map[string]any{}
	}
	metaJSON, err := json.Marshal(meta)
	if err != nil {
		return err
	}
	var resource, session *uuid.UUID
	if e.ResourceID != uuid.Nil {
		resource = &e.ResourceID
	}
	if e.SessionID != uuid.Nil {
		session = &e.SessionID
	}
	var actor *uuid.UUID
	if e.ActorID != uuid.Nil {
		actor = &e.ActorID
	}
	requestID := e.RequestID
	if len(requestID) > 64 {
		requestID = requestID[:64] // the column is VARCHAR(64)
	}
	outcome := e.Outcome
	if outcome == "" {
		outcome = OutcomeOK
	}
	_, err = r.pool.Exec(ctx, `
		INSERT INTO audit_events
			(actor_id, actor_role, action, resource_type, resource_id, session_id, outcome, request_id, ip, metadata)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10::jsonb)`,
		actor, e.ActorRole, e.Action, e.ResourceType, resource, session, outcome, requestID, ip, string(metaJSON))
	return err
}
