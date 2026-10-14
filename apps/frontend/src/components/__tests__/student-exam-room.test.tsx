import { render, screen, fireEvent, waitFor } from "@testing-library/react";
import { describe, it, expect, vi, beforeEach } from "vitest";
import { StudentExamRoom } from "../student-exam-room";
import type { StudentQuestionResponse } from "@/types/exam-types";

describe("Sprint 6: StudentExamRoom Component", () => {
  const mockQuestion1: StudentQuestionResponse = {
    id: "q-uuid-1",
    examId: "exam-uuid-1",
    questionNumber: 1,
    text: "Which organelle is considered the powerhouse of the cell?",
    type: "MCQ",
    points: 5,
    isFirst: true,
    isLast: false,
    totalQuestions: 3,
    currentIndex: 1,
    options: [
      { id: "opt-1", optionKey: "A", optionText: "Nucleus", displayOrder: 1 },
      { id: "opt-2", optionKey: "B", optionText: "Mitochondria", displayOrder: 2 },
      { id: "opt-3", optionKey: "C", optionText: "Ribosome", displayOrder: 3 },
      { id: "opt-4", optionKey: "D", optionText: "Endoplasmic Reticulum", displayOrder: 4 },
    ],
  };

  const mockQuestion3: StudentQuestionResponse = {
    id: "q-uuid-3",
    examId: "exam-uuid-1",
    questionNumber: 3,
    text: "What is the primary product of photosynthesis?",
    type: "MCQ",
    points: 10,
    isFirst: false,
    isLast: true,
    totalQuestions: 3,
    currentIndex: 3,
    options: [
      { id: "opt-31", optionKey: "A", optionText: "Glucose", displayOrder: 1 },
      { id: "opt-32", optionKey: "B", optionText: "Carbon dioxide", displayOrder: 2 },
    ],
  };

  beforeEach(() => {
    vi.clearAllMocks();
  });

  it("renders question text, question number, points, and MCQ options", () => {
    render(
      <StudentExamRoom
        currentQuestion={mockQuestion1}
        answers={{}}
        onSelectOption={vi.fn()}
        onNext={vi.fn()}
        onPrevious={vi.fn()}
        onSubmit={vi.fn()}
        examTitle="Biology 101 Midterm"
        subject="Biology"
      />
    );

    expect(screen.getByText("Biology 101 Midterm")).toBeInTheDocument();
    expect(screen.getByText("Question 1 of 3")).toBeInTheDocument();
    expect(screen.getByText("5 points")).toBeInTheDocument();
    expect(
      screen.getByText("Which organelle is considered the powerhouse of the cell?")
    ).toBeInTheDocument();

    expect(screen.getByText("Nucleus")).toBeInTheDocument();
    expect(screen.getByText("Mitochondria")).toBeInTheDocument();
    expect(screen.getByText("Ribosome")).toBeInTheDocument();
    expect(screen.getByText("Endoplasmic Reticulum")).toBeInTheDocument();
  });

  it("selects an option and triggers onSelectOption callback", () => {
    const onSelectOption = vi.fn();
    render(
      <StudentExamRoom
        currentQuestion={mockQuestion1}
        answers={{}}
        onSelectOption={onSelectOption}
        onNext={vi.fn()}
        onPrevious={vi.fn()}
        onSubmit={vi.fn()}
      />
    );

    const optionB = screen.getByText("Mitochondria");
    fireEvent.click(optionB);

    expect(onSelectOption).toHaveBeenCalledWith("opt-2");
  });

  it("displays real-time save status: saving, saved, and failed with retry", () => {
    const onRetrySave = vi.fn();
    const { rerender } = render(
      <StudentExamRoom
        currentQuestion={mockQuestion1}
        answers={{ "q-uuid-1": "opt-2" }}
        saveStatus="saving"
        onSelectOption={vi.fn()}
        onNext={vi.fn()}
        onPrevious={vi.fn()}
        onSubmit={vi.fn()}
      />
    );

    expect(screen.getByTestId("answer-save-status")).toHaveTextContent("Saving answer...");

    // Saved state
    rerender(
      <StudentExamRoom
        currentQuestion={mockQuestion1}
        answers={{ "q-uuid-1": "opt-2" }}
        saveStatus="saved"
        onSelectOption={vi.fn()}
        onNext={vi.fn()}
        onPrevious={vi.fn()}
        onSubmit={vi.fn()}
      />
    );
    expect(screen.getByTestId("answer-save-status")).toHaveTextContent("Answer saved");

    // Failed state with retry button
    rerender(
      <StudentExamRoom
        currentQuestion={mockQuestion1}
        answers={{ "q-uuid-1": "opt-2" }}
        saveStatus="failed"
        saveError="Network timeout"
        onRetrySave={onRetrySave}
        onSelectOption={vi.fn()}
        onNext={vi.fn()}
        onPrevious={vi.fn()}
        onSubmit={vi.fn()}
      />
    );
    expect(screen.getByTestId("answer-save-status")).toHaveTextContent("Failed to save");
    expect(screen.getByTestId("retry-save-button")).toBeInTheDocument();

    fireEvent.click(screen.getByTestId("retry-save-button"));
    expect(onRetrySave).toHaveBeenCalledTimes(1);
  });

  it("handles boundary navigation: Previous disabled on first question, Review & Submit on last", () => {
    const onNext = vi.fn();
    const onPrevious = vi.fn();

    const { rerender } = render(
      <StudentExamRoom
        currentQuestion={mockQuestion1}
        answers={{}}
        onSelectOption={vi.fn()}
        onNext={onNext}
        onPrevious={onPrevious}
        onSubmit={vi.fn()}
      />
    );

    const prevButton = screen.getByTestId("prev-question-button");
    const nextButton = screen.getByTestId("next-question-button");

    expect(prevButton).toBeDisabled();
    expect(nextButton).not.toBeDisabled();

    fireEvent.click(nextButton);
    expect(onNext).toHaveBeenCalledTimes(1);

    // On last question
    rerender(
      <StudentExamRoom
        currentQuestion={mockQuestion3}
        answers={{ "q-uuid-1": "opt-2" }}
        onSelectOption={vi.fn()}
        onNext={onNext}
        onPrevious={onPrevious}
        onSubmit={vi.fn()}
      />
    );

    expect(screen.getByTestId("prev-question-button")).not.toBeDisabled();
    expect(screen.getByTestId("last-question-review-button")).toHaveTextContent("Review & Submit");
  });

  it("opens Submit Exam confirmation dialog and shows answered and unanswered counts", async () => {
    const onSubmit = vi.fn();
    render(
      <StudentExamRoom
        currentQuestion={mockQuestion1}
        answers={{ "q-uuid-1": "opt-2" }} // 1 answered out of 3
        totalQuestions={3}
        onSelectOption={vi.fn()}
        onNext={vi.fn()}
        onPrevious={vi.fn()}
        onSubmit={onSubmit}
      />
    );

    const submitBtn = screen.getByTestId("submit-exam-button");
    fireEvent.click(submitBtn);

    expect(screen.getByText("Submit Exam Confirmation")).toBeInTheDocument();
    expect(screen.getByText(/You have answered/)).toBeInTheDocument();
    expect(screen.getByText(/Warning: You still have/)).toBeInTheDocument();
    expect(screen.getByTestId("unanswered-count")).toHaveTextContent("2"); // 2 unanswered

    const confirmBtn = screen.getByTestId("confirm-submit-dialog-button");
    fireEvent.click(confirmBtn);

    await waitFor(() => {
      expect(onSubmit).toHaveBeenCalledTimes(1);
    });
  });

  it("toggles large text and high contrast accessibility switches", () => {
    const { container } = render(
      <StudentExamRoom
        currentQuestion={mockQuestion1}
        answers={{}}
        onSelectOption={vi.fn()}
        onNext={vi.fn()}
        onPrevious={vi.fn()}
        onSubmit={vi.fn()}
      />
    );

    const largeTextSwitch = screen.getByRole("switch", { name: /Large text/i });
    const contrastSwitch = screen.getByRole("switch", { name: /High contrast/i });

    expect(largeTextSwitch).not.toBeChecked();
    expect(contrastSwitch).not.toBeChecked();

    fireEvent.click(largeTextSwitch);
    expect(largeTextSwitch).toBeChecked();
    expect(container.firstChild).toHaveClass("text-lg");

    fireEvent.click(contrastSwitch);
    expect(contrastSwitch).toBeChecked();
    expect(container.firstChild).toHaveClass("bg-foreground text-background");
  });

  it("supports keyboard navigation shortcuts Alt+N and Alt+P", () => {
    const onNext = vi.fn();
    const onPrevious = vi.fn();

    const middleQuestion: StudentQuestionResponse = {
      ...mockQuestion1,
      questionNumber: 2,
      currentIndex: 2,
      isFirst: false,
      isLast: false,
    };

    render(
      <StudentExamRoom
        currentQuestion={middleQuestion}
        answers={{}}
        onSelectOption={vi.fn()}
        onNext={onNext}
        onPrevious={onPrevious}
        onSubmit={vi.fn()}
      />
    );

    // Alt + N -> Next
    fireEvent.keyDown(window, { key: "n", altKey: true });
    expect(onNext).toHaveBeenCalledTimes(1);

    // Alt + P -> Previous
    fireEvent.keyDown(window, { key: "p", altKey: true });
    expect(onPrevious).toHaveBeenCalledTimes(1);
  });
});
