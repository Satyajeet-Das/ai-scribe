import { render, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { beforeEach, describe, expect, it, vi } from "vitest";
import { AnswerRecordings } from "../answer-recordings";
import { StudentResponseReview } from "../student-response-review";
import { responseReviewApi } from "@/services/response-review-api";
import type { ReviewQuestion, ReviewRecording } from "@/types/response-review";
import { Q1, Q2, responses } from "./fixtures";

vi.mock("@/services/response-review-api", () => ({
  responseReviewApi: {
    listStudents: vi.fn(),
    getStudentResponses: vi.fn(),
    // Marking stays "loading" unless a test sets it up.
    getMarking: vi.fn(() => new Promise(() => {})),
    saveQuestionMarks: vi.fn(),
    finalise: vi.fn(),
    reopen: vi.fn(),
    getRecordingAudio: vi.fn(),
  },
}));

const getAudio = vi.mocked(responseReviewApi.getRecordingAudio);
const getResponses = vi.mocked(responseReviewApi.getStudentResponses);

function recording(overrides: Partial<ReviewRecording> = {}): ReviewRecording {
  return {
    id: "rec-1",
    segmentIndex: 0,
    status: "AVAILABLE",
    durationMs: 9_000,
    recordedAt: "2026-03-01T09:15:00Z",
    transcript: "plants make sugar from light",
    confirmed: true,
    ...overrides,
  };
}

function essay(
  recordings: ReviewRecording[] | undefined,
  answerText = "Plants make sugar from light."
): ReviewQuestion {
  return { ...Q2, answer: { ...Q2.answer, textAnswer: answerText }, recordings };
}

function show(question: ReviewQuestion) {
  return render(<AnswerRecordings question={question} examId="exam-1" studentId="s-1" />);
}

beforeEach(() => {
  getAudio.mockReset();
  getResponses.mockReset();
});

describe("AnswerRecordings", () => {
  it("says so when no audio was kept for a written answer", () => {
    show(essay([]));
    expect(screen.getByTestId("no-recordings")).toHaveTextContent("No audio was recorded");
    expect(screen.queryByRole("button", { name: /play/i })).toBeNull();
  });

  it("says the same for a server that does not report recordings at all", () => {
    show(essay(undefined));
    expect(screen.getByTestId("no-recordings")).toBeInTheDocument();
  });

  it("says nothing for a multiple-choice question, where nothing is recorded", () => {
    const { container } = show({ ...Q1, recordings: [] });
    expect(container).toBeEmptyDOMElement();
  });

  it("shows the unedited transcript beside a player, and warns that recognition is fallible", () => {
    show(essay([recording()]));
    expect(screen.getByText(/speech recognition can make mistakes/i)).toBeInTheDocument();
    expect(screen.getByText(/playing a recording is logged/i)).toBeInTheDocument();
    expect(screen.getByTestId("recording-transcript")).toHaveTextContent(
      "plants make sugar from light"
    );
    expect(screen.getByText(/not edited/i)).toBeInTheDocument();
    expect(screen.getByText("Used for the saved answer")).toBeInTheDocument();
    expect(
      screen.getByRole("button", { name: "Play recording 1 of 1 for question 2" })
    ).toBeInTheDocument();
  });

  it("does not fetch any audio just by being shown", () => {
    show(essay([recording(), recording({ id: "rec-2", segmentIndex: 1 })]));
    expect(getAudio).not.toHaveBeenCalled();
  });

  it("asks for exactly the recording that was chosen, for this exam and student", async () => {
    getAudio.mockResolvedValue(new Blob(["RIFF"], { type: "audio/wav" }));
    Object.defineProperty(HTMLMediaElement.prototype, "play", {
      configurable: true,
      value: vi.fn().mockResolvedValue(undefined),
    });
    Object.defineProperty(HTMLMediaElement.prototype, "pause", { configurable: true, value: vi.fn() });
    URL.createObjectURL = vi.fn(() => "blob:x");
    URL.revokeObjectURL = vi.fn();

    show(essay([recording({ id: "rec-1" }), recording({ id: "rec-2", segmentIndex: 1 })]));
    await userEvent.click(
      screen.getByRole("button", { name: "Play recording 2 of 2 for question 2" })
    );

    await waitFor(() => expect(getAudio).toHaveBeenCalledTimes(1));
    expect(getAudio).toHaveBeenCalledWith("exam-1", "s-1", "rec-2", expect.any(AbortSignal));
  });

  it("lists several recordings in order and says which one the answer used", () => {
    show(
      essay([
        recording({ id: "a", transcript: "first try", confirmed: false }),
        recording({ id: "b", transcript: "plants make sugar from light", confirmed: true }),
      ])
    );
    const items = screen.getAllByTestId("recording-item");
    expect(items).toHaveLength(2);
    expect(within(items[0]).getByText("Not used for the saved answer")).toBeInTheDocument();
    expect(within(items[0]).getByTestId("recording-transcript")).toHaveTextContent("first try");
    expect(within(items[1]).getByText("Used for the saved answer")).toBeInTheDocument();
    expect(within(items[1]).getByText(/of 2/)).toBeInTheDocument();
  });

  it("keeps the transcript of a removed recording but offers no way to play it", () => {
    show(essay([recording({ status: "REMOVED" })]));
    expect(screen.getByText("Audio removed")).toBeInTheDocument();
    expect(screen.getByRole("status")).toHaveTextContent(/has been removed/i);
    expect(screen.queryByRole("button", { name: /play/i })).toBeNull();
    expect(screen.getByTestId("recording-transcript")).toHaveTextContent("plants make sugar");
  });

  it("says when the recognizer produced no text", () => {
    show(essay([recording({ transcript: "" })]));
    expect(screen.getByText("No transcript was produced.")).toBeInTheDocument();
    expect(screen.getByRole("button", { name: /play recording/i })).toBeInTheDocument();
  });

  describe("when the saved answer and the transcript disagree", () => {
    it("points it out, and says which one counts", () => {
      show(
        essay([recording({ transcript: "plants make shugar from light" })], "Plants make sugar from light.")
      );
      const note = screen.getByTestId("transcript-differs");
      expect(note).toHaveTextContent(/different from this transcript/i);
      expect(note).toHaveTextContent(/saved answer is the one that counts/i);
    });

    it("ignores differences in capitals, punctuation and spacing", () => {
      show(essay([recording({ transcript: "Plants  make sugar, from light" })], "plants make sugar from light!"));
      expect(screen.queryByTestId("transcript-differs")).toBeNull();
    });

    it("does not compare a recording the student never used", () => {
      show(essay([recording({ transcript: "something else", confirmed: false })]));
      expect(screen.queryByTestId("transcript-differs")).toBeNull();
    });

    it("says when a confirmed recording no longer supports the saved answer", () => {
      show(
        essay(
          [recording({ transcript: "plants make sugar", confirmed: true, supportsCurrentAnswer: false })],
          "Typed over afterwards."
        )
      );
      expect(screen.getByText("Confirmed earlier, no longer the saved answer")).toBeInTheDocument();
      expect(screen.queryByText("Used for the saved answer")).toBeNull();
      expect(screen.queryByTestId("transcript-differs")).toBeNull();
    });
  });
});

describe("StudentResponseReview with recordings", () => {
  async function openEssay(question: ReviewQuestion) {
    const data = responses();
    data.questions = data.questions.map((q) => (q.id === question.id ? question : q));
    getResponses.mockResolvedValue(data);
    render(<StudentResponseReview examId="exam-1" studentId="s-1" />);
    await screen.findByText(Q1.text);
    await userEvent.click(screen.getByRole("button", { name: /Question 2/ }));
  }

  it.each([
    ["STT_CONFIRMED", /confirmed by the student after hearing the transcript/i],
    ["STUDENT_CORRECTION", /corrected by the student/i],
    ["TYPED", /typed by the student/i],
  ] as const)("tells the reviewer how a %s answer came to be", async (source, text) => {
    await openEssay({ ...Q2, answer: { ...Q2.answer, source }, recordings: [] });
    expect(await screen.findByTestId("answer-source")).toHaveTextContent(text);
  });

  it("shows the recording next to the answer it supports", async () => {
    await openEssay({
      ...Q2,
      answer: { ...Q2.answer, source: "STT_CONFIRMED" },
      recordings: [recording({ transcript: "line one of the answer" })],
    });
    expect(await screen.findByRole("heading", { name: "Recorded audio" })).toBeInTheDocument();
    expect(screen.getByTestId("recording-transcript")).toHaveTextContent("line one of the answer");
  });

  it("shows nothing about the source of a mouse selection", async () => {
    getResponses.mockResolvedValue(responses());
    render(<StudentResponseReview examId="exam-1" studentId="s-1" />);
    await screen.findByText(Q1.text);
    expect(screen.queryByTestId("answer-source")).toBeNull();
  });
});
