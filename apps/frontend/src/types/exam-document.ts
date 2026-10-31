/**
 * Types for exam PDF ingestion. They mirror the backend's response bodies
 * (apps/backend/internal/document/dto.go and static/openapi.json, tag "Exam
 * Documents") field for field; nothing here is invented by the frontend.
 */

export type DocumentStatus =
  | "QUEUED"
  | "PROCESSING"
  | "REVIEW_REQUIRED"
  | "COMPLETED"
  | "FAILED"
  | "CANCELLED";

export type DocumentStage = "READING_PAGES" | "EXTRACTING" | "VALIDATING" | "SAVING";

export type PageKind = "TEXT" | "IMAGE_ONLY" | "MIXED";

export interface DocumentPageInfo {
  number: number;
  kind: PageKind;
  chars: number;
  columns: number;
  images: number;
  imagesSkipped: number;
  /** Header and footer lines that were left out of the text that was read. */
  removedLines?: string[];
  /** Why a page could not be read, written by the server. */
  note?: string;
}

export interface ExamDocument {
  id: string;
  examId: string;
  filename: string;
  sizeBytes: number;
  pageCount: number;
  status: DocumentStatus;
  stage?: DocumentStage;
  progress: { done: number; total: number };
  attempt: number;
  failureCode?: string;
  /** Written by the server; never the provider's own words. */
  failureMessage?: string;
  provider?: string;
  draftVersion: number;
  uploadedBy: string;
  approvedBy?: string;
  approvedAt?: string;
  createdAt: string;
  updatedAt: string;
  /** Present on a single document and on a draft, not in lists. */
  pages?: DocumentPageInfo[];
}

export interface ExamDocumentList {
  documents: ExamDocument[];
  total: number;
}

export type DraftQuestionType = "MCQ" | "ESSAY";
export type Confidence = "HIGH" | "MEDIUM" | "LOW";

export interface DraftOption {
  label: string;
  text: string;
  /** Only ever set by a teacher; extraction never chooses an answer. */
  isCorrect: boolean;
}

export interface DraftTable {
  page: number;
  markdown: string;
}

export interface DraftDiagram {
  page: number;
  region: string;
  description: string;
  descriptionSource?: "MODEL" | "TEACHER";
  confirmed: boolean;
}

export interface DraftWarning {
  code: string;
  message: string;
  /** A blocking warning must be acknowledged before the draft can be approved. */
  blocking: boolean;
  acknowledged: boolean;
}

export interface DraftQuestion {
  id: string;
  position: number;
  numberLabel: string;
  type: DraftQuestionType;
  instructions: string;
  text: string;
  points: number | null;
  options: DraftOption[];
  tables: DraftTable[];
  diagrams: DraftDiagram[];
  sourcePages: number[];
  confidence: Confidence;
  warnings: DraftWarning[];
  origin: "EXTRACTED" | "TEACHER";
  edited: boolean;
  createdAt: string;
  updatedAt: string;
}

export interface UnassignedContent {
  page: number;
  text: string;
  reason: string;
}

export interface ExamDocumentDraft {
  document: ExamDocument;
  draftVersion: number;
  documentInstructions: string;
  unassigned: UnassignedContent[];
  questions: DraftQuestion[];
}

export interface DraftQuestionResult {
  question: DraftQuestion;
  draftVersion: number;
}

/** The complete state a teacher wants for one draft question (a replacement, not a patch). */
export interface DraftQuestionRequest {
  numberLabel: string;
  type: DraftQuestionType;
  instructions: string;
  text: string;
  points: number | null;
  options: DraftOption[];
  tables: DraftTable[];
  diagrams: Array<Pick<DraftDiagram, "page" | "region" | "description" | "confirmed">>;
  /** Codes of blocking warnings the teacher has checked. */
  acknowledgedWarnings: string[];
}

export interface ApprovedQuestion {
  id: string;
  questionNumber: number;
  type: string;
  points: number;
  text: string;
}

export interface ApproveResult {
  document: ExamDocument;
  questions: ApprovedQuestion[];
}

/** True while the server is still working on the document. */
export function isActiveStatus(status: DocumentStatus): boolean {
  return status === "QUEUED" || status === "PROCESSING";
}
