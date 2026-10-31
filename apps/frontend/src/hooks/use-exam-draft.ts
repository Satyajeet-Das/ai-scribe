"use client";

import { useCallback, useEffect, useRef, useState } from "react";
import { describeDocumentError, type DocumentError } from "@/lib/document-errors";
import { examDocumentsApi } from "@/services/exam-documents-api";
import type {
  ApproveResult,
  DraftQuestion,
  DraftQuestionRequest,
  ExamDocumentDraft,
} from "@/types/exam-document";

/** The outcome of one operation. Failures are values, so callers cannot forget to handle them. */
export type OpResult<T = void> = { ok: true; value: T } | { ok: false; error: DocumentError };

export interface Moved {
  from: number;
  to: number;
  total: number;
}

export interface ExamDraftState {
  draft: ExamDocumentDraft | undefined;
  /** True until the first load settles. */
  loading: boolean;
  loadError: DocumentError | undefined;
  /**
   * The draft was changed somewhere else (or could not be refreshed). Changes
   * are refused until it is reloaded, because they would be made against a
   * version the server no longer has.
   */
  stale: boolean;
  /** Operations running or waiting. */
  pending: number;
  replace: (questionId: string, request: DraftQuestionRequest) => Promise<OpResult<DraftQuestion>>;
  add: (request: DraftQuestionRequest, position?: number) => Promise<OpResult<DraftQuestion>>;
  remove: (questionId: string) => Promise<OpResult>;
  move: (questionId: string, delta: -1 | 1) => Promise<OpResult<Moved>>;
  approve: () => Promise<OpResult<ApproveResult>>;
  reload: () => Promise<OpResult>;
}

const STALE_ERROR: DocumentError = {
  kind: "conflict",
  code: "STALE_VERSION",
  message: "This draft was changed somewhere else. Reload it to see the latest before continuing.",
  retryable: false,
  fields: {},
};

/**
 * The draft of one document, and every change to it.
 *
 * Every operation goes through a single queue and reads the draft's version at
 * the moment it starts, not when it was asked for. So two quick edits cannot
 * both be sent with the same version (the second would be refused), and a slow
 * response can never overwrite the result of one that finished after it. A
 * response is applied only if this hook is still showing the same draft.
 *
 * Nothing is applied optimistically: what is shown is what the server last said.
 */
