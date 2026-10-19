# AI Exam Scribe - Enterprise Go Backend

An enterprise-grade, accessible voice-first examination platform backend designed for visually impaired candidates.

Built as a **Go Modular Monolith**, the system enforces strict domain boundaries, explicit dependency inversion, isolated platform infrastructure, and robust finite state machine runtimes.

---

## Architecture Blueprint

```text
Request
   ↓
Middleware       (auth, rate limiting, request tracing, cors, recovery)
   ↓
Handler          (internal/platform/http & internal/<domain>/handler.go)
   ↓
Service          (internal/<domain>/service.go - pure business logic)
   ↓
Repository       (internal/<domain>/repository.go - persistence interfaces)
   ↓
Platform Infra   (internal/platform/database, redis, logger, job, email)
   ↓
PostgreSQL / Redis
```

Detailed architectural documentation is available in [docs/architecture/overview.md](docs/architecture/overview.md), [SPEC.md](SPEC.md), and [docs/AI_Exam_Scribe_Work_Plan.md](docs/AI_Exam_Scribe_Work_Plan.md). Feature notes:

- [voice-protocol.md](docs/architecture/voice-protocol.md): the voice WebSocket, audio modes, confirmation, command-phrase flag
- [answer-audio.md](docs/architecture/answer-audio.md): what audio is kept, who can listen, auditing, retention
- [teacher-marking.md](docs/architecture/teacher-marking.md): marking rules, finalise and reopen
- [disconnected-session-pause-resume.md](docs/architecture/disconnected-session-pause-resume.md): resuming a session

---

## Directory Structure

```text
ai-scribe/
├── apps/
│   ├── backend/         # Enterprise Go Modular Monolith Backend
│   │   ├── cmd/
│   │   │   ├── server/  # Application entry point & composition root
│   │   │   └── migrate/ # CLI database migrator tool
│   │   ├── internal/
│   │   │   ├── auth/    # Pluggable auth, JWT tokens, and session refresh
│   │   │   ├── user/    # Candidate, educator, proctor, and roll number search
│   │   │   ├── exam/    # Exam lifecycle, publishing, archiving, and authoring
│   │   │   ├── question/# Question bank and item representations
│   │   │   ├── assignment/# Candidate exam allocations
│   │   │   ├── session/ # Active exam runtime, FSM, and timing enforcement
│   │   │   ├── answer/  # Candidate responses and their revision history
│   │   │   ├── voice/   # Voice WebSocket: speech, intents, confirmation, TTS
│   │   │   ├── recording/# Dictated-answer audio, transcriptions, retention janitor
│   │   │   ├── review/  # Read-only teacher response review and audited audio playback
│   │   │   ├── grading/ # Teacher marking: marks, feedback, finalise and reopen
│   │   │   ├── audit/   # Append-only audit events
│   │   │   ├── model/   # Shared base entity models
│   │   │   └── platform/# Technical infrastructure (db, redis, http, ws, speech, audiostore, logger, job)
│   │   ├── migrations/  # Embedded SQL schema migrations (tern)
│   │   ├── tests/       # Integration tests and assertion helpers
│   │   ├── static/      # OpenAPI 3.0 specification & Swagger UI
│   │   ├── templates/   # Email HTML templates
│   │   ├── Dockerfile   # Backend multi-stage production container build
│   │   ├── Makefile     # Backend build and test tasks
│   │   ├── go.mod       # Go module definition
│   │   └── go.sum       # Verified module checksums
│   │
│   └── frontend/        # Next.js App Router Frontend
│       ├── src/app/     # Routes: login, exams, response review, exam room (sessions/[id])
│       ├── src/components/# Accessible UI (exam room, voice panel, response review, marking)
│       ├── src/hooks/   # Voice session, marking, remote resources
│       ├── src/services/# REST clients and the voice socket, microphone and audio player
│       ├── src/lib/     # Keyboard shortcut guards, validation, constants
│       └── package.json # Frontend dependencies and scripts
│
├── docs/                # Architecture and sprint documentation
├── docker-compose.yml   # Full-stack local development environment
├── Makefile             # Root orchestration Makefile
├── turbo.json           # Turborepo task pipeline
└── SPEC.md              # System architectural specification
```

---

## Implemented Sprints

