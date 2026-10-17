import { render, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, it, expect, vi, beforeEach } from "vitest";
import { ResponseOverview } from "../response-overview";
import { responseReviewApi } from "@/services/response-review-api";
import { ApiError } from "@/services/api";
import type { ReviewStudentList } from "@/types/response-review";
import { deferred, list, row } from "./fixtures";

vi.mock("@/services/response-review-api", () => ({
  responseReviewApi: { listStudents: vi.fn(), getStudentResponses: vi.fn(),
    // Marking stays "loading" unless a test sets it up.
    getMarking: vi.fn(() => new Promise(() => {})),
    saveQuestionMarks: vi.fn(),
    finalise: vi.fn(),
    reopen: vi.fn() },
}));

const listStudents = vi.mocked(responseReviewApi.listStudents);

const FOUR = [
  row({
    studentId: "s-bob",
    name: "Bob Brown",
    rollNo: "R002",
    status: "IN_PROGRESS",
    answeredCount: 1,
    submittedAt: undefined,
  }),
  row({
    studentId: "s-cara",
    name: "Cara Clark",
    rollNo: "R003",
    status: "NOT_STARTED",
    answeredCount: undefined,
    startedAt: undefined,
    submittedAt: undefined,
  }),
  row({
    studentId: "s-dan",
    name: "Dan Davis",
    rollNo: "R004",
    status: "EXPIRED",
    answeredCount: 0,
    submittedAt: undefined,
  }),
  row({ studentId: "s-zoe" }),
];

function lastParams() {
  return listStudents.mock.calls[listStudents.mock.calls.length - 1][1];
}

