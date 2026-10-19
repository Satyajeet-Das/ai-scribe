# Voice WebSocket Protocol (v1)

The real-time voice channel for an exam session. It carries the student's audio to the
server, and transcripts, confirmations, action results and synthesized speech back.

**Invariant:** the LLM interprets, the backend decides, PostgreSQL persists. Nothing a client
or a model sends changes exam state directly; every action is validated and executed by the
existing session service (FSM) and answer service.

Code: `apps/backend/internal/voice` (module), `internal/platform/ws` (transport),
`internal/platform/speech` (STT/TTS contracts and providers), `internal/voice/intent`
(intent types, validation, classifiers).

## Connecting

```
GET /api/v1/sessions/{sessionId}/voice      (WebSocket upgrade)
Sec-WebSocket-Protocol: scribe.v1, scribe.auth.<access token>
```

- **Auth.** Send the access token either as `Authorization: Bearer <token>` (non-browser
  clients) or as the second subprotocol `scribe.auth.<token>` (browsers, which cannot set
  headers on a WebSocket). The server always selects `scribe.v1`; the token is never echoed.
- **Checks before the upgrade**, each answered with a plain HTTP error:

| Status | Meaning |
|---|---|
| 401 | missing or invalid token |
| 403 | origin not allowed, or the session belongs to another student |
| 404 | unknown session, or voice disabled |
| 409 | session is submitted, expired or not in progress |
| 429 | too many voice connections for this user |
| 503 | server at capacity, shutting down, or session lookup failed |

- **One connection per session.** A new connection for the same session replaces the old
  one, which is closed with `4409`. This is what makes reconnecting past a dead socket safe.
- **Reconnecting.** Just connect again. The server re-authenticates and re-reads the session
  from PostgreSQL; `ready.resumed` is `true` and `ready.session` holds the authoritative
  position. A pending answer confirmation does **not** survive a reconnect (nothing was saved).

## Envelope

Every JSON message, in both directions:

```json
{ "v": 1, "type": "audio.start", "id": "c-17", "replyTo": "", "ts": 0, "payload": {} }
```

| Field | Notes |
|---|---|
| `v` | protocol version, must be `1` |
| `type` | selects the payload schema |
| `id` | client request ID: required, unique per connection, ≤ 64 chars of `[A-Za-z0-9._:-]`. A repeated ID gets `duplicate_request` and is never processed twice |
| `replyTo` | on server messages: the request ID being answered |
| `ts` | server send time, Unix ms |

Unknown fields are rejected, in the envelope and in payloads. Ten malformed messages in a
row close the connection (`1008`). A message over `limits.maxMessageBytes` closes it (`1009`).

## Client → server

| Type | Payload | Effect |
|---|---|---|
| `ping` | – | `pong` |
| `session.sync` | – | `session.state` read from the session service |
| `auth.refresh` | `{token}` | adopts a fresh access token for the same user; `ack` |
| `audio.start` | `{streamId (uuid), encoding: "pcm_s16le", sampleRate: 8000\|16000\|24000\|48000, channels: 1, mode?: "command"\|"answer"}` | opens an audio stream; **barge-in**: cancels any speech or processing in progress |
| *(binary)* | audio chunk, see below | |
| `audio.end` | `{streamId, chunks?}` | ends the stream; `chunks` (count sent) is verified if present |
| `audio.cancel` | `{streamId}` | abandons the stream or its processing; `ack` |
| `tts.cancel` | `{responseId?}` | stops speech; `ack` |
| `command` | `{intent, optionKey?, confirmationId?, text?}` | a typed intent without speech (keyboard, switch device). Same validation and confirmation path as speech. `dictate_answer` is not accepted here |

## Server → client

