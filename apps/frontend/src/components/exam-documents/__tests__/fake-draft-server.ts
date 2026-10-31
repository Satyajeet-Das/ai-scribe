import { vi } from "vitest";
import { ApiError } from "@/services/api";
import type {
  ApproveResult,
  DraftQuestion,
  DraftQuestionRequest,
  ExamDocument,
  ExamDocumentDraft,
} from "@/types/exam-document";

/**
 * A small stateful stand-in for the backend's draft endpoints: versions that
 * must match, conflicts, ordering, and the approval rules. Tests drive the real
 * components against it, so the interaction of edits, versions and approval is
 * exercised and not just each call in isolation. It follows the contract in
 * apps/backend/internal/document (see docs/architecture/exam-document-ingestion.md).
 */

export function makeQuestion(over: Partial<DraftQuestion> & { id: string }): DraftQuestion {
  return {
    position: 1, numberLabel: "", type: "ESSAY", instructions: "", text: `Question ${over.id}`, points: 5,
    options: [], tables: [], diagrams: [], sourcePages: [1], confidence: "HIGH", warnings: [],
    origin: "EXTRACTED", edited: false, createdAt: "2026-01-01T00:00:00Z", updatedAt: "2026-01-01T00:00:00Z", ...over,
  };
}

export function makeDocument(over: Partial<ExamDocument> = {}): ExamDocument {
  return {
    id: "d1", examId: "e1", filename: "Physics.pdf", sizeBytes: 1000, pageCount: 3, status: "REVIEW_REQUIRED",
    progress: { done: 1, total: 1 }, attempt: 1, draftVersion: 1, uploadedBy: "u1",
    createdAt: "2026-03-01T10:00:00Z", updatedAt: "2026-03-01T10:00:00Z", ...over,
  };
}

export class FakeDraftServer {
  version = 1;
  questions: DraftQuestion[];
  document: ExamDocument;
  documentInstructions = "";
  unassigned: ExamDocumentDraft["unassigned"] = [];
  /** Set to make the next mutation fail with this error (then it clears). */
  failNext: ApiError | null = null;
  calls: Array<{ op: string; args: unknown }> = [];
  private seq = 0;

  constructor(questions: DraftQuestion[], document: ExamDocument = makeDocument()) {
    this.questions = questions.map((q, i) => ({ ...q, position: i + 1 }));
    this.document = document;
  }

  snapshot(): ExamDocumentDraft {
    return {
      document: { ...this.document, draftVersion: this.version },
      draftVersion: this.version,
      documentInstructions: this.documentInstructions,
      unassigned: this.unassigned,
      questions: this.questions.map((q) => ({ ...q })),
    };
  }

  private fail() {
    if (this.failNext) {
      const e = this.failNext;
      this.failNext = null;
      throw e;
    }
  }

  private checkVersion(expected: number) {
    if (this.document.status === "COMPLETED") throw new ApiError("approved", 409, "ALREADY_APPROVED", true);
    if (expected !== this.version) throw new ApiError("stale", 409, "STALE_VERSION", true);
  }

  private renumber() {
    this.questions.forEach((q, i) => (q.position = i + 1));
  }

  private apply(q: DraftQuestion, r: DraftQuestionRequest) {
    const text = r.text.trim();
    if ([...text].length < 2) throw new ApiError("bad", 400, "VALIDATION_ERROR", true, [{ field: "text", error: "must be at least 2 characters" }]);
    if (r.points !== null && (r.points < 0 || r.points > 1000)) {
      throw new ApiError("bad", 400, "VALIDATION_ERROR", true, [{ field: "points", error: "must be between 0 and 1000" }]);
    }
    if (r.type === "ESSAY" && r.options.length > 0) {
      throw new ApiError("bad", 400, "VALIDATION_ERROR", true, [{ field: "options", error: "are only allowed on multiple-choice questions" }]);
    }
    const ack = new Set(r.acknowledgedWarnings);
    q.numberLabel = r.numberLabel;
    q.type = r.type;
    q.instructions = r.instructions;
    q.text = text;
    q.points = r.points;
    q.options = r.options.map((o) => ({ ...o, label: o.label.toUpperCase() }));
    q.tables = r.tables;
    q.diagrams = r.diagrams.map((d, i) => ({
      ...d,
      descriptionSource: q.diagrams[i]?.description === d.description ? q.diagrams[i]?.descriptionSource : d.description ? "TEACHER" : undefined,
    }));
    q.warnings = q.warnings.map((w) => ({ ...w, acknowledged: w.blocking && ack.has(w.code) }));
    q.edited = true;
    q.updatedAt = new Date(Date.parse(q.updatedAt) + 1000).toISOString();
  }

  // ---- the endpoints, shaped like examDocumentsApi ----

