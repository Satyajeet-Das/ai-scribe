import { render, screen, fireEvent, waitFor } from "@testing-library/react";
import { describe, it, expect, vi, beforeEach } from "vitest";
import { AssignmentManager } from "../assignment-manager";
import { assignmentsApi, studentsApi } from "@/services/api";
import type { Assignment } from "@/types/exam-types";
import type { StudentSearchResult } from "@/types/auth";

vi.mock("@/services/api", () => ({
  assignmentsApi: {
    createAssignment: vi.fn(),
    revokeAssignment: vi.fn(),
  },
  studentsApi: {
    searchStudents: vi.fn(),
  },
}));

describe("AssignmentManager", () => {
  const initialAssignments: Assignment[] = [
    {
      id: "assign-1",
      examId: "exam-100",
      studentId: "student-uuid-1",
      studentRollNo: "23CS001",
      studentName: "Rahul Sharma",
      assignedAt: "2025-02-20T10:00:00Z",
      status: "ACTIVE",
    },
  ];

  const searchStudentResult: StudentSearchResult = {
    id: "student-uuid-2",
    rollNo: "23CS014",
    name: "Priya Singh",
    email: "priya@school.edu",
  };

  beforeEach(() => {
    vi.clearAllMocks();
  });

  it("renders existing assigned candidates with roll numbers", () => {
    render(
      <AssignmentManager examId="exam-100" assignments={initialAssignments} />
    );

    expect(screen.getByText("23CS001")).toBeInTheDocument();
    expect(screen.getByText("Rahul Sharma")).toBeInTheDocument();
  });

  it("opens assign dialog and allows selecting candidate via autocomplete", async () => {
    vi.mocked(studentsApi.searchStudents).mockResolvedValue([searchStudentResult]);
    vi.mocked(assignmentsApi.createAssignment).mockResolvedValue({
      id: "assign-2",
      examId: "exam-100",
      studentId: "student-uuid-2",
      assignedAt: new Date().toISOString(),
      status: "ACTIVE",
    });

    render(
      <AssignmentManager examId="exam-100" assignments={initialAssignments} />
    );

    // Click Assign Candidate button
    const openBtn = screen.getByRole("button", { name: /assign candidate/i });
    fireEvent.click(openBtn);

    expect(screen.getByText("Assign Candidate to Exam")).toBeInTheDocument();

    // Type roll number to search
    const combobox = screen.getByRole("combobox");
    fireEvent.change(combobox, { target: { value: "23CS" } });

    await waitFor(() => {
      expect(screen.getByText("Priya Singh")).toBeInTheDocument();
    });

    // Select Priya Singh
    const option = screen.getByText("Priya Singh").closest("li");
    fireEvent.click(option!);

    // Verify selected card appears with roll number and name
    expect(screen.getByTestId("selected-student-card")).toBeInTheDocument();
    expect(screen.getByText("23CS014")).toBeInTheDocument();

    // Click Confirm Assignment
    const confirmBtn = screen.getByRole("button", { name: /confirm assignment/i });
    fireEvent.click(confirmBtn);

    // Verify backend assignment API received UUID
    await waitFor(() => {
      expect(assignmentsApi.createAssignment).toHaveBeenCalledWith(
        "exam-100",
        "student-uuid-2"
      );
    });

    // Verify newly assigned student is displayed in list with roll number
    await waitFor(() => {
      expect(screen.getByText("23CS014")).toBeInTheDocument();
      expect(screen.getByText("Priya Singh")).toBeInTheDocument();
    });
  });

  it("shows error when attempting to assign a candidate already assigned", async () => {
    vi.mocked(studentsApi.searchStudents).mockResolvedValue([
      {
        id: "student-uuid-1", // already in initialAssignments
        rollNo: "23CS001",
        name: "Rahul Sharma",
        email: "rahul@school.edu",
      },
    ]);

    render(
      <AssignmentManager examId="exam-100" assignments={initialAssignments} />
    );

    const openBtn = screen.getByRole("button", { name: /assign candidate/i });
    fireEvent.click(openBtn);

    const combobox = screen.getByRole("combobox");
    fireEvent.change(combobox, { target: { value: "23CS001" } });

    await waitFor(() => {
      expect(screen.getByText("Assigned")).toBeInTheDocument();
    });
  });

  it("opens confirmation dialog and revokes student assignment when confirmed", async () => {
    vi.mocked(assignmentsApi.revokeAssignment).mockResolvedValue();
    const onAssignmentChanged = vi.fn();

    render(
      <AssignmentManager
        examId="exam-100"
        assignments={initialAssignments}
        onAssignmentChanged={onAssignmentChanged}
      />
    );

    const revokeBtn = screen.getByRole("button", {
      name: /revoke assignment for 23cs001/i,
    });
    fireEvent.click(revokeBtn);

    expect(screen.getByText("Remove Student Assignment")).toBeInTheDocument();
    expect(screen.getByText(/Are you sure you want to unassign/i)).toBeInTheDocument();

    const confirmRemovalBtn = screen.getByRole("button", { name: /confirm removal/i });
    fireEvent.click(confirmRemovalBtn);

    await waitFor(() => {
      expect(assignmentsApi.revokeAssignment).toHaveBeenCalledWith("assign-1");
      expect(onAssignmentChanged).toHaveBeenCalled();
    });
  });
});
