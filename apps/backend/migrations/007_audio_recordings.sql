-- Migration 007: retained answer audio, machine transcriptions and answer revisions.
--
-- Audio itself lives in object storage; PostgreSQL holds only metadata.
-- audio_recordings, transcriptions and answer_revisions are append-only records:
-- application code inserts rows and, for recordings, may later mark one DELETED
-- when its retention ends. Nothing here is ever rewritten to change what was
-- said or what was saved.

-- One row per retained audio segment of an answer. A row exists only once the
-- audio has been stored and its checksum verified, so there is no half-written
-- state to clean up. (session_id, stream_id) makes a retried finalize idempotent.
CREATE TABLE IF NOT EXISTS audio_recordings (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    session_id UUID NOT NULL REFERENCES sessions(id) ON DELETE RESTRICT,
    question_id UUID NOT NULL REFERENCES questions(id) ON DELETE RESTRICT,
    student_id UUID NOT NULL REFERENCES users(id) ON DELETE RESTRICT,
    stream_id UUID NOT NULL,
    segment_index INT NOT NULL DEFAULT 0 CHECK (segment_index >= 0),
    status VARCHAR(20) NOT NULL DEFAULT 'STORED' CHECK (status IN ('STORED', 'DELETED')),
    content_type VARCHAR(50) NOT NULL,
    sample_rate INT NOT NULL CHECK (sample_rate > 0),
    duration_ms INT NOT NULL CHECK (duration_ms >= 0),
    byte_size BIGINT NOT NULL CHECK (byte_size > 0),
    chunk_count INT NOT NULL CHECK (chunk_count > 0),
    sha256 BYTEA NOT NULL CHECK (octet_length(sha256) = 32),
    -- Opaque storage key: random, containing no student, exam or question data.
    storage_key TEXT NOT NULL UNIQUE,
    recorded_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    retention_until TIMESTAMPTZ,
    deleted_at TIMESTAMPTZ,
    CONSTRAINT uq_recording_stream UNIQUE (session_id, stream_id)
);

CREATE INDEX IF NOT EXISTS idx_audio_recordings_session_question
    ON audio_recordings (session_id, question_id, segment_index);
CREATE INDEX IF NOT EXISTS idx_audio_recordings_retention
    ON audio_recordings (retention_until) WHERE deleted_at IS NULL AND retention_until IS NOT NULL;

-- What the speech recognizer produced for a recording. Never edited.
CREATE TABLE IF NOT EXISTS transcriptions (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    recording_id UUID NOT NULL REFERENCES audio_recordings(id) ON DELETE RESTRICT,
    text TEXT NOT NULL,
    provider VARCHAR(50) NOT NULL DEFAULT '',
    language VARCHAR(20) NOT NULL DEFAULT '',
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT uq_transcription_recording UNIQUE (recording_id)
);

-- Every value an answer has held, in order. answers.text_answer / selected_option_id
-- remain the current value, so existing endpoints are unchanged.
-- Any save that carries a confirmation_id (a voice confirmation, whatever its
-- source) is a deliberate act and is never modified or merged. Only plain saves
-- without one coalesce: consecutive TYPED saves (autosave while typing) and OPTION
-- saves (mouse selection) update the latest such revision, so a student typing for
-- ten minutes does not create hundreds of rows.
CREATE TABLE IF NOT EXISTS answer_revisions (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    answer_id UUID NOT NULL REFERENCES answers(id) ON DELETE CASCADE,
    revision_no INT NOT NULL CHECK (revision_no > 0),
    source VARCHAR(30) NOT NULL CHECK (source IN ('OPTION', 'TYPED', 'STT_CONFIRMED', 'STUDENT_CORRECTION')),
    selected_option_id UUID REFERENCES question_options(id) ON DELETE SET NULL,
    text_answer TEXT NOT NULL DEFAULT '',
    transcription_id UUID REFERENCES transcriptions(id) ON DELETE RESTRICT,
    -- The client's confirmation ID, so a repeated confirm cannot add a revision.
    confirmation_id VARCHAR(64),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT uq_answer_revision UNIQUE (answer_id, revision_no)
);

CREATE UNIQUE INDEX IF NOT EXISTS uq_answer_revision_confirmation
    ON answer_revisions (answer_id, confirmation_id) WHERE confirmation_id IS NOT NULL;

-- Which revision a recording supported. NULL until the student confirms.
ALTER TABLE audio_recordings
    ADD COLUMN IF NOT EXISTS answer_revision_id UUID REFERENCES answer_revisions(id) ON DELETE SET NULL;

-- Append-only record of who did what to sensitive data. It never stores audio
-- or transcript content.
CREATE TABLE IF NOT EXISTS audit_events (
    id BIGSERIAL PRIMARY KEY,
    occurred_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    actor_id UUID,
    actor_role VARCHAR(50) NOT NULL DEFAULT '',
    action VARCHAR(60) NOT NULL,
    resource_type VARCHAR(40) NOT NULL,
    resource_id UUID,
    session_id UUID,
    outcome VARCHAR(20) NOT NULL DEFAULT 'ok',
    request_id VARCHAR(64) NOT NULL DEFAULT '',
    ip INET,
    metadata JSONB NOT NULL DEFAULT '{}'::jsonb
);

CREATE INDEX IF NOT EXISTS idx_audit_events_resource ON audit_events (resource_type, resource_id, occurred_at);
CREATE INDEX IF NOT EXISTS idx_audit_events_actor ON audit_events (actor_id, occurred_at);

---- create above / drop below ----

DROP TABLE IF EXISTS audit_events;
ALTER TABLE IF EXISTS audio_recordings DROP COLUMN IF EXISTS answer_revision_id;
DROP INDEX IF EXISTS uq_answer_revision_confirmation;
DROP TABLE IF EXISTS answer_revisions;
DROP TABLE IF EXISTS transcriptions;
DROP TABLE IF EXISTS audio_recordings;
