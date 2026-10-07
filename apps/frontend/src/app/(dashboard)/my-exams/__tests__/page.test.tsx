import { render, screen, fireEvent, waitFor } from "@testing-library/react";
import { describe, it, expect, vi, beforeEach } from "vitest";
import MyExamsPage from "../page";
import { assignmentsApi } from "@/services/api";
import { useAuthStore } from "@/store/auth-store";
import type { StudentAssignedExam } from "@/types/exam-types";

// Mock next/navigation
vi.mock("next/navigation", () => ({
  useRouter: () => ({
    push: vi.fn(),
    replace: vi.fn(),
    prefetch: vi.fn(),
  }),
  usePathname: () => "/my-exams",
}));

// Mock assignments API
vi.mock("@/services/api", () => ({
  assignmentsApi: {
    getMyAssignedExams: vi.fn(),
    getStudentAssignments: vi.fn(),
  },
  registerAuthFailureHandler: vi.fn(),
}));

describe("MyExamsPage (Candidate Student Dashboard)", () => {
  const mockAssignedExams: StudentAssignedExam[] = [
    {
      assignmentId: "assign-1",
      examId: "exam-1",
      title: "Data Structures & Algorithms Midterm",
      description: "Covers linked lists, trees, graphs, and sorting algorithms.",
      subject: "Computer Science",
      durationMins: 90,
      examStatus: "PUBLISHED",
      assignedAt: "2025-03-01T10:00:00Z",
      status: "ASSIGNED",
    },
    {
      assignmentId: "assign-2",
      examId: "exam-2",
      title: "Operating Systems Final",
      description: "Covers processes, threads, virtual memory, and file systems.",
      subject: "Information Technology",
      durationMins: 120,
      examStatus: "PUBLISHED",
      assignedAt: "2025-03-05T14:30:00Z",
      status: "ACTIVE",
    },
  ];

  beforeEach(() => {
    vi.clearAllMocks();
    useAuthStore.setState({
      user: {
        id: "student-uuid-1",
        email: "candidate@school.edu",
        role: "STUDENT",
        firstName: "Aarav",
        lastName: "Gupta",
        rollNo: "23CS042",
      },
      isAuthenticated: true,
      isHydrated: true,
      status: "authenticated",
    });
  });

  it("renders assigned candidate assessments with header, badge, and roll number", async () => {
    vi.mocked(assignmentsApi.getMyAssignedExams).mockResolvedValue({
      data: mockAssignedExams,
      total: 2,
      limit: 10,
      offset: 0,
    });

    render(<MyExamsPage />);

    expect(screen.getByText("My Assessments")).toBeInTheDocument();

    await waitFor(() => {
      expect(screen.getByText("Data Structures & Algorithms Midterm")).toBeInTheDocument();
      expect(screen.getByText("Operating Systems Final")).toBeInTheDocument();
    });

    // Roll number display
    expect(screen.getByText("23CS042")).toBeInTheDocument();
    // Available count badge
    expect(screen.getByText("2 Available")).toBeInTheDocument();
    // Subjects rendered (in filter buttons and exam badges)
    expect(screen.getAllByText("Computer Science").length).toBeGreaterThanOrEqual(1);
    expect(screen.getAllByText("Information Technology").length).toBeGreaterThanOrEqual(1);
    // Duration display
    expect(screen.getByText("90 minutes")).toBeInTheDocument();
    expect(screen.getByText("120 minutes")).toBeInTheDocument();

    // Launch assessment action buttons
    const launchButtons = screen.getAllByRole("button", {
      name: /launch candidate assessment/i,
    });
    expect(launchButtons).toHaveLength(2);
  });

  it("filters assessments by search query", async () => {
    vi.mocked(assignmentsApi.getMyAssignedExams).mockResolvedValue({
      data: mockAssignedExams,
      total: 2,
      limit: 10,
      offset: 0,
    });

    render(<MyExamsPage />);

    await waitFor(() => {
      expect(screen.getByText("Data Structures & Algorithms Midterm")).toBeInTheDocument();
    });

    const searchInput = screen.getByPlaceholderText(/search by title, subject/i);
    fireEvent.change(searchInput, { target: { value: "Operating" } });

    await waitFor(
      () => {
        expect(screen.queryByText("Data Structures & Algorithms Midterm")).not.toBeInTheDocument();
        expect(screen.getByText("Operating Systems Final")).toBeInTheDocument();
      },
      { timeout: 1000 }
    );
  });

  it("filters assessments by subject button chips", async () => {
    vi.mocked(assignmentsApi.getMyAssignedExams).mockResolvedValue({
      data: mockAssignedExams,
      total: 2,
      limit: 10,
      offset: 0,
    });

    render(<MyExamsPage />);

    await waitFor(() => {
      expect(screen.getByText("Data Structures & Algorithms Midterm")).toBeInTheDocument();
    });

    // Click "Computer Science" subject filter chip
    const csButton = screen.getByRole("button", { name: "Computer Science" });
    fireEvent.click(csButton);

    await waitFor(() => {
      expect(screen.getByText("Data Structures & Algorithms Midterm")).toBeInTheDocument();
      expect(screen.queryByText("Operating Systems Final")).not.toBeInTheDocument();
    });

    // Reset to "All Subjects"
    const allButton = screen.getByRole("button", { name: "All Subjects" });
    fireEvent.click(allButton);

    await waitFor(() => {
      expect(screen.getByText("Operating Systems Final")).toBeInTheDocument();
    });
  });

  it("renders clean empty state with no fallback questions or exams when student has no assignments", async () => {
    vi.mocked(assignmentsApi.getMyAssignedExams).mockResolvedValue({
      data: [] as StudentAssignedExam[],
      total: 0,
      limit: 10,
      offset: 0,
    });

    render(<MyExamsPage />);

    await waitFor(() => {
      expect(screen.getByText("No Assigned Assessments")).toBeInTheDocument();
      expect(
        screen.getByText(
          /you currently have no pending examinations assigned to your candidate profile/i
        )
      ).toBeInTheDocument();
    });

    // Check Again action is available
    expect(screen.getByRole("button", { name: /check again/i })).toBeInTheDocument();

    // No search input or exam cards
    expect(screen.queryByPlaceholderText(/search by title/i)).not.toBeInTheDocument();
    expect(screen.queryByTestId("assigned-exams-grid")).not.toBeInTheDocument();
  });

  it("renders error state when API fails and allows retry", async () => {
    vi.mocked(assignmentsApi.getMyAssignedExams).mockRejectedValueOnce(
      new Error("Failed to load assessments from server")
    );
    vi.mocked(assignmentsApi.getStudentAssignments).mockRejectedValueOnce(
      new Error("Fallback error")
    );

    render(<MyExamsPage />);

    await waitFor(() => {
      expect(screen.getByRole("alert")).toBeInTheDocument();
      expect(
        screen.getByText("Failed to load assessments from server")
      ).toBeInTheDocument();
    });

    // Retry fetching
    vi.mocked(assignmentsApi.getMyAssignedExams).mockResolvedValueOnce({
      data: mockAssignedExams,
      total: 2,
      limit: 10,
      offset: 0,
    });

    const retryBtn = screen.getByRole("button", { name: /retry/i });
    fireEvent.click(retryBtn);

    await waitFor(() => {
      expect(screen.getByText("Data Structures & Algorithms Midterm")).toBeInTheDocument();
      expect(screen.queryByRole("alert")).not.toBeInTheDocument();
    });
  });
});
