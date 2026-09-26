export type SessionStatus = "initialized" | "in_progress" | "paused" | "completed" | "terminated";

export interface ExamSession {
  id: string;
  assignmentId: string;
  candidateId: string;
  examId: string;
  status: SessionStatus;
  startedAt?: string;
  endedAt?: string;
  currentIndex: number;
}
