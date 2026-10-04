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

import type {
  User,
  UserResponse,
  LoginCredentials,
  RegisterPayload,
  LoginResponse,
  RegisterResponse,
  RefreshResponse,
  LogoutResponse,
  ApiFieldError,
  ApiAction,
  ApiErrorResponse,
} from "@/types/auth";

export type {
  User,
  UserResponse,
  LoginCredentials,
  RegisterPayload,
  LoginResponse,
  RegisterResponse,
  RefreshResponse,
  LogoutResponse,
};

const API_BASE_URL = process.env.NEXT_PUBLIC_API_URL || "http://localhost:8080/api/v1";
const DEFAULT_TIMEOUT_MS = 15000;

// -----------------------------------------------------------------------------
// Normalized API Error
// -----------------------------------------------------------------------------
export class ApiError extends Error {
  status: number;
  code?: string;
  override?: boolean;
  errors?: ApiFieldError[];
  action?: ApiAction;

  constructor(
    message: string,
    status: number,
    code?: string,
    override?: boolean,
    errors?: ApiFieldError[],
    action?: ApiAction
  ) {
    super(message);
    this.name = "ApiError";
    this.status = status;
    this.code = code;
    this.override = override;
    this.errors = errors;
    this.action = action;
  }

  isUnauthorized(): boolean {
    return this.status === 401;
  }

  isForbidden(): boolean {
    return this.status === 403;
  }

  isNotFound(): boolean {
    return this.status === 404;
  }

  isConflict(): boolean {
    return this.status === 409;
  }

  isValidation(): boolean {
    return this.status === 400 && (this.errors?.length ?? 0) > 0;
  }
}

// -----------------------------------------------------------------------------
// Token Storage (Dual-Layer: Memory + LocalStorage)
// -----------------------------------------------------------------------------
let inMemoryToken: string | null = null;
const STORAGE_TOKEN_KEY = "ai_scribe_access_token";

export function getStoredToken(): string | null {
  if (inMemoryToken) return inMemoryToken;
  if (typeof window !== "undefined") {
    try {
      inMemoryToken = localStorage.getItem(STORAGE_TOKEN_KEY);
    } catch {
      // Handle private browsing or restricted environments safely
      inMemoryToken = null;
    }
  }
  return inMemoryToken;
}

export function setStoredToken(token: string | null): void {
  inMemoryToken = token;
  if (typeof window !== "undefined") {
    try {
      if (token) {
        localStorage.setItem(STORAGE_TOKEN_KEY, token);
      } else {
        localStorage.removeItem(STORAGE_TOKEN_KEY);
      }
    } catch {
      // Silently ignore storage failures in restricted environments
    }
  }
}

export function clearStoredToken(): void {
  setStoredToken(null);
}

// -----------------------------------------------------------------------------
// Refresh Mutex & Auth Failure Callback
// -----------------------------------------------------------------------------
let refreshPromise: Promise<string | null> | null = null;
let authFailureCallback: (() => void) | null = null;

export function registerAuthFailureHandler(callback: () => void): () => void {
  authFailureCallback = callback;
  return () => {
    if (authFailureCallback === callback) {
      authFailureCallback = null;
    }
  };
}

export interface ApiFetchOptions extends RequestInit {
  timeoutMs?: number;
  skipAuth?: boolean;
  skipRefresh?: boolean;
}