  getDraft = async (): Promise<ExamDocumentDraft> => {
    this.calls.push({ op: "getDraft", args: {} });
    this.fail();
    return this.snapshot();
  };

  replaceQuestion = async (_e: string, _d: string, id: string, body: { expectedDraftVersion: number; question: DraftQuestionRequest }) => {
    this.calls.push({ op: "replace", args: { id, ...body } });
    this.fail();
    this.checkVersion(body.expectedDraftVersion);
    const q = this.questions.find((x) => x.id === id);
    if (!q) throw new ApiError("nf", 404);
    this.apply(q, body.question);
    this.version++;
    return { question: { ...q }, draftVersion: this.version };
  };

  addQuestion = async (_e: string, _d: string, body: { expectedDraftVersion: number; position?: number; question: DraftQuestionRequest }) => {
    this.calls.push({ op: "add", args: body });
    this.fail();
    this.checkVersion(body.expectedDraftVersion);
    const q = makeQuestion({ id: `new-${++this.seq}`, origin: "TEACHER", sourcePages: [] });
    this.apply(q, body.question);
    q.edited = false;
    const pos = body.position && body.position >= 1 && body.position <= this.questions.length ? body.position : this.questions.length + 1;
    this.questions.splice(pos - 1, 0, q);
    this.renumber();
    this.version++;
    return { question: { ...q }, draftVersion: this.version };
  };

  deleteQuestion = async (_e: string, _d: string, id: string, expected: number) => {
    this.calls.push({ op: "delete", args: { id, expected } });
    this.fail();
    this.checkVersion(expected);
    if (!this.questions.some((q) => q.id === id)) throw new ApiError("nf", 404);
    this.questions = this.questions.filter((q) => q.id !== id);
    this.renumber();
    this.version++;
    return this.snapshot();
  };

  reorder = async (_e: string, _d: string, body: { expectedDraftVersion: number; questionIds: string[] }) => {
    this.calls.push({ op: "reorder", args: body });
    this.fail();
    this.checkVersion(body.expectedDraftVersion);
    const have = new Set(this.questions.map((q) => q.id));
    if (body.questionIds.length !== have.size || !body.questionIds.every((id) => have.has(id))) {
      throw new ApiError("bad", 400, "VALIDATION_ERROR", true, [{ field: "questionIds", error: "must list every question exactly once" }]);
    }
    this.questions = body.questionIds.map((id) => this.questions.find((q) => q.id === id)!);
    this.renumber();
    this.version++;
    return this.snapshot();
  };

  approve = async (_e: string, _d: string, body: { expectedDraftVersion: number }): Promise<ApproveResult> => {
    this.calls.push({ op: "approve", args: body });
    this.fail();
    this.checkVersion(body.expectedDraftVersion);
    const problems: Array<{ field: string; error: string }> = [];
    for (const q of this.questions) {
      if (q.points === null) problems.push({ field: `questions[${q.position}].points`, error: "Set the marks for this question." });
      if (q.type === "MCQ" && q.options.length < 2) problems.push({ field: `questions[${q.position}].options`, error: "A multiple-choice question needs at least 2 options." });
      q.diagrams.forEach((d, i) => {
        if (!d.description || !d.confirmed) problems.push({ field: `questions[${q.position}].diagrams[${i}].confirmed`, error: "Confirm the diagram." });
      });
      for (const w of q.warnings) if (w.blocking && !w.acknowledged) problems.push({ field: `questions[${q.position}].warnings.${w.code}`, error: "Acknowledge this." });
    }
    if (this.questions.length === 0) problems.push({ field: "questions", error: "The draft has no questions to add." });
    if (problems.length > 0) throw new ApiError("incomplete", 409, "DRAFT_INCOMPLETE", true, problems);
    this.version++;
    this.document = { ...this.document, status: "COMPLETED", approvedAt: "2026-03-02T10:00:00Z", approvedBy: "u1", draftVersion: this.version };
    return {
      document: this.document,
      questions: this.questions.map((q, i) => ({ id: `final-${i}`, questionNumber: i + 1, type: q.type, points: q.points ?? 0, text: q.text })),
    };
  };

  /** Wires this fake into a mocked examDocumentsApi. */
  attach(api: Record<string, ReturnType<typeof vi.fn>>) {
    api.getDraft.mockImplementation(this.getDraft);
    api.replaceQuestion.mockImplementation(this.replaceQuestion);
    api.addQuestion.mockImplementation(this.addQuestion);
    api.deleteQuestion.mockImplementation(this.deleteQuestion);
    api.reorder.mockImplementation(this.reorder);
    api.approve.mockImplementation(this.approve);
  }

  count(op: string): number {
    return this.calls.filter((c) => c.op === op).length;
  }
}
