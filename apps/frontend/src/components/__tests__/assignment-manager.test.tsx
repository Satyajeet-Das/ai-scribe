import { render, screen, fireEvent, waitFor } from "@testing-library/react";
import { describe, it, expect, vi, beforeEach } from "vitest";
import { AssignmentManager } from "../assignment-manager";
import { assignmentsApi, studentsApi } from "@/services/api";
import type { Assignment } from "@/types/exam-types";
import type { StudentSearchResult } from "@/types/auth";

vi.mock("@/services/api", () => ({
  assignmentsApi: {
    createAssignment: vi.fn(),
    bulkAssign: vi.fn(),
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

  it("renders clean empty state with CTA when no candidates are assigned", () => {
    render(<AssignmentManager examId="exam-100" assignments={[]} />);

    expect(screen.getByText("No Candidates Assigned")).toBeInTheDocument();
    expect(
      screen.getByText(/No students are currently allocated to this assessment/i)
    ).toBeInTheDocument();
    expect(
      screen.getAllByRole("button", { name: /assign candidate/i }).length
    ).toBeGreaterThanOrEqual(1);
    expect(screen.queryByText("23CS001")).not.toBeInTheDocument();
    expect(screen.queryByText("Rahul Sharma")).not.toBeInTheDocument();
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

  it("filters assigned candidates by roll number or name", () => {
    const multipleAssignments: Assignment[] = [
      {
        id: "assign-1",
        examId: "exam-100",
        studentId: "student-1",
        studentRollNo: "23CS001",
        studentName: "Rahul Sharma",
        assignedAt: "2025-02-20T10:00:00Z",
        status: "ACTIVE",
      },
      {
        id: "assign-2",
        examId: "exam-100",
        studentId: "student-2",
        studentRollNo: "23CS099",
        studentName: "Ananya Patel",
        assignedAt: "2025-02-21T10:00:00Z",
        status: "ACTIVE",
      },
    ];

    render(
      <AssignmentManager examId="exam-100" assignments={multipleAssignments} />
    );

    expect(screen.getByText("Rahul Sharma")).toBeInTheDocument();
    expect(screen.getByText("Ananya Patel")).toBeInTheDocument();

    // Filter by "23CS099"
    const filterInput = screen.getByPlaceholderText(/filter assigned candidates/i);
    fireEvent.change(filterInput, { target: { value: "23CS099" } });

    expect(screen.queryByText("Rahul Sharma")).not.toBeInTheDocument();
    expect(screen.getByText("Ananya Patel")).toBeInTheDocument();
  });

  it("filters candidates by status (Active vs Revoked)", () => {
    const mixedAssignments: Assignment[] = [
      {
        id: "assign-1",
        examId: "exam-100",
        studentId: "student-1",
        studentRollNo: "23CS001",
        studentName: "Rahul Sharma",
        assignedAt: "2025-02-20T10:00:00Z",
        status: "ACTIVE",
      },
      {
        id: "assign-2",
        examId: "exam-100",
        studentId: "student-2",
        studentRollNo: "23CS002",
        studentName: "Revoked Student",
        assignedAt: "2025-02-21T10:00:00Z",
        status: "REVOKED",
      },
    ];

    render(
      <AssignmentManager examId="exam-100" assignments={mixedAssignments} />
    );

    expect(screen.getByText("Rahul Sharma")).toBeInTheDocument();
    expect(screen.getByText("Revoked Student")).toBeInTheDocument();

    // Click Active button
    const activeBtn = screen.getByRole("button", { name: /active/i });
    fireEvent.click(activeBtn);

    expect(screen.getByText("Rahul Sharma")).toBeInTheDocument();
    expect(screen.queryByText("Revoked Student")).not.toBeInTheDocument();

    // Click Revoked button
    const revokedBtn = screen.getByRole("button", { name: /revoked/i });
    fireEvent.click(revokedBtn);

    expect(screen.queryByText("Rahul Sharma")).not.toBeInTheDocument();
    expect(screen.getByText("Revoked Student")).toBeInTheDocument();
  });

  it("supports switching to batch mode and assigning multiple candidates", async () => {
    vi.mocked(studentsApi.searchStudents).mockResolvedValue([
      { id: "student-uuid-3", rollNo: "23CS030", name: "Student Three", email: "three@test.com" },
      { id: "student-uuid-4", rollNo: "23CS040", name: "Student Four", email: "four@test.com" },
    ]);
    vi.mocked(assignmentsApi.bulkAssign).mockResolvedValue({
      assigned: [
        {
          id: "bulk-1",
          examId: "exam-100",
          studentId: "student-uuid-3",
          assignedAt: new Date().toISOString(),
          status: "ASSIGNED",
        },
        {
          id: "bulk-2",
          examId: "exam-100",
          studentId: "student-uuid-4",
          assignedAt: new Date().toISOString(),
          status: "ASSIGNED",
        },
      ],
      failed: [],
      totalAssigned: 2,
      totalFailed: 0,
    });

    render(
      <AssignmentManager examId="exam-100" assignments={initialAssignments} />
    );

    const openBtn = screen.getByRole("button", { name: /assign candidate/i });
    fireEvent.click(openBtn);

    // Switch to Batch Multi-Select mode
    const batchModeBtn = screen.getByRole("button", { name: /batch multi-select/i });
    fireEvent.click(batchModeBtn);

    // Search students
    const combobox = screen.getByRole("combobox");
    fireEvent.change(combobox, { target: { value: "23CS" } });

    await waitFor(() => {
      expect(screen.getByText("Student Three")).toBeInTheDocument();
    });

    // Select Student Three
    const optionThree = screen.getByText("Student Three").closest("li");
    fireEvent.click(optionThree!);

    // Re-type to select Student Four
    fireEvent.change(combobox, { target: { value: "23CS" } });
    await waitFor(() => {
      expect(screen.getByText("Student Four")).toBeInTheDocument();
    });
    const optionFour = screen.getByText("Student Four").closest("li");
    fireEvent.click(optionFour!);

    // Confirm button shows count
    const assignBtn = screen.getByRole("button", { name: /assign 2 candidates/i });
    expect(assignBtn).toBeInTheDocument();
    fireEvent.click(assignBtn);

    await waitFor(() => {
      expect(assignmentsApi.bulkAssign).toHaveBeenCalledWith("exam-100", [
        "student-uuid-3",
        "student-uuid-4",
      ]);
    });
  });

  it("paginates candidates when total exceeds page size", () => {
    const manyAssignments: Assignment[] = Array.from({ length: 8 }).map((_, i) => ({
      id: `assign-${i + 1}`,
      examId: "exam-100",
      studentId: `student-${i + 1}`,
      studentRollNo: `23CS00${i + 1}`,
      studentName: `Student ${i + 1}`,
      assignedAt: "2025-02-20T10:00:00Z",
      status: "ACTIVE",
    }));

    render(
      <AssignmentManager examId="exam-100" assignments={manyAssignments} />
    );

    // First page shows 1 to 6
    expect(screen.getByText("Student 1")).toBeInTheDocument();
    expect(screen.getByText("Student 6")).toBeInTheDocument();
    expect(screen.queryByText("Student 7")).not.toBeInTheDocument();

    // Click Next
    const nextBtn = screen.getByRole("button", { name: /next/i });
    fireEvent.click(nextBtn);

    // Second page shows Student 7 and 8
    expect(screen.queryByText("Student 1")).not.toBeInTheDocument();
    expect(screen.getByText("Student 7")).toBeInTheDocument();
    expect(screen.getByText("Student 8")).toBeInTheDocument();
  });
});
