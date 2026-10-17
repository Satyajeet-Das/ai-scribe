import { render, screen, fireEvent, waitFor } from "@testing-library/react";
import { Suspense } from "react";
import { describe, it, expect, vi, beforeEach } from "vitest";
import ActiveSessionRoomPage from "@/app/(dashboard)/sessions/[id]/page";
import { sessionsApi, examsApi } from "@/services/api";
import { useAuthStore } from "@/store/auth-store";
import type { Session, StudentQuestionResponse } from "@/types/exam-types";

vi.mock("next/navigation", () => ({
  useRouter: () => ({ push: vi.fn(), replace: vi.fn(), prefetch: vi.fn() }),
  usePathname: () => "/sessions/sess-1",
}));

vi.mock("@/services/api", () => ({
  assignmentsApi: {},
  sessionsApi: {
    getSession: vi.fn(),
    getCurrentQuestion: vi.fn(),
    getAnswers: vi.fn(),
    getQuestionAnswer: vi.fn(),
    submitAnswer: vi.fn(),
    nextQuestion: vi.fn(),
    previousQuestion: vi.fn(),
    submitSession: vi.fn(),
  },
  examsApi: { getExam: vi.fn() },
  registerAuthFailureHandler: vi.fn(),
  getStoredToken: vi.fn(() => "valid-token"),
  API_BASE_URL: "http://localhost:8080",
  ApiError: class ApiError extends Error {
    status = 0;
    isConflict() {
      return false;
    }
  },
}));

const session: Session = {
  id: "sess-1",
  assignmentId: "asgn-1",
  examId: "exam-1",
  studentId: "student-1",
  status: "IN_PROGRESS",
  currentQuestionId: "q-essay",
  startedAt: new Date().toISOString(),
  durationMins: 45,
  remainingSeconds: 2700,
};

const essay: StudentQuestionResponse = {
  id: "q-essay",
  examId: "exam-1",
  questionNumber: 1,
  text: "Explain photosynthesis.",
  type: "ESSAY",
  points: 10,
  isFirst: true,
  isLast: false,
  totalQuestions: 2,
  currentIndex: 1,
  options: [],
};

const next: StudentQuestionResponse = {
  ...essay,
  id: "q-2",
  questionNumber: 2,
  text: "Second question",
  isFirst: false,
  isLast: true,
  currentIndex: 2,
};

async function renderRoom() {
  vi.mocked(sessionsApi.getSession).mockResolvedValue(session);
  vi.mocked(sessionsApi.getCurrentQuestion).mockResolvedValue(essay);
  vi.mocked(sessionsApi.getAnswers).mockResolvedValue([]);
  vi.mocked(examsApi.getExam).mockResolvedValue({
    id: "exam-1",
    title: "Biology",
    subject: "Biology",
    durationMins: 45,
    status: "PUBLISHED",
    createdBy: "t",
    createdAt: "",
    updatedAt: "",
    description: "",
  });
  render(
    <Suspense fallback={null}>
      <ActiveSessionRoomPage params={{ id: "sess-1" }} />
    </Suspense>
  );
  return screen.findByLabelText("Your Response");
}

describe("typed essay answers", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    useAuthStore.setState({
      user: {
        id: "student-1",
        email: "s@school.edu",
        firstName: "S",
        lastName: "C",
        role: "STUDENT" as const,
      },
      isAuthenticated: true,
      isHydrated: true,
      status: "authenticated",
      accessToken: "valid-token",
    });
  });

  it("saves typed text after a pause, as text and never as an option", async () => {
    vi.mocked(sessionsApi.submitAnswer).mockResolvedValue({} as never);
    const box = await renderRoom();

    fireEvent.change(box, { target: { value: "Plants make sugar from light" } });
    expect(screen.getByTestId("answer-save-status")).toHaveTextContent("Not saved yet");
    expect(sessionsApi.submitAnswer).not.toHaveBeenCalled();

    await waitFor(
      () =>
        expect(sessionsApi.submitAnswer).toHaveBeenCalledWith("sess-1", "q-essay", {
          textAnswer: "Plants make sugar from light",
        }),
      { timeout: 3000 }
    );
    await waitFor(() =>
      expect(screen.getByTestId("answer-save-status")).toHaveTextContent("Answer saved")
    );
  });

  it("saves immediately before moving to the next question", async () => {
    const order: string[] = [];
    vi.mocked(sessionsApi.submitAnswer).mockImplementation(async () => {
      order.push("save");
      return {} as never;
    });
    vi.mocked(sessionsApi.nextQuestion).mockImplementation(async () => {
      order.push("next");
      return undefined as never;
    });
    const box = await renderRoom();
    vi.mocked(sessionsApi.getCurrentQuestion).mockResolvedValue(next);

    fireEvent.change(box, { target: { value: "My answer" } });
    fireEvent.click(screen.getByTestId("next-question-button"));

    await waitFor(() => expect(order).toEqual(["save", "next"]));
    expect(sessionsApi.submitAnswer).toHaveBeenCalledTimes(1);
  });

  it("does not leave the question while a save has failed, and keeps the text", async () => {
    vi.mocked(sessionsApi.submitAnswer).mockRejectedValue(new Error("network down"));
    const box = await renderRoom();

    fireEvent.change(box, { target: { value: "Important text" } });
    fireEvent.click(screen.getByTestId("next-question-button"));

    await waitFor(() =>
      expect(screen.getByTestId("answer-save-status")).toHaveTextContent("Failed to save")
    );
    expect(sessionsApi.nextQuestion).not.toHaveBeenCalled();
    expect(box).toHaveValue("Important text");

    // Retrying sends the same text, and succeeds once the network is back.
    vi.mocked(sessionsApi.submitAnswer).mockResolvedValue({} as never);
    fireEvent.click(screen.getByTestId("retry-save-button"));
    await waitFor(() =>
      expect(sessionsApi.submitAnswer).toHaveBeenLastCalledWith("sess-1", "q-essay", {
        textAnswer: "Important text",
      })
    );
    await waitFor(() =>
      expect(screen.getByTestId("answer-save-status")).toHaveTextContent("Answer saved")
    );
  });

  it("will not submit the exam while typed text is unsaved", async () => {
    vi.mocked(sessionsApi.submitAnswer).mockRejectedValue(new Error("network down"));
    const box = await renderRoom();

    fireEvent.change(box, { target: { value: "Unsaved work" } });
    fireEvent.click(screen.getByTestId("submit-exam-button"));
    fireEvent.click(await screen.findByTestId("confirm-submit-dialog-button"));

    await waitFor(() =>
      expect(screen.getByTestId("answer-save-status")).toHaveTextContent("Failed to save")
    );
    expect(sessionsApi.submitSession).not.toHaveBeenCalled();
  });
});
