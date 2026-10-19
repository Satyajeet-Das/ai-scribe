"use client";

import { useCallback, useState } from "react";
import Link from "next/link";
import { ArrowLeft, Check, ChevronLeft, ChevronRight, Info, RefreshCw } from "lucide-react";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Card } from "@/components/ui/card";
import { Skeleton } from "@/components/ui/skeleton";
import { AnswerHistory } from "@/components/response-review/answer-history";
import { AnswerRecordings, SOURCE_LABEL } from "@/components/response-review/answer-recordings";
import { ErrorPanel } from "@/components/response-review/error-panel";
import {
  MarkingPanel,
  MarksSummary,
  describeMark,
} from "@/components/response-review/marking-panel";
import { ReviewStatusBadge, formatDateTime } from "@/components/response-review/status-badge";
import { useRemoteResource } from "@/hooks/use-remote-resource";
import { formatMarks, useStudentMarking, type StudentMarking } from "@/hooks/use-student-marking";
import { responseReviewApi } from "@/services/response-review-api";
import { DESIGN_TOKENS } from "@/lib/constants";
import type { MarkingQuestion } from "@/types/marking";
import type { ReviewCounts, ReviewQuestion, ReviewStudentResponses } from "@/types/response-review";

export function StudentResponseReview({
  examId,
  studentId,
}: {
  examId: string;
  studentId: string;
}) {
  const key = `${examId}|${studentId}`;
  const load = useCallback(
    (signal: AbortSignal) => responseReviewApi.getStudentResponses(examId, studentId, signal),
    [examId, studentId]
  );
  const { data, error, loading, refreshing, reload } = useRemoteResource(key, load);
  const marking = useStudentMarking(examId, studentId);
  const refresh = () => {
    reload();
    marking.reload();
  };

  // The selected question belongs to one student; switching students (or exams)
  // therefore starts again at the first question. Refreshing keeps the place.
  const [selection, setSelection] = useState<{ key: string; questionId: string } | null>(null);
  const selectedId = selection && selection.key === key ? selection.questionId : undefined;
  const select = (questionId: string) => setSelection({ key, questionId });

  const backHref = `/exams/${examId}/responses`;

  return (
    <div className="flex flex-col gap-6">
      <div>
        <Link
          href={backHref}
          className="mb-4 inline-flex items-center gap-1.5 text-sm text-muted-foreground transition-colors hover:text-foreground"
        >
          <ArrowLeft className="size-4" aria-hidden="true" />
          All students
        </Link>
        <Header data={data} loading={loading} refreshing={refreshing} onRefresh={refresh} />
      </div>

      {error ? (
        <ErrorPanel error={error} onRetry={reload} />
      ) : !data ? (
        <ReviewSkeleton />
      ) : !data.responseAvailable ? (
        <Unavailable data={data} backHref={backHref} />
      ) : (
        <>
          {data.session && !data.session.isFinal && (
            <div
              role="status"
              className="flex items-start gap-3 rounded-xl border border-amber-300 bg-amber-50 p-4 text-sm text-amber-950 dark:border-amber-800 dark:bg-amber-950/40 dark:text-amber-100"
            >
              <Info className="mt-0.5 size-4 shrink-0" aria-hidden="true" />
              <p>
                <strong>This exam is still in progress.</strong> You are seeing the answers saved so
                far. They are not final and may change.
              </p>
            </div>
          )}
          {data.counts && <CountsBar counts={data.counts} />}
          <MarksSummary marking={marking} />
          {marking.error && !marking.sheet && (
            <div
              role="alert"
              className="flex flex-wrap items-center gap-3 rounded-xl border border-destructive/40 p-4 text-sm"
            >
              <span>Marking could not be loaded. {marking.error.message}</span>
              <Button variant="outline" size="sm" onClick={marking.reload}>
                Try again
              </Button>
            </div>
          )}
          <QuestionReview
            examId={examId}
            studentId={studentId}
            questions={data.questions}
            selectedId={selectedId}
            onSelect={select}
            stale={loading}
            marking={marking}
          />
        </>
      )}
    </div>
  );
}

