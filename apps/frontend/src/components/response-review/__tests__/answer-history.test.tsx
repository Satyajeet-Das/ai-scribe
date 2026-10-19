import { render, screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { beforeEach, describe, expect, it, vi } from "vitest";
import { AnswerHistory } from "../answer-history";
import { responseReviewApi } from "@/services/response-review-api";
import type { ReviewRevisionHistory } from "@/types/response-review";

vi.mock("@/services/response-review-api", () => ({
  responseReviewApi: { getRevisions: vi.fn() },
}));

const getRevisions = vi.mocked(responseReviewApi.getRevisions);

const history: ReviewRevisionHistory = {
  available: true,
  revisions: [
    {
      revisionNo: 1,
      source: "STUDENT_CORRECTION",
      textAnswer: "plants make sugar",
      createdAt: "2026-03-01T09:15:00Z",
      transcript: "plants make shugar",
      corrected: true,
      recordingIds: ["rec-1"],
    },
    {
      revisionNo: 2,
      source: "TYPED",
      textAnswer: "plants make sugar and oxygen",
      createdAt: "2026-03-01T09:20:00Z",
      corrected: false,
      recordingIds: [],
    },
  ],
};

function show() {
  return render(
    <AnswerHistory examId="exam-1" studentId="s-1" questionId="q-2" questionNumber={2} />
  );
}

beforeEach(() => getRevisions.mockReset());

describe("AnswerHistory", () => {
  it("loads nothing until the reviewer opens it", () => {
    show();
    expect(getRevisions).not.toHaveBeenCalled();
    expect(screen.getByTestId("answer-history-toggle")).toHaveAttribute("aria-expanded", "false");
  });

  it("lists every value with where it came from and what was heard", async () => {
    getRevisions.mockResolvedValue(history);
    show();
    await userEvent.click(screen.getByTestId("answer-history-toggle"));

    expect(getRevisions).toHaveBeenCalledWith("exam-1", "s-1", "q-2", expect.any(AbortSignal));
    const items = await screen.findAllByTestId("answer-history-item");
    expect(items).toHaveLength(2);
    expect(
      within(items[0]).getByText("Dictated, then corrected by the student")
    ).toBeInTheDocument();
    expect(items[0]).toHaveTextContent("The speech recognizer heard: “plants make shugar”");
    expect(items[0]).toHaveTextContent("Confirmed with 1 recording.");
    expect(within(items[1]).getByText("Typed")).toBeInTheDocument();
    expect(within(items[1]).getByText("Saved answer")).toBeInTheDocument();
    expect(within(items[0]).queryByText("Saved answer")).toBeNull();
    expect(screen.getByTestId("answer-history-toggle")).toHaveAttribute("aria-expanded", "true");
  });

  it("offers a retry when the history cannot be loaded", async () => {
    getRevisions.mockRejectedValueOnce(new Error("network")).mockResolvedValue(history);
    show();
    await userEvent.click(screen.getByTestId("answer-history-toggle"));

    expect(await screen.findByRole("alert")).toHaveTextContent("could not be loaded");
    await userEvent.click(screen.getByRole("button", { name: /try again/i }));
    expect(await screen.findAllByTestId("answer-history-item")).toHaveLength(2);
  });

  it("explains when there is nothing to show", async () => {
    getRevisions.mockResolvedValue({
      available: false,
      unavailableReason: "IN_PROGRESS_REVIEW_DISABLED",
      revisions: [],
    });
    show();
    await userEvent.click(screen.getByTestId("answer-history-toggle"));
    expect(await screen.findByText(/once the student has finished/i)).toBeInTheDocument();
  });

  it("does not load again when closed and reopened", async () => {
    getRevisions.mockResolvedValue(history);
    show();
    const toggle = screen.getByTestId("answer-history-toggle");
    await userEvent.click(toggle);
    await screen.findAllByTestId("answer-history-item");
    await userEvent.click(toggle);
    await userEvent.click(toggle);
    expect(getRevisions).toHaveBeenCalledTimes(1);
  });
});
