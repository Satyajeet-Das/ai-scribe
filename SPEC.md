# AI Exam Scribe - Architecture Specification

## 1. System Vision
The **AI Exam Scribe** is an enterprise-grade, accessible voice-first exam platform designed to assist visually impaired students throughout high-stakes examinations. The system orchestrates voice-driven navigation, real-time speech-to-text (STT), deterministic exam state management (FSM), intent classification, question presentation, answer transcription, text-to-speech (TTS), proctoring signals, and audit event logs.

## 2. Architectural Blueprint
The system is structured as a **Go Modular Monolith**, ensuring:
- Clear separation between business features (`internal/<domain>`) and shared infrastructure (`internal/platform/<infra>`).
- Strict layer boundaries:
  $$\text{Handler} \longrightarrow \text{Service} \longrightarrow \text{Repository} \longrightarrow \text{Database}$$
- Pure domain logic without direct coupling to web frameworks, transport protocols, or external AI vendors.
- Clean paths for future extraction of high-load components (audio streaming, proctoring, audit) into standalone services without refactoring core business logic.

## 3. Core Domains
| Domain | Responsibility |
|---|---|
| **Auth** | Identity verification, session claims, JWT authorization via Clerk SDK |
| **User** | Candidate, educator, and proctor profile management |
| **Exam** | Examination definition, scheduling, time limits, and configuration |
| **Question** | Item bank, multi-format questions, accessible representations |
| **Assignment** | Linking candidates to scheduled exam instances |
| **Session** | Active examination runtime, timing enforcement, state transitions |
| **Answer** | Candidate responses, multi-attempt tracking, audio transcripts |

## 4. Reserved Future Capabilities
Architectural boundaries are established for:
- `fsm/`: Deterministic state machine governing exam flow (Reading, Answering, Reviewing, Submitted).
- `audio/`: Low-latency WebRTC / WebSocket bi-directional streaming.
- `intent/`: Candidate utterance intent parsing & classification.
- `stt/` & `tts/`: Pluggable speech-to-text and text-to-speech engine adapters.
- `audit/`: Tamper-evident event sourcing for examination integrity.
- `accessibility/`: Graph sonification, tactile/screen-reader assistive hooks.
- `proctoring/`: Acoustic and visual anomaly telemetry.

## 5. Platform Infrastructure
- **Database**: PostgreSQL with `pgx/v5` connection pooling and `tern/v2` schema migrations.
- **Cache & Queues**: Redis with `go-redis/v9` and background workers with `hibiken/asynq`.
- **HTTP Transport**: Labstack Echo v4 with structured error handlers, distributed tracing, and rate limiting.
- **Observability**: Zerolog structured JSON logging and New Relic APM integrations.