| Type | Payload | Notes |
|---|---|---|
| `ready` | `{connectionId, protocol, sessionId, resumed, limits, capabilities, session}` | first message. `capabilities.recording` is true when dictated answers are kept as audio; a client must tell the student so |
| `ack` / `pong` | – | |
| `error` | `{code, message, retryable, streamId?}` | |
| `session.state` | `{status, remainingSeconds?, question?}` | `question` is the student-facing DTO; never contains correctness data |
| `session.closed` | `{reason}` | sent just before the server closes the socket |
| `auth.expiring` | `{expiresAt}` | send `auth.refresh` or be closed with `4401` shortly after expiry |
| `stt.partial` | `{streamId, text}` | advisory, may be dropped under load |
| `stt.final` | `{streamId, text}` | |
| `intent` | `{streamId?, intent, optionKey?, confidence, classifier}` | what the classifier understood (after validation) |
| `confirm.request` | `{confirmationId, action, questionId, optionKey?, optionText?, text?, expiresAt}` | an answer is staged, **not saved** |
| `action.result` | `{action: "answer"\|"navigate", status: "ok"\|"failed"\|"rejected"\|"cancelled", saved, corrected?, code?, retryable?, duplicate?, confirmationId?, questionId?, optionKey?, answerId?, savedAt?, direction?}` | `saved: true` only after the answer service has persisted |
| `response` | `{responseId, streamId?, kind, code?, text, audio}` | what the assistant says, always as text; `audio: true` means `tts.start` follows |
| `recording.status` | `{streamId, status: "stored"\|"failed", recordingId?, durationMs?, code?}` | only when `capabilities.recording` is true. Whether the audio of a dictated answer was kept. **Says nothing about the answer**, which is saved only when `action.result` says so |
| `tts.start` | `{responseId, format}` | |
| *(binary)* | audio chunk tagged with `responseId` | |
| `tts.end` | `{responseId, reason: "completed"\|"barge_in"\|"cancelled"\|"error", code?, chunks}` | |

### Binary audio frames (both directions)

```
byte  0       frame version (1)
bytes 1–16    stream ID (client audio) or response ID (server audio), raw UUID
bytes 17–20   sequence number, big-endian uint32, from 0, +1 per chunk
bytes 21–     mono PCM16LE audio (a whole number of samples)
```

Client chunks must arrive in sequence, be ≤ `limits.maxChunkBytes`, and stay within
`limits.maxStreamBytes` / `limits.maxStreamMillis` per stream. A violation fails the stream
with one `error`; its remaining chunks are discarded silently.

**Discard server audio whose `responseId` is not the response you are playing.** After a
barge-in the server stops sending and drops frames it had already queued, but a frame already
on the wire can still arrive.

## Audio modes

`audio.start.mode` says what the utterance is for. It is chosen by a control (button or key) on
the client and is never inferred from the words. An omitted mode means `command`.

| Mode | What happens to the transcript |
|---|---|
| `command` (default) | Matched against the closed intent set (rules, then the optional LLM). **It can never become answer text**, except behind an explicit spoken phrase ("my answer is ..."). If a classifier proposes `dictate_answer` without that phrase, the result is `unknown` and the student is asked again. |
| `answer` | **Not classified.** The transcript is staged verbatim as a free-text answer (`confirm.request`), exactly as for dictation, and still needs an explicit confirm. Command words inside it ("read the question") are literal content. Not valid for multiple-choice questions. |

An unknown mode is rejected with `invalid_audio_mode`.

## Spoken dictation in command mode

In `command` mode only an explicit phrase turns speech into answer text: "my answer is ...", "the
answer is ...", "write down ..." or "write ...", optionally after a filler such as "okay", "so",
"well" or "um" (two at most). The saved text is what **follows** the phrase, in the student's words;
the phrase is not part of the answer. A model labelling speech as `dictate_answer` is not enough:
without the phrase the student is asked again, and told they can press **Dictate answer**. In
`answer` mode nothing is stripped or interpreted.

Length limits (`intent.maxTranscriptChars`, default 2000) count **characters, not bytes**, so text
in Hindi or Arabic is not limited to a third of what English allows.

## Intents

`repeat_question`, `next_question`, `previous_question`, `select_option` (+ `optionKey`),
`dictate_answer`, `read_back_answer`, `time_remaining`, `confirm`, `deny`, `unknown`.

Classifier output is validated against this set and against the current question's option
keys. Anything invalid, `unknown`, or below `SCRIBE_VOICE.INTENT.MIN_CONFIDENCE` produces a
clarification response and no action. Submitting the exam is deliberately not a voice intent.