// -----------------------------------------------------------------------------
// Core Fetch Implementation with Mutex Refresh & Retries
// -----------------------------------------------------------------------------
export async function apiFetch<T>(endpoint: string, options: ApiFetchOptions = {}): Promise<T> {
  const isAuthEndpoint =
    endpoint.includes("/auth/login") ||
    endpoint.includes("/auth/refresh") ||
    endpoint.includes("/auth/register");

  const url = endpoint.startsWith("http") ? endpoint : `${API_BASE_URL}${endpoint}`;
  const headers = new Headers(options.headers || {});

  if (!headers.has("Content-Type") && !(options.body instanceof FormData)) {
    headers.set("Content-Type", "application/json");
  }

  if (!options.skipAuth) {
    const token = getStoredToken();
    if (token && !headers.has("Authorization")) {
      headers.set("Authorization", `Bearer ${token}`);
    }
  }

  // Setup timeout and abort signal
  const timeoutMs = options.timeoutMs ?? DEFAULT_TIMEOUT_MS;
  const controller = new AbortController();
  const timeoutId = setTimeout(() => {
    controller.abort(new Error(`Request timed out after ${timeoutMs}ms`));
  }, timeoutMs);

  const signal = controller.signal;
  if (options.signal) {
    const originalSignal = options.signal;
    if (originalSignal.aborted) {
      clearTimeout(timeoutId);
      throw new ApiError("Request aborted", 0);
    }
    originalSignal.addEventListener("abort", () => {
      controller.abort();
    });
  }

  const config: RequestInit = {
    ...options,
    headers,
    signal,
    credentials: "include", // Transmit HttpOnly refresh cookie to backend
  };

  let response: Response;
  try {
    response = await fetch(url, config);
  } catch (err: unknown) {
    clearTimeout(timeoutId);
    if (err instanceof Error && err.name === "AbortError") {
      throw new ApiError("Request was cancelled or timed out", 0);
    }
    throw new ApiError(
      err instanceof Error ? err.message : "Network connection failed. Ensure backend is running.",
      0
    );
  } finally {
    clearTimeout(timeoutId);
  }

  // Handle 401 Unauthorized with Refresh Mutex Queue
  if (response.status === 401 && !isAuthEndpoint && !options.skipRefresh) {
    try {
      // Singleton Refresh Promise: all concurrent 401s wait for this one
      if (!refreshPromise) {
        refreshPromise = (async () => {
          try {
            const refreshRes = await authApi.refresh();
            if (refreshRes?.accessToken) {
              setStoredToken(refreshRes.accessToken);
              return refreshRes.accessToken;
            }
            return null;
          } catch {
            clearStoredToken();
            authFailureCallback?.();
            return null;
          } finally {
            refreshPromise = null;
          }
        })();
      }

      const newAccessToken = await refreshPromise;

      if (newAccessToken) {
        // Retry the original request with the new access token
        const retryHeaders = new Headers(options.headers || {});
        if (!retryHeaders.has("Content-Type") && !(options.body instanceof FormData)) {
          retryHeaders.set("Content-Type", "application/json");
        }
        retryHeaders.set("Authorization", `Bearer ${newAccessToken}`);

        return apiFetch<T>(endpoint, {
          ...options,
          headers: retryHeaders,
          skipRefresh: true, // Prevent infinite retry loops
        });
      }
    } catch {
      // Fall through to parse original 401 response
    }
  }

  if (response.status === 204) {
    return {} as T;
  }

  let data: unknown;
  const contentType = response.headers.get("content-type");
  if (contentType && contentType.includes("application/json")) {
    try {
      data = await response.json();
    } catch {
      data = null;
    }
  } else {
    try {
      data = await response.text();
    } catch {
      data = null;
    }
  }

  if (!response.ok) {
    const errorRecord =
      typeof data === "object" && data !== null ? (data as ApiErrorResponse) : null;

    const errorMsg =
      errorRecord?.message ||
      (typeof data === "string" && data.length > 0 ? data : null) ||
      response.statusText ||
      `Request failed with status ${response.status}`;

    const code = errorRecord?.code;
    const override = errorRecord?.override;
    const fieldErrors = errorRecord?.errors;
    const action = errorRecord?.action;

    throw new ApiError(errorMsg, response.status, code, override, fieldErrors, action);
  }

  return data as T;
}

