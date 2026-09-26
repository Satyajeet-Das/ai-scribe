import type {
  Exam,
  Question,
  QuestionOption,
  Assignment,
  Session,
  Answer,
  ExamStatus,
  QuestionType,
} from "@/types/exam-types";

const API_BASE_URL = process.env.NEXT_PUBLIC_API_URL || "http://localhost:8080/api/v1";

export interface UserResponse {
  id: string;
  email: string;
  firstName: string;
  lastName: string;
  role: "TEACHER" | "STUDENT" | "PROCTOR" | "ADMIN";
}

export interface LoginResponse {
  accessToken: string;
  expiresIn: number;
  user: UserResponse;
}

export interface RegisterResponse {
  user: UserResponse;
}

export interface RefreshResponse {
  accessToken: string;
  expiresIn: number;
}

export interface ExamListResponse {
  exams: Exam[];
  total: number;
  limit: number;
  offset: number;
}

export class ApiError extends Error {
  status: number;
  code?: string;
  override?: boolean;

  constructor(message: string, status: number, code?: string, override?: boolean) {
    super(message);
    this.name = "ApiError";
    this.status = status;
    this.code = code;
    this.override = override;
  }
}

// Token storage in memory with localStorage backup for browser reload
let inMemoryToken: string | null = null;

export function getStoredToken(): string | null {
  if (inMemoryToken) return inMemoryToken;
  if (typeof window !== "undefined") {
    inMemoryToken = localStorage.getItem("ai_scribe_access_token");
  }
  return inMemoryToken;
}

export function setStoredToken(token: string | null): void {
  inMemoryToken = token;
  if (typeof window !== "undefined") {
    if (token) {
      localStorage.setItem("ai_scribe_access_token", token);
    } else {
      localStorage.removeItem("ai_scribe_access_token");
    }
  }
}

export function clearStoredToken(): void {
  setStoredToken(null);
}

// Low-level fetch wrapper
async function apiFetch<T>(endpoint: string, options: RequestInit = {}): Promise<T> {
  const url = `${API_BASE_URL}${endpoint}`;
  const headers = new Headers(options.headers || {});

  if (!headers.has("Content-Type") && !(options.body instanceof FormData)) {
    headers.set("Content-Type", "application/json");
  }

  const token = getStoredToken();
  if (token && !headers.has("Authorization")) {
    headers.set("Authorization", `Bearer ${token}`);
  }

  const config: RequestInit = {
    ...options,
    headers,
    credentials: "include", // Automatically passes HTTP-only refresh cookies
  };

  let response: Response;
  try {
    response = await fetch(url, config);
  } catch (err) {
    throw new ApiError(
      err instanceof Error ? err.message : "Network connection failed. Ensure backend is running.",
      0
    );
  }

  // Handle 401: attempt silent refresh once
  if (response.status === 401 && !endpoint.includes("/auth/")) {
    try {
      const refreshRes = await authApi.refresh();
      if (refreshRes?.accessToken) {
        setStoredToken(refreshRes.accessToken);
        headers.set("Authorization", `Bearer ${refreshRes.accessToken}`);
        response = await fetch(url, { ...config, headers });
      }
    } catch {
      clearStoredToken();
    }
  }

  if (response.status === 204) {
    return {} as T;
  }

  let data: any;
  const contentType = response.headers.get("content-type");
  if (contentType && contentType.includes("application/json")) {
    data = await response.json();
  } else {
    data = await response.text();
  }

  if (!response.ok) {
    const errorMsg =
      (typeof data === "object" && (data.message || data.error)) ||
      response.statusText ||
      "Request failed";
    const code = typeof data === "object" ? data.code : undefined;
    const override = typeof data === "object" ? data.override : undefined;
    throw new ApiError(errorMsg, response.status, code, override);
  }

  return data as T;
}

// -----------------------------------------------------------------------------
// Authentication API
// -----------------------------------------------------------------------------
export const authApi = {
  async register(payload: {
    email: string;
    password: string;
    firstName: string;
    lastName: string;
    role: string;
  }): Promise<RegisterResponse> {
    return apiFetch<RegisterResponse>("/auth/register", {
      method: "POST",
      body: JSON.stringify(payload),
    });
  },

  async login(payload: { email: string; password: string }): Promise<LoginResponse> {
    const res = await apiFetch<LoginResponse>("/auth/login", {
      method: "POST",
      body: JSON.stringify(payload),
    });
    if (res.accessToken) {
      setStoredToken(res.accessToken);
    }
    return res;
  },

  async refresh(): Promise<RefreshResponse> {
    const res = await apiFetch<RefreshResponse>("/auth/refresh", {
      method: "POST",
    });
    if (res.accessToken) {
      setStoredToken(res.accessToken);
    }
    return res;
  },

  async logout(): Promise<void> {
    try {
      await apiFetch("/auth/logout", { method: "POST" });
    } finally {
      clearStoredToken();
    }
  },

  async getMe(): Promise<UserResponse> {
    return apiFetch<UserResponse>("/auth/me");
  },
};

