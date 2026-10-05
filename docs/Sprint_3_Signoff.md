# Sprint 3 Completion Sign-off: Exam Management

## Overview
Sprint 3: **Exam Management** has been successfully completed and verified for production readiness. The backend now provides complete, robust capabilities for educators and administrators to author, manage, publish, unpublish, archive, filter, and soft-delete exams while enforcing strict state machine transitions, owner authorization, and database integrity.

---

## Completed Deliverables

### 1. Database & Schema Enhancements
- **Migration `004_exam_management.sql`**:
  - Added `deleted_at TIMESTAMPTZ` to support soft-deletion of exams without loss of historical relational context.
  - Added indexes:
    - `idx_exams_deleted_at ON exams (deleted_at)`
    - `idx_exams_subject ON exams (subject)`
    - `idx_exams_created_by_status ON exams (created_by, status)`
  - Reversible with matching down-migration.

### 2. Domain Models & Lifecycle State Machine
- Strict FSM transitions:
  - `DRAFT → PUBLISHED`: Validates duration > 0, non-empty subject and title, sets `published_at = NOW()`.
  - `PUBLISHED → DRAFT` (Unpublish): Reverts to DRAFT and clears `published_at` **only if** no active student assignments or sessions exist (`repo.HasActiveSessionsOrAssignments(ctx, id)`).
  - `DRAFT → ARCHIVED` & `PUBLISHED → ARCHIVED`: Transitions to terminal `ARCHIVED` status.
  - Immutability of Archived Exams: Once archived, no modifications or transitions out of `ARCHIVED` are permitted (`ErrExamAlreadyArchived`).
  - Structural Integrity of Published Exams: Prevents alterations to exam timing (`DurationMins`) while published (`ErrPublishedStructuralChange`), while permitting safe non-structural updates (e.g. description).
  - Soft-Delete Protection: Prevents deleting exams with active assignments or sessions (`ErrCannotDeleteActiveExam`), requiring archiving instead.

### 3. Backend-Authoritative Authorization
- Teacher Scope:
  - Teachers may only access, edit, publish, unpublish, archive, or delete exams they created (`exam.CreatedBy == callerID`).
  - Access attempts to other teachers' exams fail with `403 Forbidden` (`ErrUnauthorizedCreator`).
  - `ListExams` automatically scopes queries to the teacher's UUID.
- Administrator Privileges:
  - Administrators have global access across all exams and can filter listings by any teacher.
- Students:
  - Exam management endpoints require `RoleTeacher` or `RoleAdmin`; unauthorized roles receive `403 Forbidden`.

### 4. REST API & OpenAPI
- **Standardized REST Endpoints in `apps/backend/internal/exam/handler.go`**:
  - `POST /api/v1/exams` (201 Created)
  - `GET /api/v1/exams` (200 OK — paginated with `limit`, `offset`, `total`, and query filters `status`, `subject`, `search`/`q`, `createdBy`)
  - `GET /api/v1/exams/:id` (200 OK)
  - `PUT /api/v1/exams/:id` & `PATCH /api/v1/exams/:id` (200 OK)
  - `DELETE /api/v1/exams/:id` (200 OK)
  - `POST /api/v1/exams/:id/publish` (200 OK)
  - `POST /api/v1/exams/:id/unpublish` (200 OK)
  - `POST /api/v1/exams/:id/archive` (200 OK)
- **OpenAPI 3.0**: Synchronized `apps/backend/static/openapi.json` with all endpoints, DTOs, parameters, error responses, and `ExamListResponse` envelope.

---

## Verification & Test Results

1. **Service Unit Tests (`internal/exam/service_test.go`)**:
   - Table-driven unit tests covering all happy paths and error paths.
   - Comprehensive lifecycle state transition validations.
   - Owner and administrator authorization checks.
   - Multi-goroutine concurrent update test.
2. **HTTP Handler Tests (`internal/exam/handler_test.go`)**:
   - Echo context tests verifying HTTP status codes (200, 201, 400, 403, 404).
   - Validation failures and request binding tests.
   - Parameterized search, filter, and pagination response envelope tests.
3. **Integration Tests (`tests/integration/exam_test.go`)**:
   - End-to-end repository and service integration against real PostgreSQL 16 container.
   - Verified active relation guards preventing unpublish and delete when student assignments exist.
4. **Race Detection & Static Analysis**:
   - `go test -race ./...` — PASS (0 data races).
   - `go vet ./...` — PASS (0 warnings).
   - `golangci-lint run ./...` — PASS (0 warnings).

---

## Conclusion
Sprint 3 is signed off as complete and production ready. The foundation is ready for Sprint 4 (Question Management).
