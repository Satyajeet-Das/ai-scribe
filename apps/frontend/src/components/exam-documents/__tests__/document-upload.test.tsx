import { describe, it, expect, vi, beforeEach } from "vitest";
import { render, screen, waitFor, within, fireEvent } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { ApiError } from "@/services/api";
import type { ExamDocument } from "@/types/exam-document";

vi.mock("@/services/exam-documents-api", () => ({
  examDocumentsApi: { upload: vi.fn() },
}));

import { examDocumentsApi } from "@/services/exam-documents-api";
import { DocumentUpload } from "../document-upload";

const upload = vi.mocked(examDocumentsApi.upload);
const MB = 1024 * 1024;

function pdf(name = "Physics.pdf", size = 2 * MB): File {
  const f = new File(["%PDF-1.4"], name, { type: "application/pdf" });
  Object.defineProperty(f, "size", { value: size });
  return f;
}

const queued = { id: "d1", status: "QUEUED", filename: "Physics.pdf" } as ExamDocument;

function setup(props: Partial<React.ComponentProps<typeof DocumentUpload>> = {}) {
  const onUploaded = vi.fn();
  // Keep the accept filter off so a wrong file type reaches the component's own check.
  const user = userEvent.setup({ applyAccept: false });
  render(<DocumentUpload examId="exam-1" onUploaded={onUploaded} {...props} />);
  return { user, onUploaded, input: screen.getByLabelText(/exam paper \(pdf\)/i) as HTMLInputElement };
}

// One-shot, so a test also proves the upload was attempted exactly once.
function rejectWith(err: unknown) {
  upload.mockRejectedValueOnce(err);
}

