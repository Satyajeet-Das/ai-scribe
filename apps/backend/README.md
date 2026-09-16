# AI Exam Scribe - Backend Service

Modular monolith backend built in Go for the AI Exam Scribe platform.

## Architecture

The backend follows a domain-driven modular monolith pattern with clean architectural separation:

```
apps/backend/
├── cmd/
│   └── server/          # Composition root (main.go)
├── internal/
│   ├── auth/            # Authentication, Clerk SDK integration, JWT validation
│   ├── user/            # User account management & profile domain
│   ├── exam/            # Exam configuration & metadata domain
│   ├── question/        # Question bank & item definitions domain
│   ├── assignment/      # Student exam assignment domain
│   ├── session/         # Active exam session runtime domain
│   ├── answer/          # Student response & transcript domain
│   ├── model/           # Shared base models and pagination types
│   └── platform/        # Shared infrastructure & technical capabilities
│       ├── config/      # Environment & configuration loader (Koanf)
│       ├── database/    # PostgreSQL connection pool (pgx/v5) & Tern migrations
│       ├── redis/       # Redis client with New Relic telemetry hooks
│       ├── http/        # Echo v4 web framework, routing, & middlewares
│       ├── logger/      # Structured logging with Zerolog
│       ├── job/         # Background job processor (Asynq)
│       ├── email/       # Transactional email service (Resend)
│       └── validation/  # Struct-level validation (validator/v10)
├── migrations/          # Embedded SQL schema migrations
├── static/              # Swagger & OpenAPI documentation assets
├── templates/           # Email HTML templates
└── tests/               # Unit and integration tests (testcontainers)
```

## Getting Started

### Prerequisites
- Go 1.24+
- PostgreSQL 16+
- Redis 7+

### Environment Configuration
```bash
cp .env.example .env
```

### Running Locally
```bash
# Download dependencies
go mod download

# Run database migrations and start server
go run ./cmd/server
```

### Running Tests
```bash
# Run unit tests
go test -v ./...

# Run static analysis
go vet ./...
```
