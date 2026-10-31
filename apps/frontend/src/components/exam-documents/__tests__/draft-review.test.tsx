import { describe, it, expect, vi, beforeEach } from "vitest";
import { render, screen } from "@testing-library/react";
import type { ExamDocument } from "@/types/exam-document";
import type { Exam } from "@/types/exam-types";

const state = vi.hoisted(() => ({
  docs: { documents: [] as unknown[], loading: false, error: undefined as unknown, reload: () => {} },
  exam: { data: undefined as unknown, loading: false, error: undefined as unknown },
}));

vi.mock("@/hooks/use-exam-documents", () => ({ useExamDocuments: () => state.docs }));
vi.mock("@/hooks/use-remote-resource", () => ({ useRemoteResource: () => state.exam }));
vi.mock("@/services/api", async (orig) => ({
  ...(await orig<typeof import("@/services/api")>()),
  examsApi: { getExam: vi.fn() },
}));
vi.mock("../draft-workspace", () => ({
  DraftWorkspace: (p: { examId: string; documentId: string; exam?: Exam }) => (
    <div data-testid="workspace">{`${p.examId}/${p.documentId}/${p.exam?.title ?? "no exam"}`}</div>
  ),
}));

import { DraftReview } from "../draft-review";

function doc(over: Partial<ExamDocument> = {}): ExamDocument {
  return {
    id: "d1", examId: "e1", filename: "Physics.pdf", sizeBytes: 1000, pageCount: 3, status: "REVIEW_REQUIRED",
    progress: { done: 1, total: 1 }, attempt: 1, draftVersion: 1, uploadedBy: "u1",
    createdAt: "2026-03-01T10:00:00Z", updatedAt: "2026-03-01T10:00:00Z", ...over,
  };
}

const show = () => render(<DraftReview examId="e1" documentId="d1" />);

describe("DraftReview", () => {
  beforeEach(() => {
    state.docs = { documents: [doc()], loading: false, error: undefined, reload: () => {} };
    state.exam = { data: { id: "e1", title: "Physics Midterm", status: "DRAFT" }, loading: false, error: undefined };
  });

  it("has a heading, a way back to the exam, and says which paper and exam it is for", () => {
    show();
    expect(screen.getByRole("heading", { level: 1, name: "Review extracted questions" })).toBeInTheDocument();
    expect(screen.getByRole("link", { name: /back to the exam/i })).toHaveAttribute("href", "/exams/e1");
    expect(screen.getByText(/Physics\.pdf[\s\S]*3 pages[\s\S]*Physics Midterm/)).toBeInTheDocument();
  });

  it("opens the editor, with the exam, once the draft is ready", () => {
    show();
    expect(screen.getByTestId("workspace")).toHaveTextContent("e1/d1/Physics Midterm");
  });

  it("shows an approved draft in the same editor, which makes it read-only", () => {
    state.docs.documents = [doc({ status: "COMPLETED" })];
    show();
    expect(screen.getByTestId("workspace")).toBeInTheDocument();
  });

  it("shows a loading state", () => {
    state.docs = { documents: [], loading: true, error: undefined, reload: () => {} };
    show();
    expect(screen.getByRole("status")).toHaveTextContent(/loading/i);
    expect(screen.queryByTestId("workspace")).not.toBeInTheDocument();
  });

  it("says plainly when the paper does not exist", () => {
    state.docs.documents = [];
    show();
    expect(screen.getByRole("alert")).toHaveTextContent(/could not be found.*deleted/i);
  });

  it.each([
    ["QUEUED", undefined, /waiting to start/i],
    ["PROCESSING", "EXTRACTING", /reading the paper: finding the questions/i],
  ] as const)("while %s, shows where reading has got to and no editor", (status, stage, text) => {
    state.docs.documents = [doc({ status, stage })];
    show();
    expect(screen.getByRole("status")).toHaveTextContent(text);
    expect(screen.getByRole("status")).toHaveTextContent(/not ready to review yet/i);
    expect(screen.queryByTestId("workspace")).not.toBeInTheDocument();
  });

  it("moves from the waiting message to the editor as soon as the draft is ready", () => {
    state.docs.documents = [doc({ status: "PROCESSING", stage: "SAVING" })];
    const { rerender } = show();
    expect(screen.queryByTestId("workspace")).not.toBeInTheDocument();

    state.docs = { ...state.docs, documents: [doc({ status: "REVIEW_REQUIRED" })] };
    rerender(<DraftReview examId="e1" documentId="d1" />);
    expect(screen.getByTestId("workspace")).toBeInTheDocument();
  });

  it("explains a failure in the server's words and says what to do", () => {
    state.docs.documents = [
      doc({ status: "FAILED", failureCode: "NO_QUESTIONS_FOUND", failureMessage: "No questions could be found in this document." }),
    ];
    show();
    const alert = screen.getByRole("alert");
    expect(alert).toHaveTextContent("This paper could not be read.");
    expect(alert).toHaveTextContent("No questions could be found in this document.");
    expect(alert).toHaveTextContent(/add the questions yourself/i);
    expect(screen.queryByTestId("workspace")).not.toBeInTheDocument();
  });

  it("says a cancelled paper has no draft", () => {
    state.docs.documents = [doc({ status: "CANCELLED" })];
    show();
    expect(screen.getByRole("alert")).toHaveTextContent(/cancelled[\s\S]*no draft to review/i);
  });

  it("warns that the page may be out of date when it cannot refresh, but keeps the draft usable", () => {
    state.docs.error = { kind: "network", message: "Could not reach the server.", retryable: true, fields: {} };
    show();
    expect(screen.getAllByRole("alert")[0]).toHaveTextContent(/may be out of date/i);
    expect(screen.getByTestId("workspace")).toBeInTheDocument();
  });

  it("does not call a refusal a missing paper", () => {
    state.docs = {
      documents: [],
      loading: false,
      error: { kind: "forbidden", message: "You do not have permission to do this.", retryable: false, fields: {} },
      reload: () => {},
    };
    show();
    expect(screen.getByRole("alert")).toHaveTextContent(/do not have permission/i);
  });
});
