import { render, screen, fireEvent, waitFor, act } from "@testing-library/react";
import { Suspense } from "react";
import { describe, it, expect, vi, beforeEach } from "vitest";
import ActiveSessionRoomPage from "@/app/(dashboard)/sessions/[id]/page";
import { sessionsApi, examsApi } from "@/services/api";
import { useAuthStore } from "@/store/auth-store";
import type { VoiceActivity } from "@/components/voice/voice-assistant";
import type { Session, StudentQuestionResponse } from "@/types/exam-types";

vi.mock("next/navigation", () => ({
  useRouter: () => ({ push: vi.fn(), replace: vi.fn(), prefetch: vi.fn() }),
  usePathname: () => "/sessions/sess-1",
}));

// The real assistant needs a socket and a microphone. The page only depends on
// the activity it reports, so a stand-in that reports it on demand is enough.
let reportActivity: (a: VoiceActivity) => void = () => {};
vi.mock("@/components/voice/voice-assistant", () => ({
  VoiceAssistant: (props: { onActivityChange?: (a: VoiceActivity) => void }) => {
    reportActivity = (a) => props.onActivityChange?.(a);
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
  ApiError: class ApiError extends Error {},
}));

const session: Session = {
  id: "sess-1",
  assignmentId: "a",
  examId: "exam-1",
  studentId: "s",
  status: "IN_PROGRESS",
  currentQuestionId: "q-1",
  startedAt: new Date().toISOString(),
  durationMins: 45,
  remainingSeconds: 2700,
};

const middle: StudentQuestionResponse = {
  id: "q-1",
  examId: "exam-1",
  questionNumber: 2,
  text: "A question",
  type: "MCQ",
  points: 1,
  isFirst: false,
  isLast: false,
  totalQuestions: 3,
  currentIndex: 2,
  options: [{ id: "o1", optionKey: "A", optionText: "One", displayOrder: 1 }],
};

const idle: VoiceActivity = { recording: false, processing: false, awaitingConfirmation: false };

describe("navigation guard", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    useAuthStore.setState({
      user: { id: "s", email: "s@x.edu", firstName: "S", lastName: "C", role: "STUDENT" as const },
      isAuthenticated: true,
      isHydrated: true,
      status: "authenticated",
      accessToken: "valid-token",
    });
    vi.mocked(sessionsApi.getSession).mockResolvedValue(session);
    vi.mocked(sessionsApi.getCurrentQuestion).mockResolvedValue(middle);
    vi.mocked(sessionsApi.getAnswers).mockResolvedValue([]);
    vi.mocked(sessionsApi.nextQuestion).mockResolvedValue(undefined as never);
    vi.mocked(sessionsApi.previousQuestion).mockResolvedValue(undefined as never);
    vi.mocked(examsApi.getExam).mockResolvedValue({
      id: "exam-1",
      title: "T",
      subject: "S",
      durationMins: 45,
      status: "PUBLISHED",
      createdBy: "t",
      createdAt: "",
      updatedAt: "",
      description: "",
    });
  });

  async function room() {
    render(
      <Suspense fallback={null}>
        <ActiveSessionRoomPage params={{ id: "sess-1" }} />
      </Suspense>
    );
    await screen.findByTestId("next-question-button");
  }

  const cases: Array<[string, VoiceActivity, RegExp]> = [
    ["a recording is in progress", { ...idle, recording: true }, /recording is in progress/i],
    ["a recording is still being processed", { ...idle, processing: true }, /still being processed/i],
    [
      "an answer waits for confirmation",
      { ...idle, awaitingConfirmation: true },
      /waiting for confirmation/i,
    ],
  ];

  it.each(cases)("refuses to change question while %s", async (_name, activity, message) => {
    await room();
    act(() => reportActivity(activity));

    fireEvent.click(screen.getByTestId("next-question-button"));
    expect(await screen.findByTestId("navigation-notice")).toHaveTextContent(message);
    fireEvent.click(screen.getByTestId("prev-question-button"));
    fireEvent.keyDown(document.body, { key: "ArrowRight" });
    fireEvent.keyDown(document.body, { key: "n", altKey: true });

    await act(async () => {});
    expect(sessionsApi.nextQuestion).not.toHaveBeenCalled();
    expect(sessionsApi.previousQuestion).not.toHaveBeenCalled();
  });

  it("allows the move again, and clears the message, once the activity ends", async () => {
    await room();
    act(() => reportActivity({ ...idle, recording: true }));
    fireEvent.click(screen.getByTestId("next-question-button"));
    expect(await screen.findByTestId("navigation-notice")).toBeInTheDocument();

    act(() => reportActivity(idle));
    await waitFor(() => expect(screen.queryByTestId("navigation-notice")).toBeNull());

    fireEvent.click(screen.getByTestId("next-question-button"));
    await waitFor(() => expect(sessionsApi.nextQuestion).toHaveBeenCalledTimes(1));
  });
});
