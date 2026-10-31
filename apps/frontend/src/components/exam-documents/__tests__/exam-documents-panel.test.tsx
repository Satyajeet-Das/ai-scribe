import { describe, it, expect, vi, beforeEach } from "vitest";
import { render, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import type { ExamDocument } from "@/types/exam-document";

const hook = vi.hoisted(() => ({
  value: { documents: [] as unknown[], loading: false, error: undefined as unknown, reload: () => {} },
  reload: undefined as unknown as ReturnType<typeof import("vitest").vi.fn>,
}));
vi.mock("@/hooks/use-exam-documents", () => ({ useExamDocuments: () => hook.value }));
vi.mock("@/services/exam-documents-api", () => ({
  examDocumentsApi: { upload: vi.fn(), cancel: vi.fn(), retry: vi.fn(), remove: vi.fn(), getFile: vi.fn() },
}));

import { examDocumentsApi } from "@/services/exam-documents-api";
import { ExamDocumentsPanel } from "../exam-documents-panel";

const doc = (over: Partial<ExamDocument> = {}): ExamDocument => ({
  id: "d1", examId: "e1", filename: "Physics.pdf", sizeBytes: 1000, pageCount: 3, status: "PROCESSING", stage: "EXTRACTING",
  progress: { done: 1, total: 3 }, attempt: 1, draftVersion: 0, uploadedBy: "u1",
  createdAt: "2026-03-01T10:00:00Z", updatedAt: "2026-03-01T10:00:00Z", ...over,
});

describe("ExamDocumentsPanel", () => {
  const reload = vi.fn();
  beforeEach(() => {
    reload.mockReset();
    hook.value = { documents: [], loading: false, error: undefined, reload };
    vi.mocked(examDocumentsApi.upload).mockReset();
  });

  it("is a labelled section with the upload and the uploaded papers", () => {
    render(<ExamDocumentsPanel examId="e1" examStatus="DRAFT" />);
    const section = screen.getByRole("region", { name: /import questions from a pdf/i });
    expect(within(section).getByLabelText(/exam paper \(pdf\)/i)).toBeEnabled();
    expect(within(section).getByRole("heading", { level: 3, name: "Uploaded papers" })).toBeInTheDocument();
    expect(within(section).getByText(/nothing is published automatically/i)).toBeInTheDocument();
  });

  it("lists what has been uploaded and how far it has got", () => {
    hook.value.documents = [doc()];
    render(<ExamDocumentsPanel examId="e1" examStatus="DRAFT" />);
    expect(screen.getByText("Physics.pdf")).toBeInTheDocument();
    expect(screen.getByText(/part 1 of 3 done/i)).toBeInTheDocument();
  });

  it.each(["PUBLISHED", "ARCHIVED"] as const)("cannot upload while the exam is %s, and says why", (status) => {
    render(<ExamDocumentsPanel examId="e1" examStatus={status} />);
    expect(screen.getByLabelText(/exam paper \(pdf\)/i)).toBeDisabled();
    expect(screen.getByText(/only be uploaded while the exam is a draft/i)).toBeInTheDocument();
  });

  it("still shows earlier papers when the exam is no longer a draft", () => {
    hook.value.documents = [doc({ status: "COMPLETED", approvedAt: "2026-03-02T10:00:00Z" })];
    render(<ExamDocumentsPanel examId="e1" examStatus="PUBLISHED" />);
    expect(screen.getByRole("link", { name: /view approved draft/i })).toBeInTheDocument();
  });

  it("refreshes the list after an upload", async () => {
    vi.mocked(examDocumentsApi.upload).mockResolvedValue(doc({ status: "QUEUED" }));
    const user = userEvent.setup({ applyAccept: false });
    render(<ExamDocumentsPanel examId="e1" examStatus="DRAFT" />);
    await user.upload(
      screen.getByLabelText(/exam paper \(pdf\)/i),
      new File(["%PDF-1.4"], "Paper.pdf", { type: "application/pdf" })
    );
    await user.click(screen.getByRole("button", { name: /upload paper/i }));
    await waitFor(() => expect(reload).toHaveBeenCalledTimes(1));
  });

  it("warns when the list cannot be refreshed", () => {
    hook.value.documents = [doc()];
    hook.value.error = { kind: "network", message: "Could not reach the server.", retryable: true, fields: {} };
    render(<ExamDocumentsPanel examId="e1" examStatus="DRAFT" />);
    expect(screen.getByRole("alert")).toHaveTextContent(/may be out of date/i);
  });
});
