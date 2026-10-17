import { render, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { beforeEach, describe, expect, it, vi } from "vitest";
import { StudentResponseReview } from "../student-response-review";
import { responseReviewApi } from "@/services/response-review-api";
import { ApiError } from "@/services/api";
import type { MarkingSheet } from "@/types/marking";
import { responses } from "./fixtures";
import { finalSheet, markingQuestion, markingSheet } from "./marking-fixtures";

vi.mock("@/services/response-review-api", () => ({
  responseReviewApi: {
    listStudents: vi.fn(),
    getStudentResponses: vi.fn(),
    getMarking: vi.fn(),
    saveQuestionMarks: vi.fn(),
    finalise: vi.fn(),
    reopen: vi.fn(),
  },
}));

const api = vi.mocked(responseReviewApi);

/** The draft paper with the essay marked, so that it can be finalised. */
function completeDraft(): MarkingSheet {
  const s = markingSheet();
  s.questions[1] = markingQuestion({
    teacherMarks: 7.5,
    effectiveMarks: 7.5,
    markSource: "TEACHER",
    version: 1,
  });
  s.paper = { ...s.paper, version: 4, totalMarks: 9.5, markedCount: 3 };
  return s;
}

async function open(sheet: MarkingSheet) {
  api.getStudentResponses.mockResolvedValue(responses());
  api.getMarking.mockResolvedValue(sheet);
  const user = userEvent.setup();
  render(<StudentResponseReview examId="exam-1" studentId="s-1" />);
  await screen.findByLabelText("Paper marks");
  return user;
}

describe("finalising and reopening a paper", () => {
  beforeEach(() => {
    Object.values(api).forEach((fn) => fn.mockReset());
  });

  it("cannot finalise while a question has no mark, and names it", async () => {
    await open(markingSheet());
    expect(screen.getByRole("button", { name: "Finalise paper" })).toBeDisabled();
    expect(screen.getByText(/Not marked: 2\./)).toBeInTheDocument();
  });

  it("cannot finalise with unsaved changes", async () => {
    const user = await open(completeDraft());
    const button = screen.getByRole("button", { name: "Finalise paper" });
    expect(button).toBeEnabled();

    await user.type(screen.getByLabelText("Marks (out of 2)"), "1");
    expect(button).toBeDisabled();
    expect(screen.getByText("Save your changes first.")).toBeInTheDocument();
  });

  it("asks for confirmation, then finalises with the paper version it showed", async () => {
    const user = await open(completeDraft());
    api.finalise.mockResolvedValue(finalSheet());

    await user.click(screen.getByRole("button", { name: "Finalise paper" }));
    const dialog = await screen.findByRole("alertdialog");
    expect(within(dialog).getByText("Finalise this paper?")).toBeInTheDocument();
    expect(within(dialog).getByText("9.5 / 17")).toBeInTheDocument();
    expect(api.finalise).not.toHaveBeenCalled();

    await user.click(within(dialog).getByRole("button", { name: "Finalise" }));

    await waitFor(() => expect(screen.queryByRole("alertdialog")).not.toBeInTheDocument());
    expect(api.finalise).toHaveBeenCalledWith("exam-1", "s-1", {
      sessionId: "sess-1",
      expectedPaperVersion: 4,
    });
    expect(screen.getByText("Final")).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Reopen for changes" })).toBeInTheDocument();
    expect(screen.getByText(/Finalised .*2026/)).toBeInTheDocument();
    expect(screen.queryByRole("button", { name: "Finalise paper" })).not.toBeInTheDocument();
  });

  it("changes nothing when the teacher keeps marking", async () => {
    const user = await open(completeDraft());
    await user.click(screen.getByRole("button", { name: "Finalise paper" }));
    const dialog = await screen.findByRole("alertdialog");
    await user.click(within(dialog).getByRole("button", { name: "Keep marking" }));
    await waitFor(() => expect(screen.queryByRole("alertdialog")).not.toBeInTheDocument());
    expect(api.finalise).not.toHaveBeenCalled();
  });

  it("keeps the dialog open and says why when the server refuses", async () => {
    const user = await open(completeDraft());
    api.finalise.mockRejectedValue(
      new ApiError(
        "Every question needs a mark before finalising. Missing: 2",
        409,
        "PAPER_INCOMPLETE"
      )
    );
    api.getMarking.mockResolvedValue(completeDraft());

    await user.click(screen.getByRole("button", { name: "Finalise paper" }));
    const dialog = await screen.findByRole("alertdialog");
    await user.click(within(dialog).getByRole("button", { name: "Finalise" }));

    expect(await within(dialog).findByRole("alert")).toHaveTextContent(/Missing: 2/);
    expect(screen.getByText("Draft")).toBeInTheDocument();
  });

  it("will not reopen without a reason, then reopens with it trimmed", async () => {
    const user = await open(finalSheet());
    api.reopen.mockResolvedValue(completeDraft());

    await user.click(screen.getByRole("button", { name: "Reopen for changes" }));
    const dialog = await screen.findByRole("alertdialog");
    const confirm = within(dialog).getByRole("button", { name: "Reopen" });
    expect(confirm).toBeDisabled();

    const reason = within(dialog).getByLabelText("Reason for reopening");
    await user.type(reason, "ab");
    expect(confirm).toBeDisabled();
    await user.type(reason, "c  ");
    expect(confirm).toBeEnabled();

    await user.click(confirm);
    await waitFor(() => expect(screen.queryByRole("alertdialog")).not.toBeInTheDocument());
    expect(api.reopen).toHaveBeenCalledWith("exam-1", "s-1", {
      sessionId: "sess-1",
      reason: "abc",
    });
    expect(screen.getByText("Draft")).toBeInTheDocument();
    expect(screen.getByLabelText("Marks (out of 2)")).toBeInTheDocument();
  });

  it("reports a refused reopen without closing the dialog", async () => {
    const user = await open(finalSheet());
    api.reopen.mockRejectedValue(new ApiError("Not found", 404));
    await user.click(screen.getByRole("button", { name: "Reopen for changes" }));
    const dialog = await screen.findByRole("alertdialog");
    await user.type(within(dialog).getByLabelText("Reason for reopening"), "appeal upheld");
    await user.click(within(dialog).getByRole("button", { name: "Reopen" }));
    expect(await within(dialog).findByRole("alert")).toHaveTextContent("Not found");
    expect(screen.getByRole("alertdialog")).toBeInTheDocument();
  });
});
