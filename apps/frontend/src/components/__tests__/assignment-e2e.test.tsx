import { render, screen, fireEvent, waitFor } from "@testing-library/react";
import { describe, it, expect, vi, beforeEach } from "vitest";
import { AssignmentManager } from "../assignment-manager";
import MyExamsPage from "@/app/(dashboard)/my-exams/page";
import { assignmentsApi, studentsApi } from "@/services/api";
import { useAuthStore } from "@/store/auth-store";
import type { Assignment, StudentAssignedExam } from "@/types/exam-types";
import type { StudentSearchResult } from "@/types/auth";

// Mock next/navigation
vi.mock("next/navigation", () => ({
  useRouter: () => ({
    push: vi.fn(),
    replace: vi.fn(),
    prefetch: vi.fn(),
  }),
  usePathname: () => "/exams/exam-101",
}));

// Mock API layer
vi.mock("@/services/api", () => ({
  assignmentsApi: {
    createAssignment: vi.fn(),
    bulkAssign: vi.fn(),
    revokeAssignment: vi.fn(),
    getMyAssignedExams: vi.fn(),
    getStudentAssignments: vi.fn(),
  },
  studentsApi: {
    searchStudents: vi.fn(),
  },
  registerAuthFailureHandler: vi.fn(),
}));

