import { render, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { beforeEach, describe, expect, it, vi } from "vitest";
import { StudentResponseReview } from "../student-response-review";
import { responseReviewApi } from "@/services/response-review-api";
import { ApiError } from "@/services/api";
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

async function open(sheet = markingSheet()) {
  api.getStudentResponses.mockResolvedValue(responses());
  api.getMarking.mockResolvedValue(sheet);
  const user = userEvent.setup();
  render(<StudentResponseReview examId="exam-1" studentId="s-1" />);
  await screen.findByRole("heading", { name: /Question 1/ });
  if (sheet.markingAllowed) await screen.findByLabelText("Paper marks");
  else await waitFor(() => expect(api.getMarking).toHaveBeenCalled());
  return user;
}

async function goToQuestion(user: ReturnType<typeof userEvent.setup>, n: number) {
  const nav = screen.getByRole("navigation", { name: "Question navigator" });
  await user.click(within(nav).getByRole("button", { name: new RegExp(`^Question ${n},`) }));
  await screen.findByRole("heading", { name: new RegExp(`Question ${n}`) });
}

describe("marking in the response review", () => {
  beforeEach(() => {
    Object.values(api).forEach((fn) => fn.mockReset());
  });

  it("shows the server's totals and whether the paper is a draft", async () => {
    await open();
    const summary = screen.getByLabelText("Paper marks");
    expect(within(summary).getByTestId("paper-total")).toHaveTextContent("2 / 17");
    expect(within(summary).getByText("2 of 3")).toBeInTheDocument();
    expect(within(summary).getByText("Draft")).toBeInTheDocument();
  });

  it("shows an auto-marked question with its mark and an option to use it", async () => {
    const user = await open();
    expect(screen.getByTestId("current-mark")).toHaveTextContent("Counts: 2 of 2");
    expect(screen.getByLabelText("Marks (out of 2)")).toHaveValue("");
    expect(screen.getByText(/Auto-marked: 2 of 2/)).toBeInTheDocument();

    await user.click(screen.getByRole("button", { name: "Use auto mark (2)" }));
    expect(screen.getByLabelText("Marks (out of 2)")).toHaveValue("2");
    expect(api.saveQuestionMarks).not.toHaveBeenCalled();
    expect(screen.getByText("Unsaved changes")).toBeInTheDocument();
  });

  it("marks an essay: sends the attempt and version, then shows the new totals and 'Saved'", async () => {
    const user = await open();
    await goToQuestion(user, 2);
    expect(screen.getByTestId("current-mark")).toHaveTextContent("Not marked yet");

    api.saveQuestionMarks.mockResolvedValue({
      question: markingQuestion({
        teacherMarks: 7.5,
        effectiveMarks: 7.5,
        markSource: "TEACHER",
        comment: "Good",
        version: 1,
      }),
      paper: { ...markingSheet().paper, version: 1, totalMarks: 9.5, markedCount: 3 },
    });

    await user.type(screen.getByLabelText("Marks (out of 10)"), "7.5");
    await user.type(screen.getByLabelText(/Feedback for the student/), "Good");
    await user.click(screen.getByRole("button", { name: "Save marks" }));

    await waitFor(() => expect(screen.getByText("Saved")).toBeInTheDocument());
    expect(api.saveQuestionMarks).toHaveBeenCalledWith("exam-1", "s-1", "q2", {
      sessionId: "sess-1",
      marks: 7.5,
      comment: "Good",
      expectedVersion: 0,
    });
    expect(screen.getByTestId("paper-total")).toHaveTextContent("9.5 / 17");
    expect(screen.getByText("3 of 3")).toBeInTheDocument();
    expect(screen.getByTestId("current-mark")).toHaveTextContent("Counts: 7.5 of 10");
    expect(screen.getByLabelText("Marks (out of 10)")).toHaveValue("7.5");
  });

  it("saves with Enter in the marks field", async () => {
    const user = await open();
    await goToQuestion(user, 2);
    api.saveQuestionMarks.mockResolvedValue({
      question: markingQuestion({ teacherMarks: 4, effectiveMarks: 4, version: 1 }),
      paper: markingSheet().paper,
    });
    await user.type(screen.getByLabelText("Marks (out of 10)"), "4{Enter}");
    await waitFor(() => expect(api.saveQuestionMarks).toHaveBeenCalledTimes(1));
  });

  it.each([
    ["7.25", /whole number or a half mark/],
    ["-1", /cannot be negative/],
    ["11", /cannot be more than 10/],
    ["abc", /whole number or a half mark/],
  ])("explains %j without sending anything", async (typed, message) => {
    const user = await open();
    await goToQuestion(user, 2);
    const field = screen.getByLabelText("Marks (out of 10)");
    await user.type(field, typed);

    expect(field).toHaveAttribute("aria-invalid", "true");
    expect(screen.getByRole("alert")).toHaveTextContent(message);
    await user.click(screen.getByRole("button", { name: "Save marks" }));
    expect(api.saveQuestionMarks).not.toHaveBeenCalled();
  });

  it("keeps what was typed when moving to another question and back", async () => {
    const user = await open();
    await goToQuestion(user, 2);
    await user.type(screen.getByLabelText("Marks (out of 10)"), "6");

    await goToQuestion(user, 3);
    const nav = screen.getByRole("navigation", { name: "Question navigator" });
    expect(
      within(nav).getByRole("button", { name: /^Question 2, .*unsaved changes/ })
    ).toBeInTheDocument();
    expect(screen.getByLabelText("Marks (out of 5)")).toHaveValue("");

    await goToQuestion(user, 2);
    expect(screen.getByLabelText("Marks (out of 10)")).toHaveValue("6");
  });

  it("labels the navigator buttons with each question's marking state", async () => {
    await open();
    const nav = screen.getByRole("navigation", { name: "Question navigator" });
    expect(
      within(nav).getByRole("button", { name: "Question 1, answered, marked 2 of 2" })
    ).toBeInTheDocument();
    expect(
      within(nav).getByRole("button", { name: "Question 2, answered, not marked yet" })
    ).toBeInTheDocument();
    expect(
      within(nav).getByRole("button", { name: "Question 3, not answered, marked 0 of 5" })
    ).toBeInTheDocument();
  });

  it("keeps the typed text and offers the latest when the mark was changed elsewhere", async () => {
    const user = await open();
    await goToQuestion(user, 2);
    api.saveQuestionMarks.mockRejectedValue(new ApiError("changed", 409, "STALE_VERSION"));
    await user.type(screen.getByLabelText("Marks (out of 10)"), "6");
    await user.click(screen.getByRole("button", { name: "Save marks" }));

    expect(
      await screen.findByText(/changed elsewhere/i, { selector: "span[role=alert]" })
    ).toBeInTheDocument();
    expect(screen.getByLabelText("Marks (out of 10)")).toHaveValue("6");

    const latest = markingSheet();
    latest.questions[1] = markingQuestion({
      teacherMarks: 9,
      effectiveMarks: 9,
      version: 3,
      markSource: "TEACHER",
    });
    api.getMarking.mockResolvedValue(latest);
    await user.click(screen.getByRole("button", { name: "Load latest" }));
    await waitFor(() => expect(screen.getByLabelText("Marks (out of 10)")).toHaveValue("9"));
  });

  it("warns when a saved mark is above what the question is now worth", async () => {
    const sheet = markingSheet();
    sheet.questions[1] = markingQuestion({
      teacherMarks: 9,
      maxMarks: 5,
      exceedsMax: true,
      version: 2,
      markSource: "TEACHER",
    });
    const user = await open(sheet);
    await goToQuestion(user, 2);
    expect(screen.getByRole("alert")).toHaveTextContent(/now worth 5/);
  });

  it("asks for a mark when a multiple-choice question has no stored answer key", async () => {
    const sheet = markingSheet();
    sheet.questions[0] = {
      ...sheet.questions[0],
      suggestedMarks: null,
      effectiveMarks: null,
      markSource: "NONE",
      correctOptionId: undefined,
    };
    await open(sheet);
    expect(screen.getByText(/No correct option is stored/)).toBeInTheDocument();
    expect(screen.queryByRole("button", { name: /Use auto mark/ })).not.toBeInTheDocument();
  });

  it("shows no marking while the student is still sitting the exam", async () => {
    await open(markingSheet({ markingAllowed: false, reason: "SESSION_NOT_ENDED", questions: [] }));
    expect(screen.queryByLabelText("Paper marks")).not.toBeInTheDocument();
    expect(screen.queryByRole("button", { name: "Save marks" })).not.toBeInTheDocument();
  });

  it("shows a finalised paper read-only with the frozen marks", async () => {
    const user = await open(finalSheet());
    expect(screen.getByText("Final")).toBeInTheDocument();
    expect(screen.getByTestId("paper-total")).toHaveTextContent("9.5 / 17");
    expect(screen.queryByRole("button", { name: "Save marks" })).not.toBeInTheDocument();
    expect(screen.queryByLabelText(/Marks \(out of/)).not.toBeInTheDocument();
    expect(screen.getByTestId("final-marks")).toHaveTextContent("2 of 2");

    await goToQuestion(user, 2);
    expect(screen.getByTestId("final-marks")).toHaveTextContent("7.5 of 10");
  });

  it("still shows the answers when marking cannot be loaded, and says so", async () => {
    api.getStudentResponses.mockResolvedValue(responses());
    api.getMarking.mockRejectedValue(new ApiError("boom", 500));
    render(<StudentResponseReview examId="exam-1" studentId="s-1" />);
    expect(await screen.findByRole("heading", { name: /Question 1/ })).toBeInTheDocument();
    expect(await screen.findByText(/Marking could not be loaded/)).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Try again" })).toBeInTheDocument();
  });
});
