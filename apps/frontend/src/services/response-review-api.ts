import { apiFetch } from "@/services/api";
import type { MarkingSheet, SaveMarksRequest, SaveMarksResponse } from "@/types/marking";
import type {
  ReviewListParams,
  ReviewStudentList,
  ReviewStudentResponses,
} from "@/types/response-review";

function markingPath(examId: string, studentId: string): string {
  return `/exams/${encodeURIComponent(examId)}/responses/${encodeURIComponent(studentId)}/marking`;
}

/**
 * Client for teacher response review and marking. Reads accept an
 * AbortSignal so a superseded request can be cancelled, and ask the browser
 * not to cache: these responses contain student answers.
 */
export const responseReviewApi = {
  listStudents(
    examId: string,
    params: ReviewListParams = {},
    signal?: AbortSignal
  ): Promise<ReviewStudentList> {
    const query = new URLSearchParams();
    const search = params.search?.trim();
    if (search) query.set("search", search);
    if (params.status) query.set("status", params.status);
    if (params.sort) query.set("sort", params.sort);
    if (params.order) query.set("order", params.order);
    if (params.limit) query.set("limit", String(params.limit));
    if (params.offset) query.set("offset", String(params.offset));
    const qs = query.toString();
    return apiFetch<ReviewStudentList>(
      `/exams/${encodeURIComponent(examId)}/responses${qs ? `?${qs}` : ""}`,
      { signal, cache: "no-store" }
    );
  },

  getStudentResponses(
    examId: string,
    studentId: string,
    signal?: AbortSignal
  ): Promise<ReviewStudentResponses> {
    return apiFetch<ReviewStudentResponses>(
      `/exams/${encodeURIComponent(examId)}/responses/${encodeURIComponent(studentId)}`,
      { signal, cache: "no-store" }
    );
  },

  getMarking(examId: string, studentId: string, signal?: AbortSignal): Promise<MarkingSheet> {
    return apiFetch<MarkingSheet>(markingPath(examId, studentId), { signal, cache: "no-store" });
  },

  /** Replaces the mark and comment for one answer. */
  saveQuestionMarks(
    examId: string,
    studentId: string,
    questionId: string,
    body: SaveMarksRequest
  ): Promise<SaveMarksResponse> {
    return apiFetch<SaveMarksResponse>(
      `${markingPath(examId, studentId)}/questions/${encodeURIComponent(questionId)}`,
      { method: "PUT", body: JSON.stringify(body), cache: "no-store" }
    );
  },

  finalise(
    examId: string,
    studentId: string,
    body: { sessionId: string; expectedPaperVersion: number }
  ): Promise<MarkingSheet> {
    return apiFetch<MarkingSheet>(`${markingPath(examId, studentId)}/finalise`, {
      method: "POST",
      body: JSON.stringify(body),
      cache: "no-store",
    });
  },

  reopen(
    examId: string,
    studentId: string,
    body: { sessionId: string; reason: string }
  ): Promise<MarkingSheet> {
    return apiFetch<MarkingSheet>(`${markingPath(examId, studentId)}/reopen`, {
      method: "POST",
      body: JSON.stringify(body),
      cache: "no-store",
    });
  },

  /**
   * Fetches the original audio of a recorded answer. The server checks access
   * and writes an audit record for every call, so this is only called when the
   * reviewer asks to listen, never to prepare a page. The audio is never given
   * a URL of its own: it is fetched with the reviewer's credentials and played
   * from memory.
   */
  getRecordingAudio(
    examId: string,
    studentId: string,
    recordingId: string,
    signal?: AbortSignal
  ): Promise<Blob> {
    return apiFetch<Blob>(
      `/exams/${encodeURIComponent(examId)}/responses/${encodeURIComponent(studentId)}/recordings/${encodeURIComponent(recordingId)}/audio`,
      { signal, cache: "no-store", responseType: "blob", timeoutMs: 60_000 }
    );
  },
};
