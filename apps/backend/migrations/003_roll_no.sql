-- Migration 003: Add roll_no to users table for student identification

ALTER TABLE users ADD COLUMN IF NOT EXISTS roll_no VARCHAR(50);

-- Unique index on roll_no (allows multiple NULLs for non-students)
CREATE UNIQUE INDEX IF NOT EXISTS idx_users_roll_no_unique ON users (roll_no) WHERE roll_no IS NOT NULL;

-- Fast index for autocomplete searches on students
CREATE INDEX IF NOT EXISTS idx_users_role_roll_no ON users (role, roll_no);
CREATE INDEX IF NOT EXISTS idx_users_role_names ON users (role, first_name, last_name);

---- create above / drop below ----

DROP INDEX IF EXISTS idx_users_role_names;
DROP INDEX IF EXISTS idx_users_role_roll_no;
DROP INDEX IF EXISTS idx_users_roll_no_unique;
ALTER TABLE users DROP COLUMN IF EXISTS roll_no;
