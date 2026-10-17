"use client";

import { useCallback, useEffect, useMemo, useState } from "react";
import { ApiError } from "@/services/api";
import { responseReviewApi } from "@/services/response-review-api";
import { useRemoteResource, type ResourceError } from "@/hooks/use-remote-resource";
import { parseMarksInput } from "@/lib/validations";
import type { MarkingQuestion, MarkingSheet } from "@/types/marking";

export const MAX_COMMENT_LENGTH = 2000;

/** What the teacher has typed for one question and not yet saved. */
export interface MarkDraft {
  marks: string;
  comment: string;
}

export type SaveState = "idle" | "saving" | "saved" | "error" | "conflict";

export interface QuestionStatus {
  state: SaveState;
  message?: string;
}

export type ActionResult = { ok: true } | { ok: false; message: string };

/** Marks as typed: 7 for 7, 7.5 for 7.5. */
export function formatMarks(value: number): string {
  return Number.isInteger(value) ? String(value) : value.toFixed(1);
}

function initialDraft(q: MarkingQuestion): MarkDraft {
  return {
    marks: q.teacherMarks === null ? "" : formatMarks(q.teacherMarks),
    comment: q.comment ?? "",
  };
}

function failure(err: unknown, fallback: string): string {
  return err instanceof ApiError && err.message ? err.message : fallback;
}

/** Problems with a draft, in words for the teacher. Empty when it can be saved. */
export function draftProblems(
  q: MarkingQuestion,
  draft: MarkDraft
): { marks?: string; comment?: string } {
  const out: { marks?: string; comment?: string } = {};
  const parsed = parseMarksInput(draft.marks, q.maxMarks);
  if (!parsed.ok) out.marks = parsed.message;
  if (draft.comment.trim().length > MAX_COMMENT_LENGTH) {
    out.comment = `Keep the comment under ${MAX_COMMENT_LENGTH} characters.`;
  }
  return out;
}

/**
 * Marking state for one student's paper: the server's sheet, the teacher's
 * unsaved drafts, and the actions that change either. Totals always come from
 * the server's reply; nothing is added up here.
 */
