import { apiFetch } from "@/services/api";
import type {
  ApproveResult,
  DraftQuestionRequest,
  DraftQuestionResult,
  ExamDocument,
  ExamDocumentDraft,
  ExamDocumentList,
} from "@/types/exam-document";

/** A large PDF on a slow connection needs far longer than the 15 s default. */
const UPLOAD_TIMEOUT_MS = 120_000;
/** Fetching the original PDF back is a download of the same size. */
const FILE_TIMEOUT_MS = 60_000;

function base(examId: string): string {
  return `/exams/${encodeURIComponent(examId)}/documents`;
}

function docPath(examId: string, documentId: string): string {
  return `${base(examId)}/${encodeURIComponent(documentId)}`;
}

const noStore = { cache: "no-store" } as const;

/**
 * Client for exam PDF ingestion. Every request is made with the teacher's own
 * credentials and asks the browser not to cache: the responses contain exam
 * content. Reads accept an AbortSignal so a superseded request can be cancelled.
 *
 * Nothing here talks to storage directly: the original PDF is fetched through
 * the API (which checks ownership) and held in memory, never given a URL of the
 * storage's own.
 */
export const examDocumentsApi = {
  /**
   * Uploads a PDF as multipart/form-data in the field "file". `fetch` cannot
   * report how much of an upload has been sent, so there is no percentage.
   */
  upload(examId: string, file: File, signal?: AbortSignal): Promise<ExamDocument> {
    const body = new FormData();
    body.append("file", file, file.name);
    return apiFetch<ExamDocument>(base(examId), {
      method: "POST",
      body,
      signal,
      timeoutMs: UPLOAD_TIMEOUT_MS,
      ...noStore,
    });
  },

  list(examId: string, signal?: AbortSignal): Promise<ExamDocumentList> {
    return apiFetch<ExamDocumentList>(base(examId), { signal, ...noStore });
  },

  get(examId: string, documentId: string, signal?: AbortSignal): Promise<ExamDocument> {
    return apiFetch<ExamDocument>(docPath(examId, documentId), { signal, ...noStore });
  },

  /** The original PDF, fetched with the teacher's credentials and returned as a Blob. */
  getFile(examId: string, documentId: string, signal?: AbortSignal): Promise<Blob> {
    return apiFetch<Blob>(`${docPath(examId, documentId)}/file`, {
      signal,
      responseType: "blob",
      timeoutMs: FILE_TIMEOUT_MS,
      ...noStore,
    });
  },

  cancel(examId: string, documentId: string): Promise<ExamDocument> {
    return apiFetch<ExamDocument>(`${docPath(examId, documentId)}/cancel`, {
      method: "POST",
      ...noStore,
    });
  },

  /**
   * Runs a failed or cancelled document again. For a document that has a draft
   * waiting for review, `discardDraft` and the version last seen are required.
   */
  retry(
    examId: string,
    documentId: string,
    body: { discardDraft?: boolean; expectedDraftVersion?: number } = {}
  ): Promise<ExamDocument> {
    return apiFetch<ExamDocument>(`${docPath(examId, documentId)}/retry`, {
      method: "POST",
      body: JSON.stringify(body),
      ...noStore,
    });
  },

  remove(examId: string, documentId: string): Promise<void> {
    return apiFetch<void>(docPath(examId, documentId), { method: "DELETE" });
  },

  getDraft(examId: string, documentId: string, signal?: AbortSignal): Promise<ExamDocumentDraft> {
    return apiFetch<ExamDocumentDraft>(`${docPath(examId, documentId)}/draft`, {
      signal,
      ...noStore,
    });
  },

  /** Adds a question the extraction missed. `position` omitted puts it last. */
  addQuestion(
    examId: string,
    documentId: string,
    body: { expectedDraftVersion: number; position?: number; question: DraftQuestionRequest }
  ): Promise<DraftQuestionResult> {
    return apiFetch<DraftQuestionResult>(`${docPath(examId, documentId)}/draft/questions`, {
      method: "POST",
      body: JSON.stringify(body),
      ...noStore,
    });
  },

  /** Replaces everything a teacher can edit in one draft question. */
  replaceQuestion(
    examId: string,
    documentId: string,
    questionId: string,
    body: { expectedDraftVersion: number; question: DraftQuestionRequest }
  ): Promise<DraftQuestionResult> {
    return apiFetch<DraftQuestionResult>(
      `${docPath(examId, documentId)}/draft/questions/${encodeURIComponent(questionId)}`,
      { method: "PUT", body: JSON.stringify(body), ...noStore }
    );
  },

  /** Removes a draft question; the whole draft comes back. */
  deleteQuestion(
    examId: string,
    documentId: string,
    questionId: string,
    expectedDraftVersion: number
  ): Promise<ExamDocumentDraft> {
    return apiFetch<ExamDocumentDraft>(
      `${docPath(examId, documentId)}/draft/questions/${encodeURIComponent(questionId)}?expectedDraftVersion=${expectedDraftVersion}`,
      { method: "DELETE", ...noStore }
    );
  },

  /** `questionIds` must list every draft question exactly once, in the new order. */
  reorder(
    examId: string,
    documentId: string,
    body: { expectedDraftVersion: number; questionIds: string[] }
  ): Promise<ExamDocumentDraft> {
    return apiFetch<ExamDocumentDraft>(`${docPath(examId, documentId)}/draft/reorder`, {
      method: "PUT",
      body: JSON.stringify(body),
      ...noStore,
    });
  },

  /** Copies the reviewed draft into the (draft) exam. Does not publish. */
  approve(
    examId: string,
    documentId: string,
    body: { expectedDraftVersion: number }
  ): Promise<ApproveResult> {
    return apiFetch<ApproveResult>(`${docPath(examId, documentId)}/approve`, {
      method: "POST",
      body: JSON.stringify(body),
      ...noStore,
    });
  },
};
