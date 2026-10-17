import type { MarkingQuestion, MarkingSheet } from "@/types/marking";

export function markingQuestion(overrides: Partial<MarkingQuestion> = {}): MarkingQuestion {
  return {
    questionId: "q2",
    questionNumber: 2,
    type: "ESSAY",
    maxMarks: 10,
    answered: true,
    suggestedMarks: null,
    teacherMarks: null,
    effectiveMarks: null,
    markSource: "NONE",
    comment: null,
    version: 0,
    exceedsMax: false,
    ...overrides,
  };
}

/**
 * A submitted paper in draft, matching the review fixtures: Q1 multiple choice,
 * auto-marked 2 of 2; Q2 an essay not yet marked (10); Q3 unanswered, so 0 of 5
 * automatically.
 */
export function markingSheet(overrides: Partial<MarkingSheet> = {}): MarkingSheet {
  return {
    sessionId: "sess-1",
    sessionStatus: "SUBMITTED",
    markingAllowed: true,
    paper: {
      status: "DRAFT",
      version: 0,
      totalMarks: 2,
      maxMarks: 17,
      markedCount: 2,
      totalQuestions: 3,
    },
    questions: [
      markingQuestion({
        questionId: "q1",
        questionNumber: 1,
        type: "MCQ",
        maxMarks: 2,
        correctOptionId: "o2",
        suggestedMarks: 2,
        effectiveMarks: 2,
        markSource: "AUTO",
      }),
      markingQuestion(),
      markingQuestion({
        questionId: "q3",
        questionNumber: 3,
        maxMarks: 5,
        answered: false,
        suggestedMarks: 0,
        effectiveMarks: 0,
        markSource: "AUTO",
      }),
    ],
    ...overrides,
  };
}

/** The same paper once Q2 has been marked 7.5 and the paper finalised. */
export function finalSheet(): MarkingSheet {
  const s = markingSheet();
  return {
    ...s,
    paper: {
      status: "FINAL",
      version: 3,
      totalMarks: 9.5,
      maxMarks: 17,
      markedCount: 3,
      totalQuestions: 3,
      finalisedAt: "2026-03-02T10:00:00Z",
    },
    questions: s.questions.map((q) =>
      q.questionId === "q2"
        ? {
            ...q,
            teacherMarks: 7.5,
            effectiveMarks: 7.5,
            markSource: "TEACHER" as const,
            version: 1,
          }
        : { ...q, correctOptionId: undefined }
    ),
  };
}
