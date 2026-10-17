"use client";

import { AlertTriangle, Check, CheckCircle2, Loader2, Lock } from "lucide-react";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Textarea } from "@/components/ui/textarea";
import { PaperActions } from "@/components/response-review/paper-actions";
import {
  MAX_COMMENT_LENGTH,
  draftProblems,
  formatMarks,
  type StudentMarking,
} from "@/hooks/use-student-marking";
import type { MarkingQuestion } from "@/types/marking";

/** "7.5 of 10" for a question's mark, or "Not marked yet". */
export function describeMark(q: MarkingQuestion): string {
  return q.effectiveMarks === null
    ? "Not marked yet"
    : `${formatMarks(q.effectiveMarks)} of ${formatMarks(q.maxMarks)}`;
}

function autoNote(q: MarkingQuestion): string | null {
  if (q.suggestedMarks === null) return null;
  if (!q.answered) return `Not answered: ${formatMarks(q.suggestedMarks)} automatically`;
  return `Auto-marked: ${formatMarks(q.suggestedMarks)} of ${formatMarks(q.maxMarks)}`;
}

/**
 * Where a teacher gives marks and feedback for one answer. A finalised paper
 * shows them read-only. Totals are never worked out here.
 */
export function MarkingPanel({
  question,
  marking,
  final,
}: {
  question: MarkingQuestion;
  marking: StudentMarking;
  final: boolean;
}) {
  const id = question.questionId;
  const heading = `marking-heading-${id}`;

  if (final) {
    return (
      <section aria-labelledby={heading} className="rounded-lg border bg-muted/30 p-4">
        <h3 id={heading} className="mb-2 flex items-center gap-2 text-sm font-semibold">
          <Lock className="size-3.5" aria-hidden="true" />
          Marks
        </h3>
        <p className="text-base font-semibold tabular-nums" data-testid="final-marks">
          {describeMark(question)}
        </p>
        {question.markSource === "AUTO" && (
          <p className="text-xs text-muted-foreground">Marked automatically</p>
        )}
        {question.comment && (
          <p className="mt-2 whitespace-pre-wrap break-words text-sm">{question.comment}</p>
        )}
      </section>
    );
  }

  const draft = marking.draftFor(question);
  const dirty = marking.isDirty(question);
  const status = marking.statusOf(id);
  const problems = draftProblems(question, draft);
  // Wrong input is explained as it is typed, but a clean field says nothing.
  const marksError = dirty ? problems.marks : undefined;
  const commentError = dirty ? problems.comment : undefined;
  const auto = autoNote(question);
  const autoText = question.suggestedMarks === null ? null : formatMarks(question.suggestedMarks);
  const saving = status.state === "saving";
  const marksId = `marks-${id}`;
  const commentId = `comment-${id}`;
  const marksNote = `marks-note-${id}`;
  const remaining = MAX_COMMENT_LENGTH - draft.comment.length;

  const save = () => {
    void marking.save(id);
  };

  return (
    <section
      aria-labelledby={heading}
      className="flex flex-col gap-4 rounded-lg border border-primary/30 p-4"
    >
      <div className="flex flex-wrap items-center justify-between gap-2">
        <h3 id={heading} className="text-sm font-semibold">
          Marking
        </h3>
        <span className="text-sm text-muted-foreground tabular-nums" data-testid="current-mark">
          Counts: {describeMark(question)}
        </span>
      </div>

      {question.exceedsMax && (
        <p
          role="alert"
          className="flex items-start gap-2 rounded-md border border-amber-300 bg-amber-50 p-3 text-sm text-amber-950 dark:border-amber-800 dark:bg-amber-950/40 dark:text-amber-100"
        >
          <AlertTriangle className="mt-0.5 size-4 shrink-0" aria-hidden="true" />
          <span>
            This question is now worth {formatMarks(question.maxMarks)}, less than the mark saved
            before. Enter a new mark.
          </span>
        </p>
      )}

      <div className="flex flex-col gap-1.5">
        <Label htmlFor={marksId}>Marks (out of {formatMarks(question.maxMarks)})</Label>
        <div className="flex flex-wrap items-center gap-2">
          <Input
            id={marksId}
            value={draft.marks}
            inputMode="decimal"
            autoComplete="off"
            className="w-28 tabular-nums"
            aria-invalid={marksError ? true : undefined}
            aria-describedby={marksError ? `${marksId}-error ${marksNote}` : marksNote}
            onChange={(e) => marking.setMarks(question, e.target.value)}
            onKeyDown={(e) => {
              if (e.key === "Enter") {
                e.preventDefault();
                save();
              }
            }}
          />
          {autoText !== null && draft.marks.trim() !== autoText && (
            <Button
              type="button"
              variant="outline"
              size="sm"
              onClick={() => marking.applyAutoMark(question)}
            >
              Use auto mark ({autoText})
            </Button>
          )}
        </div>
        <p id={marksNote} className="text-xs text-muted-foreground">
          Whole numbers or halves, such as 7 or 7.5.
          {auto && <> {auto}.</>}
          {question.type === "MCQ" && question.suggestedMarks === null && (
            <> No correct option is stored, so this needs your mark.</>
          )}
        </p>
        {marksError && (
          <p id={`${marksId}-error`} role="alert" className="text-sm text-destructive">
            {marksError}
          </p>
        )}
      </div>

      <div className="flex flex-col gap-1.5">
        <Label htmlFor={commentId}>Feedback for the student (optional)</Label>
        <Textarea
          id={commentId}
          value={draft.comment}
          rows={3}
          aria-invalid={commentError ? true : undefined}
          aria-describedby={commentError ? `${commentId}-error` : undefined}
          onChange={(e) => marking.setComment(question, e.target.value)}
        />
        <p
          className={`text-xs tabular-nums ${remaining < 0 ? "text-destructive" : "text-muted-foreground"}`}
        >
          {remaining < 0
            ? `${-remaining} characters over the limit`
            : `${remaining} characters left`}
        </p>
        {commentError && (
          <p id={`${commentId}-error`} role="alert" className="text-sm text-destructive">
            {commentError}
          </p>
        )}
      </div>

      <div className="flex flex-wrap items-center gap-3">
        <Button type="button" onClick={save} disabled={saving || !dirty}>
          {saving && <Loader2 className="size-4 motion-safe:animate-spin" aria-hidden="true" />}
          Save marks
        </Button>
        <SaveStatus status={status} dirty={dirty} />
        {status.state === "conflict" && (
          <Button
            type="button"
            variant="outline"
            size="sm"
            onClick={() => void marking.loadLatest(id)}
          >
            Load latest
          </Button>
        )}
      </div>
    </section>
  );
}

