import { describe, it, expect, vi, beforeEach } from "vitest";
import { render, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { ApiError } from "@/services/api";
import type { ExamDocument } from "@/types/exam-document";

vi.mock("@/services/exam-documents-api", () => ({
  examDocumentsApi: {
    cancel: vi.fn(), retry: vi.fn(), remove: vi.fn(), getFile: vi.fn(),
  },
}));

import { examDocumentsApi } from "@/services/exam-documents-api";
import { DocumentList } from "../document-list";

const api = vi.mocked(examDocumentsApi);

function doc(over: Partial<ExamDocument> = {}): ExamDocument {
  return {
    id: "d1", examId: "e1", filename: "Physics.pdf", sizeBytes: 2 * 1024 * 1024, pageCount: 4, status: "QUEUED",
    progress: { done: 0, total: 4 }, attempt: 1, draftVersion: 0, uploadedBy: "u1",
    createdAt: "2026-03-01T10:00:00Z", updatedAt: "2026-03-01T10:00:00Z", ...over,
  };
}

function setup(documents: ExamDocument[], extra: Partial<React.ComponentProps<typeof DocumentList>> = {}) {
  const onChanged = vi.fn();
  const utils = render(
    <DocumentList examId="e1" documents={documents} loading={false} onChanged={onChanged} {...extra} />
  );
  return { onChanged, ...utils, user: userEvent.setup() };
}

describe("DocumentList", () => {
  beforeEach(() => Object.values(api).forEach((f) => f.mockReset()));

  it("says so when nothing has been uploaded", () => {
    setup([]);
    expect(screen.getByText(/no papers uploaded/i)).toBeInTheDocument();
  });

  it("shows a loading state", () => {
    render(<DocumentList examId="e1" documents={[]} loading onChanged={vi.fn()} />);
    expect(screen.getByRole("status")).toHaveTextContent(/loading uploaded papers/i);
  });

  it("shows name, size and pages as a heading, with the state in words", () => {
    setup([doc()]);
    expect(screen.getByRole("heading", { name: "Physics.pdf", level: 3 })).toBeInTheDocument();
    expect(screen.getByText(/2\.0 MB · 4 pages/)).toBeInTheDocument();
    expect(screen.getByText("Waiting to start")).toBeInTheDocument();
  });

  it.each([
    ["QUEUED", "Waiting to start"],
    ["PROCESSING", "Reading the paper"],
    ["REVIEW_REQUIRED", "Ready for review"],
    ["COMPLETED", "Approved"],
    ["FAILED", "Failed"],
    ["CANCELLED", "Cancelled"],
  ] as const)("labels %s in words, not colour alone", (status, label) => {
    setup([doc({ status, failureCode: status === "FAILED" ? "INVALID_PDF" : undefined })]);
    expect(screen.getByText(label)).toBeInTheDocument();
  });

  describe("progress", () => {
    it("shows the numbers only while finding the questions, where they mean something", () => {
      setup([doc({ status: "PROCESSING", stage: "EXTRACTING", progress: { done: 1, total: 3 } })]);
      expect(screen.getByText("Finding the questions…")).toBeInTheDocument();
      expect(screen.getByText(/part 1 of 3 done/i)).toBeInTheDocument();
      expect(screen.getByRole("progressbar", { name: /reading progress/i })).toBeInTheDocument();
    });

    it("does not show '0 of N' while the pages are being read", () => {
      setup([doc({ status: "PROCESSING", stage: "READING_PAGES", progress: { done: 0, total: 12 } })]);
      expect(screen.getByText("Reading the pages…")).toBeInTheDocument();
      expect(screen.queryByRole("progressbar")).not.toBeInTheDocument();
      expect(screen.queryByText(/of 12/)).not.toBeInTheDocument();
    });

    it("does not invent progress for a queued document", () => {
      setup([doc({ status: "QUEUED" })]);
      expect(screen.queryByRole("progressbar")).not.toBeInTheDocument();
      expect(screen.getByText(/you can leave this page/i)).toBeInTheDocument();
    });
  });

  describe("actions only where the backend supports them", () => {
    it("queued and processing documents can be cancelled, not reviewed or retried", () => {
      setup([doc({ id: "a", status: "QUEUED" }), doc({ id: "b", status: "PROCESSING", stage: "SAVING" })]);
      expect(screen.getAllByRole("button", { name: /^cancel/i })).toHaveLength(2);
      expect(screen.queryByRole("link", { name: /review questions/i })).not.toBeInTheDocument();
      expect(screen.queryByRole("button", { name: /try again/i })).not.toBeInTheDocument();
    });

    it("a failed document says why, says what to do, and can be retried", async () => {
      api.retry.mockResolvedValue(doc({ status: "QUEUED", attempt: 2 }));
      const { user, onChanged } = setup([
        doc({ status: "FAILED", failureCode: "PROVIDER_RATE_LIMITED", failureMessage: "The reading service is busy right now. Try again in a few minutes." }),
      ]);
      expect(screen.getByText(/reading service is busy right now/i)).toBeInTheDocument();
      expect(screen.getByText(/usually temporary/i)).toBeInTheDocument();

      await user.click(screen.getByRole("button", { name: /try again/i }));
      await waitFor(() => expect(onChanged).toHaveBeenCalled());
      expect(api.retry).toHaveBeenCalledWith("e1", "d1");
      expect(screen.getByText("Processing started again.")).toBeInTheDocument();
    });

    it("gives configuration failures to the administrator, not the teacher", () => {
      setup([doc({ status: "FAILED", failureCode: "MODEL_NOT_FOUND", failureMessage: "The configured reading model is no longer available." })]);
      expect(screen.getByText(/ask an administrator/i)).toBeInTheDocument();
    });

    it("a cancelled document can be retried", () => {
      setup([doc({ status: "CANCELLED" })]);
      expect(screen.getByRole("button", { name: /try again/i })).toBeInTheDocument();
    });

    it("a document ready for review links to its review page", () => {
      setup([doc({ id: "doc 1", status: "REVIEW_REQUIRED", draftVersion: 1 })]);
      const link = screen.getByRole("link", { name: /review questions/i });
      expect(link).toHaveAttribute("href", "/exams/e1/documents/doc%201");
      expect(screen.queryByRole("button", { name: /^cancel/i })).not.toBeInTheDocument();
    });

    it("an approved document can only be viewed", () => {
      setup([doc({ status: "COMPLETED", approvedAt: "2026-03-02T10:00:00Z" })]);
      expect(screen.getByRole("link", { name: /view approved draft/i })).toBeInTheDocument();
      expect(screen.getByText(/approved into the exam on/i)).toBeInTheDocument();
      expect(screen.queryByRole("button", { name: /try again|read again|cancel/i })).not.toBeInTheDocument();
    });

    it("cancels a queued document", async () => {
      api.cancel.mockResolvedValue(doc({ status: "CANCELLED" }));
      const { user, onChanged } = setup([doc({ status: "PROCESSING", stage: "EXTRACTING" })]);
      await user.click(screen.getByRole("button", { name: /^cancel/i }));
      await waitFor(() => expect(onChanged).toHaveBeenCalled());
      expect(api.cancel).toHaveBeenCalledWith("e1", "d1");
    });
  });

  describe("destructive actions are confirmed first", () => {
    it("deleting needs a confirmation that says approved questions stay", async () => {
      api.remove.mockResolvedValue(undefined);
      const { user, onChanged } = setup([doc({ status: "REVIEW_REQUIRED", draftVersion: 2 })]);
      await user.click(screen.getByRole("button", { name: /^delete/i }));
      const dialog = await screen.findByRole("dialog");
      expect(within(dialog).getByText(/already approved into the exam stay/i)).toBeInTheDocument();
      expect(api.remove).not.toHaveBeenCalled();

      await user.click(within(dialog).getByRole("button", { name: /delete paper/i }));
      await waitFor(() => expect(api.remove).toHaveBeenCalledWith("e1", "d1"));
      await waitFor(() => expect(onChanged).toHaveBeenCalled());
    });

    it("backing out of a delete does nothing", async () => {
      const { user } = setup([doc({ status: "FAILED", failureCode: "INVALID_PDF" })]);
      await user.click(screen.getByRole("button", { name: /^delete/i }));
      const dialog = await screen.findByRole("dialog");
      await user.click(within(dialog).getByRole("button", { name: /^cancel$/i }));
      await waitFor(() => expect(screen.queryByRole("dialog")).not.toBeInTheDocument());
      expect(api.remove).not.toHaveBeenCalled();
    });

    it("reading a reviewed paper again warns that the draft and corrections are lost, and sends the version", async () => {
      api.retry.mockResolvedValue(doc({ status: "QUEUED", attempt: 2 }));
      const { user } = setup([doc({ status: "REVIEW_REQUIRED", draftVersion: 7 })]);
      await user.click(screen.getByRole("button", { name: /read .* again/i }));
      const dialog = await screen.findByRole("dialog");
      expect(within(dialog).getByText(/corrections you have made .* will be thrown away/i)).toBeInTheDocument();
      expect(api.retry).not.toHaveBeenCalled();

      await user.click(within(dialog).getByRole("button", { name: /discard draft and read again/i }));
      await waitFor(() => expect(api.retry).toHaveBeenCalledWith("e1", "d1", { discardDraft: true, expectedDraftVersion: 7 }));
    });

    it("shows the reason when the server refuses (the draft moved on)", async () => {
      api.retry.mockRejectedValue(new ApiError("stale", 409, "STALE_VERSION", true));
      const { user } = setup([doc({ status: "REVIEW_REQUIRED", draftVersion: 7 })]);
      await user.click(screen.getByRole("button", { name: /read .* again/i }));
      await user.click(within(await screen.findByRole("dialog")).getByRole("button", { name: /discard draft/i }));
      expect(await screen.findByText(/changed somewhere else/i)).toBeInTheDocument();
    });
  });

  it("allows only one action at a time", async () => {
    let finish!: (d: ExamDocument) => void;
    api.cancel.mockReturnValue(new Promise((r) => (finish = r)));
    const { user } = setup([doc({ id: "a", filename: "A.pdf", status: "QUEUED" }), doc({ id: "b", filename: "B.pdf", status: "QUEUED" })]);
    const [first, second] = screen.getAllByRole("button", { name: /^cancel/i });
    await user.click(first);
    expect(first).toHaveAttribute("aria-busy", "true");
    expect(second).toBeDisabled();
    await user.click(second);
    expect(api.cancel).toHaveBeenCalledTimes(1);
    finish(doc({ status: "CANCELLED" }));
    await waitFor(() => expect(second).toBeEnabled());
  });

  it("downloads the original through the API, not a storage address", async () => {
    api.getFile.mockResolvedValue(new Blob(["%PDF"], { type: "application/pdf" }));
    const create = vi.fn(() => "blob:mem");
    const revoke = vi.fn();
    vi.stubGlobal("URL", Object.assign(URL, { createObjectURL: create, revokeObjectURL: revoke }));
    const click = vi.spyOn(HTMLAnchorElement.prototype, "click").mockImplementation(() => {});
    const { user } = setup([doc({ status: "REVIEW_REQUIRED" })]);
    await user.click(screen.getByRole("button", { name: /download pdf/i }));
    await waitFor(() => expect(click).toHaveBeenCalled());
    expect(api.getFile).toHaveBeenCalledWith("e1", "d1");
    expect(create).toHaveBeenCalled();
    click.mockRestore();
    vi.unstubAllGlobals();
  });

  describe("screen reader announcements", () => {
    it("says nothing on first load", () => {
      setup([doc({ status: "PROCESSING", stage: "EXTRACTING" })]);
      expect(screen.getByTestId("status-announcer")).toHaveTextContent("");
    });

    it("announces a change of state once, and not a change of progress", () => {
      const first = doc({ status: "PROCESSING", stage: "EXTRACTING", progress: { done: 1, total: 4 } });
      const { rerender } = render(<DocumentList examId="e1" documents={[first]} loading={false} onChanged={vi.fn()} />);

      rerender(<DocumentList examId="e1" documents={[{ ...first, progress: { done: 2, total: 4 } }]} loading={false} onChanged={vi.fn()} />);
      expect(screen.getByTestId("status-announcer")).toHaveTextContent("");

      rerender(<DocumentList examId="e1" documents={[{ ...first, status: "REVIEW_REQUIRED" }]} loading={false} onChanged={vi.fn()} />);
      expect(screen.getByTestId("status-announcer")).toHaveTextContent("Physics.pdf: ready for review.");
      expect(screen.getByTestId("status-announcer").closest("[aria-live='polite']")).not.toBeNull();
    });

    it("does not announce a document that has just appeared in the list", () => {
      const a = doc({ id: "a", filename: "A.pdf", status: "REVIEW_REQUIRED" });
      const { rerender } = render(<DocumentList examId="e1" documents={[a]} loading={false} onChanged={vi.fn()} />);
      rerender(<DocumentList examId="e1" documents={[a, doc({ id: "b", filename: "B.pdf" })]} loading={false} onChanged={vi.fn()} />);
      expect(screen.getByTestId("status-announcer")).toHaveTextContent("");
    });
  });

  it("warns that the list may be out of date when a refresh fails", () => {
    setup([doc()], { refreshFailed: "Could not reach the server." });
    expect(screen.getByRole("alert")).toHaveTextContent(/may be out of date/i);
  });
});
