/**
 * Contracts for the teacher response review endpoints. They mirror the backend
 * DTOs exactly; nothing here is derived or invented on the client.
 *
 *   GET /exams/:examId/responses
 *   GET /exams/:examId/responses/:studentId
 */

export type ReviewStatus = "NOT_STARTED" | "IN_PROGRESS" | "SUBMITTED" | "EXPIRED";

export type ReviewSortField = "name" | "rollNo" | "submittedAt";
export type ReviewSortOrder = "asc" | "desc";

export interface ReviewExam {
  id: string;
  title: string;
  subject: string;
  status: string;
  durationMins: number;
  totalQuestions: number;
}

/** Counts every student assigned to the exam, ignoring list filters. */
export interface ReviewSummary {
  assigned: number;
  notStarted: number;
  inProgress: number;
  submitted: number;
  expired: number;
}

export interface ReviewStudentRow {
  studentId: string;
  rollNo: string;
  name: string;
  status: ReviewStatus;
  startedAt?: string;
  submittedAt?: string;
  /** Absent when the student has no session. */
  answeredCount?: number;
}

export interface ReviewStudentList {
  exam: ReviewExam;
  summary: ReviewSummary;
  data: ReviewStudentRow[];
  total: number;
  limit: number;
  offset: number;
}

export interface ReviewStudent {
  id: string;
  rollNo: string;
  name: string;
}

export interface ReviewSession {
  id: string;
  status: ReviewStatus;
  startedAt: string;
  submittedAt?: string;
  /** False while the session is still in progress. */
  isFinal: boolean;
}

export interface ReviewCounts {
  total: number;
  answered: number;
  unanswered: number;
}

export interface ReviewOption {
  id: string;
  key: string;
  text: string;
  displayOrder: number;
  /** Present only when the backend policy allows answer keys. */
  isCorrect?: boolean;
}

export type ReviewAnswerStatus = "ANSWERED" | "UNANSWERED";

export interface ReviewAnswer {
  status: ReviewAnswerStatus;
  selectedOptionId?: string;
  selectedOptionKey?: string;
  selectedOptionText?: string;
  textAnswer?: string;
  savedAt?: string;
  /**
   * Where the saved value came from. Absent for answers saved before this was
   * recorded, and for multiple-choice selections made with the mouse.
   */
  source?: ReviewAnswerSource;
}

export type ReviewAnswerSource = "OPTION" | "TYPED" | "STT_CONFIRMED" | "STUDENT_CORRECTION";

/** AVAILABLE can be played; REMOVED means the audio was deleted but the transcript remains. */
export type ReviewRecordingStatus = "AVAILABLE" | "REMOVED";

/** One kept audio segment of an answer. The audio itself is fetched on request. */
export interface ReviewRecording {
  id: string;
  segmentIndex: number;
  status: ReviewRecordingStatus;
  durationMs: number;
  recordedAt: string;
  /** What the speech recognizer heard, unedited. It can differ from the saved answer. */
  transcript: string;
  /** The student confirmed an answer using this audio. */
  confirmed: boolean;
}

export interface ReviewQuestion {
  id: string;
  questionNumber: number;
  text: string;
  type: "MCQ" | "ESSAY" | "VOICE";
  maxMarks: number;
  options?: ReviewOption[];
  answer: ReviewAnswer;
  /** Absent from servers that do not keep audio; empty when none was kept for this question. */
  recordings?: ReviewRecording[];
}

export type ReviewUnavailableReason = "NOT_STARTED" | "IN_PROGRESS_REVIEW_DISABLED";

export interface ReviewStudentResponses {
  exam: ReviewExam;
  student: ReviewStudent;
  status: ReviewStatus;
  session?: ReviewSession;
  responseAvailable: boolean;
  unavailableReason?: ReviewUnavailableReason;
  counts?: ReviewCounts;
  questions: ReviewQuestion[];
}

export interface ReviewListParams {
  search?: string;
  status?: ReviewStatus;
  sort?: ReviewSortField;
  order?: ReviewSortOrder;
  limit?: number;
  offset?: number;
}