export function useStudentMarking(examId: string, studentId: string) {
  const key = `${examId}|${studentId}`;
  const load = useCallback(
    (signal: AbortSignal) => responseReviewApi.getMarking(examId, studentId, signal),
    [examId, studentId]
  );
  const { data: loaded, error, loading, reload } = useRemoteResource(key, load);

  // Replies to saves replace the sheet without a fresh load. They are dropped
  // as soon as a newer load arrives, so the server's latest always wins.
  const [local, setLocal] = useState<{ source: MarkingSheet; sheet: MarkingSheet } | null>(null);
  const sheet = loaded ? (local && local.source === loaded ? local.sheet : loaded) : undefined;
  const replaceSheet = useCallback(
    (next: MarkingSheet) => {
      if (loaded) setLocal({ source: loaded, sheet: next });
    },
    [loaded]
  );

  // Drafts and statuses belong to one student; another student starts clean.
  const [store, setStore] = useState<{
    key: string;
    drafts: Record<string, MarkDraft>;
    status: Record<string, QuestionStatus>;
  }>({ key, drafts: {}, status: {} });
  const current = store.key === key ? store : { key, drafts: {}, status: {} };
  const { drafts, status } = current;

  const patch = useCallback(
    (
      fn: (s: { drafts: Record<string, MarkDraft>; status: Record<string, QuestionStatus> }) => {
        drafts: Record<string, MarkDraft>;
        status: Record<string, QuestionStatus>;
      }
    ) =>
      setStore((prev) => {
        const base = prev.key === key ? prev : { key, drafts: {}, status: {} };
        return { key, ...fn(base) };
      }),
    [key]
  );

  const byId = useMemo(() => {
    const m = new Map<string, MarkingQuestion>();
    sheet?.questions.forEach((q) => m.set(q.questionId, q));
    return m;
  }, [sheet]);

  const draftFor = useCallback(
    (q: MarkingQuestion): MarkDraft => drafts[q.questionId] ?? initialDraft(q),
    [drafts]
  );

  const isDirty = useCallback(
    (q: MarkingQuestion): boolean => {
      const d = drafts[q.questionId];
      if (!d) return false;
      const base = initialDraft(q);
      return d.marks.trim() !== base.marks || d.comment.trim() !== base.comment;
    },
    [drafts]
  );

  const dirtyIds = useMemo(
    () => (sheet ? sheet.questions.filter(isDirty).map((q) => q.questionId) : []),
    [sheet, isDirty]
  );
  const anyDirty = dirtyIds.length > 0;
  const anySaving = Object.values(status).some((s) => s.state === "saving");

  // Leaving the page would lose typed marks.
  useEffect(() => {
    if (!anyDirty) return;
    const warn = (e: BeforeUnloadEvent) => {
      e.preventDefault();
      e.returnValue = "";
    };
    window.addEventListener("beforeunload", warn);
    return () => window.removeEventListener("beforeunload", warn);
  }, [anyDirty]);

  const setStatus = useCallback(
    (questionId: string, next: QuestionStatus | undefined) =>
      patch((s) => {
        const copy = { ...s.status };
        if (next) copy[questionId] = next;
        else delete copy[questionId];
        return { drafts: s.drafts, status: copy };
      }),
    [patch]
  );

  const edit = useCallback(
    (q: MarkingQuestion, change: Partial<MarkDraft>) =>
      patch((s) => {
        const status = { ...s.status };
        // A changed field no longer matches "Saved" or a failed attempt.
        if (status[q.questionId]?.state !== "saving") delete status[q.questionId];
        return {
          drafts: {
            ...s.drafts,
            [q.questionId]: { ...(s.drafts[q.questionId] ?? initialDraft(q)), ...change },
          },
          status,
        };
      }),
    [patch]
  );

  const setMarks = useCallback((q: MarkingQuestion, marks: string) => edit(q, { marks }), [edit]);
  const setComment = useCallback(
    (q: MarkingQuestion, comment: string) => edit(q, { comment }),
    [edit]
  );

  /** Puts the automatic mark in the field, for the teacher to save. */
  const applyAutoMark = useCallback(
    (q: MarkingQuestion) => {
      if (q.suggestedMarks !== null) edit(q, { marks: formatMarks(q.suggestedMarks) });
    },
    [edit]
  );

  /** Fetches the sheet again and shows it, dropping typed text for the given question only. */
  const refetch = useCallback(
    async (dropDraftFor?: string) => {
      const latest = await responseReviewApi.getMarking(examId, studentId);
      replaceSheet(latest);
      if (dropDraftFor) {
        patch((s) => {
          const drafts = { ...s.drafts };
          delete drafts[dropDraftFor];
          const status = { ...s.status };
          delete status[dropDraftFor];
          return { drafts, status };
        });
      }
      return latest;
    },
    [examId, studentId, replaceSheet, patch]
  );

  const loadLatest = useCallback(
    async (questionId: string) => {
      try {
        await refetch(questionId);
      } catch (err) {
        setStatus(questionId, {
          state: "error",
          message: failure(err, "Could not load the latest marks. Try again."),
        });
      }
    },
    [refetch, setStatus]
  );

  const save = useCallback(
    async (questionId: string): Promise<ActionResult> => {
      const q = byId.get(questionId);
      if (!sheet || !sheet.sessionId || !q) return { ok: false, message: "Nothing to save." };
      if (status[questionId]?.state === "saving") return { ok: false, message: "Already saving." };

      const draft = drafts[questionId] ?? initialDraft(q);
      const problems = draftProblems(q, draft);
      if (problems.marks || problems.comment) {
        const message = problems.marks ?? problems.comment ?? "Check the marks.";
        setStatus(questionId, { state: "error", message });
        return { ok: false, message };
      }
      const parsed = parseMarksInput(draft.marks, q.maxMarks);
      const marks = parsed.ok ? parsed.value : null;
      const comment = draft.comment.trim() === "" ? null : draft.comment.trim();

      setStatus(questionId, { state: "saving" });
      try {
        const res = await responseReviewApi.saveQuestionMarks(examId, studentId, questionId, {
          sessionId: sheet.sessionId,
          marks,
          comment,
          expectedVersion: q.version,
        });
        replaceSheet({
          ...sheet,
          paper: res.paper,
          questions: sheet.questions.map((x) => (x.questionId === questionId ? res.question : x)),
        });
        patch((s) => {
          const drafts = { ...s.drafts };
          delete drafts[questionId];
          return { drafts, status: { ...s.status, [questionId]: { state: "saved" } } };
        });
        return { ok: true };
      } catch (err) {
        if (err instanceof ApiError && err.status === 409) {
          if (err.code === "STALE_VERSION") {
            const message = "This mark was changed elsewhere. Your text is kept below.";
            setStatus(questionId, { state: "conflict", message });
            return { ok: false, message };
          }
          // The paper was finalised, or another attempt is now shown: show the truth.
          setStatus(questionId, { state: "error", message: err.message });
          await refetch().catch(() => undefined);
          return { ok: false, message: err.message };
        }
        const message = failure(err, "Could not save. Check your connection and try again.");
        setStatus(questionId, { state: "error", message });
        return { ok: false, message };
      }
    },
    [byId, sheet, status, drafts, examId, studentId, replaceSheet, patch, setStatus, refetch]
  );

  const finalise = useCallback(async (): Promise<ActionResult> => {
    if (!sheet || !sheet.sessionId) return { ok: false, message: "Nothing to finalise." };
    if (anyDirty || anySaving) {
      return { ok: false, message: "Save or discard your changes first." };
    }
    try {
      const next = await responseReviewApi.finalise(examId, studentId, {
        sessionId: sheet.sessionId,
        expectedPaperVersion: sheet.paper.version,
      });
      replaceSheet(next);
      return { ok: true };
    } catch (err) {
      if (err instanceof ApiError && err.status === 409) await refetch().catch(() => undefined);
      return { ok: false, message: failure(err, "Could not finalise. Please try again.") };
    }
  }, [sheet, anyDirty, anySaving, examId, studentId, replaceSheet, refetch]);

  const reopen = useCallback(
    async (reason: string): Promise<ActionResult> => {
      if (!sheet || !sheet.sessionId) return { ok: false, message: "Nothing to reopen." };
      try {
        const next = await responseReviewApi.reopen(examId, studentId, {
          sessionId: sheet.sessionId,
          reason: reason.trim(),
        });
        replaceSheet(next);
        return { ok: true };
      } catch (err) {
        if (err instanceof ApiError && err.status === 409) await refetch().catch(() => undefined);
        return { ok: false, message: failure(err, "Could not reopen. Please try again.") };
      }
    },
    [sheet, examId, studentId, replaceSheet, refetch]
  );

  const unmarkedNumbers = useMemo(
    () =>
      sheet
        ? sheet.questions.filter((q) => q.effectiveMarks === null).map((q) => q.questionNumber)
        : [],
    [sheet]
  );

  return {
    sheet,
    error: error as ResourceError | undefined,
    loading,
    reload,
    draftFor,
    isDirty,
    anyDirty,
    anySaving,
    dirtyIds,
    statusOf: (questionId: string): QuestionStatus => status[questionId] ?? { state: "idle" },
    unmarkedNumbers,
    setMarks,
    setComment,
    applyAutoMark,
    save,
    loadLatest,
    finalise,
    reopen,
  };
}
