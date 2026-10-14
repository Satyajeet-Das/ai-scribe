import { render, screen, fireEvent, waitFor } from "@testing-library/react";
import { Suspense } from "react";
import { describe, it, expect, vi, beforeEach } from "vitest";
import MyExamsPage from "@/app/(dashboard)/my-exams/page";
import ActiveSessionRoomPage from "@/app/(dashboard)/sessions/[id]/page";
import { assignmentsApi, sessionsApi, examsApi } from "@/services/api";
import { useAuthStore } from "@/store/auth-store";
import type { StudentAssignedExam, Session, StudentQuestionResponse, Answer } from "@/types/exam-types";

// Mock next/navigation
const mockPush = vi.fn();
vi.mock("next/navigation", () => ({
  useRouter: () => ({
    push: mockPush,
    replace: vi.fn(),
    prefetch: vi.fn(),
  }),
  usePathname: () => "/my-exams",
}));

// Mock API layer
vi.mock("@/services/api", () => ({
  assignmentsApi: {
    getMyAssignedExams: vi.fn(),
    getStudentAssignments: vi.fn(),
  },
  sessionsApi: {
    getSessions: vi.fn(),
    getSession: vi.fn(),
    startSession: vi.fn(),
    getCurrentQuestion: vi.fn(),
    getAnswers: vi.fn(),
    submitAnswer: vi.fn(),
    nextQuestion: vi.fn(),
    previousQuestion: vi.fn(),
    submitSession: vi.fn(),
  },
  examsApi: {
    getExam: vi.fn(),
  },
  registerAuthFailureHandler: vi.fn(),
}));

