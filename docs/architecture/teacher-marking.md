# Teacher marking

Teachers mark a student's paper on the response review page: marks for each answer, a
feedback comment, a total for the paper, and a way to finish the evaluation. **Marking is
manual.** Nothing here asks a language model to mark an answer, and nothing marks an essay
or spoken answer automatically.

## Who, when

- Only the exam's owner or an administrator. A student, proctor or another teacher is
  refused; for another teacher the exam looks as if it does not exist.
- Only once the student's session has ended (`SUBMITTED` or `EXPIRED`). While it is in
  progress there is nothing to mark, and the answer key is not sent.
- The marking API sits beside the review API under
  `/exams/:exam_id/responses/:student_id/marking`. The review package stays read-only;
  marking is the new `internal/grading` package and is the only writer of marks.

## Marks

- Marks go from 0 to the question's points in **steps of 0.5**. They are stored as whole
  **half-marks** (7.5 marks is `15`), so every stored value is a multiple of 0.5 and totals
  never drift. Text like `7.25`, `1e1` or `-1` is refused, not rounded.
- A multiple-choice answer is **marked automatically** when the stored correct option is
  known: full points for a correct choice, 0 otherwise. An unanswered question is 0. An
  essay or spoken answer, or a multiple-choice question with no correct option stored, has
  no automatic mark and needs the teacher's.
- The teacher can override any mark. A teacher's mark replaces the automatic one; clearing
  it brings the automatic one back. Automatic marks are worked out when read and are never
  stored, so they follow the question while the paper is a draft.
- An optional comment per question, up to 2000 characters, is stored with the mark.
- Totals are always computed by the server. The browser shows what it is sent.

## Draft and final

A paper is a **draft** until the teacher **finalises** it. Finalising needs every question to
have a mark that counts and the paper version the teacher last saw. It freezes every
question's mark in a snapshot, and from then on the paper is read from that snapshot. This
matters because an exam can be taken off the air and edited after students sat it; editing a
question's points or its correct option must not change a finalised result.

The exam's owner or an administrator can **reopen** a final paper, with a reason of 3 to
500 characters. The paper returns to a draft, and what had been final, together with the
reason, stays in the history.

## Safe concurrent use

- Every change to a paper first locks that paper's grading record, so saves, finalising and
  reopening happen one after another and what was checked is still true when it is written.
  A save racing a finalise either lands in the snapshot or is refused as the paper is final.
- Each answer's mark has a version. A save carries the version it read and is refused as
  `STALE_VERSION` if the mark has changed; the same check is part of the write, so it holds
  even if two saves arrive together.
- A save names the attempt it was looking at, and is refused (`SESSION_CHANGED`) if a
  different attempt is now the one shown (a student who retakes an exam).
- Repeating a save, finalise or reopen that already went through changes nothing, so a retry
  after a lost reply is safe.

## Records

| Table | Holds |
|---|---|
| `paper_gradings` | One row per marked session: draft or final, version, total, snapshot. |
| `answer_marks` | What the teacher entered per answer (half-marks and comment, version). |
| `paper_grading_events` | Finalise and reopen history, with reason and snapshot. Append only. |

Each save, finalise and reopen also writes an `audit_events` row **in the same transaction**.
If the audit row cannot be written, the change is not made (the API answers `503
AUDIT_UNAVAILABLE`). Audit entries hold ids, marks and totals only: never the comment, the
reopen reason or any answer text.

## API

| Call | Purpose | Conflicts (409) |
|---|---|---|
| `GET …/marking` | The marking sheet: per question the maximum, automatic, teacher's and counting marks, comment and version; the paper's total and status. The correct option id is included for a draft only. | |
| `PUT …/marking/questions/:question_id` | Replace one answer's mark and comment (`marks: null` clears). | `STALE_VERSION`, `PAPER_FINALISED`, `SESSION_NOT_ENDED`, `SESSION_CHANGED` |
| `POST …/marking/finalise` | Freeze a fully marked paper. | `PAPER_INCOMPLETE` (lists question numbers), `STALE_VERSION`, `SESSION_NOT_ENDED`, `SESSION_CHANGED` |
| `POST …/marking/reopen` | Return a final paper to a draft; reason required. | `SESSION_NOT_ENDED`, `SESSION_CHANGED` |

All responses are `Cache-Control: no-store`.

## Not included

Students seeing their marks, CSV export, and totals or a marking status in the student
roster are not built. Nothing a student can reach reads these tables.
