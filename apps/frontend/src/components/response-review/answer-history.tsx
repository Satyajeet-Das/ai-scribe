"use client";

import { useEffect, useRef, useState } from "react";
import { ChevronDown, History, Loader2, RotateCcw } from "lucide-react";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { formatDateTime } from "@/components/response-review/status-badge";
import { responseReviewApi } from "@/services/response-review-api";
import type {
  ReviewAnswerSource,
  ReviewRevision,
  ReviewRevisionHistory,
} from "@/types/response-review";

const SOURCE_TEXT: Record<ReviewAnswerSource, string> = {
  OPTION: "Chosen option",
  TYPED: "Typed",
  STT_CONFIRMED: "Dictated and confirmed",
  STUDENT_CORRECTION: "Dictated, then corrected by the student",
};

const UNAVAILABLE_TEXT = {
  NOT_STARTED: "The student has not started this exam.",
  IN_PROGRESS_REVIEW_DISABLED: "The history is shown once the student has finished the exam.",
} as const;

type Load =
  | { phase: "idle" | "loading" | "error" }
  | { phase: "loaded"; history: ReviewRevisionHistory };

/**
 * Every value an answer has held, oldest first, with where each came from and
 * what the speech recognizer heard for a dictated one. It is loaded only when
 * the reviewer opens it.
 */
export function AnswerHistory({
  examId,
  studentId,
  questionId,
  questionNumber,
}: {
  examId: string;
  studentId: string;
  questionId: string;
  questionNumber: number;
}) {
  const [open, setOpen] = useState(false);
  const [load, setLoad] = useState<Load>({ phase: "idle" });
  const abortRef = useRef<AbortController | null>(null);
  useEffect(() => () => abortRef.current?.abort(), []);

  const fetchHistory = async () => {
    abortRef.current?.abort();
    const ac = new AbortController();
    abortRef.current = ac;
    setLoad({ phase: "loading" });
    try {
      const history = await responseReviewApi.getRevisions(
        examId,
        studentId,
        questionId,
        ac.signal
      );
      if (!ac.signal.aborted) setLoad({ phase: "loaded", history });
    } catch {
      if (!ac.signal.aborted) setLoad({ phase: "error" });
    }
  };

  const toggle = () => {
    const next = !open;
    setOpen(next);
    if (next && load.phase !== "loaded" && load.phase !== "loading") void fetchHistory();
  };

  const regionId = `answer-history-${questionId}`;
  return (
    <section className="flex flex-col gap-2" data-testid="answer-history">
      <Button
        variant="outline"
        size="sm"
        className="w-fit gap-1.5"
        aria-expanded={open}
        aria-controls={regionId}
        onClick={toggle}
        data-testid="answer-history-toggle"
      >
        <History className="size-3.5" aria-hidden="true" />
        {open ? "Hide" : "Show"} how this answer changed
        <ChevronDown
          className={`size-3.5 transition-transform ${open ? "rotate-180" : ""}`}
          aria-hidden="true"
        />
      </Button>

      {open && (
        <div
          id={regionId}
          aria-label={`History of the answer to question ${questionNumber}`}
          role="region"
        >
          {load.phase === "loading" && (
            <p className="flex items-center gap-2 text-sm text-muted-foreground" role="status">
              <Loader2 className="size-3.5 motion-safe:animate-spin" aria-hidden="true" />
              Loading the history…
            </p>
          )}
          {load.phase === "error" && (
            <div className="flex items-center gap-3 text-sm text-destructive" role="alert">
              The history could not be loaded.
              <Button
                size="xs"
                variant="outline"
                onClick={() => void fetchHistory()}
                className="gap-1"
              >
                <RotateCcw className="size-3" aria-hidden="true" />
                Try again
              </Button>
            </div>
          )}
          {load.phase === "loaded" && <HistoryList history={load.history} />}
        </div>
      )}
    </section>
  );
}

function HistoryList({ history }: { history: ReviewRevisionHistory }) {
  if (!history.available) {
    return (
      <p className="text-sm text-muted-foreground">
        {(history.unavailableReason && UNAVAILABLE_TEXT[history.unavailableReason]) ??
          "The history is not available."}
      </p>
    );
  }
  if (history.revisions.length === 0) {
    return <p className="text-sm text-muted-foreground">No answer was saved for this question.</p>;
  }
  const last = history.revisions.length - 1;
  return (
    <ol className="flex flex-col gap-2" data-testid="answer-history-list">
      {history.revisions.map((r, i) => (
        <RevisionItem key={r.revisionNo} revision={r} current={i === last} />
      ))}
    </ol>
  );
}

function RevisionItem({ revision: r, current }: { revision: ReviewRevision; current: boolean }) {
  const when = formatDateTime(r.createdAt);
  const value = r.selectedOptionKey ? `Option ${r.selectedOptionKey}` : (r.textAnswer ?? "");
  const heardDiffers =
    r.transcript !== undefined && r.transcript.trim() !== (r.textAnswer ?? "").trim();
  return (
    <li className="rounded-md border p-3 text-sm" data-testid="answer-history-item">
      <div className="mb-1 flex flex-wrap items-center gap-2 text-xs text-muted-foreground">
        <span className="font-semibold text-foreground">{SOURCE_TEXT[r.source] ?? r.source}</span>
        {when && <span>{when}</span>}
        {current && <Badge variant="secondary">Saved answer</Badge>}
      </div>
      <p className="whitespace-pre-wrap break-words">{value || "(empty)"}</p>
      {heardDiffers && (
        <p className="mt-1 text-xs text-muted-foreground">
          The speech recognizer heard: <span className="italic">&ldquo;{r.transcript}&rdquo;</span>
        </p>
      )}
      {r.recordingIds.length > 0 && (
        <p className="mt-1 text-xs text-muted-foreground">
          {r.recordingIds.length === 1
            ? "Confirmed with 1 recording."
            : `Confirmed with ${r.recordingIds.length} recordings.`}
        </p>
      )}
    </li>
  );
}