## Intent classification providers

`SCRIBE_VOICE.INTENT.PROVIDER` selects who interprets speech that is not an exact command:
`rules` (default, offline), `anthropic`, `groq` or `ollama`. With any model provider the exact
commands ("next question", "yes", "option B") are matched locally first and **never leave the
server**; only other speech is sent. The model returns only an intent from the closed set above,
and the backend validates it (and the option key against the current question) before acting.

**Groq.** Set `PROVIDER=groq` and `API_KEY=gsk_...` in your private environment. `MODEL` defaults to
`llama-3.3-70b-versatile` (any Groq chat model works, for example `llama-3.1-8b-instant` for lower
latency or `openai/gpt-oss-20b`) and `BASE_URL` to `https://api.groq.com/openai/v1`. Leave
both unset unless overriding: the server refuses to start if `MODEL` is a Claude model or `BASE_URL`
is the Ollama address, which are the values an earlier example file set for other providers.

- **What is sent:** the transcript, the question type, the option keys and texts as the student
  sees them, and whether an answer is awaiting confirmation. Never answer keys, grading data or
  anything about the student. Transcripts of speech the rules cannot place do go to Groq, so cover it in
  the same notice and data-processing terms as the speech recogniser.
- **How:** JSON object mode (supported by every Groq model) at temperature 0, with the schema stated
  in the prompt. The reply is then validated strictly like every other provider's: an unknown
  intent, extra fields, a missing field, an out-of-range confidence, an option the question does
  not have, or a reply cut off by the length limit is refused.
- **Failures** never change exam state: a rate limit, a bad key, a timeout or an outage gives the
  student "intent unavailable" and exact commands keep working. The key and the transcript never
  appear in errors or logs.

## Answer flow

```
student: "option B"
  ← stt.final, intent{select_option, B}
  ← confirm.request{confirmationId, optionKey: B}      nothing saved yet
  ← response{kind: confirmation_request} (+ audio)
student: "yes"   (or command{intent: confirm, confirmationId})
  ← action.result{action: answer, status: ok, saved: true, answerId}
  ← response{kind: saved} (+ audio)
```

- Answer selection **always** requires confirmation. `no`/`deny` discards it; so does any
  other request (the response says the answer was not saved).
- **Correcting a dictated answer.** Before saving, the student may fix what the recognizer got
  wrong: `command{intent: "confirm", confirmationId, text}`. `text` is the student's own wording
  (never model output). It is valid only with `confirm`, only with the `confirmationId` of the
  staged answer, only for a dictated (free-text) answer, non-blank, and no longer than
  `intent.maxTranscriptChars`; anything else is refused and the answer stays staged. If `text`
  equals what was heard, it is an ordinary confirmation. If it differs, that wording is saved
  and `action.result` carries `corrected: true`. The recognizer's original words stay in the
  stored transcript, and the saved answer is recorded as a `STUDENT_CORRECTION` revision that
  still points at the recording it corrects.
- **A confirmation is applied once.** Repeating a `confirmationId` that was already applied (for
  example after a reconnect) changes nothing, even if the student has since saved something else:
  the earlier value is never put back over a later one. Within a single request, the answer and its
  history are written in one transaction.
- A confirmation expires after `limits.confirmTtlMillis` and is void if the exam has moved to
  another question.
- For free-text questions the saved text is the recognizer's transcript verbatim; no model
  output is ever used as answer content.
- A confirm that carries an already-completed `confirmationId` returns the original result
  with `duplicate: true` and does not write again.
- On a persistence failure `saved` is `false`. If `retryable`, the student may confirm again;
  the server never retries a write on its own.
- Once a save has started it is carried through even if the student interrupts or the socket
  drops, and `action.result` is still sent.

## Answer audio retention

Off unless the server is configured with `SCRIBE_RECORDING.ENABLED=true`: keeping audio of
students is an institutional decision (notice, consent or authorization, retention period).

- **Only `answer`-mode utterances on written-answer questions are kept**, and only once a
  transcript exists. Commands, silence, multiple-choice utterances and interrupted utterances
  are never retained.