| Sprint | Status | Focus | Deliverables |
|---|---|---|---|
| **Sprint 1** | Completed | Project Foundation | Go modular monolith, pgxpool, redis, structured logging, centralized error handling |
| **Sprint 2** | Completed | Pluggable Auth & Exam FSM | JWT token issuance, refresh rotation, revocation, FSM runtime, distributed locks |
| **Roll Number** | Completed | Student Identification | `roll_no` unique column, indexed autocomplete search API (`/api/v1/students/search`) |
| **Sprint 3** | Completed | Exam Management | Strict lifecycle FSM, owner/admin authorization, pagination, search, soft delete |
| **Sprint 4** | Completed | Question Management | Questions and options, ordering, correct option, question studio |
| **Sprint 5** | Completed | Assignments & Student Access | Assign by student or roll number, bulk assign, revoke, candidate dashboard |
| **Sprint 6** | Completed | Exam Session & Voice | Session FSM, current question, resume; voice WebSocket, speech recognition, intents, confirmation, speech output |
| **Answer audio** | Completed | Recording & Transcription | Dictated-answer audio in object storage, immutable transcriptions, answer revision history, retention cleanup |
| **Response review** | Completed | Teacher Review | Read-only review of each student's answers, audited audio playback, answer history |
| **Marking** | Completed | Teacher Marking | Half-mark scores, feedback, automatic MCQ marks, finalise and reopen |
| **Hardening** | Completed | Reliability & Accessibility | No lost answers, stale-save protection, immediate end of voice on submit, consistent keyboard rules, command-phrase safeguard |

---

## Exam Room: Answers, Voice and Keyboard

The candidate answers in the exam room (`/sessions/:id`) by typing, choosing an option, or speaking.
Every way of answering ends in the same answer service, and **nothing is saved by voice without an
explicit yes**.

**Saving answers**
- Typed text is saved 800 ms after the last keystroke; a choice is saved at once. Saves run one at
  a time and each sends only the newest value, so quick changes end on the last one.
- An answer whose save fails is kept for its own question until it is saved: moving to another
  question, by button or by voice, never loses it. The room says so ("has not been saved yet") and
  offers *Retry saving*; leaving the page warns, and submitting waits.
- Every save says which revision it replaces (`expectedRevisionNo`). If the answer changed
  elsewhere meanwhile (for example a dictation was confirmed), the server refuses with `409
  ANSWER_CHANGED` and the student chooses *Keep my version* or *Use the saved answer*.
- Submitting while a recording is in progress, still being processed, or waiting for confirmation
  says what would be lost first (*Submit anyway*). When time runs out, typed text is saved first and
  the server decides whether the exam is over.

**Voice** (see [voice-protocol.md](docs/architecture/voice-protocol.md))
- *Start speaking* listens for a command; *Dictate answer* records the answer itself. The mode comes
  from the control used, never from the words: a dictated "read the question" is answer text, and a
  command can only become an answer through an explicit "my answer is …".
- A dictated answer that is word for word a command ("read the question", "next question") is
  flagged: the confirmation says it sounds like a command, focus goes to *No, cancel*, and a
  *Read the question instead* button runs the command.
- A newer selection or dictation closes the one waiting for confirmation. Submitting or expiring
  ends the voice connection at once; nothing from an unfinished recording is kept.
- A request with no reply gives up after 30 s instead of blocking navigation.

**Keyboard** (when no button, field or dialog has focus, unless noted)

| Key | Action |
|---|---|
| `Enter` | Start a recording (dictation on a written question, a command on multiple choice); `Enter` again stops and sends. Never confirms an answer |
| `Escape` | Discard the recording in progress (not while a dialog is open) |
| `←` / `→` | Previous / next question. On a radio group or text field they keep their own meaning. Refused, with a reason, while recording or while an answer waits |
| `Alt+M` / `Alt+D` | Start or stop a voice command / dictation (also from fields; not inside dialogs) |
| `Alt+S` | Stop speech |
| `Alt+N` / `Alt+P` | Next / previous question (not while typing) |

Alt shortcuts match the physical key, so `Option+M` works on a Mac. Every start of a recording
follows one rule and says why it refuses (answer waiting, still processing, not connected).

---

## Teacher Review, Recordings and Marking

- **Response review** (`/exams/:id/responses/:studentId`) shows each answer, the attempt it belongs
  to (the submitted one, otherwise the latest) and, for dictated answers, every kept recording with
  what the recognizer heard. It is strictly read-only.
- **Answer history**: *Show how this answer changed* lists every value an answer held, where it came
  from (chosen, typed, dictated, dictated then corrected), when, and what was heard. A recording the
  student later typed over is labelled *Confirmed earlier, no longer the saved answer*.
