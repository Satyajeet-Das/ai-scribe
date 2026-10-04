# AI Exam Scribe - Backend Architectural Context & Engineering Guide for LLMs

> **Purpose**: This document is specifically structured to provide an LLM or software engineer complete, unambiguous technical context about the Go backend architecture of the **AI Exam Scribe** platform. Feed this document to any LLM prompt or context window to immediately ground it in project patterns, file paths, conventions, and architectural boundaries.

---

## 1. Project Overview & Product Context

- **Repository**: `ai-scribe`
- **Monorepo Layout**:
  - `apps/backend/` — The enterprise Go Modular Monolith backend service.
  - `apps/frontend/` — The Next.js (App Router) web client.
  - `packages/` — Shared OpenAPI specs, Zod schemas, and email templates.
- **Product Domain**: An accessible voice-first examination platform for visually impaired students.
- **Current Architectural Phase**: Exam Runtime Foundation (Sprint 2 Complete).
  - **Implemented**: Core modular monolith foundation, domain boundaries, PostgreSQL pool, Redis session caching, Echo HTTP server, logging, background jobs, Clerk auth middleware, strict error mapping, pure domain Finite State Machines (FSM), distributed locking, and idempotent session orchestration.
  - **Intentionally Deferred (DO NOT implement prematurely)**: WebSockets audio streaming, real-time STT/TTS engine, LLM evaluation pipelines, proctoring algorithms, PDF parsing.

---

## 2. Go Backend Architecture: Modular Monolith

The backend adheres strictly to **Clean Architecture / Domain-Driven Design (DDD)** packaged as a Go **Modular Monolith**.

```text
HTTP Request (Client)
       │
       ▼
[ internal/platform/http/middleware ]  (RequestID, Tracing, RateLimiter, CORS, Clerk Auth)
       │
       ▼
[ internal/<domain>/handler.go ]       (Decodes JSON/Query, calls Service, maps HTTP response)
       │
       ▼
[ internal/<domain>/service.go ]       (Pure business logic, orchestration, validation)
       │
       ▼
[ internal/<domain>/repository.go ]    (Direct SQL queries via pgxpool.Pool, returns Domain Models)
       │
       ▼
[ PostgreSQL 16+ ]
```

### Key Architectural Invariants
1. **Explicit Constructor Dependency Injection**:
   - Monolithic `*server.Server` parameter passing is strictly forbidden.
   - Every component defines its dependencies explicitly in constructor functions:
     `NewHandler(service Service) -> NewService(repo Repository, log *zerolog.Logger) -> NewRepository(db *pgxpool.Pool)`.
2. **Strict Layer Direction**:
   - `Handler` depends on `Service`.
   - `Service` depends on `Repository` (or interfaces of other domain services).
   - `Repository` depends on `*pgxpool.Pool`.
   - Domain layers NEVER import platform HTTP packages directly for business logic.
3. **Domain Isolation**:
   - Each business capability lives in its own package under `apps/backend/internal/<domain>/`.
   - Cross-domain communication is done strictly via public Service methods or shared interfaces, never by reaching into another domain's repository or database tables.
4. **Platform Decoupling**:
   - Technical cross-cutting concerns (database, redis, logging, telemetry, email, jobs, validation) live isolated under `apps/backend/internal/platform/`.

---

## 3. Directory Layout (`apps/backend/`)

