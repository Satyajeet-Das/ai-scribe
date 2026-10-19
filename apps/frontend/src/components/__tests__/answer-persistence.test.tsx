import { render, screen, fireEvent, waitFor, act } from "@testing-library/react";
import { Suspense } from "react";
import { describe, it, expect, vi, beforeEach } from "vitest";
import ActiveSessionRoomPage from "@/app/(dashboard)/sessions/[id]/page";
import { sessionsApi, examsApi } from "@/services/api";
import { useAuthStore } from "@/store/auth-store";
import type { VoiceSessionState } from "@/services/voice/protocol";
import type { Session, StudentQuestionResponse } from "@/types/exam-types";

vi.mock("next/navigation", () => ({
  useRouter: () => ({ push: vi.fn(), replace: vi.fn(), prefetch: vi.fn() }),
  usePathname: () => "/sessions/sess-1",
}));

// A stand-in for the voice assistant that lets a test play what the backend
// reports: voice navigation and answers saved by voice.
const voice: {
  sessionState: (s: VoiceSessionState) => void;
  answerSaved: (questionId: string) => Promise<void> | void;
} = { sessionState: () => {}, answerSaved: () => {} };
vi.mock("@/components/voice/voice-assistant", () => ({
  VoiceAssistant: (props: {
    onSessionState?: (s: VoiceSessionState) => void;
    onAnswerSaved?: (questionId: string) => Promise<void> | void;
  }) => {
    voice.sessionState = (s) => props.onSessionState?.(s);
    voice.answerSaved = (id) => props.onAnswerSaved?.(id);
    return null;
  },
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

const mcq: StudentQuestionResponse = {
  ...essay,
  id: "q-mcq",
  questionNumber: 2,
  text: "Which organelle makes proteins?",
  type: "MCQ",
  isFirst: false,
  isLast: true,
  currentIndex: 2,
  options: [
    { id: "opt-a", optionKey: "A", optionText: "Nucleus", displayOrder: 1 },
    { id: "opt-b", optionKey: "B", optionText: "Ribosome", displayOrder: 2 },
    { id: "opt-c", optionKey: "C", optionText: "Golgi body", displayOrder: 3 },
    { id: "opt-d", optionKey: "D", optionText: "Vacuole", displayOrder: 4 },
  ],
};

function deferred<T = unknown>() {
  let resolve!: (v: T) => void;
  let reject!: (e: unknown) => void;
  const promise = new Promise<T>((res, rej) => {
    resolve = res;
    reject = rej;
  });
  return { promise, resolve, reject };
}

async function renderRoom(first: StudentQuestionResponse = essay) {
  vi.mocked(sessionsApi.getSession).mockResolvedValue(session);
  vi.mocked(sessionsApi.getCurrentQuestion).mockResolvedValue(first);
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
  await screen.findByText(first.text);
}

describe("answers the server has not confirmed", () => {
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

  it("keeps typed text when voice moves on while its save fails, and saves it on retry", async () => {
    vi.mocked(sessionsApi.submitAnswer).mockRejectedValue(new Error("network down"));
    await renderRoom();

    fireEvent.change(screen.getByLabelText("Your Response"), {
      target: { value: "Light becomes sugar" },
    });
    act(() => voice.sessionState({ status: "IN_PROGRESS", question: mcq }));

    await screen.findByText(mcq.text);
    const notice = await screen.findByTestId("unsaved-elsewhere-notice");
    expect(notice).toHaveTextContent("Your answer to another question has not been saved yet");

    vi.mocked(sessionsApi.submitAnswer).mockResolvedValue({} as never);
    fireEvent.click(screen.getByTestId("retry-elsewhere-button"));
    await waitFor(() =>
      expect(sessionsApi.submitAnswer).toHaveBeenLastCalledWith("sess-1", "q-essay", {
        textAnswer: "Light becomes sugar",
      })
    );
    await waitFor(() =>
      expect(screen.queryByTestId("unsaved-elsewhere-notice")).not.toBeInTheDocument()
    );
  });

  it("will not submit while an answer to another question is unsaved", async () => {
    vi.mocked(sessionsApi.submitAnswer).mockRejectedValue(new Error("network down"));
    await renderRoom();

    fireEvent.change(screen.getByLabelText("Your Response"), {
      target: { value: "Light becomes sugar" },
    });
    act(() => voice.sessionState({ status: "IN_PROGRESS", question: mcq }));
    await screen.findByTestId("unsaved-elsewhere-notice");

    fireEvent.click(screen.getByTestId("submit-exam-button"));
    fireEvent.click(await screen.findByTestId("confirm-submit-dialog-button"));

    await waitFor(() => expect(sessionsApi.submitAnswer).toHaveBeenCalledTimes(2));
    expect(sessionsApi.submitSession).not.toHaveBeenCalled();
  });

  it("sends only the last of several quick option choices after the one in flight", async () => {
    const first = deferred();
    vi.mocked(sessionsApi.submitAnswer)
      .mockImplementationOnce(() => first.promise as never)
      .mockResolvedValue({} as never);
    await renderRoom(mcq);

    fireEvent.click(screen.getByText("Nucleus"));
    await waitFor(() => expect(sessionsApi.submitAnswer).toHaveBeenCalledTimes(1));
    fireEvent.click(screen.getByText("Ribosome"));
    fireEvent.click(screen.getByText("Golgi body"));
    fireEvent.click(screen.getByText("Vacuole"));

    await act(async () => first.resolve({}));
    await waitFor(() => expect(sessionsApi.submitAnswer).toHaveBeenCalledTimes(2));
    expect(sessionsApi.submitAnswer).toHaveBeenNthCalledWith(1, "sess-1", "q-mcq", {
      selectedOptionId: "opt-a",
      textAnswer: "",
    });
    expect(sessionsApi.submitAnswer).toHaveBeenNthCalledWith(2, "sess-1", "q-mcq", {
      selectedOptionId: "opt-d",
      textAnswer: "",
    });
    await waitFor(() =>
      expect(screen.getByTestId("answer-save-status")).toHaveTextContent("Answer saved")
    );
  });

  it("keeps a failed option choice and retries it", async () => {
    vi.mocked(sessionsApi.submitAnswer).mockRejectedValueOnce(new Error("network down"));
    await renderRoom(mcq);

    fireEvent.click(screen.getByText("Ribosome"));
    await waitFor(() =>
      expect(screen.getByTestId("answer-save-status")).toHaveTextContent("Failed to save")
    );

    vi.mocked(sessionsApi.submitAnswer).mockResolvedValue({} as never);
    fireEvent.click(screen.getByTestId("retry-save-button"));
    await waitFor(() =>
      expect(sessionsApi.submitAnswer).toHaveBeenLastCalledWith("sess-1", "q-mcq", {
        selectedOptionId: "opt-b",
        textAnswer: "",
      })
    );
    await waitFor(() =>
      expect(screen.getByTestId("answer-save-status")).toHaveTextContent("Answer saved")
    );
  });

  it("does not replace text typed after a voice answer was saved", async () => {
    vi.mocked(sessionsApi.submitAnswer).mockImplementation(() => new Promise(() => {}) as never);
    vi.mocked(sessionsApi.getQuestionAnswer).mockResolvedValue({
      id: "ans-1",
      sessionId: "sess-1",
      questionId: "q-essay",
      textAnswer: "dictated text",
    } as never);
    await renderRoom();

    const box = screen.getByLabelText("Your Response");
    fireEvent.change(box, { target: { value: "typed afterwards" } });
    await act(async () => voice.answerSaved("q-essay"));

    expect(box).toHaveValue("typed afterwards");
    expect(sessionsApi.getQuestionAnswer).not.toHaveBeenCalled();
  });

  it("shows the dictated answer in place of typed text that never reached the server", async () => {
    vi.mocked(sessionsApi.submitAnswer).mockRejectedValue(new Error("network down"));
    vi.mocked(sessionsApi.getQuestionAnswer).mockResolvedValue({
      id: "ans-1",
      sessionId: "sess-1",
      questionId: "q-essay",
      textAnswer: "dictated text",
    } as never);
    await renderRoom();

    const box = screen.getByLabelText("Your Response");
    fireEvent.change(box, { target: { value: "typed before" } });
    fireEvent.click(screen.getByTestId("next-question-button"));
    await waitFor(() =>
      expect(screen.getByTestId("answer-save-status")).toHaveTextContent("Failed to save")
    );

    await act(async () => voice.answerSaved("q-essay"));
    expect(box).toHaveValue("dictated text");
    await waitFor(() =>
      expect(screen.getByTestId("answer-save-status")).toHaveTextContent("Answer saved")
    );

    // Nothing is left to retry that would overwrite the dictated answer.
    vi.mocked(sessionsApi.submitAnswer).mockClear();
    vi.mocked(sessionsApi.nextQuestion).mockResolvedValue(undefined as never);
    vi.mocked(sessionsApi.getCurrentQuestion).mockResolvedValue(mcq);
    fireEvent.click(screen.getByTestId("next-question-button"));
    await screen.findByText(mcq.text);
    expect(sessionsApi.submitAnswer).not.toHaveBeenCalled();
  });

  it("says so when moving to another question fails", async () => {
    vi.mocked(sessionsApi.nextQuestion).mockRejectedValue(new Error("network down"));
    await renderRoom();

    fireEvent.click(screen.getByTestId("next-question-button"));
    expect(await screen.findByTestId("navigation-notice")).toHaveTextContent(
      "Could not move to the next question"
    );
  });
});
