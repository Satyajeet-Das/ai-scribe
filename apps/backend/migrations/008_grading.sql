-- Migration 008: teacher marking of student papers.
--
-- Marks are stored as integer HALF-marks (7.5 marks is 15), so every stored
-- value is a multiple of 0.5 by construction and totals never drift.
-- Automatic marks for multiple-choice questions are never stored here: while a
-- paper is a draft they are worked out when read. Finalising a paper freezes
-- every question's mark in `paper_gradings.snapshot`, so later edits to a
-- question's points or correct option cannot change a finalised paper.

-- One row per marked paper (a student's session). Created on the first write;
-- until then a paper is a draft with version 0.
CREATE TABLE IF NOT EXISTS paper_gradings (
    session_id UUID PRIMARY KEY REFERENCES sessions(id) ON DELETE CASCADE,
    exam_id UUID NOT NULL REFERENCES exams(id) ON DELETE RESTRICT,
    status VARCHAR(10) NOT NULL DEFAULT 'DRAFT' CHECK (status IN ('DRAFT', 'FINAL')),
    -- Bumped by every mark write, finalise and reopen. The same row is locked
    -- by all of them, so they cannot interleave.
    version INT NOT NULL DEFAULT 0 CHECK (version >= 0),
    finalised_by UUID REFERENCES users(id) ON DELETE RESTRICT,
    finalised_at TIMESTAMPTZ,
    total_half INT CHECK (total_half >= 0),
    max_half INT CHECK (max_half >= 0),
    snapshot JSONB,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT chk_total_within_max CHECK (total_half IS NULL OR max_half IS NULL OR total_half <= max_half),
    -- A final paper has everything a final paper needs, and a draft carries no
    -- half-finished finalisation.
    CONSTRAINT chk_final_is_complete CHECK (
        (status = 'FINAL') = (
            finalised_at IS NOT NULL AND finalised_by IS NOT NULL AND total_half IS NOT NULL
            AND max_half IS NOT NULL AND snapshot IS NOT NULL
        )
    )
);

CREATE INDEX IF NOT EXISTS idx_paper_gradings_exam ON paper_gradings (exam_id);

-- What the teacher entered for one answer. Only teacher input lives here.
CREATE TABLE IF NOT EXISTS answer_marks (
    session_id UUID NOT NULL REFERENCES sessions(id) ON DELETE CASCADE,
    question_id UUID NOT NULL REFERENCES questions(id) ON DELETE CASCADE,
    -- NULL means "no teacher mark": the mark was cleared, or only a comment was saved.
    marks_half INT,
    -- The question's points * 2 when this was written.
    max_half INT NOT NULL CHECK (max_half >= 0),
    comment TEXT CHECK (comment IS NULL OR char_length(comment) <= 2000),
    -- Starts at 1 and rises with every write. A cleared mark keeps its row so
    -- the version keeps rising: deleting it would let a stale writer succeed
    -- against "version 0" again.
    version INT NOT NULL DEFAULT 1 CHECK (version >= 1),
    marked_by UUID NOT NULL REFERENCES users(id) ON DELETE RESTRICT,
    marked_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    PRIMARY KEY (session_id, question_id),
    CONSTRAINT chk_marks_within_max CHECK (marks_half IS NULL OR (marks_half >= 0 AND marks_half <= max_half))
);

-- Finalising and reopening a paper, in order. Append-only.
CREATE TABLE IF NOT EXISTS paper_grading_events (
    id BIGSERIAL PRIMARY KEY,
    session_id UUID NOT NULL REFERENCES sessions(id) ON DELETE CASCADE,
    exam_id UUID NOT NULL REFERENCES exams(id) ON DELETE RESTRICT,
    event VARCHAR(10) NOT NULL CHECK (event IN ('FINALISED', 'REOPENED')),
    actor_id UUID NOT NULL REFERENCES users(id) ON DELETE RESTRICT,
    actor_role VARCHAR(50) NOT NULL DEFAULT '',
    reason TEXT CHECK (reason IS NULL OR char_length(reason) <= 500),
    -- The totals and per-question lines as they were, so a reopened paper keeps
    -- a record of what had been finalised.
    total_half INT,
    max_half INT,
    snapshot JSONB,
    occurred_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    -- Reopening a finalised paper always has a stated reason.
    CONSTRAINT chk_reopen_has_reason CHECK (event <> 'REOPENED' OR (reason IS NOT NULL AND char_length(btrim(reason)) >= 3))
);

CREATE INDEX IF NOT EXISTS idx_paper_grading_events_session ON paper_grading_events (session_id, occurred_at);

---- create above / drop below ----

DROP TABLE IF EXISTS paper_grading_events;
DROP TABLE IF EXISTS answer_marks;
DROP TABLE IF EXISTS paper_gradings;
