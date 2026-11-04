# AI Exam Scribe — Product Guide

*A detailed, plain-language guide to what this product is, how it behaves, what is already built, and what is planned. It is a product document, not a technical one: it describes what people see and experience, not how the code is written.*

**Status key used throughout:**

| Mark | Meaning |
|---|---|
| ✅ Built | Working today |
| 🟡 Built, off by default | Working, but an institution must switch it on deliberately |
| 🔧 In progress | Partly done or being polished |
| 📋 Planned | Designed or on the roadmap, not built yet |
| 💡 Idea | Not committed; a possible direction |

---

## Contents

1. [Vision and purpose](#1-vision-and-purpose)
2. [Who uses it](#2-who-uses-it)
3. [Design principles](#3-design-principles)
4. [The product at a glance](#4-the-product-at-a-glance)
5. [Accounts and sign-in](#5-accounts-and-sign-in)
6. [Teacher experience](#6-teacher-experience)
7. [Student experience](#7-student-experience)
8. [The voice assistant in depth](#8-the-voice-assistant-in-depth)
9. [Protecting every answer](#9-protecting-every-answer)
10. [Answer audio and answer history](#10-answer-audio-and-answer-history)
11. [Reviewing and marking responses](#11-reviewing-and-marking-responses)
12. [Importing an exam from a PDF](#12-importing-an-exam-from-a-pdf)
13. [Trust, privacy and audit](#13-trust-privacy-and-audit)
14. [Settings an institution controls](#14-settings-an-institution-controls)
15. [How the product was built so far](#15-how-the-product-was-built-so-far)
16. [Known limits today](#16-known-limits-today)
17. [What is coming next](#17-what-is-coming-next)
18. [Full roadmap table](#18-full-roadmap-table)
19. [Glossary](#19-glossary)

---

## 1. Vision and purpose

### The problem
Visually impaired students sitting high-stakes exams usually depend on a **human scribe**: a person who reads the questions aloud and writes down what the student says. This has real drawbacks:

- Scribes cost money and must be scheduled in advance.
- Quality varies. A scribe may mishear, paraphrase, rush, or tire.
- The student is never fully independent; they rely on another person's patience and accuracy.
- It is hard for a school to prove afterwards exactly what was said and written.

Ordinary online exam tools do not solve this. They assume the student can see a screen, read small text and use a mouse.

### The idea
**AI Exam Scribe replaces the human scribe with a voice-first assistant.** The student can run the entire exam by listening and speaking, or by keyboard. The assistant reads each question, listens to the student's answer, writes it down, reads it back, and only saves it once the student says yes.

For schools, the same platform provides the normal exam machinery (creating exams, assigning students, reviewing answers, marking) together with an **auditable record** of what happened.

### What success looks like
- A blind or low-vision student can sit a full exam **independently**, with confidence that nothing was misheard or lost.
- A teacher can prepare an exam quickly (including by uploading an existing PDF paper) and mark it with all the evidence in one place.
- An institution can show, with a record, that the process was fair and unaltered.

### Initial target scale
The system is being designed and validated for roughly **1,000 users to begin with**. It is deliberately built so it can grow later, without over-engineering for millions of users now.

---

## 2. Who uses it

| Role | Who they are | What they can do |
|---|---|---|
| **Student (candidate)** | A learner sitting assigned exams, often visually impaired | See their assigned exams, take them by voice/keyboard/typing, submit |
| **Teacher (educator)** | Creates and marks exams | Create exams and questions, import PDFs, assign students, review responses, listen to recordings, mark papers. **Only sees their own exams.** |
| **Administrator** | Oversees the institution | Everything a teacher can do, across all teachers' exams, including filtering by teacher |
| **Proctor** 📋 | Monitors live exams | Role exists in the system; the monitoring tools for it are planned (see §17) |

**Access rules in plain words**
- A student can never open teacher tools.
- A teacher can never see, edit, publish or mark another teacher's exam. To them, that exam simply looks like it does not exist.
- Administrators have universal management permission.

---

## 3. Design principles

These principles drive every feature. When something in this guide seems unusually cautious, one of these is the reason.

1. **Voice first, keyboard always.** Every action can be done by speaking or by keys. Nothing requires a mouse or sight.
2. **Never lose a student's answer.** Network drops, double-saves, switching questions, running out of time — all are handled so that work is not silently lost.
3. **Nothing is saved by voice without an explicit "yes".** The assistant reads back what it understood and waits.
4. **The AI interprets; the system decides; the database records.** The AI never changes an exam directly. Everything it suggests is checked against strict rules first.
5. **The AI is never an answer machine.** It classifies what the student *meant* to do. It does not generate answers, give hints, or pick a correct option.
6. **Humans approve.** Imported questions, marks and finalisation need a teacher's deliberate action.
7. **Privacy by default.** Student audio is not recorded unless the institution decides to.
8. **Everything important leaves a trace.** Sensitive actions are written to an audit log, and if the log cannot be written the action does not happen.
9. **Say why.** When the product refuses something, it says so in words ("an answer is waiting for confirmation"), never silently.
10. **Honest limits.** Where the product is unsure (a doubtful PDF extraction, a command-like dictation), it flags the doubt rather than hiding it.

---

## 4. The product at a glance

### The main parts

| Part | Used by | Purpose |
|---|---|---|
| Landing and sign-in | Everyone | Entry point; sends people to the right place by role |
| **Exam dashboard** | Teachers | List, search, create and manage exams |
| **Exam page** (questions, assignments, PDF import) | Teachers | Build an exam, assign students, import from PDF |
| **Draft review** screen | Teachers | Check and correct questions extracted from a PDF |
| **Responses roster and response review** | Teachers | See each student's paper, recordings and history; mark it |
| **My exams** | Students | Personal list of assigned exams |
| **Exam room** | Students | Take the exam by voice, keyboard or typing |

### The life of an exam, end to end

```text
Teacher                                           Student
-------                                           -------
Create exam (Draft)
Add questions (by hand or from a PDF)
Publish
Assign students  ───────────────────────────────▶ Sees exam in "My exams"
                                                  Starts the exam (timer begins)
                                                  Listens, answers by voice/keys/typing
                                                  Confirms answers; navigates
                                                  Submits (or time runs out)
Open the student's responses  ◀──────────────────
Review answers, history, audio (if enabled)
Mark (half-marks, feedback)
Finalise the paper
(Reopen with a reason, if a correction is needed)
```

---

## 5. Accounts and sign-in  ✅

- **Registration** for students and teachers, with clear "required" markers on form fields and checking as the person leaves each field.
- Students register with a **roll number**, a unique identifier teachers can later use to find them.
- **Sign-in** with credentials; the sign-in endpoint is rate-limited to resist guessing.
- **Secure, renewable sessions.** Access renews automatically in the background (refresh tokens rotate), and signing out revokes access immediately.
- **Role-based redirects.** After sign-in the person lands where their role belongs. The landing page itself sends already-signed-in visitors onward.
- **Friendly error pages** for "not signed in" and "not allowed", so a blocked person understands what happened.
- **Pluggable identity.** The system is built so the sign-in method can be swapped for an external identity provider later without changing the rest of the product.

---

## 6. Teacher experience

### 6.1 Exam dashboard  ✅
- A list of the teacher's exams (administrators see all, and can filter by teacher).
- **Search** by title or text, **filter** by status and subject, and **page** through long lists.
- Create a new exam from a dialog; edit an exam's details; each exam shows its status.
- **Click an exam's card** to open it and manage its questions.

### 6.2 Exam details and lifecycle  ✅

An exam has a title, subject, description and a **duration in minutes**. It moves through a deliberate lifecycle:

```text
          DRAFT  ──publish──▶  PUBLISHED  ──archive──▶  ARCHIVED
            ▲                     │
            └────── unpublish ────┘   (only if nobody has started or been assigned)
```

| Status | What it means | What you can do |
|---|---|---|
| **Draft** | Being prepared | Edit everything: title, subject, description, duration, questions |
| **Published** | Live for assigned students | Minor edits like the description are fine. **Structure such as duration is locked** so students are never surprised |
| **Archived** | Finished and kept as a record | Read-only. Cannot be edited, published or unpublished |

Additional rules:
- **Unpublish** (an emergency "take it back") is allowed only if there are **no active assignments or student sessions**. Otherwise it is refused with an explanation.
- **Delete** is a soft delete (the record is hidden, not erased). It is refused if students are assigned or sitting the exam; archive the exam instead.
- A teacher can only touch their own exams.

### 6.3 Question studio  ✅

**Question types**

| Type | What the student does |
|---|---|
| **Multiple choice (MCQ)** | Picks one option |
| **Essay** | Writes or dictates a long free-text answer |
| **Voice** | A spoken written-style answer, dictated and transcribed |

**What a teacher can do**
- Add, edit and delete questions; set the marks (points) for each.
- Add, edit, delete and reorder options; mark the one **correct option**.
- Reorder questions with a dedicated reorder action.
- Questions remember **where they came from**: typed by hand, or imported from a particular PDF.
- A clear preview with readable option text and contrast.
- Honest empty states when there are no questions yet (no fake sample data).

### 6.4 Assigning students  ✅
- **Find students** with an autocomplete search that matches roll number prefix, name, or email.
- **Assign** by picking a student, or by typing a roll number.
- **Bulk assign** many students at once.
- **Revoke** an assignment.
- The list of who is assigned to an exam is always visible on the exam page.
- Assignment is allowed on draft exams so teachers can prepare ahead.

### 6.5 Importing from a PDF  ✅ (🟡 off by default, 🔧 being polished)
Covered in full in [§12](#12-importing-an-exam-from-a-pdf).

### 6.6 Reviewing responses and marking  ✅
Covered in full in [§11](#11-reviewing-and-marking-responses).

---

## 7. Student experience

### 7.1 "My exams"  ✅
- A personal dashboard of exams assigned to the student.
- Each exam can be started, or resumed if already begun.
- If there is nothing assigned, the page says so clearly instead of showing placeholders.

### 7.2 Before the exam starts  ✅
An **instructions dialog** explains the controls (voice modes and keyboard shortcuts) so the student knows how the room works before the clock matters.

### 7.3 Inside the exam room  ✅

The exam room is a single focused screen:

- **The current question** with its options (for multiple choice) or an answer box (for written answers).
- **A visible and announced timer** based on time kept by the server, not by the browser, so refreshing the page or changing devices cannot cheat or lose time.
- **Question navigation:** next/previous by button, keyboard or voice.
- **A voice panel** with *Start speaking* (for commands) and *Dictate answer* (to speak the answer itself), plus status of what the assistant is doing.
- **Clear messages** for anything that needs attention — an unsaved answer, a refused action, a changed answer.

### 7.4 Three ways to answer  ✅

| Way | How it works |
|---|---|
| **Type** | Type in the answer box. It saves automatically shortly after typing stops. |
| **Choose** | Select an option on screen or say it ("option B"). Choice is saved at once; by voice it must first be confirmed. |
| **Speak** | Dictate the answer. The assistant transcribes it, reads it back, and saves only after a yes. The student can **correct the wording** before confirming. |

### 7.5 What happens if something goes wrong  ✅

| Situation | What the student experiences |
|---|---|
| Internet drops and returns | The student can rejoin the same session and continue from where they were. Their saved answers are still there. |
| A save fails | The room says the answer "has not been saved yet" and offers *Retry saving*. Moving to another question does not lose it. |
| Closing or leaving the page with unsaved work | A warning appears. |
| Submitting while a recording is in progress, being processed, or waiting for confirmation | The room says exactly what would be lost and offers *Submit anyway*. |
| Time runs out | Typed text is saved first; then the server decides if the exam is over. Voice is shut off immediately. |
| The answer changed elsewhere (for example, a dictation was confirmed while the student was typing) | The student chooses **Keep my version** or **Use the saved answer**. Nothing is overwritten silently. |

> **Note on the timer:** today the exam clock runs on the server's wall-clock time from when the exam started, so time spent disconnected still counts. A fairer approach that **pauses the clock during disconnections** (with safeguards against misuse) has been designed and is planned. See [§17.2](#172-fair-timer-pause-and-resume-for-disconnections).

---

## 8. The voice assistant in depth  ✅

The voice assistant is the heart of the product. It is designed around one fear: *a blind student must never have an answer saved that they did not intend.*

### 8.1 The conversation loop

```text
Student speaks
   → speech is turned into text
   → the system works out what was meant (a command? an answer?)
   → the system checks that it is allowed and sensible
   → the assistant speaks back what it understood
   → the student confirms (or says no / corrects)
   → only then is anything saved
```

### 8.2 Two modes, chosen by the student's action

| Mode | Started by | Purpose |
|---|---|---|
| **Command mode** | *Start speaking* / Alt+M / Enter on a multiple-choice question | Speak an instruction: "read the question", "next question", "option B" |
| **Dictation mode** | *Dictate answer* / Alt+D / Enter on a written question | Speak the answer itself |

The key idea: **the mode is chosen by the control used, never guessed from the words.**

- In dictation mode, everything spoken is answer text. If the student says "next question" as part of an essay, it is written down, not obeyed.
- In command mode, speech can never silently become an answer. The only exception is an explicit spoken phrase such as **"my answer is …"**, **"the answer is …"**, or **"write down …"**. The saved answer is what comes *after* that phrase.

### 8.3 Voice commands

| What the student can say | Result |
|---|---|
| Repeat / read the question | The assistant reads the question and options again |
| Next question / previous question (or "go back") | Moves, if allowed |
| Option A / B / C … | Stages that choice for confirmation |
| My answer is … | Stages a written answer for confirmation |
| Read back my answer | Reads the currently saved answer |
| How much time is left | Tells the remaining time |
| Yes / confirm | Confirms a staged answer |
| No / cancel | Discards a staged answer |

**Submitting the exam is deliberately not a voice command.** Ending an exam is too serious to trigger by a mishearing; it needs a deliberate button press.

### 8.4 Confirmation before saving

- Every answer chosen or dictated by voice is **staged, not saved**. The assistant reads it back.
- Only **one** answer waits for confirmation at a time. A newer selection or dictation cancels the older one, and the older one can no longer be confirmed by accident.
- A confirmation **expires** after a short time and is voided if the student moves to another question.
- **Correcting before saving:** if the recogniser misheard, the student can fix the wording before confirming. The corrected wording becomes the answer; the recogniser's original words are still kept for the teacher to see.
- A confirmation **applies once only.** Repeating it after a reconnect cannot put an older value back over a newer one.
- If saving fails, the student is told and may confirm again. The system never quietly retries a write.

### 8.5 Safeguard: a dictation that is only a command  ✅

If a dictated answer is, word for word, a command such as "read the question" or "next question":
- The assistant says: *"That sounds like a command, not an answer."*
- Focus defaults to **No, cancel**.
- A **Read the question instead** button runs the command.

Words inside a longer sentence never trigger this, and ordinary answers like "yes", "no" or "correct" are never flagged.

### 8.6 Speaking and interrupting

- The assistant reads questions, options, answers, confirmations and results aloud.
- **Barge-in:** if the student starts speaking (or presses a key) while the assistant is talking, the assistant stops at once.
- **Stop speech** at any time with Alt+S.
- The assistant's audio is tied to each response, so a late audio fragment from something already interrupted is ignored.

### 8.7 How the system understands speech

| Step | What is used |
|---|---|
| Speech to text | A speech-recognition provider (Deepgram is supported). Can be switched off. |
| Text to speech | A speech-synthesis provider (Deepgram is supported). Can be switched off. |
| Understanding commands | First, **fixed rules** match exact commands entirely on the institution's own server. Only speech the rules cannot place is sent to an optional AI model (Groq, Anthropic or a local Ollama), or the institution can stay on rules only (the default, fully offline). |

What the AI model is given, when used: the transcript, the question type, the option keys and texts as the student sees them, and whether an answer is waiting. **Never** the answer key, grading data or anything about the student. The model may only reply with one intent from a closed list; anything else is refused, and the system double-checks the intent against the current question before acting.

If the AI provider is down, rate-limited, or misconfigured, the exam is not affected: the student is told the AI helper is unavailable, and exact commands keep working.

### 8.8 Multilingual-ready
Length limits count characters, not bytes, so answers in languages such as Hindi or Arabic are not unfairly restricted compared with English.

### 8.9 Keyboard control  ✅

Keys apply when no button, field or dialog has focus, unless noted.

| Key | Action |
|---|---|
| **Enter** | Start a recording (dictation on a written question; a command on multiple choice). Press again to stop and send. **Never confirms an answer.** |
| **Escape** | Discard the recording in progress (not while a dialog is open) |
| **← / →** | Previous / next question. On a radio group or text field they keep their normal meaning. Refused, with a reason, while recording or while an answer is waiting. |
| **Alt+M** | Start/stop a voice command (works from fields too; not inside dialogs) |
| **Alt+D** | Start/stop dictation |
| **Alt+S** | Stop speech |
| **Alt+N / Alt+P** | Next / previous question (not while typing) |

Alt shortcuts follow the **physical key**, so they also work on a Mac with Option. Whenever the room refuses to start a recording (answer waiting, still processing, not connected), it says why.

### 8.10 Connection handling  ✅
- One voice connection per exam session; opening a new one cleanly replaces the old one.
- Reconnecting is simply connecting again; the server re-reads the authoritative state.
- A staged-but-unconfirmed answer does not survive a reconnect — nothing was saved, so nothing is lost or half-saved.
- When the exam ends (submitted or expired), the voice connection is closed at once and any recording in progress is discarded.
- A request with no reply gives up after 30 seconds so navigation never hangs.

---

## 9. Protecting every answer  ✅

A set of protections built after careful testing of "what could go wrong" in the exam room.

| Protection | What it does for the student |
|---|---|
| **Serialized saving** | Saves are sent one at a time, each carrying only the newest value, so rapid changes end on the last one |
| **Auto-save for typing** | Text saves 800 ms after the last keystroke |
| **Per-question holding** | A failed save is kept for *its own* question; moving elsewhere never loses it |
| **Stale-save refusal** | Each save names the version it replaces. If the answer changed meanwhile, the save is refused and the student chooses which version to keep |
| **Unload warning** | Leaving the page with unsaved work asks first |
| **Submit awareness** | Submitting waits for pending saves and warns about unfinished voice work |
| **Single expiry** | Running out of time is handled once, saving typed text first |
| **Instant end of voice** | On submit/expiry, voice stops immediately and nothing from an unfinished recording is kept |
| **Large answers** | Written answers up to 50,000 characters |
| **Exact-once confirmation** | A repeated confirmation never writes twice |

---

## 10. Answer audio and answer history

### 10.1 Answer history  ✅
Every answer keeps a **full history of every value it has held**, oldest first. For each value the teacher can see:
- **How it got there:** chosen, typed, dictated, or dictated-then-corrected.
- **When** it happened.
- **What the recogniser heard**, for dictated values.
- Whether the student corrected it.
- Which recordings (if any) support it.

In the review screen this is a *"Show how this answer changed"* view. A recording that the student later typed over is labelled *"Confirmed earlier, no longer the saved answer"*, so a teacher is never misled into thinking old audio supports the final answer.

### 10.2 Recording of dictated answers  🟡 off by default

Keeping a student's voice is a significant institutional decision (notice, consent, retention), so **nothing is recorded unless switched on.**

**What is kept**

| Kept | Not kept |
|---|---|
| Audio of an utterance in *dictation mode* on a *written-answer* question, once a transcript exists | Voice commands, silence, multiple-choice utterances, interrupted utterances |

- Audio is stored in secure object storage (a local folder, or an S3-compatible service such as MinIO/AWS S3), under random keys that carry no student, exam or question information. Audio is never stored in the main database.
- The **transcript** of what the recogniser heard is written once and never edited.
- **An answer never depends on its audio.** If audio can't be stored, the student is told and can still confirm the answer.
- Audio is never recorded for a session that has already ended.
- The student sees a notice before dictating when recording is on.

### 10.3 Who can listen  ✅
- Only the **exam's owner** and **administrators**. Students and proctors cannot.
- Only for the attempt the review shows, for a student still assigned to that exam.
- Playback happens **only when the reviewer presses Play** — pages never pre-load audio.
- Audio is never cached, never given a permanent or shareable link, and has no identifying file name. The browser fetches it with the reviewer's credentials and plays it from memory.
- Playback supports seeking.
- Limited to **60 plays per minute** per reviewer.
- Every listening is written to the audit log (see [§13](#13-trust-privacy-and-audit)).

### 10.4 Retention and clean-up  ✅ (opt-in)
A background clean-up job (off unless enabled) removes audio that has no reason to exist:

| What is removed | When |
|---|---|
| **Expired audio** | When a recording's retention date has passed. Audited first. |
| **Never-confirmed dictations** | After a grace period (default 1 hour): if the student said no, re-dictated or left, that audio supports no answer and isn't covered by the student's notice |
| **Orphaned objects** (optional) | Stored files no database record points to, only if explicitly enabled and old enough |

Important behaviours:
- A recording with **no retention date is never touched.**
- When audio expires, the **transcript, the answer and its history are kept**; only the audio goes. The review then shows *"Audio removed"* with the transcript, and playback explains that the audio was removed.
- A recording is **never deleted if it supports a confirmed answer** under the "never confirmed" rule.
- If the audit entry can't be written, the audio is kept and retried next time.
- If audio goes missing from storage, it is flagged loudly for a human but **never silently altered**.
- With several servers running, only one runs the clean-up at a time.

### 10.5 Not yet done (audio)  📋
- Encryption at rest (to be provided by the storage service).
- A built-in retention policy tool, legal-hold or deletion-on-request.
- A consent step (students currently see a notice, but are not asked to agree).
- Teacher annotations on recordings.
- Longer dictation (currently limited to about 45 seconds per spoken stream).

---

## 11. Reviewing and marking responses

### 11.1 Roster  ✅
For each exam, the teacher sees a **roster of assigned students** with status counts (not started, in progress, submitted, expired and so on).

### 11.2 Response review  ✅
Opening a student shows their paper, **question by question, strictly read-only**:
- The question and the student's answer.
- Which attempt is shown (the submitted one, otherwise the latest).
- For dictated answers: every kept recording, what the recogniser heard, and whether each recording still supports the current answer.
- Audited audio playback with an accessible player.
- Answer history.
- **Keyboard navigation** between questions (← / →), with focus moving to the question heading for screen-reader users.
- Clear error panels if something can't load.

### 11.3 Marking  ✅

Marking is **fully manual**. Nothing asks an AI to mark an answer, and nothing automatically marks essays or spoken answers.

**Who and when**
- Only the exam owner or an administrator.
- Only once the student's session has **ended** (submitted or expired). While it's in progress, there's nothing to mark, and the answer key is not shown.

**Marks**
- From 0 up to the question's points, in **half-mark steps** (0.5). Invalid values like 7.25 are refused, not rounded.
- **Multiple choice** is marked **automatically** when a correct option is stored: full marks for correct, 0 otherwise; unanswered is 0.
- Essays and spoken answers need the teacher's mark.
- A teacher can **override** any mark. Clearing it brings the automatic mark back.
- An optional **feedback comment** per question (up to 2,000 characters).
- **Totals are computed by the server**, so what the browser shows is always what is stored.

**Draft and final**
- A paper is a **draft** until the teacher **finalises** it. Finalising requires every question to have a counting mark.
- Finalising **freezes a snapshot**. Later edits to the exam (for example changing a question's points or correct option) cannot change a finalised result.
- A final paper can be **reopened** with a written reason (3 to 500 characters). It returns to draft, and the earlier final result plus the reason stay in the history.

**Safety under simultaneous use**
- Two teachers (or two tabs) can't overwrite each other silently: each mark carries a version, and a stale save is refused with a clear message.
- A save made against an attempt that is no longer the one shown is refused (for example, if the student retook the exam).
- Repeating a save, finalise or reopen changes nothing the second time, so a retry after a lost reply is safe.
- Every save, finalise and reopen is audited in the same step. If the audit entry fails, the change is not made. The audit entry holds IDs, marks and totals only — never comments or answers.

### 11.4 Not built yet (marking)  📋
- Students viewing their own marks and feedback.
- Exporting results (for example to a spreadsheet).
- Marking status and totals in the roster view.
- AI-assisted marking suggestions (see [§17.8](#178-ideas)).

---

## 12. Importing an exam from a PDF

> 🟡 **Off by default.** Requires an institution to enable it and choose an extraction provider. 🔧 Backend and the main screens are done; real-world testing and polish are ongoing.

Typing a long paper into the question studio is slow. A teacher can upload an existing exam paper instead.

### 12.1 The teacher's steps
1. **Upload** the PDF from the exam page. The system checks the file immediately (see below) and queues it.
2. **Watch progress** in a status panel: queued → reading pages → extracting → validating → saving.
3. When ready, **open the draft review screen** to see the proposed questions beside their warnings.
4. **Correct** wording, options, marks and order; add or delete questions.
5. **Acknowledge warnings** and confirm any diagram descriptions.
6. **Approve.** The reviewed questions are copied into the (draft) exam in one all-or-nothing step.
7. Continue as normal: publish, assign.

### 12.2 What the system reads
- **Typed text**, kept in reading order, with two-column pages read left then right. Running headers, footers and page numbers are set aside (recorded, not discarded).
- **Scanned pages and figures**, when the PDF stores them as ordinary pictures, are read by a vision model (if one is configured).
- **Tables** are kept as text.
- **Diagrams** are described by the model from the picture and its caption.

### 12.3 What the system checks
- Wording is compared with the page's own text. Anything not found is flagged as **unverified** and its confidence lowered. It is flagged, never deleted.
- Marks the model reports but the page doesn't show are dropped.
- Text read from a picture is flagged as such and is never given top confidence.
- **The system never invents a correct answer.** The extraction has no field for an answer key; only a teacher can mark an option correct.

### 12.4 What a teacher must do before approval
Approval is blocked, with every problem listed together, unless:
- Every question has marks.
- Every multiple-choice question has at least two options.
- Option labels are valid and unique, with at most one correct option.
- Every diagram has a description that the teacher has **confirmed**.
- Every blocking warning has been acknowledged.

**Why diagrams need confirmation:** the exam room reads the question aloud. A diagram a blind student cannot see would otherwise be silently missing, so the confirmed description is built into the question text.

### 12.5 States of a document

```text
QUEUED ──▶ PROCESSING ──▶ REVIEW_REQUIRED ──▶ COMPLETED
              │                  │
              ├─▶ FAILED         └─ retry (discarding the draft) ─▶ QUEUED
              └─▶ CANCELLED
```
- **Cancel** stops processing; **retry** runs again as a fresh attempt; **delete** removes the record and file (already-approved questions stay).
- If extraction fails, the teacher sees a plain-language reason written by this product, never the provider's raw message (which could echo the paper).
- The original PDF can be downloaded again by the exam owner.

### 12.6 Safety of the upload
- Size, PDF signature, structure, encryption and page count are checked **before anything is stored** (default limits: 20 MiB, 60 pages).
- Encrypted PDFs are refused.
- Files are stored as private objects under random names; the client's file name is only a label.
- Uploads are rate-limited per user.
- Text inside a paper that says "ignore the above" does nothing; the paper's content is treated as data, never as instructions.

### 12.7 Edits that don't collide
Every draft edit names the version it was based on. If a teacher has the draft open in two places, a stale edit is refused rather than silently overwriting newer work. Review edits are saved one at a time, and the screen warns before leaving with unsaved changes.

### 12.8 Reliability
- Extraction runs in the background on its own worker pool, so a backlog of papers can never delay exam-critical work.
- A crashed worker's document is picked up by another; two workers never process one document at once.
- Temporary failures (rate limits, timeouts) retry automatically with backoff; permanent ones fail clearly.
- A retry replaces the draft rather than adding to it, so duplicates are impossible.

### 12.9 Privacy note
When the Groq provider is used, **the text (and, for scanned pages, images) of uploaded papers is sent to Groq.** With the provider disabled (the default), uploads are refused and nothing leaves the institution's server. Institutions should check the provider's data terms before sending unreleased papers.

### 12.10 Limits today
- Scans are read only if the PDF stores them as ordinary JPEG/PNG pictures. Some scan formats and vector-drawn diagrams are reported as "not read". Handwriting is not supported.
- Maths set in symbol fonts can come out garbled; it's flagged rather than "fixed".
- Unusual page layouts may interleave columns. Tables are kept as text, not structured.
- Free-tier provider rate limits can slow large papers.
- Only Groq is implemented, though the design allows other providers.
- If the server dies at exactly the wrong moment, an orphaned file may remain; there is no sweeper for it yet.
- A teacher-confirmed diagram description is a starting point; richer storage of tables and figures is planned with STEM accessibility.

---

## 13. Trust, privacy and audit

### 13.1 Audit log  ✅ (foundation) / 🔧 (full exam-interaction trail planned)
An append-only record of important actions. Currently covers:
- Every **audio playback** (and every playback attempt that failed because audio was unreadable).
- Every **mark save, finalise and reopen**.
- **Audio removal** by clean-up, with the reason.
- Denied review access.

Key properties:
- An entry records **who, in what role, when, which item, request ID and client address**, but never audio, transcripts, comments or answers.
- **Fail closed:** if the audit entry can't be written, the sensitive action does not happen. (No audio plays; no mark is changed.)
- Audit rows for marking are written in the **same transaction** as the change, so they can't disagree.

The wider vision — an immutable trail of every step of an exam (question read, navigation, answer transcribed/confirmed/saved, speech interrupted) — is part of the original plan and is not complete yet. See [§17.7](#177-fuller-audit-trail).

### 13.2 Privacy defaults
- Audio recording: **off**.
- PDF import: **off**; nothing is sent anywhere until a provider and key are set.
- AI intent model: **off** (rules only, fully offline).
- Speech services: **off** until configured.
- Exam answer keys are **never** sent to a student's browser or to AI providers.
- Logs carry IDs, counts and codes, never exam text, answers, transcripts or credentials.

### 13.3 Authorization
Checked on the server for every request, not just hidden in the interface. Students cannot reach teacher functions; teachers cannot reach each other's exams; administrators have universal reach.

### 13.4 Protection against abuse
Rate limits on sign-in, voice connections, uploads and audio playback. Origin checks on the voice connection. Limits on message and audio sizes.

---

## 14. Settings an institution controls

Everything sensitive is a **switch the institution flips deliberately**.

| Area | Setting | Default |
|---|---|---|
| Voice | Speech recognition provider | Off |
| Voice | Speech output provider | Off |
| Voice | Command understanding | Rules only (offline); optional Groq / Anthropic / Ollama |
| Voice | Minimum confidence for AI interpretation | Configurable |
| Voice | Maximum transcript length | 2,000 characters |
| Recording | Keep dictated-answer audio | **Off** |
| Recording | Storage | Local folder, or S3-compatible (MinIO, AWS S3) |
| Recording | Retention days | 0 (no automatic expiry) |
| Recording | Clean-up job | Off (strongly advised on whenever recording is on) |
| Recording | Delete orphaned audio | Off |
| PDF import | Enabled | **Off** |
| PDF import | Extraction provider | Disabled (Groq supported) |
| PDF import | Max file size / pages | 20 MiB / 60 |
| PDF import | Parallel jobs | 2 |
| PDF import | Per-job timeout | 15 minutes |
| Production | Encrypted storage connection required | Enforced |

In production mode the server refuses to start with unsafe combinations (for example, unencrypted audio storage), rather than quietly running insecurely.

---

## 15. How the product was built so far

The work has been done in sprints. Each was completed and tested before the next began.

| Stage | What it delivered |
|---|---|
| **Sprint 1 — Foundation** | Stable backend base: database, cache, logging, consistent error handling |
| **Sprint 2 — Sign-in & exam state machine** | Secure sign-in with renewal and revocation; the exam-session state machine and safe locks |
| **Roll numbers** | Unique student roll numbers and fast search |
| **Sprint 3 — Exam management** | The full exam lifecycle, ownership rules, search and soft delete, with front-end screens |
| **Sprint 4 — Question management** | Questions, options, ordering, correct option; the question studio |
| **Sprint 5 — Assignments** | Assign by student or roll number, bulk assign, revoke; student dashboard |
| **Sprint 6 — Sessions & voice** | Timed sessions, current question, resume; the voice connection, speech recognition, intents, confirmation, speech output |
| **Answer audio** | Optional dictated-answer audio, immutable transcripts, answer history, retention |
| **Response review** | Read-only teacher review, audited playback, answer history view |
| **Teacher marking** | Half-mark scoring, feedback, automatic MCQ marks, finalise/reopen |
| **Hardening** | No lost answers, stale-save protection, instant end of voice, consistent keyboard rules, command-phrase safeguard |
| **Sprint 12 — PDF import** | Backend extraction pipeline and teacher screens for upload, status, review and approval |

Quality approach: features are developed one at a time, with automated tests (including tests against a real database and real storage), and the interactive API documentation is generated from the code so it never drifts out of date.

---

## 16. Known limits today

An honest list of what the product does **not** do yet.

- **Timer pause for disconnections** — time still runs while a student is offline. (Designed; see §17.2.)
- **STEM content** — maths and graphs are not yet given special accessible treatment; diagrams rely on confirmed text descriptions.
- **Live monitoring** — no proctor dashboard.
- **Students can't see their marks** — marking is visible to teachers only.
- **No result export.**
- **Single speech provider** — Deepgram for both speech recognition and speech output.
- **Single extraction provider** — Groq for PDF import.
- **Audio retention tooling** is a mechanism, not a full policy suite (no legal hold, no deletion-on-request, no consent tick-box).
- **Audit trail** covers sensitive actions, not yet every step of every exam.
- **Not yet production-deployed** — hardening and deployment work is still ahead.

---

## 17. What is coming next

### 17.1 Finish and polish PDF import  🔧
- Test with a wider variety of real exam papers (scans, tables, two-column layouts).
- Improve how warnings are explained to teachers.
- Keep the project documentation in step with the screens that now exist.

### 17.2 Fair timer: pause and resume for disconnections  📋
**Why:** Assistive technology can crash, and Wi-Fi can drop. Today a 15-minute outage in a 60-minute exam costs the student 15 minutes of working time. That is unfair to the people this product exists to help.

**The design (already written up):**
- The exam clock counts only **active connected time**, measured by a regular heartbeat from the exam room.
- If the heartbeat stops, the clock **pauses**.
- To prevent misuse (disconnecting on purpose to look things up), there are guardrails:
  - A **maximum offline allowance** per exam (suggested 15 minutes in total). Exceed it and the session expires.
  - An **absolute cut-off window**, so an exam cannot drag on indefinitely.
  - **Every disconnect and reconnect is recorded** and visible to the teacher.
- **Accessible announcements:** a loud, clear message when the connection drops ("Connection interrupted · timer paused, reconnect within …") and when it returns ("Connection restored; your timer has resumed").
- **Teacher visibility:** an "Integrity & Connectivity" view per candidate — connected, paused (and for how long), or completed with N disconnects.

### 17.3 STEM accessibility (Sprint 13)  📋
Maths and graphs are the hardest content for blind students. Planned:
- **Graph sonification:** turn a graph into sound, with time along the x-axis and pitch representing the y-value, so a student can *hear* the shape of a curve.
- **Maths transcription:** equations stored in a precise notation (LaTeX) and read out in clear, natural spoken form.
- **Accessible verbal descriptions** of figures and diagrams.
- **Richer storage of tables and figures** than the current "text inside the question".

### 17.4 Proctoring and monitoring (Sprint 14)  📋
An administrative monitoring foundation:
- See **active sessions**, connection status and session health.
- See **exam progress** (for example, how far through the paper a student is) without exposing answers unnecessarily.
- **System alerts** and flags for suspicious *technical* events.
- Administrative dashboards and operational metrics.

Design constraint: monitoring must not reveal exam answers beyond what is truly needed.

### 17.5 Production hardening (Sprint 15)  📋
- **Security:** review of sign-in and permissions; rate limits; input checks; secure headers; secrets handling; token security; dependency scanning; audit review.
- **Reliability:** timeouts, retries, circuit breakers, graceful shutdown, connection and resource limits; clean behaviour when an external provider, the cache or the database fails.
- **Observability:** structured logging, request IDs, metrics, tracing, latency and error rates, plus voice and audio pipeline health.
- **Testing at scale:** end-to-end, load, failure and concurrency tests.

### 17.6 Deployment and scale (Sprint 16)  📋
- Production container images and automated build/deploy pipelines.
- Cloud deployment with encrypted connections.
- Production database and cache configuration.
- Monitoring, alerting, **backups** and a **disaster-recovery plan**.
- Load testing and horizontal-scaling validation for about **1,000 users**.

### 17.7 Fuller audit trail  📋
Extend the audit log from "sensitive actions" to a complete, immutable story of each exam:

```text
session created → started → question read → navigation requested/executed
→ answer transcribed → confirmed → saved → speech started/interrupted → session submitted
```

### 17.8 Ideas  💡
Not committed, but fit the product's direction:
- Students viewing their own marks and feedback, with an accessible results page.
- Exporting results to a spreadsheet.
- Marks and marking status in the roster.
- More question types and richer marking tools (for example rubrics).
- Teacher annotations on recordings.
- AI-assisted marking *suggestions* that a teacher must accept (never automatic marks).
- Additional speech and AI providers so institutions aren't tied to one vendor.
- Sign-in through an external identity provider.
- Reports and analytics for teachers and administrators.
- A consent step before recording.

---

## 18. Full roadmap table

| Area | Status |
|---|---|
| Foundation, sign-in, roles | ✅ Built |
| Roll numbers and student search | ✅ Built |
| Exam lifecycle (draft / published / archived) | ✅ Built |
| Question studio (MCQ, essay, voice) | ✅ Built |
| Assigning students (single, bulk, revoke) | ✅ Built |
| Student dashboard ("My exams") | ✅ Built |
| Timed exam room with resume after reconnect | ✅ Built |
| Voice assistant (commands, dictation, confirmation, speech output, barge-in) | ✅ Built |
| Keyboard control and instructions dialog | ✅ Built |
| Answer protection (no lost answers, stale-save handling) | ✅ Built |
| Command-phrase dictation safeguard | ✅ Built |
| Answer history | ✅ Built |
| Answer audio recording, playback, retention clean-up | 🟡 Built, off by default |
| Teacher response review | ✅ Built |
| Teacher marking, finalise, reopen | ✅ Built |
| Audit log for sensitive actions | ✅ Built (foundation) |
| PDF import — backend | 🟡 Built, off by default |
| PDF import — upload, status, review and approval screens | 🔧 Built, being polished |
| Timer pause/resume during disconnections | 📋 Designed, not built |
| STEM accessibility (graphs, maths) | 📋 Planned |
| Proctoring and monitoring | 📋 Planned |
| Complete exam-interaction audit trail | 📋 Planned |
| Production hardening | 📋 Planned |
| Deployment and scale | 📋 Planned |
| Student-visible results, export, analytics | 💡 Idea |

---

## 19. Glossary

| Term | Meaning |
|---|---|
| **Archived** | A finished exam kept as a read-only record |
| **Assignment** | The link that lets a particular student sit a particular exam |
| **Audit log** | A permanent, append-only record of important actions |
| **Barge-in** | Interrupting the assistant by speaking, so it stops talking |
| **Command mode** | Voice mode for giving instructions to the assistant |
| **Confirmation** | The assistant reads back what it understood and waits for a yes before saving |
| **Dictation mode** | Voice mode where everything spoken is the answer |
| **Draft (exam)** | An exam still being prepared; fully editable |
| **Draft (PDF import)** | Questions proposed from a PDF that are not yet part of any exam |
| **Draft (marking)** | A paper being marked that can still change |
| **Finalise** | Lock a marked paper with a snapshot of its marks |
| **Intent** | What the student meant to do, for example "go to the next question" |
| **Orphaned audio** | Stored audio that no database record points to |
| **Published** | An exam that is live for assigned students |
| **Reopen** | Return a finalised paper to draft, with a recorded reason |
| **Retention** | How long recorded audio is kept before removal |
| **Scribe** | A person who writes down a student's spoken answers; this product's assistant plays that role |
| **Session** | One student's attempt at one exam |
| **Sonification** | Representing data such as a graph as sound |
| **STT / TTS** | Speech-to-text (listening) and text-to-speech (speaking) |
| **Transcript** | The text the speech recogniser produced from audio |