- **Playback** is for the exam owner and administrators only, fetched only when *Play* is pressed,
  never cached or given a URL, limited to 60 plays a minute per reviewer, and written to the audit
  log (after the audio is read, before it is sent; if the log cannot be written nothing plays).
  Recording is **off by default**. See [answer-audio.md](docs/architecture/answer-audio.md).
- **Marking** by hand from the same page: marks in half-steps, feedback, automatic marks for
  multiple choice, then finalise and reopen. See [teacher-marking.md](docs/architecture/teacher-marking.md).
- The review page supports `←` / `→` between questions, with focus moving to the question heading.

---

## Exam Management (Sprint 3)

### Exam Lifecycle State Machine

```text
             ┌─────────────────────────┐
             │          DRAFT          │
             └───────────┬─────────────┘
                         │
        publish (valid)  │  ▲ unpublish (no active sessions/assignments)
                         ▼  │
             ┌─────────────────────────┐
             │        PUBLISHED        │
             └───────────┬─────────────┘
                         │
                 archive │
                         ▼
             ┌─────────────────────────┐
             │        ARCHIVED         │
             └─────────────────────────┘
```

#### Lifecycle Rules:
1. **DRAFT**:
   - Newly created exams always start as `DRAFT`.
   - Full editing capability: `title`, `subject`, `description`, `durationMins`.
2. **PUBLISHED**:
   - Transitioned via `POST /api/v1/exams/:id/publish`.
   - Structural parameters (e.g. `durationMins`) are frozen. Any attempt to alter duration returns `400 Bad Request` (`ErrPublishedStructuralChange`).
   - Non-structural updates (e.g. `description`) are safely permitted.
3. **UNPUBLISH**:
   - Reverts exam from `PUBLISHED` to `DRAFT` via `POST /api/v1/exams/:id/unpublish`.
   - **Business Guard**: Only allowed if there are **no active assignments or student sessions**. If assignments or sessions exist, returns `400 Bad Request` (`ErrCannotUnpublishActiveExam`).
4. **ARCHIVED**:
   - Terminal state transitioned via `POST /api/v1/exams/:id/archive`.
   - **Immutable**: Cannot be edited, unpublished, or published.
5. **SOFT DELETE**:
   - Executed via `DELETE /api/v1/exams/:id`.
   - Sets `deleted_at = NOW()`.
   - If an exam has active assignments or sessions, deletion is rejected (`ErrCannotDeleteActiveExam`) — the caller must archive it instead.

### Authorization & Scope:
- **Teachers**: Can only view, edit, publish, unpublish, archive, or delete exams they own (`exam.CreatedBy == callerID`). List endpoints automatically scope queries to the teacher's ID. Accessing another educator's exam returns `403 Forbidden`.
- **Administrators**: Universal management permissions across all exams. Can filter listings by any teacher.
- **Students**: Denied direct access to exam management endpoints (`403 Forbidden`).

---

## API Endpoints

### Authentication & Identity
| Method | Endpoint | Description | Access |
|---|---|---|---|
| `POST` | `/api/v1/auth/login` | Authenticate with credentials and receive JWT pair | Public (Rate Limited) |
| `POST` | `/api/v1/auth/register` | Register new student or teacher account | Public |
| `POST` | `/api/v1/auth/refresh` | Rotate refresh token and obtain new access token | Authenticated |
| `POST` | `/api/v1/auth/logout` | Revoke session and invalidate tokens in Redis | Authenticated |
| `GET` | `/api/v1/auth/me` | Fetch authenticated caller profile and role | Authenticated |

### Student Directory & Autocomplete
| Method | Endpoint | Description | Access |
|---|---|---|---|
| `GET` | `/api/v1/students/search` | Search students by roll number prefix, name, or email | Teacher, Admin |

### Exam Management (Sprint 3)
| Method | Endpoint | Description | Access |
|---|---|---|---|
| `POST` | `/api/v1/exams` | Create draft exam | Teacher, Admin |
| `GET` | `/api/v1/exams` | List exams with pagination (`limit`, `offset`), `status`, `subject`, and `search`/`q` | Teacher (Own), Admin (All) |
| `GET` | `/api/v1/exams/:id` | Get exam details and metadata | Owner Teacher, Admin |
| `PUT` | `/api/v1/exams/:id` | Full update exam (draft or safe published update) | Owner Teacher, Admin |
| `PATCH` | `/api/v1/exams/:id` | Partial update exam | Owner Teacher, Admin |
| `DELETE` | `/api/v1/exams/:id` | Soft-delete exam (prohibited if active sessions exist) | Owner Teacher, Admin |
| `POST` | `/api/v1/exams/:id/publish` | Publish draft exam (`DRAFT → PUBLISHED`) | Owner Teacher, Admin |
| `POST` | `/api/v1/exams/:id/unpublish` | Revert to draft (`PUBLISHED → DRAFT`) | Owner Teacher, Admin |
| `POST` | `/api/v1/exams/:id/archive` | Transition to terminal archived state | Owner Teacher, Admin |

