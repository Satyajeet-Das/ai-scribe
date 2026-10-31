"use client";

import { useCallback, useEffect, useRef, useState } from "react";
import { describeDocumentError, type DocumentError } from "@/lib/document-errors";
import { examDocumentsApi } from "@/services/exam-documents-api";
import { isActiveStatus, type ExamDocument } from "@/types/exam-document";

/** First check after a change, and the pace while something is moving. */
export const POLL_FAST_MS = 2_000;
/** The slowest the check gets while a document is active but nothing is changing. */
export const POLL_SLOW_MS = 10_000;
/** The slowest the check gets after repeated failures. */
export const POLL_ERROR_MAX_MS = 30_000;

/** What the teacher can see change: when this changes, the pace speeds up again. */
function signature(docs: ExamDocument[]): string {
  return docs
    .map((d) => [d.id, d.status, d.stage ?? "", d.progress.done, d.progress.total, d.attempt, d.draftVersion].join(":"))
    .join("|");
}

export interface ExamDocumentsState {
  documents: ExamDocument[];
  /** True until the first answer (or failure) arrives. */
  loading: boolean;
  /** The last attempt to refresh failed; the documents shown may be out of date. */
  error: DocumentError | undefined;
  /** Check now, and keep checking while any document is being worked on. */
  reload: () => void;
}

/**
 * The exam's documents, kept fresh while any of them is being worked on.
 *
 * It asks for the list (one request, however many documents), waits, and asks
 * again; it never has two requests in flight. It speeds up when something
 * changes and slows down when nothing does, does not ask while the tab is
 * hidden, stops once no document is queued or processing, and cancels
 * everything when the component unmounts or the exam changes. A response that
 * arrives after that is dropped.
 *
 * Only what the server reports is shown: progress is never estimated.
 */
export function useExamDocuments(examId: string): ExamDocumentsState {
  // Tied to the exam it was loaded for, so a different exam never shows this one's documents.
  const [view, setView] = useState<{ examId: string; documents: ExamDocument[]; error?: DocumentError } | null>(null);
  const [token, setToken] = useState(0);
  // `reload` also restarts a loop that had gone idle.
  const reload = useCallback(() => setToken((t) => t + 1), []);

  const timer = useRef<ReturnType<typeof setTimeout> | null>(null);

  useEffect(() => {
    let stopped = false;
    let controller: AbortController | null = null;
    let delay = POLL_FAST_MS;
    let lastSignature: string | null = null;

    const schedule = (ms: number) => {
      if (stopped) return;
      timer.current = setTimeout(tick, ms);
    };

    async function tick() {
      timer.current = null;
      if (stopped) return;
      // A hidden tab does not need fresh data; it checks again when shown.
      if (typeof document !== "undefined" && document.hidden) {
        schedule(delay);
        return;
      }
      controller = new AbortController();
      const mine = controller;
      try {
        const res = await examDocumentsApi.list(examId, mine.signal);
        if (stopped || mine.signal.aborted) return;
        const docs = res.documents ?? [];
        setView({ examId, documents: docs });

        const sig = signature(docs);
        delay = sig !== lastSignature ? POLL_FAST_MS : Math.min(Math.round(delay * 1.5), POLL_SLOW_MS);
        lastSignature = sig;
        if (docs.some((d) => isActiveStatus(d.status))) schedule(delay);
      } catch (err) {
        if (stopped || mine.signal.aborted) return;
        const described = describeDocumentError(err);
        // Keep what was already shown, and say that it may be out of date.
        setView((prev) => ({ examId, documents: prev?.examId === examId ? prev.documents : [], error: described }));
        // Keep trying only for failures that can pass; a refusal will not.
        if (described.retryable || described.kind === "network") {
          delay = Math.min(delay * 2, POLL_ERROR_MAX_MS);
          schedule(delay);
        }
      }
    }

    const onVisible = () => {
      if (stopped || document.hidden || timer.current === null) return;
      // Something was pending while the tab was hidden: look now.
      clearTimeout(timer.current);
      void tick();
    };
    document.addEventListener("visibilitychange", onVisible);

    void tick();

    return () => {
      stopped = true;
      document.removeEventListener("visibilitychange", onVisible);
      if (timer.current) clearTimeout(timer.current);
      timer.current = null;
      controller?.abort();
    };
  }, [examId, token]);

  const current = view?.examId === examId ? view : null;
  return { documents: current?.documents ?? [], loading: current === null, error: current?.error, reload };
}
