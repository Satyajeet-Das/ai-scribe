# Exam document ingestion (Sprint 12)

A teacher uploads an exam paper as a PDF. The server reads it and proposes
**draft** questions. The teacher corrects the draft and then **explicitly
approves** it, which copies the questions into a **draft exam**. Publishing and
assigning stay exactly as they were.

Nothing the reader produces ever reaches an exam without that approval, and the
reader never decides a correct answer.

- Code: [`internal/document`](../../apps/backend/internal/document) (service, worker, handler),
  [`internal/document/extract`](../../apps/backend/internal/document/extract) (model-independent extraction),
  [`internal/platform/pdf`](../../apps/backend/internal/platform/pdf) (the only code that touches a PDF parser)
- Migration: [`009_exam_documents.sql`](../../apps/backend/migrations/009_exam_documents.sql)
- Contract: [`static/openapi.json`](../../apps/backend/static/openapi.json), tag **Exam Documents**
- Example paper, scripted model replies and the resulting response:
  [`internal/document/testdata/`](../../apps/backend/internal/document/testdata)

## 1. What a teacher does

```text
POST   /exams/:exam_id/documents                          upload a PDF           → 202 QUEUED
GET    /exams/:exam_id/documents/:id                      status and progress    → poll
GET    /exams/:exam_id/documents/:id/draft                the extracted draft    → 200 once REVIEW_REQUIRED
PUT    …/draft/questions/:qid                             correct a question     (and POST / DELETE / reorder)
POST   /exams/:exam_id/documents/:id/approve              copy into the exam     → 200
POST   /exams/:exam_id/publish                            existing endpoint, unchanged
```

## 2. The pipeline

| # | Stage | Where | Notes |
|---|---|---|---|
| 1 | Upload and register | HTTP request | Size, `%PDF-` signature, structure, encryption and page count are checked **before** anything is stored. The file is stored under a random key; the client's name is only a label. |
| 2 | Read the text layer | worker, `platform/pdf` | Text is put in reading order, with two-column pages read left column then right. Running headers, footers and page numbers are found by height and set aside (and recorded, not discarded). |
| 3 | Pictures | worker | Pages with no text layer (scans), and pages with figures, have their embedded JPEG/PNG images extracted — within a page and size budget — for a vision model, if one is configured. |
| 4 | Structure into questions | `extract`, via a `Model` | Pages go to the model in batches; the reply is JSON that must match a schema. |
| 5 | Validate | `extract` | Reply parsed and checked. Wording is compared with the page's own text. Anything doubtful becomes a **warning**, never a deletion. |
| 6 | Save the draft | one short transaction | Replaces the document's draft atomically. No transaction is open during parsing or model calls. |
| 7 | Teacher review | HTTP | Edit, add, delete, reorder. Every change names the draft version it was made against. |
| 8 | Approve | one transaction | Document row and then exam row locked; every rule re-checked; questions imported; document marked COMPLETED. |

### What is checked against the page

For pages that have a text layer, each question's wording, instructions and
options must be found in that page's text (compared after folding Unicode
compatibility forms, so `x²` matches `x2`, ignoring spacing and line-end
hyphenation). Wording that is not found is flagged `UNVERIFIED_TEXT` — a
*blocking* warning the teacher must acknowledge — and the question's confidence
drops to LOW. Marks that the model reports but the page does not show are
dropped (`MARKS_NOT_ON_PAGE`). Text read from a picture is flagged
`READ_FROM_IMAGE` and is never HIGH confidence.

Nothing is invented: the prompt forbids it, the schema has no field for an
answer key, and `isCorrect` can only be set by a teacher.

## 3. States

```text
QUEUED ──> PROCESSING ──> REVIEW_REQUIRED ──> COMPLETED
   ^            │               │
   │            ├──> FAILED     └─(retry, discarding the draft)─> QUEUED
   └─(retry)────┴──> CANCELLED
```

There is no separate `UPLOADED` state: the row is created only once the file is
stored and valid, already `QUEUED`. `PROCESSING` carries a `stage`
(`READING_PAGES`, `EXTRACTING`, `VALIDATING`, `SAVING`) and `progress {done,total}`.
A `FAILED` document has a `failureCode` and a `failureMessage` **written by this
server** — never the provider's words, which could echo the paper.

