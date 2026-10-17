-- Migration 006: Index for teacher response review.
-- The roster picks one session per assignment (submitted first, then latest),
-- which this index serves without a sort.

CREATE INDEX IF NOT EXISTS idx_sessions_assignment_started ON sessions (assignment_id, started_at DESC);

---- create above / drop below ----

DROP INDEX IF EXISTS idx_sessions_assignment_started;
