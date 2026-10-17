# Answer audio: keeping, reviewing and auditing

Teachers can listen to the original audio of a student's dictated answer, so a doubtful
transcript can be checked against what was actually said. This document describes what is
kept, who can hear it, and what is recorded about listening.

**Off by default.** Keeping audio of students is an institutional decision (notice, consent or
authorization, retention). Nothing is recorded unless the server is started with
`SCRIBE_RECORDING.ENABLED=true`.

## What is kept

| Kept | Not kept |
|---|---|
| Audio of an utterance spoken in **answer mode** (`audio.start.mode = "answer"`) on a **written-answer** question, once the recognizer produced a transcript | Voice commands, silence, utterances on multiple-choice questions, utterances the student interrupted |

Each kept recording has:

- **`audio_recordings`**: metadata (duration, size, sha-256 of the audio, which student, session
  and question). The audio itself is in object storage under a random key that carries no
  student, exam or question data. Never in PostgreSQL.
- **`transcriptions`**: what the speech recognizer produced. Written once, never edited.
- **`answer_revisions`**: every value an answer has held. A confirmed dictation is an
  `STT_CONFIRMED` revision, written in the **same transaction** as the answer and linked to the
  recording and transcription. If the student corrected the recognized text before confirming,
  it is a `STUDENT_CORRECTION` revision instead: the student's wording is the saved answer, and
  the recognizer's original words stay untouched in the transcript, so a teacher can see both. `STT_CONFIRMED` and `STUDENT_CORRECTION` revisions are never
  modified; consecutive `TYPED` autosaves and `OPTION` changes coalesce into one revision.