- The audio is stored first, then its metadata, so a database row always points at audio that
  exists. If the metadata write fails the object is removed again. Re-saving the same `streamId`
  with the same audio stores it once. A `streamId` that already holds audio of another question,
  another student or **different audio** is refused (`recording.status{status: "failed", code:
  "stream_reused"}`): the transcript is still staged, but linked to no recording, so a confirmed
  answer never points at audio of something else.
- **Answers never depend on it.** If the audio cannot be stored the student gets
  `recording.status{status: "failed"}` and the transcript is staged for confirmation exactly as
  it would be without recording. The answer is saved only when `action.result.saved` is true.
- The machine transcript is stored once and never edited. When the student confirms, the answer
  gets a revision (`STT_CONFIRMED`) in the same transaction as the answer, linked to the
  transcription and the recording. A repeated confirmation adds no second revision.
- Storage keys are random and carry no student, exam or question data. Audio is not served by any
  endpoint yet; teacher playback is a separate, audited feature.

## Close codes

| Code | Meaning |
|---|---|
| 1001 | server shutting down — reconnect |
| 1008 | repeated protocol violations |
| 1009 | message too big |
| 4401 | access token expired without `auth.refresh` |
| 4403 | not authorized for this session |
| 4409 | replaced by a newer connection for the same session |
| 4410 | session ended (`session.closed.reason`: `submitted`, `expired`, `inactive`, `not_found`) |
| 4429 | client is not reading fast enough |

## Error codes

Protocol: `invalid_message`, `unsupported_version`, `unknown_type`, `duplicate_request`,
`rate_limited`, `unauthorized`, `forbidden`.
Audio: `invalid_audio_format`, `invalid_audio_mode`, `stream_already_active`, `unknown_stream`, `invalid_frame`,
`chunk_too_large`, `chunk_out_of_order`, `stream_too_large`, `stream_too_long`,
`chunk_count_mismatch`, `empty_stream`, `audio_backpressure`.
Providers: `stt_unavailable`, `stt_failed`, `stt_timeout`, `provider_busy`,
`intent_unavailable`, `tts_unavailable`, `tts_failed`.
Dialog: `turn_cancelled`, `no_speech`, `not_understood`, `unsupported_intent`,
`invalid_option`, `nothing_to_confirm`, `confirmation_expired`, `confirmation_mismatch`,
`no_next_question`, `no_previous_question`, `no_answer`.
Session/infra: `session_expired`, `session_submitted`, `session_inactive`,
`exam_unavailable`, `busy`, `service_unavailable`, `internal_error`.

## Behaviour during outages

| Dependency down | Behaviour |
|---|---|
| PostgreSQL | new connections get 503; actions return `service_unavailable` (retryable), nothing is reported as saved; open connections are kept |
| Redis | navigation and answer writes need the session lock, so they fail as above; reads still work; the connection is kept. Redis is never the source of truth for answers |
| STT | the stream fails with `stt_failed` / `stt_timeout` / `provider_busy`; nothing is interpreted. `command` messages still work |
| Intent LLM | exact commands are matched locally and still work; other utterances get `intent_unavailable` and change nothing |
| TTS | `response` text is still delivered, followed by `tts.end{reason: "error"}`; the client can speak it locally |

## Observability

Structured log events (`event` field): `voice.connected`, `voice.disconnected`,
`voice.rejected`, `voice.intent`, `voice.command`, `voice.answer_saved`,
`voice.answer_failed`, `voice.navigated`, `voice.turn_failed`, `voice.session_closed`, and
provider failure events. Each carries `connection_id`, `session_id`, `user_id` and, per
turn, `request_id` / `stream_id`. Raw audio, transcripts, answer text and tokens are never
logged (only lengths, intent names and IDs).

Metrics (in-process, and `Custom/Voice/*` in New Relic when configured): connections
active/opened/closed/rejected, reconnects, stream count/duration/bytes, provider latency
(`stt_open`, `stt_final`, `intent`, `tts_first_chunk`, `answer_save`), provider errors,
cancellations (`barge_in`, `client`, …), failures by code, intents, and actions by outcome.