### Questions & Options
| Method | Endpoint | Description | Access |
|---|---|---|---|
| `GET` / `POST` | `/api/v1/exams/:exam_id/questions` | List / create questions of an exam | Owner Teacher, Admin |
| `PUT` | `/api/v1/exams/:exam_id/questions/reorder` | Reorder questions | Owner Teacher, Admin |
| `GET` / `PUT` / `PATCH` / `DELETE` | `/api/v1/questions/:id` | Read, update or delete a question | Owner Teacher, Admin |
| `GET` / `POST` | `/api/v1/questions/:id/options` | List / add options | Owner Teacher, Admin |
| `PUT` / `PATCH` / `DELETE` | `/api/v1/questions/:id/options/:option_id` | Update or delete an option | Owner Teacher, Admin |
| `PUT` | `/api/v1/questions/:id/options/:option_id/correct` | Mark the correct option | Owner Teacher, Admin |

### Assignments
| Method | Endpoint | Description | Access |
|---|---|---|---|
| `GET` / `POST` | `/api/v1/exams/:exam_id/assignments` | List / assign a student (by ID or roll number) | Owner Teacher, Admin |
| `POST` | `/api/v1/exams/:exam_id/assignments/bulk` | Assign many students at once | Owner Teacher, Admin |
| `DELETE` | `/api/v1/exams/:exam_id/assignments/:student_id` | Revoke an assignment | Owner Teacher, Admin |
| `GET` | `/api/v1/assignments/my-exams` | The signed-in student's assigned exams | Student |

The flat `/api/v1/assignments…` routes (list, create, bulk, check, revoke) remain for existing clients.

### Exam Sessions & Answers
| Method | Endpoint | Description | Access |
|---|---|---|---|
| `POST` | `/api/v1/sessions` | Start (or resume) a session for an assignment | Assigned Student |
| `GET` | `/api/v1/sessions` · `/api/v1/sessions/:id` | List sessions / read one, with remaining time | Own Student |
| `GET` | `/api/v1/sessions/:id/current-question` | The current question (never includes the answer key) | Own Student |
| `POST` | `/api/v1/sessions/:id/next` · `/previous` | Move to the next / previous question | Own Student |
| `POST` | `/api/v1/sessions/:id/submit` | Submit; also ends the session's voice connection at once | Own Student |
| `GET` | `/api/v1/sessions/:session_id/answers` | All saved answers, each with `revisionNo` | Own Student |
| `GET` / `PUT` | `/api/v1/sessions/:session_id/questions/:question_id/answer` | Read / save an answer. Optional `expectedRevisionNo`; a newer answer refuses the save with `409 ANSWER_CHANGED`. Text up to 50,000 characters | Own Student |
| `GET` (WebSocket) | `/api/v1/sessions/:id/voice` | Voice channel, protocol `scribe.v1` ([voice-protocol.md](docs/architecture/voice-protocol.md)) | Own Student |

### Response Review & Marking
| Method | Endpoint | Description | Access |
|---|---|---|---|
| `GET` | `/api/v1/exams/:exam_id/responses` | Roster of assigned students with status counts | Owner Teacher, Admin |
| `GET` | `/api/v1/exams/:exam_id/responses/:student_id` | One student's answers and recordings (`supportsCurrentAnswer` per recording) | Owner Teacher, Admin |
| `GET` | `/api/v1/exams/:exam_id/responses/:student_id/questions/:question_id/revisions` | Every value an answer held, oldest first, with source and transcript | Owner Teacher, Admin |
| `GET` | `/api/v1/exams/:exam_id/responses/:student_id/recordings/:recording_id/audio` | Original audio (`Range`, `no-store`); audited, 60 a minute per reviewer | Owner Teacher, Admin |
| `GET` | `/api/v1/exams/:exam_id/responses/:student_id/marking` | Marking sheet: marks, auto marks, totals | Owner Teacher, Admin |
| `PUT` | `/api/v1/exams/:exam_id/responses/:student_id/marking/questions/:question_id` | Save a mark and feedback for one answer | Owner Teacher, Admin |
| `POST` | `/api/v1/exams/:exam_id/responses/:student_id/marking/finalise` · `/reopen` | Finalise (lock) or reopen a paper | Owner Teacher, Admin |