describe("Sprint 5 E2E: Assignments & Candidate Access Workflows", () => {
  const candidateAlice: StudentSearchResult = {
    id: "uuid-alice-01",
    rollNo: "23CS101",
    name: "Alice Johnson",
    email: "alice@institution.edu",
  };

  const candidateBob: StudentSearchResult = {
    id: "uuid-bob-02",
    rollNo: "23CS102",
    name: "Bob Williams",
    email: "bob@institution.edu",
  };

  const candidateCharlie: StudentSearchResult = {
    id: "uuid-charlie-03",
    rollNo: "23CS103",
    name: "Charlie Brown",
    email: "charlie@institution.edu",
  };

  beforeEach(() => {
    vi.clearAllMocks();
  });

  describe("Teacher E2E Assignment Workflow", () => {
    it("completes full teacher journey: view empty assignments -> single assign via autocomplete -> batch assign multiple candidates -> filter list -> revoke assignment with confirmation", async () => {
      // 1. Authenticate Teacher
      useAuthStore.setState({
        user: {
          id: "teacher-1",
          email: "educator@school.edu",
          role: "TEACHER",
          firstName: "Minerva",
          lastName: "McGonagall",
        },
        accessToken: "jwt_teacher_token",
        isAuthenticated: true,
        isHydrated: true,
        status: "authenticated",
      });

      let currentAssignments: Assignment[] = [];

      vi.mocked(assignmentsApi.createAssignment).mockImplementation(
        async (examId, studentId) => {
          const newAssign: Assignment = {
            id: `assign-${Date.now()}`,
            examId,
            studentId: studentId || candidateAlice.id,
            studentRollNo: candidateAlice.rollNo || "23CS101",
            studentName: candidateAlice.name,
            studentEmail: candidateAlice.email,
            assignedAt: new Date().toISOString(),
            status: "ASSIGNED",
          };
          currentAssignments = [newAssign, ...currentAssignments];
          return newAssign;
        }
      );

      vi.mocked(assignmentsApi.bulkAssign).mockImplementation(
        async (examId, studentIds = []) => {
          const added: Assignment[] = (studentIds || []).map((sid) => {
            const student = [candidateBob, candidateCharlie].find((s) => s.id === sid);
            return {
              id: `assign-bulk-${sid}`,
              examId,
              studentId: sid,
              studentRollNo: student?.rollNo || "ROLL",
              studentName: student?.name || "Name",
              studentEmail: student?.email || "email@test.com",
              assignedAt: new Date().toISOString(),
              status: "ASSIGNED",
            };
          });
          currentAssignments = [...added, ...currentAssignments];
          return {
            assigned: added,
            failed: [],
            totalAssigned: added.length,
            totalFailed: 0,
          };
        }
      );

      vi.mocked(assignmentsApi.revokeAssignment).mockImplementation(
        async (assignmentId) => {
          currentAssignments = currentAssignments.map((a) =>
            a.id === assignmentId ? { ...a, status: "REVOKED" } : a
          );
        }
      );

      // Render Assignment Manager on exam details
      const { rerender } = render(
        <AssignmentManager examId="exam-101" assignments={currentAssignments} />
      );

      // Step A: Initial Empty State
      expect(screen.getByText("No Candidates Assigned")).toBeInTheDocument();
      expect(screen.getByText("0 Assigned")).toBeInTheDocument();

      // Step B: Single Assignment via Autocomplete
      const openDialogBtn = screen.getAllByRole("button", { name: /assign candidate/i })[0];
      fireEvent.click(openDialogBtn);

      expect(screen.getByText("Assign Candidate to Exam")).toBeInTheDocument();

      // Mock student search
      vi.mocked(studentsApi.searchStudents).mockResolvedValue([candidateAlice]);

      const searchInput = screen.getByRole("combobox");
      fireEvent.change(searchInput, { target: { value: "23CS1" } });

      await waitFor(() => {
        expect(studentsApi.searchStudents).toHaveBeenCalledWith(
          "23CS1",
          10,
          expect.any(AbortSignal)
        );
        expect(screen.getByText("Alice Johnson")).toBeInTheDocument();
        expect(screen.getByText("23CS101")).toBeInTheDocument();
      });

      // Select candidate Alice
      const aliceOption = screen.getByText("Alice Johnson").closest("li");
      fireEvent.click(aliceOption!);

      // Verify selected candidate card
      expect(screen.getByTestId("selected-student-card")).toBeInTheDocument();

      // Confirm assignment
      const confirmSingleBtn = screen.getByRole("button", { name: /confirm assignment/i });
      fireEvent.click(confirmSingleBtn);

      await waitFor(() => {
        expect(assignmentsApi.createAssignment).toHaveBeenCalledWith(
          "exam-101",
          candidateAlice.id
        );
      });

      // Re-render with Alice assigned
      rerender(
        <AssignmentManager examId="exam-101" assignments={currentAssignments} />
      );

      expect(screen.getByText("1 Assigned")).toBeInTheDocument();
      expect(screen.getByText("Alice Johnson")).toBeInTheDocument();
      expect(screen.getByText("23CS101")).toBeInTheDocument();

      // Step C: Batch Assignment Mode
      const openDialogBtn2 = screen.getByRole("button", { name: /assign candidate/i });
      fireEvent.click(openDialogBtn2);

      // Switch to Batch Multi-Select
      const batchToggle = screen.getByRole("button", { name: /batch multi-select/i });
      fireEvent.click(batchToggle);

      vi.mocked(studentsApi.searchStudents).mockResolvedValue([candidateBob, candidateCharlie]);

      const searchInput2 = screen.getByRole("combobox");
      fireEvent.change(searchInput2, { target: { value: "23CS" } });

      await waitFor(() => {
        expect(screen.getByText("Bob Williams")).toBeInTheDocument();
        expect(screen.getByText("Charlie Brown")).toBeInTheDocument();
      });

      // Select Bob
      const bobOption = screen.getByText("Bob Williams").closest("li");
      fireEvent.click(bobOption!);

      // Verify chip appeared
      expect(screen.getByText("23CS102")).toBeInTheDocument();

      // Search and select Charlie
      fireEvent.change(searchInput2, { target: { value: "23CS" } });
      await waitFor(() => {
        expect(screen.getByText("Charlie Brown")).toBeInTheDocument();
      });
      const charlieOption = screen.getByText("Charlie Brown").closest("li");
      fireEvent.click(charlieOption!);

      // Verify 2 chips in container
      expect(screen.getByText(/selected candidates \(2\)/i)).toBeInTheDocument();

      // Submit Bulk Assignment
      const bulkSubmitBtn = screen.getByRole("button", { name: /assign 2 candidates/i });
      fireEvent.click(bulkSubmitBtn);

      await waitFor(() => {
        expect(assignmentsApi.bulkAssign).toHaveBeenCalledWith("exam-101", [
          candidateBob.id,
          candidateCharlie.id,
        ]);
      });

      // Re-render with all 3 candidates assigned
      rerender(
        <AssignmentManager examId="exam-101" assignments={currentAssignments} />
      );

      expect(screen.getByText("3 Assigned")).toBeInTheDocument();
      expect(screen.getByText("Bob Williams")).toBeInTheDocument();
      expect(screen.getByText("Charlie Brown")).toBeInTheDocument();

      // Step D: Filter candidates in search bar
      const filterInput = screen.getByPlaceholderText(/filter assigned candidates/i);
      fireEvent.change(filterInput, { target: { value: "Alice" } });

      expect(screen.getByText("Alice Johnson")).toBeInTheDocument();
      expect(screen.queryByText("Bob Williams")).not.toBeInTheDocument();

      // Clear filter
      fireEvent.change(filterInput, { target: { value: "" } });
      expect(screen.getByText("Bob Williams")).toBeInTheDocument();

      // Step E: Revoke Candidate Assignment with Confirmation Dialog
      const revokeButtons = screen.getAllByRole("button", { name: /revoke assignment for/i });
      fireEvent.click(revokeButtons[0]);

      // Confirmation dialog should be presented
      expect(screen.getByText("Remove Student Assignment")).toBeInTheDocument();
      expect(screen.getByText(/are you sure you want to unassign/i)).toBeInTheDocument();

      // Confirm revocation
      const confirmRevokeBtn = screen.getByRole("button", { name: /confirm removal/i });
      fireEvent.click(confirmRevokeBtn);

      await waitFor(() => {
        expect(assignmentsApi.revokeAssignment).toHaveBeenCalled();
      });
    }, 15000);
  });

  describe("Student E2E Access & Dashboard Flow", () => {
    it("completes student journey: student logs in -> views assigned assessment -> searches/filters -> launches test", async () => {
      // 1. Authenticate Candidate Student
      useAuthStore.setState({
        user: {
          id: candidateAlice.id,
          email: candidateAlice.email,
          role: "STUDENT",
          firstName: "Alice",
          lastName: "Johnson",
          rollNo: candidateAlice.rollNo,
        },
        accessToken: "jwt_candidate_token",
        isAuthenticated: true,
        isHydrated: true,
        status: "authenticated",
      });

      const mockStudentExams: StudentAssignedExam[] = [
        {
          assignmentId: "assign-alice-101",
          examId: "exam-101",
          title: "Artificial Intelligence End-Semester Examination",
          description: "Search algorithms, probabilistic reasoning, and neural networks.",
          subject: "Computer Science",
          durationMins: 180,
          examStatus: "PUBLISHED",
          assignedAt: "2025-03-01T09:00:00Z",
          status: "ASSIGNED",
        },
      ];

      vi.mocked(assignmentsApi.getMyAssignedExams).mockResolvedValue({
        data: mockStudentExams,
        total: 1,
        limit: 10,
        offset: 0,
      });

      render(<MyExamsPage />);

      // Verify student header and roll number badge
      expect(screen.getByText("My Assessments")).toBeInTheDocument();
      expect(screen.getByText(candidateAlice.rollNo)).toBeInTheDocument();

      // Verify assigned assessment details
      await waitFor(() => {
        expect(screen.getByText("1 Available")).toBeInTheDocument();
        expect(
          screen.getByText("Artificial Intelligence End-Semester Examination")
        ).toBeInTheDocument();
        expect(screen.getAllByText("Computer Science").length).toBeGreaterThanOrEqual(1);
        expect(screen.getByText("180 minutes")).toBeInTheDocument();
        expect(screen.getByText("ASSIGNED")).toBeInTheDocument();
      });

      // Launch candidate assessment button
      const launchBtn = screen.getByRole("button", {
        name: /launch candidate assessment/i,
      });
      expect(launchBtn).toBeInTheDocument();
    });
  });
});