function SaveStatus({
  status,
  dirty,
}: {
  status: ReturnType<StudentMarking["statusOf"]>;
  dirty: boolean;
}) {
  // One polite region, mirrored for screen readers; failures also alert.
  let text = "";
  let tone = "text-muted-foreground";
  let icon = null;
  if (status.state === "saving") {
    text = "Saving…";
  } else if (status.state === "saved") {
    text = "Saved";
    tone = "text-emerald-700 dark:text-emerald-300";
    icon = <CheckCircle2 className="size-4" aria-hidden="true" />;
  } else if (status.state === "conflict" || status.state === "error") {
    text = status.state === "conflict" ? "Not saved: changed elsewhere" : "Not saved";
    tone = "text-destructive";
    icon = <AlertTriangle className="size-4" aria-hidden="true" />;
  } else if (dirty) {
    text = "Unsaved changes";
    tone = "text-amber-800 dark:text-amber-200";
  }

  return (
    <>
      <span
        role="status"
        aria-live="polite"
        className={`inline-flex items-center gap-1 text-sm ${tone}`}
      >
        {icon}
        {text}
      </span>
      {(status.state === "conflict" || status.state === "error") && status.message && (
        <span role="alert" className="text-sm text-destructive">
          {status.message}
        </span>
      )}
    </>
  );
}

/** Marks given so far, from the server's totals. */
export function MarksSummary({ marking }: { marking: StudentMarking }) {
  const sheet = marking.sheet;
  if (!sheet || !sheet.markingAllowed) return null;
  const { paper } = sheet;
  const final = paper.status === "FINAL";
  return (
    <div className="flex flex-wrap items-center gap-x-6 gap-y-3 rounded-xl border bg-card p-4">
      <dl className="flex flex-wrap items-center gap-x-6 gap-y-2" aria-label="Paper marks">
        <div>
          <dt className="text-xs font-semibold uppercase tracking-wide text-muted-foreground">
            Marks
          </dt>
          <dd className="text-2xl font-bold tabular-nums" data-testid="paper-total">
            {formatMarks(paper.totalMarks)}
            <span className="text-base font-normal text-muted-foreground">
              {" "}
              / {formatMarks(paper.maxMarks)}
            </span>
          </dd>
        </div>
        <div>
          <dt className="text-xs font-semibold uppercase tracking-wide text-muted-foreground">
            Marked
          </dt>
          <dd className="text-lg font-semibold tabular-nums">
            {paper.markedCount} of {paper.totalQuestions}
          </dd>
        </div>
        <div>
          <dt className="sr-only">Status</dt>
          <dd>
            {final ? (
              <Badge variant="default" className="gap-1">
                <Check aria-hidden="true" />
                Final
              </Badge>
            ) : (
              <Badge variant="secondary">Draft</Badge>
            )}
          </dd>
        </div>
      </dl>
      <PaperActions marking={marking} />
    </div>
  );
}