describe("ResponseOverview", () => {
  beforeEach(() => {
    listStudents.mockReset();
  });

  it("shows exam metadata, the summary and every student with a clear status", async () => {
    listStudents.mockResolvedValue(list(FOUR));
    render(<ResponseOverview examId="exam-1" />);

    expect(await screen.findByText("Physics Midterm")).toBeInTheDocument();
    expect(screen.getByText(/3 questions/)).toBeInTheDocument();
    expect(screen.getByText(/60 minutes/)).toBeInTheDocument();

    const summary = screen.getByLabelText("Exam summary");
    expect(within(summary).getByText("Assigned").nextSibling).toHaveTextContent("4");
    expect(within(summary).getByText("Not started").nextSibling).toHaveTextContent("1");
    expect(within(summary).getByText("Submitted").nextSibling).toHaveTextContent("1");

    const items = within(screen.getByRole("list", { name: "Students" })).getAllByRole("listitem");
    expect(items).toHaveLength(4);
    expect(within(items[0]).getByText("In progress")).toBeInTheDocument();
    expect(within(items[0]).getByText("1 of 3 answered")).toBeInTheDocument();
    expect(within(items[1]).getByText("Not started")).toBeInTheDocument();
    expect(within(items[2]).getByText("Time expired")).toBeInTheDocument();
    expect(within(items[3]).getByText("Submitted")).toBeInTheDocument();
    expect(within(items[3]).getByText(/Submitted .*2026/)).toBeInTheDocument();
    expect(within(items[3]).getByText("3 of 3 answered")).toBeInTheDocument();
  });

  it("links to the student's responses, except for students who never started", async () => {
    listStudents.mockResolvedValue(list(FOUR));
    render(<ResponseOverview examId="exam-1" />);
    await screen.findByText("Zoe Adams");

    const link = screen.getByRole("link", { name: /Review responses for Zoe Adams/ });
    expect(link).toHaveAttribute("href", "/exams/exam-1/responses/s-zoe");

    expect(screen.queryByRole("link", { name: /Cara Clark/ })).not.toBeInTheDocument();
    expect(screen.getByText("Nothing to review")).toBeInTheDocument();
  });

  it("shows loading skeletons until the first response", async () => {
    const d = deferred<ReviewStudentList>();
    listStudents.mockReturnValue(d.promise);
    render(<ResponseOverview examId="exam-1" />);
    expect(screen.getByRole("status", { name: "Loading students" })).toBeInTheDocument();
    d.resolve(list(FOUR));
    await screen.findByText("Zoe Adams");
    expect(screen.queryByRole("status", { name: "Loading students" })).not.toBeInTheDocument();
  });

  it("shows an empty state when nobody is assigned", async () => {
    listStudents.mockResolvedValue(
      list([], { summary: { assigned: 0, notStarted: 0, inProgress: 0, submitted: 0, expired: 0 } })
    );
    render(<ResponseOverview examId="exam-1" />);
    expect(await screen.findByText("No students are assigned to this exam")).toBeInTheDocument();
  });

  it("debounces search and sends the trimmed text once", async () => {
    listStudents.mockResolvedValue(list(FOUR));
    const user = userEvent.setup();
    render(<ResponseOverview examId="exam-1" />);
    await screen.findByText("Zoe Adams");
    expect(listStudents).toHaveBeenCalledTimes(1);

    await user.type(screen.getByLabelText(/Search students/), "  ana");
    // No request per keystroke.
    expect(listStudents).toHaveBeenCalledTimes(1);
    await waitFor(() => expect(listStudents).toHaveBeenCalledTimes(2));
    expect(lastParams()).toMatchObject({ search: "ana", offset: 0 });
  });

  it("shows a no-results state for a search with no matches, and clearing restores the list", async () => {
    listStudents.mockImplementation(async (_id, params) =>
      params?.search ? list([], { total: 0 }) : list(FOUR)
    );
    const user = userEvent.setup();
    render(<ResponseOverview examId="exam-1" />);
    await screen.findByText("Zoe Adams");

    await user.type(screen.getByLabelText(/Search students/), "nobody");
    expect(await screen.findByText("No students match your search")).toBeInTheDocument();

    await user.clear(screen.getByLabelText(/Search students/));
    expect(await screen.findByText("Zoe Adams")).toBeInTheDocument();
  });

  it("filters by status and exposes the pressed state", async () => {
    listStudents.mockResolvedValue(list(FOUR));
    const user = userEvent.setup();
    render(<ResponseOverview examId="exam-1" />);
    await screen.findByText("Zoe Adams");

    const group = screen.getByRole("group", { name: "Filter by status" });
    const all = within(group).getByRole("button", { name: /^All/ });
    const submitted = within(group).getByRole("button", { name: /^Submitted/ });
    expect(all).toHaveAttribute("aria-pressed", "true");
    expect(submitted).toHaveTextContent("(1)");

    await user.click(submitted);
    await waitFor(() => expect(lastParams()).toMatchObject({ status: "SUBMITTED", offset: 0 }));
    expect(submitted).toHaveAttribute("aria-pressed", "true");
    expect(all).toHaveAttribute("aria-pressed", "false");

    await user.click(all);
    await waitFor(() => expect(lastParams()?.status).toBeUndefined());
  });

  it("paginates with the backend's contract and returns to page one when filters change", async () => {
    const many = Array.from({ length: 20 }, (_, i) =>
      row({ studentId: `s${i}`, name: `Student ${i}`, rollNo: `R${i}` })
    );
    listStudents.mockImplementation(async (_id, params) =>
      list(many, { total: 45, offset: params?.offset ?? 0 })
    );
    const user = userEvent.setup();
    render(<ResponseOverview examId="exam-1" />);
    await screen.findByText("Student 0");

    const nav = screen.getByRole("navigation", { name: "Pagination" });
    expect(within(nav).getByText("1–20 of 45")).toBeInTheDocument();
    expect(within(nav).getByRole("button", { name: /Previous/ })).toBeDisabled();

    await user.click(within(nav).getByRole("button", { name: /Next/ }));
    await waitFor(() => expect(lastParams()).toMatchObject({ offset: 20, limit: 20 }));
    await waitFor(() => expect(within(nav).getByText("21–40 of 45")).toBeInTheDocument());

    await user.click(within(nav).getByRole("button", { name: /Next/ }));
    await waitFor(() => expect(within(nav).getByText("41–45 of 45")).toBeInTheDocument());
    expect(within(nav).getByRole("button", { name: /Next/ })).toBeDisabled();

    await user.click(screen.getByRole("button", { name: /^Not started/ }));
    await waitFor(() => expect(lastParams()).toMatchObject({ status: "NOT_STARTED", offset: 0 }));
  });

  it("does not render pagination when everything fits on one page", async () => {
    listStudents.mockResolvedValue(list(FOUR));
    render(<ResponseOverview examId="exam-1" />);
    await screen.findByText("Zoe Adams");
    expect(screen.queryByRole("navigation", { name: "Pagination" })).not.toBeInTheDocument();
  });

  it("ignores a slow response that was superseded by a newer search", async () => {
    const slow = deferred<ReviewStudentList>();
    const fast = deferred<ReviewStudentList>();
    listStudents.mockImplementation((_id, params) => {
      if (params?.search === "old") return slow.promise;
      if (params?.search === "new") return fast.promise;
      return Promise.resolve(list(FOUR));
    });
    const user = userEvent.setup();
    render(<ResponseOverview examId="exam-1" />);
    await screen.findByText("Zoe Adams");

    const box = screen.getByLabelText(/Search students/);
    await user.type(box, "old");
    await waitFor(() => expect(lastParams()?.search).toBe("old"));
    await user.clear(box);
    await user.type(box, "new");
    await waitFor(() => expect(lastParams()?.search).toBe("new"));

    fast.resolve(list([row({ studentId: "s-new", name: "Newest Match" })]));
    await screen.findByText("Newest Match");

    // The older request finishes last and must not replace the newer list.
    slow.resolve(list([row({ studentId: "s-old", name: "Outdated Match" })]));
    await new Promise((r) => setTimeout(r, 20));
    expect(screen.queryByText("Outdated Match")).not.toBeInTheDocument();
    expect(screen.getByText("Newest Match")).toBeInTheDocument();
  });

  it("aborts the in-flight request when the filters change", async () => {
    const signals: AbortSignal[] = [];
    listStudents.mockImplementation((_id, _params, signal) => {
      if (signal) signals.push(signal);
      return new Promise(() => {});
    });
    const user = userEvent.setup();
    render(<ResponseOverview examId="exam-1" />);
    await user.click(screen.getByRole("button", { name: /^Submitted/ }));
    await waitFor(() => expect(signals.length).toBeGreaterThanOrEqual(2));
    expect(signals[0].aborted).toBe(true);
  });

  it("aborts the request on unmount", async () => {
    let signal: AbortSignal | undefined;
    listStudents.mockImplementation((_id, _params, s) => {
      signal = s;
      return new Promise(() => {});
    });
    const { unmount } = render(<ResponseOverview examId="exam-1" />);
    unmount();
    expect(signal?.aborted).toBe(true);
  });

  it("announces the result count in a polite live region", async () => {
    listStudents.mockResolvedValue(list(FOUR));
    render(<ResponseOverview examId="exam-1" />);
    await screen.findByText("Zoe Adams");
    await waitFor(() =>
      expect(screen.getByText("Showing 1 to 4 of 4 students.")).toHaveAttribute(
        "aria-live",
        "polite"
      )
    );
  });

  it("refreshes on demand without duplicating requests while loading", async () => {
    const first = deferred<ReviewStudentList>();
    listStudents.mockReturnValueOnce(first.promise).mockResolvedValue(list(FOUR));
    const user = userEvent.setup();
    render(<ResponseOverview examId="exam-1" />);

    const refresh = screen.getByRole("button", { name: /Refresh/ });
    expect(refresh).toBeDisabled();
    await user.click(refresh);
    expect(listStudents).toHaveBeenCalledTimes(1);

    first.resolve(list(FOUR));
    await screen.findByText("Zoe Adams");
    expect(refresh).toBeEnabled();
    await user.click(refresh);
    await waitFor(() => expect(listStudents).toHaveBeenCalledTimes(2));
    // The list stays on screen while it reloads.
    expect(screen.getByText("Zoe Adams")).toBeInTheDocument();
  });

  describe("failures", () => {
    it("shows a retryable error for server failures and recovers", async () => {
      listStudents.mockRejectedValueOnce(new ApiError("boom", 500)).mockResolvedValue(list(FOUR));
      const user = userEvent.setup();
      render(<ResponseOverview examId="exam-1" />);

      expect(await screen.findByRole("alert")).toHaveTextContent(/Something went wrong/);
      await user.click(screen.getByRole("button", { name: /Try again/ }));
      expect(await screen.findByText("Zoe Adams")).toBeInTheDocument();
      expect(screen.queryByRole("alert")).not.toBeInTheDocument();
    });

    it("explains an expired session and offers sign-in instead of retry", async () => {
      listStudents.mockRejectedValue(new ApiError("Unauthorized", 401));
      render(<ResponseOverview examId="exam-1" />);
      expect(await screen.findByRole("alert")).toHaveTextContent(/session has expired/i);
      expect(screen.getByRole("link", { name: "Sign in" })).toHaveAttribute("href", "/login");
      expect(screen.queryByRole("button", { name: /Try again/ })).not.toBeInTheDocument();
    });

    it("explains missing permission without leaking server text", async () => {
      listStudents.mockRejectedValue(new ApiError("internal detail", 403));
      render(<ResponseOverview examId="exam-1" />);
      const alert = await screen.findByRole("alert");
      expect(alert).toHaveTextContent(/do not have permission/i);
      expect(alert).not.toHaveTextContent("internal detail");
      expect(screen.queryByRole("button", { name: /Try again/ })).not.toBeInTheDocument();
    });

    it("explains a missing exam", async () => {
      listStudents.mockRejectedValue(new ApiError("exam not found", 404));
      render(<ResponseOverview examId="exam-1" />);
      expect(await screen.findByRole("alert")).toHaveTextContent(/could not be found/i);
    });

    it("treats a network failure as retryable", async () => {
      listStudents.mockRejectedValue(new ApiError("Failed to fetch", 0));
      render(<ResponseOverview examId="exam-1" />);
      expect(await screen.findByRole("alert")).toHaveTextContent(/Could not reach the server/);
      expect(screen.getByRole("button", { name: /Try again/ })).toBeInTheDocument();
    });
  });

  it("does not show another exam's students after the exam changes", async () => {
    const second = deferred<ReviewStudentList>();
    listStudents.mockImplementation((id) =>
      id === "exam-1"
        ? Promise.resolve(list([row({ name: "First Exam Student" })]))
        : second.promise
    );
    const { rerender } = render(<ResponseOverview examId="exam-1" />);
    await screen.findByText("First Exam Student");

    rerender(<ResponseOverview examId="exam-2" />);
    expect(screen.queryByText("First Exam Student")).not.toBeInTheDocument();
    expect(screen.getByRole("status", { name: "Loading students" })).toBeInTheDocument();
  });

  it("is operable from the keyboard", async () => {
    listStudents.mockResolvedValue(list(FOUR));
    const user = userEvent.setup();
    render(<ResponseOverview examId="exam-1" />);
    await screen.findByText("Zoe Adams");

    const tabTo = async (target: HTMLElement) => {
      for (let i = 0; i < 20 && document.activeElement !== target; i++) await user.tab();
      expect(target).toHaveFocus();
    };

    const search = screen.getByLabelText(/Search students/);
    await tabTo(search);
    await user.keyboard("zoe");
    expect(search).toHaveValue("zoe");

    const submitted = screen.getByRole("button", { name: /^Submitted/ });
    await tabTo(submitted);
    await user.keyboard("{Enter}");
    await waitFor(() => expect(lastParams()).toMatchObject({ status: "SUBMITTED", search: "zoe" }));

    await tabTo(screen.getByRole("link", { name: /Review responses for Zoe Adams/ }));
  });
});