function Header({
  data,
  loading,
  refreshing,
  onRefresh,
}: {
  data: ReviewStudentResponses | undefined;
  loading: boolean;
  refreshing: boolean;
  onRefresh: () => void;
}) {
  const submitted = formatDateTime(data?.session?.submittedAt);
  const started = formatDateTime(data?.session?.startedAt);
  return (
    <div className="flex flex-wrap items-start justify-between gap-3">
      <div className="min-w-0">
        {data ? (
          <>
            <div className="flex flex-wrap items-center gap-3">
              <h1 className={DESIGN_TOKENS.typography.h2}>{data.student.name || "Student"}</h1>
              <ReviewStatusBadge status={data.status} />
            </div>
            <p className="mt-1 text-sm text-muted-foreground">
              Roll no. {data.student.rollNo || "—"}
              {" · "}
              <span className="font-semibold text-foreground">{data.exam.title}</span>
            </p>
            {(started || submitted) && (
              <p className="mt-0.5 text-sm text-muted-foreground">
                {started && <>Started {started}</>}
                {started && submitted && " · "}
                {submitted && <>Submitted {submitted}</>}
              </p>
            )}
          </>
        ) : (
          <div className="space-y-2">
            <Skeleton className="h-8 w-64" />
            <Skeleton className="h-4 w-80" />
          </div>
        )}
      </div>
      <Button
        variant="outline"
        size="sm"
        onClick={onRefresh}
        disabled={loading}
        className="gap-1.5"
      >
        <RefreshCw
          className={`size-3.5 ${refreshing ? "motion-safe:animate-spin" : ""}`}
          aria-hidden="true"
        />
        Refresh
      </Button>
    </div>
  );
}

function CountsBar({ counts }: { counts: ReviewCounts }) {
  const items = [
    { label: "Questions", value: counts.total },
    { label: "Answered", value: counts.answered },
    { label: "Not answered", value: counts.unanswered },
  ];
  return (
    <dl className="grid grid-cols-3 gap-3" aria-label="Answer counts">
      {items.map((item) => (
        <Card key={item.label} className="gap-1 p-4">
          <dt className="text-xs font-semibold uppercase tracking-wide text-muted-foreground">
            {item.label}
          </dt>
          <dd className="text-2xl font-bold tabular-nums">{item.value}</dd>
        </Card>
      ))}
    </dl>
  );
}

function Unavailable({ data, backHref }: { data: ReviewStudentResponses; backHref: string }) {
  const message =
    data.unavailableReason === "NOT_STARTED"
      ? `${data.student.name || "This student"} has not started this exam, so there are no responses to review.`
      : data.unavailableReason === "IN_PROGRESS_REVIEW_DISABLED"
        ? "This exam is still in progress, and responses cannot be viewed until the student finishes."
        : "No responses are available for this student.";
  return (
    <Card className="items-center gap-3 px-6 py-12 text-center">
      <Info className="size-10 text-muted-foreground/50" aria-hidden="true" />
      <h2 className="text-lg font-semibold">Responses not available</h2>
      <p className="max-w-md text-sm text-muted-foreground" role="status">
        {message}
      </p>
      <Link
        href={backHref}
        className="inline-flex h-8 items-center rounded-lg border px-3 text-sm font-medium hover:bg-muted"
      >
        Back to all students
      </Link>
    </Card>
  );
}

function ReviewSkeleton() {
  return (
    <div
      className="grid gap-6 lg:grid-cols-[16rem_1fr]"
      role="status"
      aria-label="Loading responses"
    >
      <Skeleton className="h-40 rounded-xl" />
      <div className="space-y-4">
        <Skeleton className="h-8 w-48" />
        <Skeleton className="h-24 w-full rounded-xl" />
        <Skeleton className="h-32 w-full rounded-xl" />
      </div>
    </div>
  );
}

function isAnswered(q: ReviewQuestion): boolean {
  return q.answer.status === "ANSWERED";
}