### System & Documentation
| Method | Endpoint | Description | Access |
|---|---|---|---|
| `GET` | `/status` | Health check (PostgreSQL, Redis, server uptime) | Public |
| `GET` | `/docs` | Interactive OpenAPI 3.0 UI (Swagger / Redoc) | Public |
| `GET` | `/static/openapi.json` | OpenAPI 3.0 specification JSON | Public |

> [!TIP]
> The OpenAPI specification is automatically kept in sync with the Go codebase. In development, changes to Go structs and routes are detected by a background file watcher and regenerated live. You can also run `make openapi` to regenerate on demand.

---

## Quick Start

### 1. Prerequisites
- **Go 1.24+**
- **Node.js 20+** (for frontend)
- **Docker & Docker Compose** (for PostgreSQL 16 & Redis 7, and optionally MinIO for answer-audio storage)

### 2. Environment Configuration
Copy `.env.example` to `.env`:
```bash
cp .env.example .env
```

### 3. Run with Docker Compose
Start PostgreSQL, Redis, backend, and frontend:
```bash
docker compose up -d
```

To run the backend and frontend from source instead, start only the infrastructure
(PostgreSQL, Redis and MinIO) with the dev compose file:
```bash
make docker-dev-up        # docker compose -f docker-compose.dev.yml up -d
make docker-dev-down      # stop them; named volumes (your data) are kept
```

MinIO holds the audio of dictated answers and is only used if you turn recording on
(`SCRIBE_RECORDING.ENABLED=true`, off by default because keeping students' audio is an
institutional decision). Both compose files start it on `127.0.0.1:9000` (S3 API) and
`127.0.0.1:9001` (console) with the development login `scribe-dev` / `scribe-dev-secret`. The
settings to point the backend at it are in `apps/backend/.env.example`, and
[docs/architecture/answer-audio.md](docs/architecture/answer-audio.md) explains what is kept, who can
listen, and how retention works. Docker Hub no longer serves new pulls of `minio/minio`: use a copy
you already have, or set `MINIO_IMAGE` to one from your own registry.

Teachers mark papers by hand from the response review page (marks in half-steps, feedback, then
finalise). [docs/architecture/teacher-marking.md](docs/architecture/teacher-marking.md) explains the rules,
concurrency and audit records.

### 4. Run Locally from Source

#### Root Commands (via Root Makefile)
```bash
# Display all available commands
make help

# Build Go server binary
make build-backend

# Run backend server
make run-backend

# Run database migrations
make migrate-backend

# Run backend unit tests with race detection
make test-backend

# Run static analysis checks on backend
make lint-backend

# Format Go source files
make fmt-backend

# Run Next.js frontend dev server
make dev-frontend

# Build Next.js frontend
make build-frontend

# Start full Docker stack
make docker-up

# Stop Docker stack
make docker-down
```

#### Directly in `apps/backend`
```bash
cd apps/backend

# Run server
go run ./cmd/server

# Run unit and handler tests with race detection
go test -v -race ./...

# Run static analysis
go vet ./...
golangci-lint run ./...
```

---

## Testing & Quality Assurance

The codebase enforces strict testing standards:
- **Table-Driven Unit Tests**: Every domain service and handler is verified with table-driven tests.
- **Race Condition Verification**: All test runs pass under Go's race detector (`go test -race ./...`).
- **Integration Tests**: Repository tests execute against real PostgreSQL and Redis containers using Testcontainers (`apps/backend/tests/integration/`). Docker must be running: `go test ./tests/integration/`.
- **Static Analysis**: Zero warnings on `go vet ./...` and `golangci-lint run ./...`.
- **Frontend Tests**: Vitest and Testing Library in `apps/frontend` (`npx vitest run`, `npx tsc --noEmit`), covering the exam room's saving, voice and keyboard rules and the review page.

> [!NOTE]
> On machines with limited memory (seen on Windows), building every package at once can crash the Go toolchain with "build failed". Limit parallelism with `go test -p 2 ./...`, and run Vitest with `--maxWorkers=2`.