describe("Sprint 6 E2E: Candidate Exam Session & FSM Journey", () => {
  const studentUser = {
    id: "student-uuid-999",
    email: "candidate@school.edu",
    firstName: "Sarah",
    lastName: "Connor",
    role: "STUDENT" as const,
  };

  const sampleExam: StudentAssignedExam = {
    assignmentId: "asgn-uuid-1",
    examId: "exam-uuid-100",
    title: "Cellular Biology Final",
    subject: "Biology",
    description: "Comprehensive assessment covering cell structures and metabolism.",
    durationMins: 45,
    examStatus: "PUBLISHED",
    assignedAt: "2026-10-12T00:00:00Z",
    status: "ASSIGNED",
  };

  const activeSession: Session = {
    id: "sess-uuid-500",
    assignmentId: "asgn-uuid-1",
    examId: "exam-uuid-100",
    studentId: "student-uuid-999",
    status: "IN_PROGRESS",
    currentQuestionId: "q-uuid-1",
    startedAt: new Date().toISOString(),
    durationMins: 45,
    remainingSeconds: 2700,
  };

  const question1: StudentQuestionResponse = {
    id: "q-uuid-1",
    examId: "exam-uuid-100",
    questionNumber: 1,
    text: "What is the primary function of ribosomes in a cell?",
    type: "MCQ",
    points: 5,
    isFirst: true,
    isLast: false,
    totalQuestions: 2,
    currentIndex: 1,
    options: [
      { id: "opt-1", optionKey: "A", optionText: "Lipid storage", displayOrder: 1 },
      { id: "opt-2", optionKey: "B", optionText: "Protein synthesis", displayOrder: 2 },
    ],
  };

  const question2: StudentQuestionResponse = {
    id: "q-uuid-2",
    examId: "exam-uuid-100",
    questionNumber: 2,
    text: "Which phase of mitosis involves chromosome alignment at the equator?",
    type: "MCQ",
    points: 5,
    isFirst: false,
    isLast: true,
    totalQuestions: 2,
    currentIndex: 2,
    options: [
      { id: "opt-3", optionKey: "A", optionText: "Prophase", displayOrder: 1 },
      { id: "opt-4", optionKey: "B", optionText: "Metaphase", displayOrder: 2 },
    ],
  };

  beforeEach(() => {
    vi.clearAllMocks();
    useAuthStore.setState({
      user: studentUser,
      isAuthenticated: true,
      isHydrated: true,
      status: "authenticated",
      accessToken: "valid-token",
    });
  });

  it("Step 1: Candidate views assigned assessments and launches start exam confirmation", async () => {
    vi.mocked(assignmentsApi.getMyAssignedExams).mockResolvedValue({
      data: [sampleExam],
      total: 1,
      limit: 100,
      offset: 0,
    });
    vi.mocked(sessionsApi.getSessions).mockResolvedValue({
      sessions: [],
      total: 0,
    });
    vi.mocked(sessionsApi.startSession).mockResolvedValue(activeSession);

    render(<MyExamsPage />);

    expect(await screen.findByText("Cellular Biology Final")).toBeInTheDocument();
    expect(screen.getByText("45 minutes")).toBeInTheDocument();

    // Click Start Assessment -> Opens Instructions Dialog
    const startBtn = screen.getByTestId("start-assessment-exam-uuid-100");
    fireEvent.click(startBtn);

    expect(await screen.findByText("Allotted Duration: 45 minutes")).toBeInTheDocument();
    expect(screen.getByText(/The countdown timer starts as soon as you confirm/i)).toBeInTheDocument();

    // Confirm Start -> Starts session and routes to /sessions/[id]
    const confirmStartBtn = screen.getByRole("button", { name: /Confirm & Start Exam/i });
    fireEvent.click(confirmStartBtn);

    await waitFor(() => {
      expect(sessionsApi.startSession).toHaveBeenCalledWith({
        assignmentId: "asgn-uuid-1",
        examId: "exam-uuid-100",
      });
      expect(mockPush).toHaveBeenCalledWith("/sessions/sess-uuid-500");
    });
  });

  it("Step 2: Candidate in active exam room answers questions, navigates, and completes final submission", async () => {
    vi.mocked(sessionsApi.getSession).mockResolvedValue(activeSession);
    vi.mocked(sessionsApi.getCurrentQuestion).mockResolvedValue(question1);
    vi.mocked(sessionsApi.getAnswers).mockResolvedValue([]);
    vi.mocked(examsApi.getExam).mockResolvedValue({
      id: "exam-uuid-100",
      title: "Cellular Biology Final",
      subject: "Biology",
      durationMins: 45,
      status: "PUBLISHED",
      createdBy: "teacher-1",
      createdAt: "",
      updatedAt: "",
      description: "",
    });

    render(
      <Suspense fallback={<div>Loading exam room...</div>}>
        <ActiveSessionRoomPage params={{ id: "sess-uuid-500" }} />
      </Suspense>
    );

    expect(await screen.findByText("What is the primary function of ribosomes in a cell?")).toBeInTheDocument();
    expect(screen.getByText("Question 1 of 2")).toBeInTheDocument();
    expect(screen.getByText("Protein synthesis")).toBeInTheDocument();

    // Answer Question 1 (Select Option B)
    vi.mocked(sessionsApi.submitAnswer).mockResolvedValue({
      id: "ans-1",
      sessionId: "sess-uuid-500",
      questionId: "q-uuid-1",
      selectedOptionId: "opt-2",
      textAnswer: "",
    });

    const proteinSynthesisOpt = screen.getByText("Protein synthesis");
    fireEvent.click(proteinSynthesisOpt);

    await waitFor(() => {
      expect(sessionsApi.submitAnswer).toHaveBeenCalledWith("sess-uuid-500", "q-uuid-1", {
        selectedOptionId: "opt-2",
        textAnswer: "",
      });
      expect(screen.getByTestId("answer-save-status")).toHaveTextContent("Answer saved");
    });

    // Navigate to Question 2
    vi.mocked(sessionsApi.nextQuestion).mockResolvedValue({
      ...activeSession,
      currentQuestionId: "q-uuid-2",
    });
    vi.mocked(sessionsApi.getCurrentQuestion).mockResolvedValue(question2);

    const nextBtn = screen.getByTestId("next-question-button");
    fireEvent.click(nextBtn);

    expect(await screen.findByText("Which phase of mitosis involves chromosome alignment at the equator?")).toBeInTheDocument();
    expect(screen.getByText("Question 2 of 2")).toBeInTheDocument();

    // Answer Question 2 (Select Metaphase)
    vi.mocked(sessionsApi.submitAnswer).mockResolvedValue({
      id: "ans-2",
      sessionId: "sess-uuid-500",
      questionId: "q-uuid-2",
      selectedOptionId: "opt-4",
      textAnswer: "",
    });

    const metaphaseOpt = screen.getByText("Metaphase");
    fireEvent.click(metaphaseOpt);

    await waitFor(() => {
      expect(sessionsApi.submitAnswer).toHaveBeenCalledWith("sess-uuid-500", "q-uuid-2", {
        selectedOptionId: "opt-4",
        textAnswer: "",
      });
    });

    // Click Submit Exam on Last Question
    const reviewAndSubmitBtn = screen.getByTestId("last-question-review-button");
    fireEvent.click(reviewAndSubmitBtn);

    expect(await screen.findByText("Submit Exam Confirmation")).toBeInTheDocument();

    // Confirm Final Submission
    vi.mocked(sessionsApi.submitSession).mockResolvedValue({
      id: "sess-uuid-500",
      status: "SUBMITTED",
      submittedAt: "2026-10-12T10:55:00Z",
      message: "Exam session submitted successfully",
    });

    const confirmSubmitDialogBtn = screen.getByTestId("confirm-submit-dialog-button");
    fireEvent.click(confirmSubmitDialogBtn);

    // View Final Submission Screen
    expect(await screen.findByTestId("exam-submitted-card")).toBeInTheDocument();
    expect(screen.getByText("Exam Submitted Successfully")).toBeInTheDocument();
    expect(screen.getByText(/Return to Candidate Dashboard/i)).toBeInTheDocument();
  });

  it("handles page reload during an active session and seamlessly restores state", async () => {
    vi.mocked(sessionsApi.getSession).mockResolvedValue({
      ...activeSession,
      remainingSeconds: 1800,
    });
    vi.mocked(sessionsApi.getCurrentQuestion).mockResolvedValue(question2);
    vi.mocked(sessionsApi.getAnswers).mockResolvedValue([
      {
        id: "ans-1",
        sessionId: "sess-uuid-500",
        questionId: "q-uuid-1",
        selectedOptionId: "opt-2",
        textAnswer: "",
      },
      {
        id: "ans-2",
        sessionId: "sess-uuid-500",
        questionId: "q-uuid-2",
        selectedOptionId: "opt-4",
        textAnswer: "",
      },
    ]);
    vi.mocked(examsApi.getExam).mockResolvedValue({
      id: "exam-uuid-100",
      title: "Cellular Biology Final",
      subject: "Biology",
      durationMins: 45,
      status: "PUBLISHED",
      createdBy: "teacher-1",
      createdAt: "",
      updatedAt: "",
      description: "",
    });

    render(
      <Suspense fallback={<div>Loading exam room...</div>}>
        <ActiveSessionRoomPage params={{ id: "sess-uuid-500" }} />
      </Suspense>
    );

    // Current question 2 should be rendered with Option B (Metaphase) pre-selected
    expect(await screen.findByText("Which phase of mitosis involves chromosome alignment at the equator?")).toBeInTheDocument();
    const metaphaseRadio = screen.getByDisplayValue("opt-4");
    expect(metaphaseRadio).toBeChecked();
  });

  it("renders expired session view when session status is EXPIRED", async () => {
    vi.mocked(sessionsApi.getSession).mockResolvedValue({
      ...activeSession,
      status: "EXPIRED",
      remainingSeconds: 0,
    });

    render(
      <Suspense fallback={<div>Loading exam room...</div>}>
        <ActiveSessionRoomPage params={{ id: "sess-uuid-500" }} />
      </Suspense>
    );

    expect(await screen.findByTestId("exam-expired-card")).toBeInTheDocument();
    expect(screen.getByText("Exam Time Expired")).toBeInTheDocument();
  });
});