## 4. Background work

Extraction runs on its **own** asynq server (queue `documents`, concurrency
`SCRIBE_DOCUMENTS.JOB.MAX_CONCURRENT`), so a backlog of papers can never delay a
session expiring, and the number of PDFs parsed at once is bounded exactly.

Safety rules:

- **Attempts.** Every (re)queue raises `attempt`. A job carries its attempt and the
  task id is derived from it, so the same attempt cannot be queued twice. A job for
  an old attempt does nothing.
- **Claiming.** `QUEUED → PROCESSING` is one conditional `UPDATE`; only one worker can
  win. A worker owns the document for `LEASE` and renews it every few seconds. If it
  dies, the lease runs out and a later delivery takes over. A second delivery while
  the lease is live is told to come back later (`ErrLeaseHeld`), never run in parallel.
- **Stale workers cannot write.** Every worker write (`progress`, `heartbeat`,
  `save draft`, `fail`, `requeue`) is conditional on *this attempt* still being
  `PROCESSING`. The heartbeat also notices cancellation, deletion or a restart and
  cancels the job's context, which stops the model call in flight.
- **Failures.** Transient (rate limit, 5xx, timeout, database trouble): the document
  goes back to `QUEUED` and the queue retries with backoff; on the last delivery it
  becomes `FAILED`. Permanent (unusable output, bad credentials, retired model, damaged
  PDF): `FAILED` at once. A reply the model did not format correctly is asked for once
  more.
- **Idempotent.** A draft is *replaced*, never appended to, so a retry cannot duplicate
  questions or leave two conflicting drafts.

## 5. Approval

`POST …/approve` with the `expectedDraftVersion` the teacher last saw.

1. Lock the document row; it must be `REVIEW_REQUIRED` and at that version.
2. Lock the **exam** row; it must still be a **DRAFT** owned by the caller (or an
   administrator). Because the row is locked, the exam cannot be published between this
   check and the import. (Covered by a test that races approve against publish.)
3. Re-check the whole draft. All problems are returned together as `409 DRAFT_INCOMPLETE`
   with `errors[].field` such as `questions[2].points`:
   marks set on every question; at least two options on every multiple-choice question;
   labels valid and unique; at most one correct option; every diagram described **and
   confirmed** by the teacher; every blocking warning acknowledged.
4. Import through the question module's own rules (`question.Service.ImportQuestions`),
   numbered after the questions already in the exam.
5. Mark the document `COMPLETED`.

All of it is one transaction. If any step fails nothing is added and the document is
still waiting for review. Approving twice is `409 ALREADY_APPROVED`; the draft id is
also unique among questions, so even a bug could not create the same question twice.

### Why the question text contains tables and diagrams

A question has one `text` field, and the exam room reads it aloud to candidates. So on
approval the draft's parts are composed into it, in the order a reader meets them:

```text
<instructions>

<question>

Table (page 1):
Time (s) | Speed (m/s)
0 | 0
5 | 20

Diagram (page 1): <description a teacher confirmed>
```

This is why every diagram needs a confirmed description before approval: it would
otherwise be silently missing for a candidate who cannot see it. The smallest change to
the existing schema is therefore two nullable provenance columns on `questions`
(`source_document_id`, `source_draft_question_id`); richer storage of tables and figures
is left for Sprint 13 (STEM accessibility).

## 6. Security and privacy

- Teachers and administrators only; the exam's owner (or an admin) only. Someone else's
  exam, a missing exam, and a document reached through another exam's URL all return the
  same `404`. Every endpoint is checked in the service as well as by route middleware.
- Uploads: streamed (never spooled to disk), cut off at the size limit, rate limited per
  user. Checked for the PDF signature and end marker, then parsed under `recover()`, a
  timeout and a concurrency gate. Encrypted PDFs are refused. Parser error text, which
  can quote the file, is never passed on.
- Files are private objects, served only by `GET …/file` after the ownership check, as an
  attachment, `no-store`, `nosniff`. Keys are random UUIDs that reveal nothing.
