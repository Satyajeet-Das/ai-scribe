-- Migration 009: exam document ingestion (Sprint 12).
--
-- A teacher uploads an exam PDF; a background job extracts draft questions; the
-- teacher corrects them and approves. Nothing here is a published question: the
-- draft lives in its own tables and reaches `questions` only through the
-- explicit approval, in one transaction.
--
-- The PDF itself is NOT stored here. It lives in object storage under
-- `storage_key`, a random key that carries nothing about the exam or teacher.

CREATE TABLE IF NOT EXISTS exam_documents (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    exam_id UUID NOT NULL REFERENCES exams(id) ON DELETE CASCADE,
    uploaded_by UUID NOT NULL REFERENCES users(id) ON DELETE RESTRICT,
    -- Display name only, cleaned of path parts. Never used as a storage path.
    original_filename VARCHAR(255) NOT NULL,
    storage_key VARCHAR(64) NOT NULL UNIQUE,
    size_bytes BIGINT NOT NULL CHECK (size_bytes > 0),
    sha256 CHAR(64) NOT NULL,
    page_count INT NOT NULL CHECK (page_count > 0),

    status VARCHAR(20) NOT NULL DEFAULT 'QUEUED'
        CHECK (status IN ('QUEUED', 'PROCESSING', 'REVIEW_REQUIRED', 'COMPLETED', 'FAILED', 'CANCELLED')),
    -- Where a PROCESSING document is; NULL otherwise.
    stage VARCHAR(20) CHECK (stage IS NULL OR stage IN ('READING_PAGES', 'EXTRACTING', 'VALIDATING', 'SAVING')),
    progress_done INT NOT NULL DEFAULT 0 CHECK (progress_done >= 0),
    progress_total INT NOT NULL DEFAULT 0 CHECK (progress_total >= 0),

    -- Raised by every (re)queue. A job carries the attempt it was queued for, and
    -- every write a worker makes is conditional on it, so a stale or duplicate
    -- worker cannot overwrite a newer result.
    attempt INT NOT NULL DEFAULT 1 CHECK (attempt >= 1),
    -- A PROCESSING document is owned by one worker until this passes; the worker
    -- renews it while it works. A crashed worker's document can then be claimed.
    lease_expires_at TIMESTAMPTZ,

    provider VARCHAR(50) NOT NULL DEFAULT '',
    prompt_version VARCHAR(20) NOT NULL DEFAULT '',
    failure_code VARCHAR(50),
    -- Safe text written by this application, never provider output.
    failure_message VARCHAR(500),

    -- What was found about each page (kind, characters, headers removed). Kept so
    -- nothing the pipeline set aside is lost silently.
    page_summary JSONB NOT NULL DEFAULT '[]'::jsonb,
    -- Instructions that apply to the whole paper, and text the model could not
    -- attach to any question. Both are shown to the teacher.
    document_instructions TEXT NOT NULL DEFAULT '',
    unassigned_content JSONB NOT NULL DEFAULT '[]'::jsonb,

    -- Raised by every change to the draft. Edits say which version they were
    -- made against, so two editors cannot silently overwrite each other.
    draft_version INT NOT NULL DEFAULT 0 CHECK (draft_version >= 0),
    approved_by UUID REFERENCES users(id) ON DELETE RESTRICT,
    approved_at TIMESTAMPTZ,

    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),

    CONSTRAINT chk_documents_approved CHECK (
        (status = 'COMPLETED') = (approved_at IS NOT NULL AND approved_by IS NOT NULL)
    ),
    CONSTRAINT chk_documents_failed CHECK (
        (status = 'FAILED') = (failure_code IS NOT NULL)
    ),
    CONSTRAINT chk_documents_progress CHECK (progress_done <= progress_total OR progress_total = 0)
);

CREATE INDEX IF NOT EXISTS idx_exam_documents_exam ON exam_documents (exam_id, created_at DESC);
CREATE INDEX IF NOT EXISTS idx_exam_documents_lease ON exam_documents (status, lease_expires_at);
-- The same file cannot be uploaded to an exam twice while an upload of it is
-- live. A failed or cancelled one does not block uploading it again.
CREATE UNIQUE INDEX IF NOT EXISTS uq_exam_documents_live_sha
    ON exam_documents (exam_id, sha256) WHERE status NOT IN ('FAILED', 'CANCELLED');

-- The extracted draft, separate from published questions.
CREATE TABLE IF NOT EXISTS exam_document_questions (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    document_id UUID NOT NULL REFERENCES exam_documents(id) ON DELETE CASCADE,
    -- Order in the draft. Deferrable so a reorder can renumber in one statement.
    position INT NOT NULL CHECK (position > 0),
    -- The number as printed in the source ("3", "Q4", "2(b)"), kept as found.
    number_label VARCHAR(30) NOT NULL DEFAULT '',
    type VARCHAR(10) NOT NULL CHECK (type IN ('MCQ', 'ESSAY')),
    instructions TEXT NOT NULL DEFAULT '',
    text TEXT NOT NULL DEFAULT '',
    -- NULL until the teacher sets it: marks are never guessed.
    points INT CHECK (points IS NULL OR points >= 0),
    -- [{label, text, isCorrect}]. isCorrect is set only by the teacher.
    options JSONB NOT NULL DEFAULT '[]'::jsonb,
    tables JSONB NOT NULL DEFAULT '[]'::jsonb,
    -- [{page, region, description, descriptionSource, confirmed}]
    diagrams JSONB NOT NULL DEFAULT '[]'::jsonb,
    source_pages INT[] NOT NULL DEFAULT '{}',
    confidence VARCHAR(10) NOT NULL DEFAULT 'MEDIUM' CHECK (confidence IN ('HIGH', 'MEDIUM', 'LOW')),
    -- [{code, message, blocking, acknowledged}]
    warnings JSONB NOT NULL DEFAULT '[]'::jsonb,
    origin VARCHAR(10) NOT NULL DEFAULT 'EXTRACTED' CHECK (origin IN ('EXTRACTED', 'TEACHER')),
    edited BOOLEAN NOT NULL DEFAULT FALSE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT uq_document_question_position UNIQUE (document_id, position) DEFERRABLE INITIALLY IMMEDIATE
);

CREATE INDEX IF NOT EXISTS idx_exam_document_questions_doc ON exam_document_questions (document_id, position);

-- Where an approved question came from. SET NULL: deleting the document keeps
-- the exam's questions. The unique index means approving twice cannot create
-- the same question twice.
ALTER TABLE questions ADD COLUMN IF NOT EXISTS source_document_id UUID REFERENCES exam_documents(id) ON DELETE SET NULL;
ALTER TABLE questions ADD COLUMN IF NOT EXISTS source_draft_question_id UUID;
CREATE UNIQUE INDEX IF NOT EXISTS uq_questions_source_draft ON questions (source_draft_question_id)
    WHERE source_draft_question_id IS NOT NULL;

---- create above / drop below ----

DROP INDEX IF EXISTS uq_questions_source_draft;
ALTER TABLE questions DROP COLUMN IF EXISTS source_draft_question_id;
ALTER TABLE questions DROP COLUMN IF EXISTS source_document_id;
DROP TABLE IF EXISTS exam_document_questions;
DROP TABLE IF EXISTS exam_documents;
