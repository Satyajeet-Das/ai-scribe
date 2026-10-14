-- Migration 005: Add current_question_id to sessions for durable question tracking and performance indexes

ALTER TABLE sessions ADD COLUMN IF NOT EXISTS current_question_id UUID REFERENCES questions(id) ON DELETE SET NULL;

CREATE INDEX IF NOT EXISTS idx_sessions_current_question_id ON sessions (current_question_id);
CREATE INDEX IF NOT EXISTS idx_sessions_student_id_status ON sessions (student_id, status);
CREATE INDEX IF NOT EXISTS idx_sessions_exam_student_status ON sessions (exam_id, student_id, status);

---- create above / drop below ----

DROP INDEX IF EXISTS idx_sessions_exam_student_status;
DROP INDEX IF EXISTS idx_sessions_student_id_status;
DROP INDEX IF EXISTS idx_sessions_current_question_id;
ALTER TABLE sessions DROP COLUMN IF EXISTS current_question_id;