describe("DocumentUpload", () => {
  beforeEach(() => {
    upload.mockReset();
  });

  it("states the rules before anything is chosen", () => {
    setup({ maxBytes: 20 * MB });
    expect(screen.getByText(/PDF only, up to 20.0 MB/i)).toBeInTheDocument();
    expect(screen.getByText(/nothing is added to the exam until you have reviewed and approved/i)).toBeInTheDocument();
    expect(screen.getByRole("button", { name: /upload paper/i })).toBeDisabled();
  });

  it("shows the chosen file's name and size, and enables upload", async () => {
    const { user, input } = setup();
    await user.upload(input, pdf("Midterm.pdf", 3 * MB));
    expect(screen.getByTestId("chosen-file-name")).toHaveTextContent("Midterm.pdf");
    expect(screen.getByText("3.0 MB")).toBeInTheDocument();
    expect(screen.getByRole("button", { name: /upload paper/i })).toBeEnabled();
    expect(screen.queryByRole("alert")).not.toBeInTheDocument();
  });

  it("refuses a file that is not a PDF, explains why, and does not allow upload", async () => {
    const { user, input } = setup();
    await user.upload(input, new File(["x"], "notes.docx", { type: "application/msword" }));
    const alert = screen.getByRole("alert");
    expect(alert).toHaveTextContent(/only pdf files/i);
    expect(input).toHaveAttribute("aria-invalid", "true");
    expect(input.getAttribute("aria-describedby")).toContain(alert.id);
    expect(screen.getByRole("button", { name: /upload paper/i })).toBeDisabled();
    expect(upload).not.toHaveBeenCalled();
  });

  it("refuses a file over the limit and says how big it is", async () => {
    const { user, input } = setup({ maxBytes: 5 * MB });
    await user.upload(input, pdf("Big.pdf", 9 * MB));
    expect(screen.getByRole("alert")).toHaveTextContent(/9\.0 MB.*5\.0 MB/);
    expect(screen.getByRole("button", { name: /upload paper/i })).toBeDisabled();
  });

  it("uploads, confirms that reading has started, and clears the selection", async () => {
    upload.mockResolvedValue(queued);
    const { user, input, onUploaded } = setup();
    await user.upload(input, pdf());
    await user.click(screen.getByRole("button", { name: /upload paper/i }));

    await waitFor(() => expect(onUploaded).toHaveBeenCalledWith(queued));
    expect(upload).toHaveBeenCalledTimes(1);
    expect(upload.mock.calls[0][0]).toBe("exam-1");
    expect(upload.mock.calls[0][1]).toMatchObject({ name: "Physics.pdf" });
    expect(screen.getByRole("status")).toHaveTextContent(/uploaded physics\.pdf.*reading has started/i);
    expect(screen.queryByTestId("chosen-file-name")).not.toBeInTheDocument();
    expect((screen.getByLabelText(/exam paper \(pdf\)/i) as HTMLInputElement).value).toBe("");
  });

  it("sends a double click or a repeated Enter only once", async () => {
    let finish!: (d: ExamDocument) => void;
    upload.mockReturnValue(new Promise((r) => (finish = r)));
    const { user, input, onUploaded } = setup();
    await user.upload(input, pdf());
    const button = screen.getByRole("button", { name: /upload paper/i });

    await user.dblClick(button);
    button.focus();
    await user.keyboard("{Enter}{Enter}");
    expect(upload).toHaveBeenCalledTimes(1);
    expect(screen.getByRole("button", { name: /uploading/i })).toBeDisabled();
    expect(screen.getByRole("button", { name: /uploading/i })).toHaveAttribute("aria-busy", "true");
    expect(input).toBeDisabled();

    finish(queued);
    await waitFor(() => expect(onUploaded).toHaveBeenCalledTimes(1));
  });

  it("offers a retry after a network failure, and a retry makes one document", async () => {
    upload.mockRejectedValueOnce(new ApiError("down", 0)).mockResolvedValueOnce(queued);
    const { user, input, onUploaded } = setup();
    await user.upload(input, pdf());
    await user.click(screen.getByRole("button", { name: /upload paper/i }));

    const alert = await screen.findByRole("alert");
    expect(alert).toHaveTextContent(/could not reach the server/i);
    expect(screen.getByTestId("chosen-file-name")).toBeInTheDocument(); // the file is kept
    await user.click(within(alert).getByRole("button", { name: /try again/i }));

    await waitFor(() => expect(onUploaded).toHaveBeenCalledTimes(1));
    expect(upload).toHaveBeenCalledTimes(2);
  });

  it("reports 'already uploaded' when an earlier attempt got through, and refreshes without a new document", async () => {
    rejectWith(new ApiError("dup", 409, "DOCUMENT_ALREADY_UPLOADED", true, [{ field: "existingDocumentId", error: "d9" }])
    );
    const { user, input, onUploaded } = setup();
    await user.upload(input, pdf());
    await user.click(screen.getByRole("button", { name: /upload paper/i }));

    expect(await screen.findByRole("alert")).toHaveTextContent(/already been uploaded/i);
    expect(onUploaded).toHaveBeenCalledWith(); // refresh only
    expect(upload).toHaveBeenCalledTimes(1);
    expect(screen.queryByTestId("chosen-file-name")).not.toBeInTheDocument();
    expect(screen.queryByRole("button", { name: /try again/i })).not.toBeInTheDocument();
  });

  it.each([
    ["INVALID_PDF", 422, /damaged, incomplete or password protected/i],
    ["PAYLOAD_TOO_LARGE", 413, /larger than the server accepts/i],
    ["UNSUPPORTED_MEDIA_TYPE", 415, /only pdf files/i],
    ["EXAM_NOT_DRAFT", 409, /no longer a draft/i],
    ["EXTRACTION_UNAVAILABLE", 503, /isn't set up on this server/i],
  ])("explains a %s rejection without offering a pointless retry", async (code, status, text) => {
    rejectWith(new ApiError("server words that should not be shown", status, code, true));
    const { user, input } = setup();
    await user.upload(input, pdf());
    await user.click(screen.getByRole("button", { name: /upload paper/i }));

    const alert = await screen.findByRole("alert");
    expect(alert).toHaveTextContent(text);
    expect(upload).toHaveBeenCalledTimes(1);
    expect(alert).not.toHaveTextContent(/server words/);
    expect(within(alert).queryByRole("button", { name: /try again/i })).not.toBeInTheDocument();
  });

  it("offers a retry for a temporary server problem, and never shows internal details", async () => {
    rejectWith(new ApiError("panic: runtime error at /app/internal/document/service.go:42", 500));
    const { user, input } = setup();
    await user.upload(input, pdf());
    await user.click(screen.getByRole("button", { name: /upload paper/i }));
    const alert = await screen.findByRole("alert");
    expect(alert).toHaveTextContent(/something went wrong on our side/i);
    expect(alert).not.toHaveTextContent(/panic|service\.go|runtime/);
    expect(within(alert).getByRole("button", { name: /try again/i })).toBeInTheDocument();
  });

  it("tells the teacher to wait when uploads are being rate limited", async () => {
    rejectWith(new ApiError("slow down", 429));
    const { user, input } = setup();
    await user.upload(input, pdf());
    await user.click(screen.getByRole("button", { name: /upload paper/i }));
    expect(await screen.findByRole("alert")).toHaveTextContent(/wait a minute/i);
    expect(upload).toHaveBeenCalledTimes(1);
  });

  it("can cancel an upload that is under way", async () => {
    upload.mockImplementation(
      (_exam, _file, signal) =>
        new Promise((_res, rej) => signal?.addEventListener("abort", () => rej(new ApiError("aborted", 0))))
    );
    const { user, input, onUploaded } = setup();
    await user.upload(input, pdf());
    await user.click(screen.getByRole("button", { name: /upload paper/i }));
    await user.click(screen.getByRole("button", { name: /cancel upload/i }));

    expect(await screen.findByRole("alert")).toHaveTextContent(/upload cancelled/i);
    expect(onUploaded).not.toHaveBeenCalled();
    expect(screen.getByRole("button", { name: /upload paper/i })).toBeEnabled(); // can try again
  });

  it("is switched off, with the reason, when the exam is not a draft", async () => {
    const { input } = setup({ disabledReason: "Papers can only be uploaded while the exam is a draft." });
    expect(input).toBeDisabled();
    expect(screen.getByText(/only be uploaded while the exam is a draft/i)).toBeInTheDocument();
    expect(screen.getByRole("button", { name: /upload paper/i })).toBeDisabled();
  });

  it("works from the keyboard: the file field and the buttons are reachable in order", async () => {
    upload.mockResolvedValue(queued);
    const { user, input, onUploaded } = setup();
    await user.tab();
    expect(input).toHaveFocus();

    await user.upload(input, pdf());
    await user.tab(); // Remove
    expect(screen.getByRole("button", { name: /remove physics\.pdf/i })).toHaveFocus();
    await user.tab(); // Upload
    const uploadButton = screen.getByRole("button", { name: /upload paper/i });
    expect(uploadButton).toHaveFocus();
    await user.keyboard(" ");
    await waitFor(() => expect(onUploaded).toHaveBeenCalled());
  });

  it("accepts one dropped PDF and refuses several", () => {
    setup();
    const zone = screen.getByLabelText(/exam paper \(pdf\)/i).parentElement!;
    fireEvent.drop(zone, { dataTransfer: { files: [pdf("Dropped.pdf")] } });
    expect(screen.getByTestId("chosen-file-name")).toHaveTextContent("Dropped.pdf");

    fireEvent.drop(zone, { dataTransfer: { files: [pdf("a.pdf"), pdf("b.pdf")] } });
    expect(screen.getByRole("alert")).toHaveTextContent(/one pdf at a time/i);
    expect(screen.queryByTestId("chosen-file-name")).not.toBeInTheDocument();
  });

  it("removes a chosen file", async () => {
    const { user, input } = setup();
    await user.upload(input, pdf());
    await user.click(screen.getByRole("button", { name: /remove physics\.pdf/i }));
    expect(screen.queryByTestId("chosen-file-name")).not.toBeInTheDocument();
    expect(screen.getByRole("button", { name: /upload paper/i })).toBeDisabled();
  });
});
