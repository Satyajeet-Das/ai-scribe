import { render, screen, fireEvent, waitFor, act } from "@testing-library/react";
import { describe, it, expect, vi, beforeEach, afterEach } from "vitest";
import { StudentAutocompleteSelector } from "../student-autocomplete-selector";
import { studentsApi } from "@/services/api";
import type { StudentSearchResult } from "@/types/auth";

vi.mock("@/services/api", () => ({
  studentsApi: {
    searchStudents: vi.fn(),
  },
}));

describe("StudentAutocompleteSelector", () => {
  const mockStudents: StudentSearchResult[] = [
    {
      id: "student-1",
      rollNo: "23CS001",
      name: "Rahul Sharma",
      email: "rahul@school.edu",
    },
    {
      id: "student-2",
      rollNo: "23CS014",
      name: "Priya Singh",
      email: "priya@school.edu",
    },
  ];

  beforeEach(() => {
    vi.clearAllMocks();
  });

  afterEach(() => {
    vi.useRealTimers();
  });

  it("renders with accessible combobox attributes", () => {
    render(
      <StudentAutocompleteSelector onSelect={vi.fn()} selectedStudent={null} />
    );

    const combobox = screen.getByRole("combobox");
    expect(combobox).toBeInTheDocument();
    expect(combobox).toHaveAttribute("aria-expanded", "false");
    expect(combobox).toHaveAttribute("aria-haspopup", "listbox");
    expect(combobox).toHaveAttribute("aria-autocomplete", "list");
    expect(combobox).toHaveAttribute("placeholder", expect.stringContaining("roll number"));
  });

  it("does not trigger search when query is shorter than minChars", async () => {
    render(
      <StudentAutocompleteSelector onSelect={vi.fn()} selectedStudent={null} minChars={2} />
    );

    const combobox = screen.getByRole("combobox");
    fireEvent.change(combobox, { target: { value: "2" } });

    await act(async () => {
      await new Promise((resolve) => setTimeout(resolve, 350));
    });

    expect(studentsApi.searchStudents).not.toHaveBeenCalled();
    expect(screen.queryByRole("listbox")).not.toBeInTheDocument();
  });

  it("debounces and searches students when typing 2+ characters", async () => {
    vi.mocked(studentsApi.searchStudents).mockResolvedValue(mockStudents);

    render(
      <StudentAutocompleteSelector onSelect={vi.fn()} selectedStudent={null} />
    );

    const combobox = screen.getByRole("combobox");
    fireEvent.change(combobox, { target: { value: "23CS" } });

    await waitFor(
      () => {
        expect(studentsApi.searchStudents).toHaveBeenCalledWith(
          "23CS",
          10,
          expect.any(AbortSignal)
        );
      },
      { timeout: 1000 }
    );

    await waitFor(() => {
      expect(screen.getByRole("listbox")).toBeInTheDocument();
      expect(screen.getByText("23CS001")).toBeInTheDocument();
      expect(screen.getByText("Rahul Sharma")).toBeInTheDocument();
      expect(screen.getByText("23CS014")).toBeInTheDocument();
      expect(screen.getByText("Priya Singh")).toBeInTheDocument();
    });
  });

  it("displays 'No students found' when results are empty", async () => {
    vi.mocked(studentsApi.searchStudents).mockResolvedValue([]);

    render(
      <StudentAutocompleteSelector onSelect={vi.fn()} selectedStudent={null} />
    );

    const combobox = screen.getByRole("combobox");
    fireEvent.change(combobox, { target: { value: "UnknownRoll" } });

    await waitFor(
      () => {
        expect(screen.getByTestId("no-students-found")).toBeInTheDocument();
        expect(screen.getByText(/no active student matching/i)).toBeInTheDocument();
      },
      { timeout: 1000 }
    );
  });

  it("selects student on click and invokes onSelect callback", async () => {
    vi.mocked(studentsApi.searchStudents).mockResolvedValue(mockStudents);
    const handleSelect = vi.fn();

    render(
      <StudentAutocompleteSelector onSelect={handleSelect} selectedStudent={null} />
    );

    const combobox = screen.getByRole("combobox");
    fireEvent.change(combobox, { target: { value: "23CS" } });

    await waitFor(() => {
      expect(screen.getByText("Rahul Sharma")).toBeInTheDocument();
    });

    const option = screen.getByText("Rahul Sharma").closest("li");
    expect(option).toBeInTheDocument();
    fireEvent.click(option!);

    expect(handleSelect).toHaveBeenCalledWith(mockStudents[0]);
  });

  it("navigates options with ArrowDown, ArrowUp, and Enter", async () => {
    vi.mocked(studentsApi.searchStudents).mockResolvedValue(mockStudents);
    const handleSelect = vi.fn();

    render(
      <StudentAutocompleteSelector onSelect={handleSelect} selectedStudent={null} />
    );

    const combobox = screen.getByRole("combobox");
    fireEvent.change(combobox, { target: { value: "23CS" } });

    await waitFor(() => {
      expect(screen.getByRole("listbox")).toBeInTheDocument();
    });

    // Arrow down to highlight first option
    fireEvent.keyDown(combobox, { key: "ArrowDown" });
    const options = screen.getAllByRole("option");
    expect(options[0]).toHaveAttribute("aria-selected", "true");

    // Arrow down to highlight second option
    fireEvent.keyDown(combobox, { key: "ArrowDown" });
    expect(options[1]).toHaveAttribute("aria-selected", "true");

    // Arrow up to highlight first option again
    fireEvent.keyDown(combobox, { key: "ArrowUp" });
    expect(options[0]).toHaveAttribute("aria-selected", "true");

    // Press Enter to select
    fireEvent.keyDown(combobox, { key: "Enter" });
    expect(handleSelect).toHaveBeenCalledWith(mockStudents[0]);
  });

  it("shows already assigned badge and disables clicking already assigned students", async () => {
    vi.mocked(studentsApi.searchStudents).mockResolvedValue(mockStudents);
    const handleSelect = vi.fn();

    render(
      <StudentAutocompleteSelector
        onSelect={handleSelect}
        selectedStudent={null}
        existingStudentIds={["student-1"]}
      />
    );

    const combobox = screen.getByRole("combobox");
    fireEvent.change(combobox, { target: { value: "23CS" } });

    await waitFor(() => {
      expect(screen.getByText("Assigned")).toBeInTheDocument();
    });

    const assignedOption = screen.getByText("Rahul Sharma").closest("li");
    fireEvent.click(assignedOption!);

    expect(handleSelect).not.toHaveBeenCalled();
  });

  it("renders selected student card and allows clearing selection with Change button", async () => {
    const handleSelect = vi.fn();
    render(
      <StudentAutocompleteSelector
        onSelect={handleSelect}
        selectedStudent={mockStudents[0]}
      />
    );

    expect(screen.getByTestId("selected-student-card")).toBeInTheDocument();
    expect(screen.getByText("23CS001")).toBeInTheDocument();
    expect(screen.getByText("Rahul Sharma")).toBeInTheDocument();

    const changeButton = screen.getByRole("button", { name: /change/i });
    fireEvent.click(changeButton);

    expect(handleSelect).toHaveBeenCalledWith(null);
  });

  it("supports multiSelect mode with selection chips and removing individual chips", async () => {
    vi.mocked(studentsApi.searchStudents).mockResolvedValue(mockStudents);
    const handleSelectMultiple = vi.fn();

    render(
      <StudentAutocompleteSelector
        multiSelect={true}
        selectedStudents={[mockStudents[0]]}
        onSelectMultiple={handleSelectMultiple}
      />
    );

    // Selected chip for Rahul Sharma should be displayed
    expect(screen.getByText("Rahul Sharma")).toBeInTheDocument();
    expect(screen.getByText("23CS001")).toBeInTheDocument();

    // Remove the chip
    const removeBtn = screen.getByRole("button", { name: /remove candidate rahul sharma/i });
    fireEvent.click(removeBtn);

    expect(handleSelectMultiple).toHaveBeenCalledWith([]);
  });

  it("clears all selected students in multiSelect mode", () => {
    const handleSelectMultiple = vi.fn();
    render(
      <StudentAutocompleteSelector
        multiSelect={true}
        selectedStudents={mockStudents}
        onSelectMultiple={handleSelectMultiple}
      />
    );

    const clearAllBtn = screen.getByRole("button", { name: /clear all/i });
    expect(clearAllBtn).toBeInTheDocument();
    fireEvent.click(clearAllBtn);

    expect(handleSelectMultiple).toHaveBeenCalledWith([]);
  });

  it("displays search error state and provides retry action", async () => {
    vi.mocked(studentsApi.searchStudents).mockRejectedValueOnce(
      new Error("Network connection error")
    );

    render(
      <StudentAutocompleteSelector onSelect={vi.fn()} selectedStudent={null} />
    );

    const combobox = screen.getByRole("combobox");
    fireEvent.change(combobox, { target: { value: "23CS" } });

    await waitFor(() => {
      expect(
        screen.getByText(/failed to fetch students/i)
      ).toBeInTheDocument();
    });

    const retryBtn = screen.getByRole("button", { name: /retry/i });
    expect(retryBtn).toBeInTheDocument();

    vi.mocked(studentsApi.searchStudents).mockResolvedValueOnce(mockStudents);
    fireEvent.click(retryBtn);

    await waitFor(() => {
      expect(screen.getByText("Rahul Sharma")).toBeInTheDocument();
    });
  });
});