function QuestionReview({
  examId,
  studentId,
  questions,
  selectedId,
  onSelect,
  stale,
  marking,
}: {
  examId: string;
  studentId: string;
  questions: ReviewQuestion[];
  selectedId: string | undefined;
  onSelect: (id: string) => void;
  stale: boolean;
  marking: StudentMarking;
}) {
  if (questions.length === 0) {
    return (
      <Card className="items-center px-6 py-12 text-center">
        <h2 className="text-lg font-semibold">This exam has no questions</h2>
      </Card>
    );
  }

  const found = questions.findIndex((q) => q.id === selectedId);
  const index = found === -1 ? 0 : found;
  const question = questions[index];
  const prev = questions[index - 1];
  const next = questions[index + 1];
  const lineOf = (id: string) => marking.sheet?.questions.find((m) => m.questionId === id);

  return (
    <div className="grid gap-6 lg:grid-cols-[16rem_1fr] lg:items-start">
      <nav aria-label="Question navigator" className="lg:sticky lg:top-4">
        <Card className="gap-3 p-4">
          <h2 className="text-sm font-semibold">Questions</h2>
          <ol className="grid grid-cols-5 gap-2">
            {questions.map((q) => {
              const current = q.id === question.id;
              const answered = isAnswered(q);
              const line = lineOf(q.id);
              const unsaved = line ? marking.isDirty(line) : false;
              const markNote = !line
                ? ""
                : unsaved
                  ? ", unsaved changes"
                  : line.effectiveMarks === null
                    ? ", not marked yet"
                    : `, marked ${describeMark(line)}`;
              return (
                <li key={q.id}>
                  <button
                    type="button"
                    onClick={() => onSelect(q.id)}
                    aria-current={current ? "true" : undefined}
                    aria-label={`Question ${q.questionNumber}, ${answered ? "answered" : "not answered"}${markNote}`}
                    className={[
                      "relative flex h-9 w-full items-center justify-center rounded-md border text-sm font-semibold tabular-nums",
                      "focus-visible:outline-none focus-visible:ring-[3px] focus-visible:ring-ring/50",
                      current
                        ? "border-primary bg-primary text-primary-foreground"
                        : answered
                          ? "border-emerald-500 bg-emerald-50 text-emerald-900 hover:bg-emerald-100 dark:bg-emerald-950/40 dark:text-emerald-100"
                          : "border-dashed border-muted-foreground/60 bg-background text-muted-foreground hover:bg-muted",
                    ].join(" ")}
                  >
                    {q.questionNumber}
                    {answered && (
                      <Check
                        className="absolute -right-1 -top-1 size-3.5 rounded-full bg-emerald-600 p-0.5 text-white"
                        aria-hidden="true"
                      />
                    )}
                    {line && (
                      <span
                        aria-hidden="true"
                        data-testid={`nav-mark-${q.questionNumber}`}
                        className="absolute -bottom-2 left-1/2 -translate-x-1/2 rounded border bg-background px-1 text-[10px] font-medium leading-4 text-foreground tabular-nums"
                      >
                        {unsaved
                          ? "*"
                          : line.effectiveMarks === null
                            ? "–"
                            : formatMarks(line.effectiveMarks)}
                      </span>
                    )}
                  </button>
                </li>
              );
            })}
          </ol>
          <ul className="space-y-1 text-xs text-muted-foreground" aria-label="Legend">
            <li>Check mark and solid border: answered</li>
            <li>Dashed border: not answered</li>
            <li>Filled: question being viewed</li>
            {marking.sheet?.markingAllowed && (
              <li>Small label: marks given, – not marked yet, * unsaved</li>
            )}
          </ul>
        </Card>
      </nav>

      <section aria-labelledby="question-heading" className={stale ? "opacity-70" : ""}>
        {/* Announced once per question change, not for refreshes or chunks. */}
        <p className="sr-only" role="status" aria-live="polite">
          Question {question.questionNumber} of {questions.length},{" "}
          {isAnswered(question) ? "answered" : "not answered"}.
        </p>
        <QuestionCard
          question={question}
          total={questions.length}
          examId={examId}
          studentId={studentId}
          marking={marking}
          markingLine={lineOf(question.id)}
        />
        <div className="mt-4 flex items-center justify-between gap-3">
          <Button
            variant="outline"
            onClick={() => prev && onSelect(prev.id)}
            disabled={!prev}
            className="gap-1"
          >
            <ChevronLeft className="size-4" aria-hidden="true" />
            Previous
          </Button>
          <span className="text-sm text-muted-foreground tabular-nums">
            {index + 1} / {questions.length}
          </span>
          <Button
            variant="outline"
            onClick={() => next && onSelect(next.id)}
            disabled={!next}
            className="gap-1"
          >
            Next
            <ChevronRight className="size-4" aria-hidden="true" />
          </Button>
        </div>
      </section>
    </div>
  );
}