```text
apps/backend/
├── cmd/
│   └── server/
│       └── main.go                     # Composition Root: boots config, db, redis, wires all domains, starts Echo
│
├── internal/
│   ├── model/
│   │   └── base.go                     # Shared model primitives: Base, BaseWithId, PaginatedResponse
│   │
│   ├── auth/                           # Authentication Domain
│   │   ├── model.go                    # User identity & claims representations
│   │   ├── service.go                  # Auth verification service
│   │   └── middleware.go               # Clerk SDK JWT verification & role authorization
│   │
│   ├── user/                           # User Profile Domain
│   │   ├── model.go                    # User model (candidate, educator, proctor)
│   │   ├── repository.go               # pgxpool PostgreSQL data access
│   │   └── service.go                  # User business logic
│   │
│   ├── exam/                           # Exam Management Domain
│   │   ├── model.go                    # Exam entity (title, duration, instructions, status)
│   │   ├── dto.go                      # CreateExamRequest, UpdateExamRequest, ExamResponse
│   │   ├── errors.go                   # Domain-specific errors (ErrExamNotFound, etc.)
│   │   ├── repository.go               # Exam SQL repository
│   │   ├── service.go                  # Exam lifecycle logic & validation
│   │   ├── service_test.go             # Unit tests for Exam Service
│   │   └── handler.go                  # Echo HTTP handlers (/api/v1/exams)
│   │
│   ├── question/                       # Question Bank Domain
│   │   ├── model.go                    # Question item entity, types (MCQ, essay, voice)
│   │   ├── dto.go                      # DTO representations
│   │   ├── errors.go                   # Domain-specific errors
│   │   ├── repository.go               # Question SQL repository
│   │   ├── service.go                  # Question service
│   │   └── handler.go                  # Question HTTP handlers (/api/v1/questions)
│   │
│   ├── assignment/                     # Exam Assignment Domain
│   │   ├── model.go                    # Assignment entity (candidate-exam allocation)
│   │   ├── dto.go                      # Allocation requests/responses
│   │   ├── errors.go                   # Domain-specific errors
│   │   ├── repository.go               # Assignment SQL repository
│   │   ├── service.go                  # Assignment service
│   │   └── handler.go                  # Assignment HTTP handlers (/api/v1/assignments)
│   │
│   ├── session/                        # Active Exam Session Runtime Domain
│   │   ├── model.go                    # Session entity, state, started_at, submitted_at
│   │   ├── dto.go                      # Session state transitions
│   │   ├── errors.go                   # Session domain errors
│   │   ├── repository.go               # Session SQL repository
│   │   ├── service.go                  # Session duration enforcement logic
│   │   └── handler.go                  # Session HTTP handlers (/api/v1/sessions)
│   │
│   ├── answer/                         # Candidate Answer & Transcript Domain
│   │   ├── model.go                    # Answer entity, audio/text transcript refs
│   │   ├── dto.go                      # Answer submission DTOs
│   │   ├── errors.go                   # Answer domain errors
│   │   ├── repository.go               # Answer SQL repository
│   │   ├── service.go                  # Answer submission logic
│   │   └── handler.go                  # Answer HTTP handlers (/api/v1/answers)
│   │
│   └── platform/                       # Cross-Cutting Technical Infrastructure
│       ├── config/
│       │   ├── config.go               # Koanf configuration loader (SCRIBE_ and BOILERPLATE_ prefixes)
│       │   ├── config_test.go          # Configuration validation unit tests
│       │   └── observability.go        # New Relic, tracing, and logging configuration
│       ├── database/
│       │   ├── database.go             # pgx/v5 connection pool setup & health ping
│       │   ├── migrator.go             # Tern embedded migration executor
│       │   └── sqlerr/
│       │       ├── error.go            # SQL error definitions
│       │       ├── handler.go          # PostgreSQL code mapper -> standard HTTP errors
│       │       └── handler_test.go     # SQL error mapping unit tests
│       ├── redis/
│       │   └── redis.go                # go-redis client with New Relic telemetry hook
│       ├── http/
│       │   ├── server.go               # HTTP server lifecycle, graceful shutdown
│       │   ├── router.go               # Route registration & system endpoints (/status, /docs)
│       │   ├── response.go             # Unified JSON responses: WriteSuccess, WriteError
│       │   ├── errors/
│       │   │   ├── types.go            # HTTP error types (BadRequest, NotFound, Unauthorized)
│       │   │   ├── http.go             # Centralized Echo HTTP error handler
│       │   │   └── errors_test.go      # Error handling unit tests
│       │   ├── handler/
│       │   │   ├── health.go           # Deep health check handler (/status) checking DB + Redis
│       │   │   ├── health_test.go      # Health check unit test
│       │   │   └── openapi.go          # Swagger/OpenAPI static doc server (/docs)
│       │   └── middleware/
│       │       ├── global.go           # Echo global middlewares setup
│       │       ├── request_id.go       # X-Request-ID propagation
│       │       ├── tracing.go          # Distributed tracing header propagation
│       │       ├── rate_limit.go       # In-memory / Redis rate limiting
│       │       ├── context.go          # Custom request context wrapper
│       │       └── middlewares.go      # Middleware chain assembly
│       ├── logger/
│       │   └── logger.go               # Structured Zerolog logger + New Relic APM hook
│       ├── job/
│       │   ├── job.go                  # Asynq Redis background task client & server
│       │   ├── handlers.go             # Worker registration
│       │   └── email_tasks.go          # Background email delivery worker
│       ├── email/
│       │   ├── client.go               # Resend API email client
│       │   ├── emails.go               # Email sending abstractions
│       │   ├── templates.go            # Embedded HTML template parsing
│       │   └── preview.go              # Dev preview utilities
│       ├── validation/
│       │   └── utils.go                # go-playground/validator v10 struct validation
│       └── utils/
│           └── utils.go                # String, slice, and environment helpers
│
├── migrations/
│   ├── migrations.go                   # Go embed FS wrapper: `//go:embed *.sql`
│   └── 001_setup.sql                   # Baseline database schema
│
├── static/
│   ├── openapi.html                    # Swagger UI viewer
│   └── openapi.json                    # OpenAPI contract definition
│
├── templates/
│   └── emails/
│       └── welcome.html                # Welcome email HTML template
│
├── tests/
│   ├── helpers/
│   │   └── assertions.go               # Test assertions
│   └── integration/
│       ├── container.go                # Testcontainers-go PostgreSQL & Redis helpers
│       └── transaction.go              # Isolated per-test database transaction rollbacks
│
├── Dockerfile                          # Multi-stage production container build
├── Makefile                            # Developer task runner (test, lint, build, run)
├── go.mod                              # Go module: `github.com/Satyajeet-Das/ai-scribe`
└── go.sum                              # Module checksums
```

---

## 4. Standard Domain Pattern: How Every Domain Is Implemented

When creating or modifying a domain (e.g. `exam`), adhere to this exact 5-part structure:

### 1. `model.go` (Domain Entities)
```go
package exam

