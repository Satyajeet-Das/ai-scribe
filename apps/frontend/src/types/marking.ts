/**
 * Contracts for teacher marking. They mirror the backend DTOs exactly.
 *
 *   GET  /exams/:examId/responses/:studentId/marking
 *   PUT  /exams/:examId/responses/:studentId/marking/questions/:questionId
 *   POST /exams/:examId/responses/:studentId/marking/finalise
 *   POST /exams/:examId/responses/:studentId/marking/reopen
 *
 * Marks are numbers in steps of 0.5. The server owns every total; the client
 * never adds marks up itself.
 */

export type PaperStatus = "DRAFT" | "FINAL";

/** Where the mark that counts came from. NONE means it has no mark yet. */
export type MarkSource = "TEACHER" | "AUTO" | "NONE";

/** Why marking is not possible yet. */
export type MarkingUnavailableReason = "NOT_STARTED" | "SESSION_NOT_ENDED";

export interface PaperSummary {
  status: PaperStatus;
  /** Sent back when finalising, so a paper changed meanwhile is not frozen unseen. */
  version: number;
  totalMarks: number;
  maxMarks: number;
  markedCount: number;
  totalQuestions: number;
  finalisedAt?: string;
}

export interface MarkingQuestion {
  questionId: string;
  questionNumber: number;
  type: "MCQ" | "ESSAY" | "VOICE";
  maxMarks: number;
  answered: boolean;
  /** Only for a draft multiple-choice question with a stored answer key. */
  correctOptionId?: string;
  /** The automatic mark, or null when only the teacher can decide. */
  suggestedMarks: number | null;
  teacherMarks: number | null;
  /** The mark that counts: the teacher's, else the automatic one. */
  effectiveMarks: number | null;
  markSource: MarkSource;
  comment: string | null;
  /** 0 when nothing has been saved yet. Sent back when saving. */
  version: number;
  /** The question's points were lowered below the saved mark; it must be marked again. */
  exceedsMax: boolean;
}

export interface MarkingSheet {
  sessionId: string | null;
  sessionStatus?: string;
  markingAllowed: boolean;
  reason?: MarkingUnavailableReason;
  paper: PaperSummary;
  questions: MarkingQuestion[];
}

export interface SaveMarksRequest {
  sessionId: string;
  /** null clears the teacher's mark. */
  marks: number | null;
  /** null clears the comment. */
  comment: string | null;
  expectedVersion: number;
}

export interface SaveMarksResponse {
  question: MarkingQuestion;
  paper: PaperSummary;
}
