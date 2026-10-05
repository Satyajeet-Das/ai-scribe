# AI Exam Scribe — Backend Work Plan

## Project Goal

Build a production-grade Go backend for an accessible AI exam platform, starting as a standalone REST backend and later integrating with a Next.js frontend.

The backend should remain a modular monolith initially, with clear boundaries so that authentication, realtime audio, AI integrations, exam execution, and auditing can evolve independently.

## Core Engineering Principles

- Go modular monolith
- Clean Architecture / DDD-style domain boundaries
- Explicit dependency injection
- PostgreSQL as persistent source of truth
- Redis for ephemeral state, caching, rate limiting, and coordination where appropriate
- Provider-independent authentication
- Backend-authoritative authorization
- Strong context propagation
- Parameterized SQL with pgx
- Feature/domain-based development
- Test every feature before moving forward
- Production concerns introduced incrementally rather than all at once
- Avoid premature microservices and abstractions

---

# Sprint Overview

| Sprint | Focus | Status | Primary Outcome |
|---|---|---|---|
| 1 | Foundation & Project Architecture | Completed | Stable Go backend foundation |
| 2 | Pluggable Authentication & Exam FSM | Completed | JWT, refresh rotation, revocation, FSM runtime |
| 3 | Exam Management | Completed | Complete exam lifecycle FSM, owner/admin auth, search, CRUD |
| 4 | Question Management | Next | Question bank and exam-question relationships |
| 5 | Assignments & Student Access | Exams assigned to students |
| 6 | Exam Session & FSM | Controlled exam runtime |
| 7 | WebSocket Foundation | Reliable realtime connection layer |
| 8 | Streaming Audio Pipeline | Concurrent low-latency audio transport |
| 9 | STT + Intent Classification | Voice command pipeline |
| 10 | TTS + Barge-in + Confirmation | Complete AI scribe interaction loop |
| 11 | Event Sourcing & Audit | Tamper-evident exam audit trail |
| 12 | PDF & Multimodal Ingestion | Structured exam ingestion |
| 13 | STEM Accessibility | Graph sonification and math accessibility |
| 14 | Proctoring & Monitoring | Admin monitoring foundation |
| 15 | Production Hardening | Security, reliability, observability, load testing |
| 16 | Deployment & Scale Preparation | Cloud deployment and operational readiness |

---

# Sprint 1 — Foundation & Project Architecture

## Goal

Understand and stabilize the existing Go boilerplate before implementing business features.

## Work

- Understand project structure
- Configure application settings
- Verify PostgreSQL connection
- Verify Redis connection
- Verify Echo server
- Verify logging
- Verify migrations
- Verify health checks
- Verify dependency injection
- Establish domain package conventions
- Establish testing conventions
- Establish local Docker workflow
- Document development workflow

## Study

- Go packages
- structs and interfaces
- constructors
- dependency injection
- `context.Context`
- Echo basics
- pgx/pgxpool
- Redis client
- database migrations
- Go testing
- table-driven tests
- race detector

## Acceptance Criteria

- Server starts successfully
- PostgreSQL connects
- Redis connects
- `/status` works
- migrations execute
- tests pass
- race tests pass
- production build succeeds

---

# Sprint 2 — Pluggable Authentication

## Goal

Implement production-grade authentication with JWT while keeping the application independent of the authentication provider.

## Architecture

```text
HTTP Request
    ↓
Auth Middleware
    ↓
Authentication Provider Interface
    ↓
JWT Adapter
    ↓
Provider-Independent Identity
    ↓
Authorization
    ↓
Domain Handler
```

Later:

```text
Authentication Provider Interface
          ├── JWT Adapter
          └── Clerk Adapter
```

## Work

- Define provider-independent identity model
- Define authentication interface
- Define authorization boundary
- Implement JWT provider
- Implement access tokens
- Implement refresh tokens
- Implement refresh-token rotation
- Implement logout/revocation strategy
- Define Redis responsibilities
- Add authentication middleware
- Add role/permission checks
- Handle account deactivation
- Handle expired/malformed tokens
- Add security-focused logging
- Add authentication tests

## Study

- JWT
- access vs refresh tokens
- token rotation
- token revocation
- authentication vs authorization
- middleware
- cryptographic signing
- key rotation
- Redis TTL
- session management
- HTTP security

## Important Constraint

No domain package should import JWT or Clerk libraries.

---

# Sprint 3 — Exam Management

## Goal

Build the complete educator exam-management capability.

## Work

- Exam domain
- Exam model
- DTOs
- Repository
- Service
- Handler
- Create exam
- Get exam
- List exams
- Update exam
- Delete/archive exam
- Exam status lifecycle
- Pagination
- Filtering
- Validation
- Authorization

## Study

- REST API design
- DTOs
- validation
- pagination
- SQL indexes
- transactions
- service/repository separation

## Tests

- service unit tests
- repository integration tests
- handler tests
- authorization tests
- validation tests

---

# Sprint 4 — Question Management

## Goal

