export type ExamStatus = "draft" | "scheduled" | "active" | "completed" | "archived";

export interface Exam {
  id: string;
  title: string;
  description: string;
  subject: string;
  durationMinutes: number;
  status: ExamStatus;
  scheduledStart?: string;
  scheduledEnd?: string;
  createdAt: string;
  updatedAt: string;
}

export interface CreateExamInput {
  title: string;
  description?: string;
  subject: string;
  durationMinutes: number;
  scheduledStart?: string;
  scheduledEnd?: string;
}