// -----------------------------------------------------------------------------
// Authentication API (Provider-Independent Interface)
// -----------------------------------------------------------------------------
export const authApi = {
  async register(payload: RegisterPayload): Promise<RegisterResponse> {
    return apiFetch<RegisterResponse>("/auth/register", {
      method: "POST",
      body: JSON.stringify(payload),
      skipAuth: true,
      skipRefresh: true,
    });
  },

  async login(credentials: LoginCredentials): Promise<LoginResponse> {
    const res = await apiFetch<LoginResponse>("/auth/login", {
      method: "POST",
      body: JSON.stringify(credentials),
      skipAuth: true,
      skipRefresh: true,
    });
    if (res.accessToken) {
      setStoredToken(res.accessToken);
    }
    return res;
  },

  async refresh(): Promise<RefreshResponse> {
    const res = await apiFetch<RefreshResponse>("/auth/refresh", {
      method: "POST",
      skipAuth: true,
      skipRefresh: true,
    });
    if (res.accessToken) {
      setStoredToken(res.accessToken);
    }
    return res;
  },

  async logout(): Promise<LogoutResponse> {
    try {
      return await apiFetch<LogoutResponse>("/auth/logout", {
        method: "POST",
        skipRefresh: true,
      });
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
  }): Promise<{ exams: Exam[]; total: number; limit: number; offset: number }> {
    const query = new URLSearchParams();
    if (params?.status) query.set("status", params.status);
    if (params?.limit) query.set("limit", String(params.limit));
    if (params?.offset) query.set("offset", String(params.offset));
    const qStr = query.toString() ? `?${query.toString()}` : "";
    return apiFetch<{ exams: Exam[]; total: number; limit: number; offset: number }>(`/exams${qStr}`);
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
  async createAssignment(examId: string, studentId: string): Promise<Assignment> {
    return apiFetch<Assignment>("/assignments", {
      method: "POST",
      body: JSON.stringify({ examId, studentId }),
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
// -----------------------------------------------------------------------------
// Sessions & Answers API (Backend Sprint 2 Aligned)
// -----------------------------------------------------------------------------
export const sessionsApi = {
  async startSession(payload: { assignmentId: string }): Promise<Session> {
    return apiFetch<Session>("/sessions", {
      method: "POST",
      body: JSON.stringify(payload),
    });
  },

  async getSession(id: string): Promise<Session> {
    return apiFetch<Session>(`/sessions/${id}`);
  },

  async nextQuestion(sessionId: string): Promise<Session> {
    return apiFetch<Session>(`/sessions/${sessionId}/next`, {
      method: "POST",
    });
  },

  async previousQuestion(sessionId: string): Promise<Session> {
    return apiFetch<Session>(`/sessions/${sessionId}/previous`, {
      method: "POST",
    });
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
    questionIdOrPayload:
      | string
      | { questionId: string; selectedOptionId?: string; textAnswer: string },
    payloadArg?: { selectedOptionId?: string; textAnswer: string }
  ): Promise<Answer> {
    let questionId: string;
    let payload: { selectedOptionId?: string; textAnswer: string };

    if (typeof questionIdOrPayload === "string") {
      questionId = questionIdOrPayload;
      payload = payloadArg || { textAnswer: "" };
    } else {
      questionId = questionIdOrPayload.questionId;
      payload = {
        selectedOptionId: questionIdOrPayload.selectedOptionId,
        textAnswer: questionIdOrPayload.textAnswer,
      };
    }

    return apiFetch<Answer>(`/sessions/${sessionId}/questions/${questionId}/answer`, {
      method: "PUT",
      body: JSON.stringify(payload),
    });
  },

  async getAnswers(sessionId: string): Promise<Answer[]> {
    const res = await apiFetch<Answer[] | { answers: Answer[] }>(`/sessions/${sessionId}/answers`);
    if (Array.isArray(res)) return res;
    return res.answers || [];
  },
};

export const answersApi = {
  submitAnswer: sessionsApi.submitAnswer,
  getAnswers: sessionsApi.getAnswers,
};
