-- Migration 004: Exam management enhancements (soft delete and query indexes)

ALTER TABLE exams ADD COLUMN IF NOT EXISTS deleted_at TIMESTAMPTZ;

CREATE INDEX IF NOT EXISTS idx_exams_deleted_at ON exams (deleted_at);
CREATE INDEX IF NOT EXISTS idx_exams_subject ON exams (subject);
CREATE INDEX IF NOT EXISTS idx_exams_created_by_status ON exams (created_by, status);

---- create above / drop below ----

DROP INDEX IF EXISTS idx_exams_created_by_status;
DROP INDEX IF EXISTS idx_exams_subject;
DROP INDEX IF EXISTS idx_exams_deleted_at;
ALTER TABLE exams DROP COLUMN IF EXISTS deleted_at;