import (
    "time"
    "github.com/Satyajeet-Das/ai-scribe/internal/model"
)

type Exam struct {
    model.BaseWithId
    Title        string    `json:"title"`
    Subject      string    `json:"subject"`
    DurationMins int       `json:"duration_mins"`
    Status       string    `json:"status"`
}
```

### 2. `dto.go` (Input / Output Transfer Objects)
```go
package exam

type CreateExamRequest struct {
    Title        string `json:"title" validate:"required,min=3,max=255"`
    Subject      string `json:"subject" validate:"required"`
    DurationMins int    `json:"duration_mins" validate:"required,min=1"`
}

type ExamResponse struct {
    ID           string    `json:"id"`
    Title        string    `json:"title"`
    DurationMins int       `json:"duration_mins"`
    CreatedAt    time.Time `json:"created_at"`
}
```

### 3. `errors.go` (Domain Errors)
```go
package exam

import "errors"

var (
    ErrExamNotFound      = errors.New("exam not found")
    ErrExamAlreadyClosed = errors.New("exam is already closed")
)
```

### 4. `repository.go` (Data Access Layer)
```go
package exam

import (
    "context"
    "github.com/jackc/pgx/v5/pgxpool"
)

type Repository interface {
    GetByID(ctx context.Context, id string) (*Exam, error)
    Create(ctx context.Context, exam *Exam) error
}

type repository struct {
    db *pgxpool.Pool
}

func NewRepository(db *pgxpool.Pool) Repository {
    return &repository{db: db}
}
```

### 5. `service.go` (Business Logic Layer)
```go
package exam

import (
    "context"
    "github.com/rs/zerolog"
)

type Service interface {
    CreateExam(ctx context.Context, req *CreateExamRequest) (*ExamResponse, error)
}

type service struct {
    repo Repository
    log  *zerolog.Logger
}

func NewService(repo Repository, log *zerolog.Logger) Service {
    return &service{repo: repo, log: log}
}
```

### 6. `handler.go` (HTTP Transport Layer)
```go
package exam

