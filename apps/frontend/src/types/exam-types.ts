export type ExamStatus = "DRAFT" | "PUBLISHED" | "ARCHIVED";
export type QuestionType = "MCQ" | "ESSAY" | "VOICE";
export type UserRole = "TEACHER" | "STUDENT" | "PROCTOR" | "ADMIN";
export type AssignmentStatus = "ACTIVE" | "REVOKED";

export interface Exam {
  id: string;
  title: string;
  subject: string;
  description: string;
  durationMins: number;
  status: ExamStatus;
  createdBy: string;
  publishedAt?: string;
  createdAt: string;
  updatedAt: string;
  candidates?: number;
  questions?: number;
}

export interface QuestionOption {
  id: string;
  optionKey: string;
  optionText: string;
  displayOrder: number;
  isCorrect: boolean;
}

export interface Question {
  id: string;
  examId: string;
  questionNumber: number;
  text: string;
  type: QuestionType;
  points: number;
  options: QuestionOption[];
}

export interface Assignment {
  id: string;
  examId: string;
  studentId: string;
  assignedAt: string;
  status: AssignmentStatus;
}

export interface Session {
  id: string;
  assignmentId: string;
  examId: string;
  studentId: string;
  status: "IN_PROGRESS" | "SUBMITTED" | "EXPIRED";
  startedAt: string;
  submittedAt?: string;
  durationMins?: number;
  remainingSeconds?: number;
}

export interface Answer {
  id: string;
  sessionId: string;
  questionId: string;
  selectedOptionId?: string;
  textAnswer: string;
}

export interface User {
  id: string;
  email: string;
  firstName: string;
  lastName: string;
  role: UserRole;
}

export interface AuthUser {
  id: string;
  email: string;
  firstName: string;
  lastName: string;
  role: UserRole;
}

export type AuthMode = "login" | "register";

export interface ExamQuestion extends Question {
  answer: string;
}

export interface SessionRecord {
  session: Session;
  answers: Answer[];
}
