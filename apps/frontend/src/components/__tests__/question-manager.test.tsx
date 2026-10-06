import { render, screen, fireEvent, waitFor } from "@testing-library/react";
import { describe, it, expect, vi, beforeEach } from "vitest";
import { QuestionManager } from "../question-manager";
import { questionsApi, ApiError } from "@/services/api";
import type { Question } from "@/types/exam-types";

const mockInitialQuestions: Question[] = [
  {
    id: "q-1",
    examId: "exam-101",
    questionNumber: 1,
    text: "What organelle is known as the powerhouse of the cell?",
    type: "MCQ",
    points: 2,
    options: [
      { id: "opt-1", optionKey: "A", optionText: "Ribosome", displayOrder: 1, isCorrect: false },
      { id: "opt-2", optionKey: "B", optionText: "Mitochondria", displayOrder: 2, isCorrect: true },
      { id: "opt-3", optionKey: "C", optionText: "Nucleus", displayOrder: 3, isCorrect: false },
    ],
  },
  {
    id: "q-2",
    examId: "exam-101",
    questionNumber: 2,
    text: "Explain the cellular mechanism of ATP synthesis.",
    type: "ESSAY",
    points: 5,
    options: [],
  },
];

describe("QuestionManager Component", () => {
  beforeEach(() => {
    vi.restoreAllMocks();
  });

  describe("Rendering & States", () => {
    it("renders the question studio header with question count and total points", () => {
      render(
        <QuestionManager
          examId="exam-101"
          examStatus="DRAFT"
          questions={mockInitialQuestions}
        />
      );

      expect(screen.getByText("Question Studio")).toBeInTheDocument();
      expect(screen.getByText(/2 questions/i)).toBeInTheDocument();
      expect(screen.getByText(/7 pts total/i)).toBeInTheDocument();
      expect(screen.getByText("Add Question")).toBeInTheDocument();
    });

    it("renders questions list and displays selected question preview", () => {
      render(
        <QuestionManager
          examId="exam-101"
          examStatus="DRAFT"
          questions={mockInitialQuestions}
        />
      );

      // Question 1 preview and options
      expect(
        screen.getByRole("heading", {
          name: "What organelle is known as the powerhouse of the cell?",
        })
      ).toBeInTheDocument();
      expect(screen.getByText("Ribosome")).toBeInTheDocument();
      expect(screen.getByText("Mitochondria")).toBeInTheDocument();
      expect(screen.getByText("Correct Answer")).toBeInTheDocument();
    });

    it("switches selected question when a question card is clicked", () => {
      render(
        <QuestionManager
          examId="exam-101"
          examStatus="DRAFT"
          questions={mockInitialQuestions}
        />
      );

      // Click Question 2 card
      const q2Card = screen.getByText("Explain the cellular mechanism of ATP synthesis.");
      fireEvent.click(q2Card);

      expect(screen.getByText("Written / Essay Response Mode")).toBeInTheDocument();
    });

    it("renders empty state when there are no questions and never shows fallback questions", () => {
      render(
        <QuestionManager
          examId="exam-101"
          examStatus="DRAFT"
          questions={[]}
        />
      );

      expect(screen.getByText("No Questions Configured")).toBeInTheDocument();
      expect(screen.getByText("Add Your First Question")).toBeInTheDocument();
      expect(screen.queryByText(/Which structure controls what enters and leaves a cell/i)).not.toBeInTheDocument();
    });
  });

  describe("Exam Lifecycle & Read-Only Enforcement", () => {
    it("locks studio in read-only mode for PUBLISHED exam", () => {
      render(
        <QuestionManager
          examId="exam-101"
          examStatus="PUBLISHED"
          questions={mockInitialQuestions}
        />
      );

      // Read-only notice displayed
      expect(
        screen.getByText(/Published Assessment \(Read-Only\)/i)
      ).toBeInTheDocument();

      // Modification actions must not be present
      expect(screen.queryByText("Add Question")).not.toBeInTheDocument();
      expect(screen.queryByRole("button", { name: /Edit question/i })).not.toBeInTheDocument();
      expect(screen.queryByRole("button", { name: /Delete question/i })).not.toBeInTheDocument();
      expect(screen.queryByRole("button", { name: /Move question 1 down/i })).not.toBeInTheDocument();
    });

    it("locks studio in read-only mode for ARCHIVED exam", () => {
      render(
        <QuestionManager
          examId="exam-101"
          examStatus="ARCHIVED"
          questions={mockInitialQuestions}
        />
      );

      expect(
        screen.getByText(/Archived Assessment \(Read-Only\)/i)
      ).toBeInTheDocument();
      expect(screen.queryByText("Add Question")).not.toBeInTheDocument();
    });
  });

  describe("Creating a Question (MCQ)", () => {
    it("validates required prompt and minimum options", async () => {
      render(
        <QuestionManager
          examId="exam-101"
          examStatus="DRAFT"
          questions={mockInitialQuestions}
        />
      );

      fireEvent.click(screen.getByText("Add Question"));
      expect(screen.getByText("Add a New Question")).toBeInTheDocument();

      // Submit with empty prompt
      fireEvent.click(screen.getByText("Save Question"));

      await waitFor(() => {
        expect(
          screen.getByText(/Question text must be at least 5 characters/i)
        ).toBeInTheDocument();
      });
    });

    it("creates a new MCQ with options and updates list", async () => {
      const mockCreated: Question = {
        id: "q-3",
        examId: "exam-101",
        questionNumber: 3,
        text: "What is the capital of France?",
        type: "MCQ",
        points: 3,
        options: [
          { id: "opt-a", optionKey: "A", optionText: "Berlin", displayOrder: 1, isCorrect: false },
          { id: "opt-b", optionKey: "B", optionText: "Paris", displayOrder: 2, isCorrect: true },
        ],
      };

      vi.spyOn(questionsApi, "createQuestion").mockResolvedValueOnce(mockCreated);

      render(
        <QuestionManager
          examId="exam-101"
          examStatus="DRAFT"
          questions={mockInitialQuestions}
        />
      );

      fireEvent.click(screen.getByText("Add Question"));

      // Fill in prompt
      const promptInput = screen.getByLabelText(/Question Prompt/i);
      fireEvent.change(promptInput, { target: { value: "What is the capital of France?" } });

      // Fill in options
      const optA = screen.getByLabelText("Option A text");
      fireEvent.change(optA, { target: { value: "Berlin" } });

      const optB = screen.getByLabelText("Option B text");
      fireEvent.change(optB, { target: { value: "Paris" } });

      // Mark option B as correct
      const optBButton = screen.getByRole("button", { name: /Option B/i });
      fireEvent.click(optBButton);

      // Save question
      fireEvent.click(screen.getByText("Save Question"));

      await waitFor(() => {
        expect(questionsApi.createQuestion).toHaveBeenCalledWith("exam-101", {
          questionNumber: 3,
          text: "What is the capital of France?",
          type: "MCQ",
          points: 2,
          options: [
            { optionKey: "A", optionText: "Berlin", displayOrder: 1, isCorrect: false },
            { optionKey: "B", optionText: "Paris", displayOrder: 2, isCorrect: true },
          ],
        });
      });

      await waitFor(() => {
        expect(screen.getByText(/Question 3 added successfully/i)).toBeInTheDocument();
        expect(
          screen.getByRole("heading", { name: "What is the capital of France?" })
        ).toBeInTheDocument();
      });
    });
  });

  describe("Editing a Question", () => {
    it("opens edit dialog, modifies question prompt and points, and saves", async () => {
      const mockUpdated: Question = {
        ...mockInitialQuestions[0],
        text: "Updated prompt about cell mitochondria?",
        points: 4,
      };

      vi.spyOn(questionsApi, "updateQuestion").mockResolvedValueOnce(mockUpdated);

      render(
        <QuestionManager
          examId="exam-101"
          examStatus="DRAFT"
          questions={mockInitialQuestions}
        />
      );

      // Click Edit
      fireEvent.click(screen.getByRole("button", { name: /Edit question 1/i }));
      expect(screen.getByText("Edit Question 1")).toBeInTheDocument();

      // Modify prompt and points
      const promptInput = screen.getByLabelText(/Question Prompt/i);
      fireEvent.change(promptInput, {
        target: { value: "Updated prompt about cell mitochondria?" },
      });

      const pointsInput = screen.getByLabelText(/Points/i);
      fireEvent.change(pointsInput, { target: { value: "4" } });

      // Save changes
      fireEvent.click(screen.getByText("Update Question"));

      await waitFor(() => {
        expect(questionsApi.updateQuestion).toHaveBeenCalledWith("q-1", {
          text: "Updated prompt about cell mitochondria?",
          points: 4,
        });
      });

      await waitFor(() => {
        expect(screen.getByText(/Question 1 updated successfully/i)).toBeInTheDocument();
        expect(
          screen.getByRole("heading", { name: "Updated prompt about cell mitochondria?" })
        ).toBeInTheDocument();
      });
    });
  });

  describe("Deleting a Question", () => {
    it("prompts with confirmation dialog and deletes question upon confirm", async () => {
      vi.spyOn(questionsApi, "deleteQuestion").mockResolvedValueOnce();

      render(
        <QuestionManager
          examId="exam-101"
          examStatus="DRAFT"
          questions={mockInitialQuestions}
        />
      );

      // Click Delete on selected Question 1
      fireEvent.click(screen.getByRole("button", { name: /Delete question 1/i }));

      // Confirmation dialog opens
      expect(screen.getByText("Delete Question 1?")).toBeInTheDocument();
      expect(
        screen.getByText(/Associated multiple-choice options will be removed/i)
      ).toBeInTheDocument();

      // Confirm delete
      const confirmButton = screen.getByRole("button", { name: "Delete Question" });
      fireEvent.click(confirmButton);

      await waitFor(() => {
        expect(questionsApi.deleteQuestion).toHaveBeenCalledWith("q-1");
      });

      await waitFor(() => {
        expect(screen.getByText(/Question 1 deleted and remaining questions re-sequenced/i)).toBeInTheDocument();
      });
    });
  });

  describe("Reordering Questions", () => {
    it("moves question down and calls backend reorder endpoint", async () => {
      const mockReordered: Question[] = [
        { ...mockInitialQuestions[1], questionNumber: 1 },
        { ...mockInitialQuestions[0], questionNumber: 2 },
      ];

      vi.spyOn(questionsApi, "reorderQuestions").mockResolvedValueOnce(mockReordered);

      render(
        <QuestionManager
          examId="exam-101"
          examStatus="DRAFT"
          questions={mockInitialQuestions}
        />
      );

      // Move question 1 down
      const moveDownBtn = screen.getByRole("button", { name: "Move question 1 down" });
      fireEvent.click(moveDownBtn);

      await waitFor(() => {
        expect(questionsApi.reorderQuestions).toHaveBeenCalledWith("exam-101", {
          questionIds: ["q-2", "q-1"],
        });
      });

      await waitFor(() => {
        expect(screen.getByText(/moved down successfully/i)).toBeInTheDocument();
      });
    });

    it("reverts optimistic reorder on API failure", async () => {
      vi.spyOn(questionsApi, "reorderQuestions").mockRejectedValueOnce(
        new ApiError("Failed to reorder", 400)
      );

      render(
        <QuestionManager
          examId="exam-101"
          examStatus="DRAFT"
          questions={mockInitialQuestions}
        />
      );

      const moveDownBtn = screen.getByRole("button", { name: "Move question 1 down" });
      fireEvent.click(moveDownBtn);

      await waitFor(() => {
        expect(screen.getByText("Failed to reorder")).toBeInTheDocument();
      });
    });
  });

  describe("Complete E2E Workflow", () => {
    it("completes full teacher flow: Open Draft Exam -> Add Question -> Configure MCQ -> Save -> Edit -> Reorder", async () => {
      // Step 1: Open draft exam with 1 existing question
      const initial: Question[] = [
        {
          id: "q-1",
          examId: "exam-999",
          questionNumber: 1,
          text: "First baseline question",
          type: "ESSAY",
          points: 5,
          options: [],
        },
      ];

      const newQ: Question = {
        id: "q-2",
        examId: "exam-999",
        questionNumber: 2,
        text: "What is the boiling point of pure water at sea level?",
        type: "MCQ",
        points: 3,
        options: [
          { id: "opt-1", optionKey: "A", optionText: "90 C", displayOrder: 1, isCorrect: false },
          { id: "opt-2", optionKey: "B", optionText: "100 C", displayOrder: 2, isCorrect: true },
        ],
      };

      const updatedQ2: Question = {
        ...newQ,
        text: "What is the standard boiling point of water at 1 atm?",
        points: 4,
      };

      const createSpy = vi.spyOn(questionsApi, "createQuestion").mockResolvedValueOnce(newQ);
      const updateSpy = vi.spyOn(questionsApi, "updateQuestion").mockResolvedValueOnce(updatedQ2);
      const reorderSpy = vi.spyOn(questionsApi, "reorderQuestions").mockResolvedValueOnce([
        { ...updatedQ2, questionNumber: 1 },
        { ...initial[0], questionNumber: 2 },
      ]);

      render(
        <QuestionManager
          examId="exam-999"
          examStatus="DRAFT"
          questions={initial}
        />
      );

      // Step 2: Click Add Question
      fireEvent.click(screen.getByText("Add Question"));
      expect(screen.getByText("Add a New Question")).toBeInTheDocument();

      // Step 3: Configure MCQ with prompt, points, options
      const promptInput = screen.getByLabelText(/Question Prompt/i);
      fireEvent.change(promptInput, {
        target: { value: "What is the boiling point of pure water at sea level?" },
      });

      const optA = screen.getByLabelText("Option A text");
      fireEvent.change(optA, { target: { value: "90 C" } });

      const optB = screen.getByLabelText("Option B text");
      fireEvent.change(optB, { target: { value: "100 C" } });

      // Mark B as correct
      const optBButton = screen.getByRole("button", { name: /Option B/i });
      fireEvent.click(optBButton);

      // Step 4: Save Question
      fireEvent.click(screen.getByText("Save Question"));

      await waitFor(() => {
        expect(createSpy).toHaveBeenCalledWith("exam-999", {
          questionNumber: 2,
          text: "What is the boiling point of pure water at sea level?",
          type: "MCQ",
          points: 2,
          options: [
            { optionKey: "A", optionText: "90 C", displayOrder: 1, isCorrect: false },
            { optionKey: "B", optionText: "100 C", displayOrder: 2, isCorrect: true },
          ],
        });
      });

      await waitFor(() => {
        expect(screen.getByText(/Question 2 added successfully/i)).toBeInTheDocument();
      });

      // Step 5: Edit the new question
      fireEvent.click(screen.getByRole("button", { name: /Edit question 2/i }));
      expect(screen.getByText("Edit Question 2")).toBeInTheDocument();

      const editPromptInput = screen.getByLabelText(/Question Prompt/i);
      fireEvent.change(editPromptInput, {
        target: { value: "What is the standard boiling point of water at 1 atm?" },
      });

      fireEvent.click(screen.getByText("Update Question"));

      await waitFor(() => {
        expect(updateSpy).toHaveBeenCalledWith("q-2", {
          text: "What is the standard boiling point of water at 1 atm?",
          points: 3,
        });
      });

      await waitFor(() => {
        expect(screen.getByText(/Question 2 updated successfully/i)).toBeInTheDocument();
      });

      // Step 6: Reorder Question 2 up
      const moveUpBtn = screen.getByRole("button", { name: "Move question 2 up" });
      fireEvent.click(moveUpBtn);

      await waitFor(() => {
        expect(reorderSpy).toHaveBeenCalledWith("exam-999", {
          questionIds: ["q-2", "q-1"],
        });
      });

      await waitFor(() => {
        expect(screen.getByText(/Question 2 moved up successfully/i)).toBeInTheDocument();
      });
    });
  });
});