Create a flexible question-bank system.

## Initial Types

- MCQ
- Essay
- Voice-compatible question model

## Work

- Question domain
- Question CRUD
- Question ordering
- Question types
- Options
- Marks
- Exam-question relationship
- Validation
- Authorization

Design the schema so future content can support:

- images
- diagrams
- mathematical expressions
- audio
- structured content

## Study

- relational modeling
- foreign keys
- normalization
- indexing
- transactions
- ordering models

---

# Sprint 5 — Assignments & Student Access

## Goal

Allow educators/admins to assign exams to students and allow students to retrieve their assigned exams.

## Work

- Assignment domain
- Student-exam relationship
- Assignment lifecycle
- Bulk assignment preparation
- Student assignment list
- Authorization
- Duplicate-assignment handling
- Assignment constraints

## Study

- many-to-many relationships
- composite/unique constraints
- idempotency
- authorization policies
- pagination

---

# Sprint 6 — Exam Session & FSM

## Goal

Introduce controlled exam execution.

## Initial FSM

```text
ASSIGNED
   ↓
READY
   ↓
STARTED
   ↓
IN_PROGRESS
   ↓
SUBMITTED
```

Additional terminal/error states can be introduced when required.

## Work

- Session domain
- Session creation
- Start exam
- Session status
- Duration enforcement
- Question navigation
- Current question
- Submission
- Expiration
- Idempotent transitions
- Redis-backed runtime state
- PostgreSQL persistence

## Important Rule

Redis is not the permanent source of truth.

PostgreSQL remains authoritative for persistent exam/session records.

## Study

- Finite State Machines
- Redis
- atomic operations
- concurrency
- race conditions
- idempotency
- transactions
- distributed state

---

# Sprint 7 — WebSocket Foundation

## Goal

Build a reliable realtime communication layer without integrating AI/audio yet.

## Work

- WebSocket endpoint
- authentication during connection
- connection lifecycle
- connection manager
- read/write pumps
- ping/pong
- heartbeat
- graceful disconnect
- reconnect strategy
- connection limits
- message envelope
- structured events
- concurrent connection handling

## Initial Event Model

```text
connected
disconnected
ping
pong
session.started
session.updated
question.changed
```

## Study

- WebSockets
- TCP basics
- goroutines
- channels
- mutexes
- cancellation
- backpressure
- connection lifecycle

---

# Sprint 8 — Streaming Audio Pipeline

## Goal

Transport audio reliably through the Go backend.

## Work

- binary WebSocket frames
- audio chunk handling
- per-session audio pipeline
- buffering
- backpressure
- concurrent streams
- cancellation
- memory management
- `sync.Pool`
- stream lifecycle
- resource limits

## Architecture

```text
Browser
   ↓
WebSocket
   ↓
Go Connection
   ↓
Audio Stream
   ↓
Pipeline
   ↓
External Streaming Provider
```

## Study

- streaming systems
- buffers
- backpressure
- goroutine lifecycle
- channel patterns
- memory allocation
- `sync.Pool`
- latency measurement

---

# Sprint 9 — Streaming STT + Intent Classification

## Goal

Convert speech into safe, structured navigation/interaction commands.

## Pipeline

```text
Audio
 ↓
Streaming STT
 ↓
Transcript
 ↓
Intent Classifier
 ↓
Validated Intent
 ↓
FSM / Application Logic
```

## Supported Initial Intents

Examples:

- repeat
- next question
- previous question
- read option
- select option
- start
- stop
- confirm
- cancel

## Critical Rule

The LLM is an intent classifier, not an exam-answer assistant.

It must not generate answers or provide academic assistance.

## Study

- streaming APIs
- structured LLM output
- schema validation
- prompt boundaries
- timeouts
- retries
- circuit breakers
- external API failure handling

---

# Sprint 10 — TTS + Barge-in + Confirmation

## Goal

Complete the basic AI scribe interaction loop.

## Pipeline

```text
Student Speech
      ↓
STT
      ↓
Intent / Transcript
      ↓
Application Logic
      ↓
Response
      ↓
TTS
      ↓
Browser
```

## Work

- streaming TTS
- audio output
- barge-in
- cancellation
- stop speaking
- repeat
- answer transcription
- read-back confirmation
- explicit confirmation
- answer persistence
- cancellation flow

## Critical Confirmation Flow

```text
Student speaks answer
        ↓
Transcript generated
        ↓
System reads answer back
        ↓
Student says Confirm
        ↓
Answer persisted
```

No database mutation before confirmation.

## Study

- cancellation
- context propagation
- concurrent pipelines
- state machines
- distributed failure handling

---

# Sprint 11 — Event Sourcing & Audit Trail

## Goal

Build an immutable audit trail for exam interactions.

## Events

Examples:

```text
session.created
session.started
question.read
navigation.requested
navigation.executed
answer.transcribed
answer.confirmed
answer.saved
tts.started
tts.interrupted
session.submitted
```

## Work