- Content sent to the model is data in a JSON field, never part of the instructions, and
  the prompt says document text is never an instruction ("ignore the above" in a paper
  does nothing). The reply is untrusted and validated like any other input.
- Logs carry ids, counts, stages and codes — never page text, question text, provider
  replies or credentials. Provider errors keep at most one short machine-readable code.
- **With the `groq` provider, the text (and for scanned pages, images) of uploaded papers
  is sent to Groq.** With the provider `disabled` (the default) uploads are refused and
  nothing leaves the server. Check Groq's data terms before sending unreleased papers.

## 7. Configuration

All under `SCRIBE_DOCUMENTS.*`; see `apps/backend/.env.example`. Off unless
`ENABLED=true`; and even then, uploads need `EXTRACTION.PROVIDER` and its key.

| Setting | Default | |
|---|---|---|
| `ENABLED` | `false` | |
| `MAX_BYTES` / `MAX_PAGES` | 20 MiB / 60 | per upload |
| `STORAGE.PROVIDER` | `localfs` | or `s3` (production: `USE_SSL=true`, `CREATE_BUCKET=false`) |
| `EXTRACTION.PROVIDER` | `disabled` | or `groq` |
| `EXTRACTION.API_KEY` | – | from the environment only; never logged |
| `EXTRACTION.TEXT_MODEL` | `openai/gpt-oss-120b` | needs `json_schema` structured output |
| `EXTRACTION.VISION_MODEL` | `qwen/qwen3.8-27b` | empty = pictures are reported, not read |
| `JOB.MAX_CONCURRENT` / `MAX_RETRY` / `TIMEOUT` / `LEASE` | 2 / 3 / 15m / 2m | |

Groq retires models; a retired one answers `404 model_not_found` and the document fails
with `MODEL_NOT_FOUND`. Change the setting.

## 8. Migration

`009_exam_documents.sql` applies with the normal migrator at start-up (`make migrate`).
It adds `exam_documents`, `exam_document_questions` and two nullable columns on
`questions`; nothing existing changes and the rollback section reverses it. Rolling back
removes the draft tables and the provenance columns; questions already approved are kept.

## 9. Tests

- `internal/platform/pdf`, `internal/document/...`, `internal/platform/config`: unit tests
  with an in-memory database that behaves like transactions, scripted models, and PDFs
  built in the tests. No network and no paid calls. `go test -short ./...`
- `tests/integration/document_test.go`: the real migration, SQL, row locks and
  transactions. By default it uses Docker like the other integration tests. To run it
  against any running PostgreSQL instead:
  `SCRIBE_TEST_PG_ADDR=127.0.0.1:5432 SCRIBE_TEST_PG_USER=postgres go test ./tests/integration -run 'TestDocument|TestQuestions_HandMade'`
- To regenerate the example fixture after changing it on purpose:
  `UPDATE_GOLDEN=1 go test ./internal/document -run TestExample`

## 10. Known limitations

- **Pictures.** There is no page renderer. A scanned page is read only if the PDF stores
  it as an ordinary JPEG or PNG; JBIG2/CCITT scans and drawings made of vector shapes are
  reported as not read. Handwriting is not supported. At most 3 pictures per page and 25
  picture pages per document are sent.
- **Diagrams.** The model describes a figure from what it can see and its printed
  caption; the description is a starting point and must be confirmed by a teacher.
- **Mathematics.** Maths set in symbol fonts can come out of the text layer garbled.
  Notation is kept as found and flagged when it does not match, not "fixed".
- **Layout.** Two-column detection is deliberately strict: an unusual layout is read row
  by row, which keeps lines whole but may interleave columns. Tables are kept as Markdown
  text, not as structure.
- **Providers.** Free-tier rate limits can slow large papers (retried with backoff).
  Only Groq is implemented; the `extract.Model` interface is the seam for others.
- **Resources.** A pathological PDF (for instance a stream that inflates enormously) is
  bounded by timeouts, a concurrency gate and the size limit, but not by a memory cap.
- **Orphans.** If the server dies between storing a file and recording it, the object is
  left behind; there is no sweeper for the document store yet.
- **Audit.** Every state change goes through one repository method each, which is where
  Sprint 11's audit trail can hook in; it is not built here.
