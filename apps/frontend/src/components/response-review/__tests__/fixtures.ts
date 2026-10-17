import type {
  ReviewQuestion,
  ReviewStudentList,
  ReviewStudentResponses,
  ReviewStudentRow,
} from "@/types/response-review";

export const EXAM = {
  id: "exam-1",
  title: "Physics Midterm",
  subject: "Science",
  status: "PUBLISHED",
  durationMins: 60,
  totalQuestions: 3,
};

export function row(overrides: Partial<ReviewStudentRow> = {}): ReviewStudentRow {
  return {
    studentId: "s-1",
    rollNo: "R001",
    name: "Zoe Adams",
    status: "SUBMITTED",
    startedAt: "2026-03-01T09:00:00Z",
    submittedAt: "2026-03-01T09:30:00Z",
    answeredCount: 3,
    ...overrides,
  };
}

export function list(
  rows: ReviewStudentRow[],
  overrides: Partial<ReviewStudentList> = {}
): ReviewStudentList {
  return {
    exam: EXAM,
    summary: { assigned: 4, notStarted: 1, inProgress: 1, submitted: 1, expired: 1 },
    data: rows,
    total: rows.length,
    limit: 20,
    offset: 0,
    ...overrides,
  };
}

export const Q1: ReviewQuestion = {
  id: "q1",
  questionNumber: 1,
  text: "Which planet is closest to the sun?",
  type: "MCQ",
  maxMarks: 2,
  options: [
    { id: "o1", key: "A", text: "Venus", displayOrder: 1 },
    { id: "o2", key: "B", text: "Mercury", displayOrder: 2 },
  ],
  answer: {
    status: "ANSWERED",
    selectedOptionId: "o2",
    selectedOptionKey: "B",
    selectedOptionText: "Mercury",
    savedAt: "2026-03-01T09:10:00Z",
  },
};

export const Q2: ReviewQuestion = {
  id: "q2",
  questionNumber: 2,
  text: "Explain gravity.\nUse x^2 + y^2 = z^2 if helpful.",
  type: "ESSAY",
  maxMarks: 10,
  answer: {
    status: "ANSWERED",
    textAnswer: "Line one of the answer.\nLine two with E = mc^2.",
    savedAt: "2026-03-01T09:20:00Z",
  },
};

export const Q3: ReviewQuestion = {
  id: "q3",
  questionNumber: 3,
  text: "Why is the sky blue?",
  type: "ESSAY",
  maxMarks: 5,
  answer: { status: "UNANSWERED" },
};

export function responses(overrides: Partial<ReviewStudentResponses> = {}): ReviewStudentResponses {
  return {
    exam: EXAM,
    student: { id: "s-1", rollNo: "R001", name: "Zoe Adams" },
    status: "SUBMITTED",
    session: {
      id: "sess-1",
      status: "SUBMITTED",
      startedAt: "2026-03-01T09:00:00Z",
      submittedAt: "2026-03-01T09:30:00Z",
      isFinal: true,
    },
    responseAvailable: true,
    counts: { total: 3, answered: 2, unanswered: 1 },
    questions: [Q1, Q2, Q3],
    ...overrides,
  };
}

/** A promise whose settlement the test controls. */
export function deferred<T>() {
  let resolve!: (value: T) => void;
  let reject!: (reason: unknown) => void;
  const promise = new Promise<T>((res, rej) => {
    resolve = res;
    reject = rej;
  });
  return { promise, resolve, reject };
}