// -----------------------------------------------------------------------------
// Exams API
// -----------------------------------------------------------------------------
export const examsApi = {
  async getExams(params?: {
    status?: ExamStatus;
    limit?: number;
    offset?: number;
  }): Promise<ExamListResponse> {
    const query = new URLSearchParams();
    if (params?.status) query.set("status", params.status);
    if (params?.limit) query.set("limit", String(params.limit));
    if (params?.offset) query.set("offset", String(params.offset));
    const qStr = query.toString() ? `?${query.toString()}` : "";
    return apiFetch<ExamListResponse>(`/exams${qStr}`);
  },

  async getExam(id: string): Promise<Exam> {
    return apiFetch<Exam>(`/exams/${id}`);
  },

  async createExam(payload: {
    title: string;
    subject: string;
    description: string;
    durationMins: number;
  }): Promise<Exam> {
    return apiFetch<Exam>("/exams", {
      method: "POST",
      body: JSON.stringify(payload),
    });
  },

  async updateExam(id: string, payload: Partial<Exam>): Promise<Exam> {
    return apiFetch<Exam>(`/exams/${id}`, {
      method: "PUT",
      body: JSON.stringify(payload),
    });
  },

  async publishExam(id: string): Promise<Exam> {
    return apiFetch<Exam>(`/exams/${id}/publish`, {
      method: "POST",
    });
  },

  async archiveExam(id: string): Promise<Exam> {
    return apiFetch<Exam>(`/exams/${id}/archive`, {
      method: "POST",
    });
  },
};

// -----------------------------------------------------------------------------
// Questions API
// -----------------------------------------------------------------------------
export const questionsApi = {
  async getQuestions(examId: string): Promise<Question[]> {
    const res = await apiFetch<{ questions: Question[] }>(`/exams/${examId}/questions`);
    return res.questions || [];
  },

  async getQuestion(id: string): Promise<Question> {
    return apiFetch<Question>(`/questions/${id}`);
  },

  async createQuestion(
    examId: string,
    payload: {
      questionNumber: number;
      text: string;
      type: QuestionType;
      points: number;
    }
  ): Promise<Question> {
    return apiFetch<Question>(`/exams/${examId}/questions`, {
      method: "POST",
      body: JSON.stringify(payload),
    });
  },

  async updateQuestion(
    id: string,
    payload: {
      questionNumber?: number;
      text?: string;
      points?: number;
    }
  ): Promise<Question> {
    return apiFetch<Question>(`/questions/${id}`, {
      method: "PUT",
      body: JSON.stringify(payload),
    });
  },

  async deleteQuestion(id: string): Promise<void> {
    return apiFetch<void>(`/questions/${id}`, {
      method: "DELETE",
    });
  },

  async createOption(
    questionId: string,
    payload: {
      optionKey: string;
      optionText: string;
      displayOrder: number;
      isCorrect: boolean;
    }
  ): Promise<QuestionOption> {
    return apiFetch<QuestionOption>(`/questions/${questionId}/options`, {
      method: "POST",
      body: JSON.stringify(payload),
    });
  },

  async updateOption(
    questionId: string,
    optionId: string,
    payload: Partial<QuestionOption>
  ): Promise<QuestionOption> {
    return apiFetch<QuestionOption>(`/questions/${questionId}/options/${optionId}`, {
      method: "PUT",
      body: JSON.stringify(payload),
    });
  },

  async deleteOption(questionId: string, optionId: string): Promise<void> {
    return apiFetch<void>(`/questions/${questionId}/options/${optionId}`, {
      method: "DELETE",
    });
  },
};

// -----------------------------------------------------------------------------
// Assignments API
// -----------------------------------------------------------------------------
export const assignmentsApi = {
  async createAssignment(payload: { examId: string; studentId: string }): Promise<Assignment> {
    return apiFetch<Assignment>("/assignments", {
      method: "POST",
      body: JSON.stringify(payload),
    });
  },

  async getExamAssignments(examId: string): Promise<Assignment[]> {
    const res = await apiFetch<{ assignments: Assignment[] }>(`/assignments/exam/${examId}`);
    return res.assignments || [];
  },

  async getStudentAssignments(studentId: string): Promise<Assignment[]> {
    const res = await apiFetch<{ assignments: Assignment[] }>(`/assignments/student/${studentId}`);
    return res.assignments || [];
  },

  async revokeAssignment(id: string): Promise<void> {
    return apiFetch<void>(`/assignments/${id}`, {
      method: "DELETE",
    });
  },
};

// -----------------------------------------------------------------------------
// Sessions & Answers API
// -----------------------------------------------------------------------------
export const sessionsApi = {
  async startSession(payload: { assignmentId: string }): Promise<Session> {
    return apiFetch<Session>("/sessions/start", {
      method: "POST",
      body: JSON.stringify(payload),
    });
  },

  async getSession(id: string): Promise<Session> {
    return apiFetch<Session>(`/sessions/${id}`);
  },

  async submitSession(
    id: string
  ): Promise<{ id: string; status: string; submittedAt: string; message: string }> {
    return apiFetch<{ id: string; status: string; submittedAt: string; message: string }>(
      `/sessions/${id}/submit`,
      { method: "POST" }
    );
  },

  async submitAnswer(
    sessionId: string,
    payload: {
      questionId: string;
      selectedOptionId?: string;
      textAnswer: string;
    }
  ): Promise<Answer> {
    return apiFetch<Answer>(`/sessions/${sessionId}/answers`, {
      method: "POST",
      body: JSON.stringify(payload),
    });
  },

  async getAnswers(sessionId: string): Promise<Answer[]> {
    const res = await apiFetch<{ answers: Answer[] }>(`/sessions/${sessionId}/answers`);
    return res.answers || [];
  },
};