const TYPE_LABEL: Record<ReviewQuestion["type"], string> = {
  MCQ: "Multiple choice",
  ESSAY: "Written answer",
  VOICE: "Spoken answer",
};

function QuestionCard({
  question,
  total,
  examId,
  studentId,
  marking,
  markingLine,
}: {
  question: ReviewQuestion;
  total: number;
  examId: string;
  studentId: string;
  marking: StudentMarking;
  markingLine: MarkingQuestion | undefined;
}) {
  const answered = isAnswered(question);
  const saved = formatDateTime(question.answer.savedAt);
  const options = question.options ?? [];

  return (
    <Card className="gap-5 p-5 sm:p-6">
      <div className="flex flex-wrap items-center gap-2">
        <h2 id="question-heading" className="text-lg font-bold">
          Question {question.questionNumber}
          <span className="font-normal text-muted-foreground"> of {total}</span>
        </h2>
        <Badge variant="secondary">{TYPE_LABEL[question.type] ?? question.type}</Badge>
        <Badge variant="outline">
          {question.maxMarks} {question.maxMarks === 1 ? "mark" : "marks"}
        </Badge>
      </div>

      <p className="whitespace-pre-wrap break-words text-base leading-relaxed">{question.text}</p>

      {options.length > 0 && (
        <ul className="flex flex-col gap-2" aria-label="Options">
          {options.map((o) => {
            const chosen = o.id === question.answer.selectedOptionId;
            return (
              <li
                key={o.id}
                className={`flex items-start gap-3 rounded-lg border p-3 ${
                  chosen ? "border-primary bg-primary/10" : "border-border"
                }`}
              >
                <span
                  className="flex size-7 shrink-0 items-center justify-center rounded-md border text-sm font-semibold"
                  aria-hidden="true"
                >
                  {o.key}
                </span>
                <span className="min-w-0 flex-1 whitespace-pre-wrap break-words">
                  <span className="sr-only">Option {o.key}: </span>
                  {o.text}
                </span>
                {chosen && (
                  <span className="inline-flex shrink-0 items-center gap-1 text-sm font-semibold text-primary">
                    <Check className="size-4" aria-hidden="true" />
                    Student&apos;s answer
                  </span>
                )}
                {o.isCorrect === true && (
                  <span className="shrink-0 text-sm font-medium text-emerald-700 dark:text-emerald-300">
                    Correct option
                  </span>
                )}
              </li>
            );
          })}
        </ul>
      )}

      {answered ? (
        <div className="rounded-lg border border-primary/40 bg-primary/5 p-4">
          <h3 className="mb-2 text-sm font-semibold">Student&apos;s answer</h3>
          {question.answer.textAnswer ? (
            <p className="whitespace-pre-wrap break-words text-base leading-relaxed">
              {question.answer.textAnswer}
            </p>
          ) : question.type === "MCQ" ? (
            <p>
              {question.answer.selectedOptionKey && (
                <strong>{question.answer.selectedOptionKey}. </strong>
              )}
              {question.answer.selectedOptionText}
            </p>
          ) : null}
          {question.answer.source && SOURCE_LABEL[question.answer.source] && (
            <p className="mt-2 text-sm text-muted-foreground" data-testid="answer-source">
              {SOURCE_LABEL[question.answer.source]}
            </p>
          )}
          {saved && <p className="mt-2 text-xs text-muted-foreground">Last saved {saved}</p>}
        </div>
      ) : (
        <div className="rounded-lg border border-dashed p-4 text-sm text-muted-foreground">
          <p className="font-semibold text-foreground">No answer saved</p>
          <p>The student did not answer this question.</p>
        </div>
      )}

      <AnswerRecordings question={question} examId={examId} studentId={studentId} />

      {question.answer.status === "ANSWERED" && (
        <AnswerHistory
          key={question.id}
          examId={examId}
          studentId={studentId}
          questionId={question.id}
          questionNumber={question.questionNumber}
        />
      )}

      {markingLine && marking.sheet?.markingAllowed && (
        <MarkingPanel
          question={markingLine}
          marking={marking}
          final={marking.sheet.paper.status === "FINAL"}
        />
      )}
    </Card>
  );
}
