export const APP_CONFIG = {
  name: "AI Exam Scribe",
  description: "Accessible voice-first examination platform",
  apiUrl: process.env.NEXT_PUBLIC_API_URL || "http://localhost:8080",
} as const;

export const API_ROUTES = {
  health: "/status",
  docs: "/docs",
  auth: "/api/v1/auth",
  exams: "/api/v1/exams",
  questions: "/api/v1/questions",
  assignments: "/api/v1/assignments",
  sessions: "/api/v1/sessions",
  answers: "/api/v1/answers",
} as const;