import (
    "net/http"
    "github.com/labstack/echo/v4"
    platformHttp "github.com/Satyajeet-Das/ai-scribe/internal/platform/http"
)

type Handler struct {
    service Service
}

func NewHandler(service Service) *Handler {
    return &Handler{service: service}
}

func (h *Handler) RegisterRoutes(g *echo.Group) {
    g.POST("", h.CreateExam)
}

func (h *Handler) CreateExam(c echo.Context) error {
    var req CreateExamRequest
    if err := c.Bind(&req); err != nil {
        return err
    }
    resp, err := h.service.CreateExam(c.Request().Context(), &req)
    if err != nil {
        return err
    }
    return platformHttp.WriteSuccess(c, http.StatusCreated, "Exam created", resp)
}
```

---

## 5. Composition Root (`cmd/server/main.go`)

All components are wired in `main.go` using explicit constructor calls:

```go
// 1. Technical platform infrastructure
cfg, _   := config.Load()
db, _    := database.New(ctx, cfg.Database)
rdb, _   := redis.New(ctx, cfg.Redis)
log      := logger.New(cfg.Observability)

// 2. Repositories
examRepo := exam.NewRepository(db)

// 3. Services
examSvc  := exam.NewService(examRepo, log)

// 4. Handlers
examHdl  := exam.NewHandler(examSvc)

// 5. Router registration
srv := platformHttp.NewServer(cfg.Server, log)
v1  := srv.Echo.Group("/api/v1")
examHdl.RegisterRoutes(v1.Group("/exams"))
```

---

## 6. Shared Architectural Primitives

### Base Entity Models (`internal/model/base.go`)
- `model.Base`: Contains `CreatedAt time.Time` and `UpdatedAt time.Time`.
- `model.BaseWithId`: Extends `Base` with `ID string` (UUID v7 or v4).
- `model.PaginatedResponse`: Unified envelope for paged database lists.

### Unified HTTP JSON Response Contract (`internal/platform/http/response.go`)
- **Success Format**:
  ```json
  {
    "success": true,
    "message": "Exam created",
    "data": { ... }
  }
  ```
- **Error Format**:
  ```json
  {
    "success": false,
    "code": "EXAM_NOT_FOUND",
    "message": "exam not found",
    "errors": []
  }
  ```

### Database Error Mapping (`internal/platform/database/sqlerr/handler.go`)
PostgreSQL error codes (e.g. `23505` unique violation, `23503` foreign key violation, `no rows in result set`) are automatically intercepted and converted into standard `HTTPError` instances (`409 Conflict`, `404 Not Found`, etc.) without leaking raw database strings to the client.

---

## 7. Developer Cheatsheet & Workflow Commands

Always execute commands within `apps/backend/` or use the root `Makefile`:

```bash
# Run unit tests (with race detector)
make test-backend
# or: cd apps/backend && go test -race -v ./...

# Run static analysis
make lint-backend
# or: cd apps/backend && go vet ./...

# Format Go code
make fmt-backend
# or: cd apps/backend && gofmt -s -w .

# Build production binary
make build-backend
# or: cd apps/backend && go build -o bin/server ./cmd/server

# Start server locally
make run-backend
# or: cd apps/backend && go run ./cmd/server
```

---

## 8. Rules for Future LLMs Modifying this Codebase

1. **Preserve Dependency Inversion**: Never pass a God-object `*server.Server` into handlers or services. Always pass explicit interfaces or pointer references (`Service`, `Repository`, `*pgxpool.Pool`).
2. **Do Not Implement Premature Features**: Stick strictly to current domain tasks. Do not add WebSockets, speech audio encoders, or mock AI proctoring engines unless explicitly instructed.
3. **Database Queries**: Use direct parameterized SQL with `pgx/v5`. Do not introduce heavyweight ORMs (like GORM). Keep SQL queries clear and performant.
4. **Validation**: Use `validate:"..."` struct tags on incoming request DTOs, validated via `platform/validation`.
5. **Context Propagation**: Always pass `ctx context.Context` as the first parameter to repository and service methods.
6. **Tests**: When adding a new service or handler, write accompanying unit tests using standard Go testing (`testing.T`), table-driven test cases, and mock interfaces.
