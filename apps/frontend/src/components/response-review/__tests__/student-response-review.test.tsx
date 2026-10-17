import { render, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, it, expect, vi, beforeEach } from "vitest";
import { StudentResponseReview } from "../student-response-review";
import { responseReviewApi } from "@/services/response-review-api";
import { ApiError } from "@/services/api";
import type { ReviewStudentResponses } from "@/types/response-review";
import { Q1, Q2, Q3, deferred, responses } from "./fixtures";

vi.mock("@/services/response-review-api", () => ({
  responseReviewApi: { listStudents: vi.fn(), getStudentResponses: vi.fn() },
}));

const getResponses = vi.mocked(responseReviewApi.getStudentResponses);

function navigator() {
  return screen.getByRole("navigation", { name: "Question navigator" });
}

describe("StudentResponseReview", () => {
  beforeEach(() => {
    getResponses.mockReset();
  });

  it("keeps the student and exam visible and shows the first question", async () => {
    getResponses.mockResolvedValue(responses());
    render(<StudentResponseReview examId="exam-1" studentId="s-1" />);

    expect(await screen.findByRole("heading", { name: "Zoe Adams" })).toBeInTheDocument();
    expect(screen.getByText(/Roll no\. R001/)).toBeInTheDocument();
    expect(screen.getByText("Physics Midterm")).toBeInTheDocument();
    expect(screen.getByText("Submitted")).toBeInTheDocument();
    expect(screen.getByText(/Submitted .*2026/)).toBeInTheDocument();

    const counts = screen.getByLabelText("Answer counts");
    expect(within(counts).getByText("Questions").nextSibling).toHaveTextContent("3");
    expect(within(counts).getByText("Answered").nextSibling).toHaveTextContent("2");
    expect(within(counts).getByText("Not answered").nextSibling).toHaveTextContent("1");

    expect(screen.getByRole("heading", { name: /Question 1/ })).toBeInTheDocument();
    expect(screen.getByRole("link", { name: /All students/ })).toHaveAttribute(
      "href",
      "/exams/exam-1/responses"
    );
  });

  it("marks the chosen option and shows the saved answer for a multiple-choice question", async () => {
    getResponses.mockResolvedValue(responses());
    render(<StudentResponseReview examId="exam-1" studentId="s-1" />);
    await screen.findByText(Q1.text);

    const options = within(screen.getByRole("list", { name: "Options" })).getAllByRole("listitem");
    expect(options[1]).toHaveTextContent("Mercury");
    expect(options[1]).toHaveTextContent("Student's answer");
    expect(options[0]).not.toHaveTextContent("Student's answer");
    expect(screen.getByText(/Last saved/)).toBeInTheDocument();
    expect(screen.getByText("2 marks")).toBeInTheDocument();
  });

  it("never shows a correct-answer marker unless the backend sent one", async () => {
    getResponses.mockResolvedValue(responses());
    render(<StudentResponseReview examId="exam-1" studentId="s-1" />);
    await screen.findByText(Q1.text);
    expect(screen.queryByText(/Correct option/)).not.toBeInTheDocument();
  });

  it("distinguishes answered, unanswered and current questions without relying on color", async () => {
    getResponses.mockResolvedValue(responses());
    const user = userEvent.setup();
    render(<StudentResponseReview examId="exam-1" studentId="s-1" />);
    await screen.findByText(Q1.text);

    const q1 = within(navigator()).getByRole("button", { name: "Question 1, answered" });
    const q3 = within(navigator()).getByRole("button", { name: "Question 3, not answered" });
    expect(q1).toHaveAttribute("aria-current", "true");
    expect(q3).not.toHaveAttribute("aria-current");

    await user.click(q3);
    expect(q3).toHaveAttribute("aria-current", "true");
    expect(q1).not.toHaveAttribute("aria-current");
    expect(screen.getByText("No answer saved")).toBeInTheDocument();
    expect(screen.getByText("The student did not answer this question.")).toBeInTheDocument();
    expect(screen.queryByText(/Last saved/)).not.toBeInTheDocument();
  });

  it("preserves line breaks and long text in written answers", async () => {
    const long = "word ".repeat(400).trim();
    getResponses.mockResolvedValue(
      responses({
        questions: [
          Q1,
          { ...Q2, answer: { status: "ANSWERED", textAnswer: `${Q2.answer.textAnswer}\n${long}` } },
          Q3,
        ],
      })
    );
    const user = userEvent.setup();
    render(<StudentResponseReview examId="exam-1" studentId="s-1" />);
    await screen.findByText(Q1.text);
    await user.click(within(navigator()).getByRole("button", { name: /Question 2/ }));

    const answer = screen.getByText(/Line one of the answer\./);
    expect(answer.textContent).toContain("E = mc^2.");
    expect(answer.className).toContain("whitespace-pre-wrap");
    expect(answer.className).toContain("break-words");
    // The question text keeps its formula characters and breaks verbatim.
    const q = screen.getByText(/Explain gravity\./);
    expect(q.textContent).toContain("x^2 + y^2 = z^2");
    expect(q.className).toContain("whitespace-pre-wrap");
  });

  it("moves between questions with Previous and Next and stays within bounds", async () => {
    getResponses.mockResolvedValue(responses());
    const user = userEvent.setup();
    render(<StudentResponseReview examId="exam-1" studentId="s-1" />);
    await screen.findByText(Q1.text);

    const prev = screen.getByRole("button", { name: /Previous/ });
    const next = screen.getByRole("button", { name: /Next/ });
    expect(prev).toBeDisabled();
    expect(screen.getByText("1 / 3")).toBeInTheDocument();

    await user.click(next);
    expect(screen.getByRole("heading", { name: /Question 2/ })).toBeInTheDocument();
    await user.click(next);
    expect(screen.getByRole("heading", { name: /Question 3/ })).toBeInTheDocument();
    expect(next).toBeDisabled();
    await user.click(prev);
    expect(screen.getByRole("heading", { name: /Question 2/ })).toBeInTheDocument();
  });

  it("announces the question once when it changes", async () => {
    getResponses.mockResolvedValue(responses());
    const user = userEvent.setup();
    render(<StudentResponseReview examId="exam-1" studentId="s-1" />);
    await screen.findByText(Q1.text);

    const live = screen.getByText(/Question 1 of 3, answered\./);
    expect(live).toHaveAttribute("aria-live", "polite");
    await user.click(screen.getByRole("button", { name: /Next/ }));
    expect(screen.getByText(/Question 2 of 3, answered\./)).toBeInTheDocument();
  });

  it("can be driven entirely from the keyboard", async () => {
    getResponses.mockResolvedValue(responses());
    const user = userEvent.setup();
    render(<StudentResponseReview examId="exam-1" studentId="s-1" />);
    await screen.findByText(Q1.text);

    const q3 = within(navigator()).getByRole("button", { name: "Question 3, not answered" });
    for (let i = 0; i < 20 && document.activeElement !== q3; i++) await user.tab();
    expect(q3).toHaveFocus();
    await user.keyboard("{Enter}");
    expect(screen.getByRole("heading", { name: /Question 3/ })).toBeInTheDocument();
  });

  it("flags a session that is still in progress as not final", async () => {
    getResponses.mockResolvedValue(
      responses({
        status: "IN_PROGRESS",
        session: {
          id: "x",
          status: "IN_PROGRESS",
          startedAt: "2026-03-01T09:00:00Z",
          isFinal: false,
        },
      })
    );
    render(<StudentResponseReview examId="exam-1" studentId="s-1" />);
    const banner = await screen.findByText(/This exam is still in progress\./);
    expect(banner.closest("[role=status]")).toHaveTextContent(/not final/);
    expect(screen.getByText("In progress")).toBeInTheDocument();
    expect(screen.queryByText(/Submitted .*2026/)).not.toBeInTheDocument();
  });

  it("does not show the in-progress banner for a submitted exam", async () => {
    getResponses.mockResolvedValue(responses());
    render(<StudentResponseReview examId="exam-1" studentId="s-1" />);
    await screen.findByText(Q1.text);
    expect(screen.queryByText(/still in progress/)).not.toBeInTheDocument();
  });

  it("explains when a student has not started, without inventing answers", async () => {
    getResponses.mockResolvedValue(
      responses({
        status: "NOT_STARTED",
        session: undefined,
        responseAvailable: false,
        unavailableReason: "NOT_STARTED",
        counts: undefined,
        questions: [],
      })
    );
    render(<StudentResponseReview examId="exam-1" studentId="s-1" />);
    expect(await screen.findByText(/has not started this exam/)).toBeInTheDocument();
    expect(screen.getByText("Responses not available")).toBeInTheDocument();
    expect(
      screen.queryByRole("navigation", { name: "Question navigator" })
    ).not.toBeInTheDocument();
    expect(screen.getByRole("link", { name: "Back to all students" })).toBeInTheDocument();
  });

  it("explains when in-progress review is switched off", async () => {
    getResponses.mockResolvedValue(
      responses({
        status: "IN_PROGRESS",
        responseAvailable: false,
        unavailableReason: "IN_PROGRESS_REVIEW_DISABLED",
        counts: undefined,
        questions: [],
      })
    );
    render(<StudentResponseReview examId="exam-1" studentId="s-1" />);
    expect(
      await screen.findByText(/cannot be viewed until the student finishes/)
    ).toBeInTheDocument();
  });

  it("handles an exam with no questions", async () => {
    getResponses.mockResolvedValue(
      responses({ questions: [], counts: { total: 0, answered: 0, unanswered: 0 } })
    );
    render(<StudentResponseReview examId="exam-1" studentId="s-1" />);
    expect(await screen.findByText("This exam has no questions")).toBeInTheDocument();
  });

  it("shows loading skeletons first", async () => {
    const d = deferred<ReviewStudentResponses>();
    getResponses.mockReturnValue(d.promise);
    render(<StudentResponseReview examId="exam-1" studentId="s-1" />);
    expect(screen.getByRole("status", { name: "Loading responses" })).toBeInTheDocument();
    d.resolve(responses());
    await screen.findByText(Q1.text);
  });

  describe("failures", () => {
    it("retries after a server error", async () => {
      getResponses.mockRejectedValueOnce(new ApiError("boom", 500)).mockResolvedValue(responses());
      const user = userEvent.setup();
      render(<StudentResponseReview examId="exam-1" studentId="s-1" />);
      expect(await screen.findByRole("alert")).toBeInTheDocument();
      await user.click(screen.getByRole("button", { name: /Try again/ }));
      expect(await screen.findByText(Q1.text)).toBeInTheDocument();
    });

    it("reports an expired session", async () => {
      getResponses.mockRejectedValue(new ApiError("Unauthorized", 401));
      render(<StudentResponseReview examId="exam-1" studentId="s-1" />);
      expect(await screen.findByRole("alert")).toHaveTextContent(/session has expired/i);
      expect(screen.getByRole("link", { name: "Sign in" })).toBeInTheDocument();
    });

    it("reports missing access and unknown students without retry buttons", async () => {
      getResponses.mockRejectedValueOnce(new ApiError("Forbidden", 403));
      const { unmount } = render(<StudentResponseReview examId="exam-1" studentId="s-1" />);
      expect(await screen.findByRole("alert")).toHaveTextContent(/do not have permission/i);
      expect(screen.queryByRole("button", { name: /Try again/ })).not.toBeInTheDocument();
      unmount();

      getResponses.mockRejectedValueOnce(new ApiError("not found", 404));
      render(<StudentResponseReview examId="exam-1" studentId="s-2" />);
      expect(await screen.findByRole("alert")).toHaveTextContent(/could not be found/i);
    });
  });

  describe("switching students", () => {
    it("never shows the previous student's answers while the next one loads", async () => {
      const second = deferred<ReviewStudentResponses>();
      getResponses.mockImplementation((_e, studentId) =>
        studentId === "s-1" ? Promise.resolve(responses()) : second.promise
      );
      const { rerender } = render(<StudentResponseReview examId="exam-1" studentId="s-1" />);
      await screen.findByRole("heading", { name: "Zoe Adams" });

      rerender(<StudentResponseReview examId="exam-1" studentId="s-2" />);
      expect(screen.queryByText("Zoe Adams")).not.toBeInTheDocument();
      expect(screen.queryByText(Q1.text)).not.toBeInTheDocument();
      expect(screen.getByRole("status", { name: "Loading responses" })).toBeInTheDocument();

      second.resolve(
        responses({
          student: { id: "s-2", rollNo: "R002", name: "Bob Brown" },
          questions: [{ ...Q1, text: "Bob's first question" }],
        })
      );
      expect(await screen.findByRole("heading", { name: "Bob Brown" })).toBeInTheDocument();
      expect(screen.getByText("Bob's first question")).toBeInTheDocument();
    });

    it("drops a late response for a student the teacher already left", async () => {
      const first = deferred<ReviewStudentResponses>();
      const second = deferred<ReviewStudentResponses>();
      getResponses.mockImplementation((_e, studentId) =>
        studentId === "s-1" ? first.promise : second.promise
      );
      const { rerender } = render(<StudentResponseReview examId="exam-1" studentId="s-1" />);
      rerender(<StudentResponseReview examId="exam-1" studentId="s-2" />);

      second.resolve(responses({ student: { id: "s-2", rollNo: "R002", name: "Bob Brown" } }));
      await screen.findByRole("heading", { name: "Bob Brown" });

      first.resolve(responses({ student: { id: "s-1", rollNo: "R001", name: "Zoe Adams" } }));
      await new Promise((r) => setTimeout(r, 20));
      expect(screen.queryByText("Zoe Adams")).not.toBeInTheDocument();
      expect(screen.getByRole("heading", { name: "Bob Brown" })).toBeInTheDocument();
    });

    it("aborts the earlier student's request", async () => {
      const signals: Record<string, AbortSignal | undefined> = {};
      getResponses.mockImplementation((_e, studentId, signal) => {
        signals[studentId] = signal;
        return new Promise(() => {});
      });
      const { rerender } = render(<StudentResponseReview examId="exam-1" studentId="s-1" />);
      rerender(<StudentResponseReview examId="exam-1" studentId="s-2" />);
      expect(signals["s-1"]?.aborted).toBe(true);
      expect(signals["s-2"]?.aborted).toBe(false);
    });

    it("starts at the first question for a different student", async () => {
      getResponses.mockImplementation(async (_e, studentId) =>
        responses({
          student: {
            id: studentId,
            rollNo: "R",
            name: studentId === "s-1" ? "Zoe Adams" : "Bob Brown",
          },
        })
      );
      const user = userEvent.setup();
      const { rerender } = render(<StudentResponseReview examId="exam-1" studentId="s-1" />);
      await screen.findByText(Q1.text);
      await user.click(screen.getByRole("button", { name: /Next/ }));
      expect(screen.getByRole("heading", { name: /Question 2/ })).toBeInTheDocument();

      rerender(<StudentResponseReview examId="exam-1" studentId="s-2" />);
      await screen.findByRole("heading", { name: "Bob Brown" });
      expect(screen.getByRole("heading", { name: /Question 1/ })).toBeInTheDocument();
    });
  });

  describe("refreshing", () => {
    it("keeps the teacher's place and the visible answers while reloading", async () => {
      const reload = deferred<ReviewStudentResponses>();
      getResponses.mockResolvedValueOnce(responses()).mockReturnValueOnce(reload.promise);
      const user = userEvent.setup();
      render(<StudentResponseReview examId="exam-1" studentId="s-1" />);
      await screen.findByText(Q1.text);
      await user.click(screen.getByRole("button", { name: /Next/ }));

      const refresh = screen.getByRole("button", { name: /Refresh/ });
      await user.click(refresh);
      expect(refresh).toBeDisabled();
      // Still showing question 2 while the new data loads.
      expect(screen.getByRole("heading", { name: /Question 2/ })).toBeInTheDocument();
      await user.click(refresh);
      expect(getResponses).toHaveBeenCalledTimes(2);

      reload.resolve(
        responses({
          questions: [
            Q1,
            { ...Q2, answer: { status: "ANSWERED", textAnswer: "Edited later" } },
            Q3,
          ],
        })
      );
      await waitFor(() => expect(screen.getByText("Edited later")).toBeInTheDocument());
      expect(screen.getByRole("heading", { name: /Question 2/ })).toBeInTheDocument();
    });

    it("keeps old answers visible and reports the failure if a refresh fails", async () => {
      getResponses
        .mockResolvedValueOnce(responses())
        .mockRejectedValueOnce(new ApiError("down", 500));
      const user = userEvent.setup();
      render(<StudentResponseReview examId="exam-1" studentId="s-1" />);
      await screen.findByText(Q1.text);
      await user.click(screen.getByRole("button", { name: /Refresh/ }));
      expect(await screen.findByRole("alert")).toBeInTheDocument();
    });
  });

  it("uses a responsive two-column layout that stacks on small screens", async () => {
    getResponses.mockResolvedValue(responses());
    render(<StudentResponseReview examId="exam-1" studentId="s-1" />);
    await screen.findByText(Q1.text);
    const layout = navigator().parentElement as HTMLElement;
    // Single column by default, two columns from the lg breakpoint.
    expect(layout.className).toContain("lg:grid-cols-");
    expect(layout.className).not.toMatch(/(^|\s)grid-cols-/);
  });
});
