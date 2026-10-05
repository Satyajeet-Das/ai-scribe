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

Detailed architectural documentation is available in [docs/architecture/overview.md](docs/architecture/overview.md), [SPEC.md](SPEC.md), and [docs/AI_Exam_Scribe_Work_Plan.md](docs/AI_Exam_Scribe_Work_Plan.md).

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
│   │   │   ├── answer/  # Candidate responses and transcripts
│   │   │   ├── model/   # Shared base entity models
│   │   │   └── platform/# Technical infrastructure (db, redis, http, logger, job)
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
│       ├── app/         # App Router (layout.tsx, page.tsx, globals.css)
│       ├── components/  # Accessible UI components (Tailwind, Lucide, Radix)
│       ├── lib/         # Client state, auth context, API clients
│       ├── Dockerfile   # Frontend production container build
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
- **Docker & Docker Compose** (for PostgreSQL 16 & Redis 7)

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
- **Integration Tests**: Repository tests execute against real PostgreSQL and Redis containers using Testcontainers (`apps/backend/tests/integration/`).
- **Static Analysis**: Zero warnings on `go vet ./...` and `golangci-lint run ./...`.