**An answer never depends on its audio.** If the audio cannot be stored, the student is told
(`recording.status` = `failed`) and the transcript is staged for confirmation exactly as it would
be without recording. See [voice-protocol.md](./voice-protocol.md#answer-audio-retention).

## Who can listen

`GET /api/v1/exams/:exam_id/responses/:student_id/recordings/:recording_id/audio`

- Only the exam's owner and administrators. Students and proctors are refused.
- The recording must belong to **that student's session in that exam**; any other ID is
  "not found", so an ID from another student or exam reveals nothing.
- Review of an unfinished session follows the same policy flag that decides whether its answers
  are shown (`review.Policy.AllowInProgress`).
- The response is `Cache-Control: no-store`, `X-Content-Type-Options: nosniff`, supports `Range`
  (so the player can seek), and has no identifying file name. There is no permanent or signed
  URL: the browser fetches the audio with the reviewer's credentials and plays it from memory.

| Status | Meaning |
|---|---|
| 200 / 206 | audio |
| 404 `RECORDING_NOT_FOUND` | no such recording for this student and exam, or playback is not configured |
| 410 `RECORDING_REMOVED` | the audio was deleted; the transcript remains |
| 503 `AUDIO_UNAVAILABLE` | the file cannot be read, **or the access could not be audited** (the reason is not disclosed) |

## What is audited

Every playback writes an `audit_events` row **before any audio is sent**; if that write fails,
nothing is played (fail closed). The row holds the actor, role, time, recording, session, request
ID and client address. It never holds audio, transcripts or answers. An authorized attempt whose
file turned out to be unreadable is recorded as `audio.unavailable`, not as a playback. Refused
requests play nothing and write no playback entry (denials are logged as `review.denied`).

The teacher UI fetches audio **only when the reviewer presses Play**, never to prepare a page, so
the log reflects deliberate listening.

## Configuration

```
SCRIBE_RECORDING.ENABLED=false     # master switch for keeping new audio
SCRIBE_RECORDING.PROVIDER=localfs  # localfs | s3
SCRIBE_RECORDING.RETENTION_DAYS=0  # 0 = no retention date is recorded; nothing is auto-deleted
SCRIBE_RECORDING.SAVE_TIMEOUT=5s   # most a student waits for the audio to be stored

# provider=localfs
SCRIBE_RECORDING.DIR=./data/audio  # must not be served by a web server

# provider=s3 (MinIO, AWS S3, any S3-compatible service)
SCRIBE_RECORDING.S3.ENDPOINT=minio:9000      # host:port, no scheme
SCRIBE_RECORDING.S3.BUCKET=answer-audio
SCRIBE_RECORDING.S3.ACCESS_KEY=...           # from a secret store; never logged
SCRIBE_RECORDING.S3.SECRET_KEY=...
SCRIBE_RECORDING.S3.REGION=us-east-1
SCRIBE_RECORDING.S3.USE_SSL=true
SCRIBE_RECORDING.S3.CREATE_BUCKET=false      # development convenience only
```

Playback stays available when recording is switched **off**, as long as there is a store to read:
an existing audio directory, or a configured bucket. So audio that was already kept can still be
checked. If recording is **on** and the store cannot be opened at startup, the server refuses to
start rather than silently losing students' audio.

**In production** (`PRIMARY.ENV=production`) the server refuses to start with the `s3` provider
unless `use_ssl` is true (student audio must not travel unencrypted) and `create_bucket` is false
(the bucket, with its access policy and encryption, is provisioned deliberately).

### S3 and MinIO

`docker compose up -d minio` starts a local MinIO (API on `127.0.0.1:9000`, console on
`127.0.0.1:9001`, credentials `scribe-dev` / `scribe-dev-secret` unless `MINIO_ROOT_USER` and
`MINIO_ROOT_PASSWORD` are set). Compose also points the backend at it, with recording **off**; set
`SCRIBE_RECORDING_ENABLED=true` to turn it on locally.

- **Docker Hub no longer serves new pulls of `minio/minio`.** Use a copy you already have, or set
  `MINIO_IMAGE` to your own registry. MinIO is AGPLv3; it runs as a separate service, not as part of
  the application.
- Objects are private. Nothing in the application ever creates a public or pre-signed URL, and a
  test checks that an unauthenticated request for a stored object is refused (403).
- A key is written once: the server refuses to overwrite an existing key (`If-None-Match: *`).
- Wrong credentials, an unreachable server and a missing bucket are all reported as "store
  unavailable", never as "not found", so an outage cannot be mistaken for lost audio.
- With a store that is down, a dictated answer waits up to `SAVE_TIMEOUT` (5 s) for the audio, then
  goes on without it: the student is told the audio was not kept and can still confirm the answer.

## Retention and cleanup

A background job (off unless `SCRIBE_RECORDING.CLEANUP.ENABLED=true`) deletes audio that has no
reason to exist. **Turn it on whenever recording is on:** without it, dictations a student does not
confirm keep their audio indefinitely (the server logs a warning at startup if recording is on and
cleanup is off). It invents no retention period: it acts on retention dates that already exist, and
on dictations that never became an answer.

```
SCRIBE_RECORDING.RETENTION_DAYS=0           # sets retention_until on NEW recordings; 0 = no date
SCRIBE_RECORDING.CLEANUP.ENABLED=false
SCRIBE_RECORDING.CLEANUP.INTERVAL=1h
SCRIBE_RECORDING.CLEANUP.BATCH_SIZE=100
SCRIBE_RECORDING.CLEANUP.UNCONFIRMED_GRACE=1h  # at least 10m
SCRIBE_RECORDING.CLEANUP.ORPHAN_GRACE=1h    # at least 10m + SAVE_TIMEOUT
SCRIBE_RECORDING.CLEANUP.DELETE_ORPHANS=false
```

**Purging expired audio.** A recording whose `retention_until` has passed has its audio deleted.
A recording with no retention date is never touched, so turning cleanup on does not delete
anything that was recorded without a retention period. For each recording, in this order:

1. an `audio.purged` entry is written to the audit log (actor: `SYSTEM`, reason
   `retention_expired`); if that cannot be written, the audio is kept and retried next run;
2. the audio object is deleted;
3. the row is marked `DELETED` with `deleted_at`.

A crash between 2 and 3 leaves a row that still looks stored; the next run finishes it, because
deleting an object that is already gone is not an error. The **transcription, the answer and its
revision history are kept**: only the audio expires. The review page then shows the recording as
"Audio removed" with its transcript, and playback answers `410 RECORDING_REMOVED`.

**Dictations that never became an answer.** Audio is stored when the student dictates, before they
confirm. If they say no, dictate again, or simply leave, that recording supports no answer and is
not something the notice they saw covers (it says answers are kept so a teacher can check them).
Once `UNCONFIRMED_GRACE` (default 1 hour, never under 10 minutes, far longer than the time allowed
to confirm) has passed, its audio is deleted, audited as `audio.purged` with reason
`never_confirmed`. This does not wait for a retention date. A recording that supports a confirmed
answer is never deleted by this rule, and a deleted recording can no longer be attached to an
answer.

**Orphans and missing audio.** The job compares the store with the database.

- *Orphans* are stored objects no stored recording refers to (a crash between storing audio and
  writing its row, or a delete that failed). They are always logged; they are **deleted only if
  `DELETE_ORPHANS=true`**, only if older than `ORPHAN_GRACE` (so audio being saved right now is never
  touched), and each removal is audited (`audio.orphan_removed`). Only keys this application
  generated are considered, but still use a bucket or directory **dedicated to this application**.
- *Missing audio* is a stored recording whose object is not in the store. It is logged at error
  level and counted, and **never altered**: lost data needs a person, and the row is the evidence.

Several server instances can run the job: a PostgreSQL advisory lock lets one clean at a time.

## Not done yet

- **Encryption at rest.** Objects are stored as given; turn on server-side encryption in the bucket
  (for MinIO this needs a KMS) or use a service that encrypts by default.
- **A retention policy.** The mechanism exists; the period is yours to set (`RETENTION_DAYS`), and
  there is no legal-hold or deletion-on-request feature yet.
- **Consent step.** The student sees a notice before dictating; they are not asked to agree.
- **Teacher annotations.**
- **Longer dictation.** Answer-mode audio is still limited like commands (45 s, 2 MB per stream).