- append-only event table
- event schema
- aggregate/session identifiers
- event ordering
- timestamps
- request IDs
- actor identity
- event metadata
- event integrity strategy
- audit queries

## Important Principle

Do not claim mathematical proof unless the implemented evidence model actually supports the claim.

The system should provide a verifiable record of what the platform did.

## Study

- Event Sourcing
- append-only logs
- immutable data
- event ordering
- auditability
- hash chaining where appropriate

---

# Sprint 12 — PDF & Multimodal Exam Ingestion

## Goal

Allow educators to upload exam documents and convert them into structured exam data.

## Pipeline

```text
PDF
 ↓
Document Processing
 ↓
OCR / Vision
 ↓
Multimodal Model
 ↓
Structured JSON
 ↓
Validation
 ↓
Human Review
 ↓
PostgreSQL
```

## Work

- file upload
- object storage boundary
- PDF processing
- OCR/VLM integration
- structured schema
- validation
- failed-job handling
- background processing
- review workflow
- versioning

## Study

- object storage
- background jobs
- multimodal AI
- structured extraction
- schema validation
- asynchronous workflows

---

# Sprint 13 — STEM Accessibility

## Goal

Support accessible representations of mathematical and graphical exam content.

## Work

- graph representation
- graph sonification
- x-axis/time mapping
- y-axis/pitch mapping
- DSP processing
- math transcription
- LaTeX representation
- accessible verbal descriptions

## Study

- DSP basics
- signal processing
- audio synthesis
- mathematical representations
- accessibility engineering

---

# Sprint 14 — Proctoring & Monitoring

## Goal

Build an administrative monitoring foundation.

## Work

- active session monitoring
- WebSocket connection status
- session health
- exam progress metadata
- system alerts
- administrative dashboard APIs
- suspicious technical events
- operational metrics

The monitoring system must not expose exam answers unnecessarily.

## Study

- realtime dashboards
- observability
- metrics
- operational events
- privacy
- access control

---

# Sprint 15 — Production Hardening

## Goal

Move from a functional system toward production readiness.

## Security

- authentication review
- authorization review
- rate limiting
- input validation
- secure headers
- secret management
- token security
- dependency scanning
- audit review

## Reliability

- timeouts
- retries
- circuit breakers
- graceful shutdown
- connection limits
- resource limits
- external provider failure handling
- Redis failure handling
- PostgreSQL failure handling

## Observability

- structured logging
- request IDs
- metrics
- tracing
- latency measurements
- error rates
- provider latency
- WebSocket metrics
- audio pipeline metrics

## Testing

- unit tests
- integration tests
- race detector
- E2E tests
- load tests
- failure tests
- concurrency tests

---

# Sprint 16 — Deployment & Scale Preparation

## Goal

Deploy the system and validate its operational architecture.

## Work

- Docker production image
- environment configuration
- CI/CD
- database migrations
- cloud deployment
- TLS
- PostgreSQL production configuration
- Redis production configuration
- monitoring
- alerting
- backups
- disaster recovery plan
- load testing
- horizontal scaling validation

## Initial Target

Design and validate for approximately:

```text
~1,000 users initially
```

Do not prematurely optimize for millions of users.

Architecture should allow future horizontal scaling.

---

# Development Methodology

Every sprint follows:

```text
1. Understand
      ↓
2. Study required concepts
      ↓
3. Design
      ↓
4. Create small tickets
      ↓
5. Implement
      ↓
6. Unit test
      ↓
7. Integration test
      ↓
8. Review
      ↓
9. Refactor
      ↓
10. Sprint acceptance
```

Do not move to the next sprint until the current sprint's acceptance criteria are satisfied.

---

# Learning Rule

For every ticket:

1. Identify what must be learned.
2. Learn the minimum required concept.
3. Design the solution yourself.
4. Use AI for implementation assistance where useful.
5. Review every generated line.
6. Write tests yourself.
7. Debug failures yourself first.
8. Document the architectural decision.

The project is simultaneously a production project and a learning project.

---

# Architecture Evolution

The architecture should evolve approximately as follows:

```text
Sprint 1
Go Modular Monolith
        ↓
Sprint 2
Provider-independent Auth
        ↓
Sprint 3-5
Core Exam Domains
        ↓
Sprint 6
Exam Runtime + FSM
        ↓
Sprint 7-10
Realtime + AI Scribe
        ↓
Sprint 11
Event Sourcing
        ↓
Sprint 12-14
Advanced Accessibility + Operations
        ↓
Sprint 15-16
Production Hardening + Deployment
```

Do not introduce microservices simply because the system grows.

First establish clear module boundaries inside the monolith.

Extract services only when there is a demonstrated operational or organizational reason.

---

# Current Priority

The immediate implementation target is:

**Sprint 1 → Sprint 2**

Do not implement future AI/audio/proctoring functionality until the preceding architectural foundations are stable.

The next concrete task should be to finish Sprint 1 and then implement **Sprint 2: Pluggable JWT Authentication with a provider-independent authentication interface and Clerk-ready architecture.**