export function useExamDraft(examId: string, documentId: string): ExamDraftState {
  const key = `${examId}/${documentId}`;
  const [view, setView] = useState<{ key: string; draft?: ExamDocumentDraft; loadError?: DocumentError } | null>(null);
  const [stale, setStale] = useState<{ key: string } | null>(null);
  const [pending, setPending] = useState(0);

  const draftRef = useRef<ExamDocumentDraft | undefined>(undefined);
  const staleRef = useRef(false);
  const keyRef = useRef(key);
  const alive = useRef(false);
  const queue = useRef<Promise<unknown>>(Promise.resolve());
  const controller = useRef<AbortController | null>(null);

  const apply = useCallback((forKey: string, draft: ExamDocumentDraft) => {
    // A result for a draft we are no longer showing is dropped.
    if (!alive.current || forKey !== keyRef.current) return;
    draftRef.current = draft;
    setView({ key: forKey, draft });
  }, []);

  const markStale = useCallback((forKey: string, value: boolean) => {
    if (!alive.current || forKey !== keyRef.current) return;
    staleRef.current = value;
    setStale(value ? { key: forKey } : null);
  }, []);

  const enqueue = useCallback(<T,>(task: () => Promise<OpResult<T>>): Promise<OpResult<T>> => {
    setPending((p) => p + 1);
    const result = queue.current.catch(() => undefined).then(task);
    queue.current = result;
    return result.finally(() => {
      if (alive.current) setPending((p) => p - 1);
    });
  }, []);

  /** Runs a mutation: refused while stale, and a stale answer from the server marks the draft stale. */
  const mutate = useCallback(
    <T,>(run: (forKey: string, draft: ExamDocumentDraft) => Promise<T>): Promise<OpResult<T>> =>
      enqueue(async () => {
        const forKey = keyRef.current;
        const draft = draftRef.current;
        if (!draft) return { ok: false, error: describeDocumentError(undefined) };
        if (staleRef.current) return { ok: false, error: STALE_ERROR };
        try {
          return { ok: true, value: await run(forKey, draft) };
        } catch (err) {
          const error = describeDocumentError(err);
          if (error.code === "STALE_VERSION") markStale(forKey, true);
          return { ok: false, error };
        }
      }),
    [enqueue, markStale]
  );

  const load = useCallback(
    (forKey: string, signal?: AbortSignal): Promise<OpResult> =>
      enqueue(async () => {
        try {
          const draft = await examDocumentsApi.getDraft(examId, documentId, signal);
          markStale(forKey, false);
          apply(forKey, draft);
          return { ok: true, value: undefined };
        } catch (err) {
          if (signal?.aborted) return { ok: false, error: describeDocumentError(err) };
          const error = describeDocumentError(err);
          if (alive.current && forKey === keyRef.current) {
            // Keep what is already shown, and say that it may be out of date.
            setView((prev) => ({
              key: forKey,
              draft: prev?.key === forKey ? prev.draft : undefined,
              loadError: error,
            }));
          }
          return { ok: false, error };
        }
      }),
    [apply, enqueue, examId, documentId, markStale]
  );

  useEffect(() => {
    alive.current = true;
    keyRef.current = key;
    draftRef.current = undefined;
    staleRef.current = false;
    const ctl = new AbortController();
    controller.current = ctl;
    void load(key, ctl.signal);
    return () => {
      alive.current = false;
      ctl.abort();
    };
  }, [key, load]);

  const reload = useCallback(() => load(keyRef.current), [load]);

  const replace = useCallback(
    (questionId: string, request: DraftQuestionRequest) =>
      mutate<DraftQuestion>(async (forKey, draft) => {
        const res = await examDocumentsApi.replaceQuestion(examId, documentId, questionId, {
          expectedDraftVersion: draft.draftVersion,
          question: request,
        });
        apply(forKey, {
          ...draft,
          draftVersion: res.draftVersion,
          document: { ...draft.document, draftVersion: res.draftVersion },
          questions: draft.questions.map((q) => (q.id === questionId ? res.question : q)),
        });
        return res.question;
      }),
    [apply, documentId, examId, mutate]
  );

  const add = useCallback(
    (request: DraftQuestionRequest, position?: number) =>
      mutate<DraftQuestion>(async (forKey, draft) => {
        const res = await examDocumentsApi.addQuestion(examId, documentId, {
          expectedDraftVersion: draft.draftVersion,
          position,
          question: request,
        });
        // Adding in the middle renumbers the questions after it, so take the whole draft again.
        try {
          const fresh = await examDocumentsApi.getDraft(examId, documentId);
          apply(forKey, fresh);
          return fresh.questions.find((q) => q.id === res.question.id) ?? res.question;
        } catch {
          markStale(forKey, true);
          return res.question;
        }
      }),
    [apply, documentId, examId, markStale, mutate]
  );

  const remove = useCallback(
    (questionId: string) =>
      mutate<void>(async (forKey, draft) => {
        const fresh = await examDocumentsApi.deleteQuestion(examId, documentId, questionId, draft.draftVersion);
        apply(forKey, fresh);
      }),
    [apply, documentId, examId, mutate]
  );

  const move = useCallback(
    (questionId: string, delta: -1 | 1) =>
      mutate<Moved>(async (forKey, draft) => {
        const ids = draft.questions.map((q) => q.id);
        const from = ids.indexOf(questionId);
        const to = from + delta;
        if (from < 0 || to < 0 || to >= ids.length) return { from, to: from, total: ids.length };
        [ids[from], ids[to]] = [ids[to], ids[from]];
        const fresh = await examDocumentsApi.reorder(examId, documentId, {
          expectedDraftVersion: draft.draftVersion,
          questionIds: ids,
        });
        apply(forKey, fresh);
        return { from: from + 1, to: to + 1, total: ids.length };
      }),
    [apply, documentId, examId, mutate]
  );

  const approve = useCallback(
    () =>
      mutate<ApproveResult>(async (forKey, draft) => {
        const res = await examDocumentsApi.approve(examId, documentId, { expectedDraftVersion: draft.draftVersion });
        // The document is COMPLETED now; show it as the server reports it.
        apply(forKey, { ...draft, document: res.document, draftVersion: res.document.draftVersion });
        return res;
      }),
    [apply, documentId, examId, mutate]
  );

  const current = view?.key === key ? view : null;
  return {
    draft: current?.draft,
    loading: current === null,
    loadError: current?.loadError,
    stale: stale?.key === key,
    pending,
    replace,
    add,
    remove,
    move,
    approve,
    reload,
  };
}
