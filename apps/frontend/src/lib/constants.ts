/**
 * Application & Design System Constants
 * Centralized layout, spacing, typography, and configuration tokens.
 */

import { env } from "./env";

export const APP_CONFIG = {
  name: "AI Exam Scribe",
  shortName: "Scribe",
  description: "Accessible voice-first examination platform for all learners",
  apiUrl: env.API_URL,
  version: "1.0.0",
} as const;

export const API_ROUTES = {
  health: "/status",
  auth: {
    login: "/api/v1/auth/login",
    register: "/api/v1/auth/register",
    refresh: "/api/v1/auth/refresh",
    me: "/api/v1/auth/me",
  },
  exams: "/api/v1/exams",
  questions: (examId: string) => `/api/v1/exams/${examId}/questions`,
  assignments: (examId: string) => `/api/v1/exams/${examId}/assignments`,
  myExams: "/api/v1/assignments/my-exams",
  sessions: "/api/v1/sessions",
  sessionDetail: (sessionId: string) => `/api/v1/sessions/${sessionId}`,
  submitSession: (sessionId: string) => `/api/v1/sessions/${sessionId}/submit`,
  nextQuestion: (sessionId: string) => `/api/v1/sessions/${sessionId}/next`,
  previousQuestion: (sessionId: string) => `/api/v1/sessions/${sessionId}/previous`,
  submitAnswer: (sessionId: string, questionId: string) =>
    `/api/v1/sessions/${sessionId}/questions/${questionId}/answer`,
  answers: (sessionId: string) => `/api/v1/sessions/${sessionId}/answers`,
} as const;

export const STORAGE_KEYS = {
  accessToken: "ai_scribe_access_token",
  refreshToken: "ai_scribe_refresh_token",
  user: "ai_scribe_user",
  theme: "ai_scribe_theme",
  accessibility: "ai_scribe_a11y_settings",
} as const;

/**
 * Design Tokens for consistent spacing, padding, layout & typography
 */
export const DESIGN_TOKENS = {
  layout: {
    container: "mx-auto max-w-7xl px-4 sm:px-6 lg:px-8",
    containerNarrow: "mx-auto max-w-4xl px-4 sm:px-6",
    containerWide: "mx-auto max-w-screen-2xl px-4 sm:px-6 lg:px-8",
    authContainer: "mx-auto w-full max-w-md px-4 py-8 sm:px-6",
    pageSection: "py-6 sm:py-8 lg:py-12",
  },
  spacing: {
    pageHeaderBottom: "mb-6 sm:mb-8",
    cardGap: "gap-4 sm:gap-6",
    formFieldGap: "gap-4",
    stackGap: "space-y-4 sm:space-y-6",
  },
  card: {
    base: "rounded-xl border border-border bg-card text-card-foreground shadow-sm transition-all hover:shadow-md",
    padding: "p-4 sm:p-6",
    headerPadding: "p-4 pb-2 sm:p-6 sm:pb-3",
    contentPadding: "p-4 pt-2 sm:p-6 sm:pt-3",
  },
  typography: {
    h1: "text-2xl sm:text-3xl lg:text-4xl font-extrabold tracking-tight text-foreground",
    h2: "text-xl sm:text-2xl lg:text-3xl font-bold tracking-tight text-foreground",
    h3: "text-lg sm:text-xl font-semibold tracking-tight text-foreground",
    h4: "text-base sm:text-lg font-medium text-foreground",
    lead: "text-base sm:text-lg text-muted-foreground leading-relaxed",
    body: "text-sm sm:text-base text-foreground leading-normal",
    muted: "text-xs sm:text-sm text-muted-foreground",
    caption: "text-xs text-muted-foreground tracking-wide uppercase font-semibold",
    code: "font-mono text-xs sm:text-sm rounded bg-muted px-1.5 py-0.5",
  },
  animation: {
    fadeIn: "animate-in fade-in duration-300",
    slideUp: "animate-in fade-in slide-in-from-bottom-4 duration-300",
    pulse: "animate-pulse",
  },
} as const;

export const NAV_LINKS = [
  { label: "Dashboard", href: "/exams", role: "TEACHER" },
  { label: "Take Exam", href: "/sessions", role: "STUDENT" },
] as const;

export const DEFAULT_PAGE_SIZE = 6;
export const SEARCH_DEBOUNCE_MS = 350;

export const ROLE_PORTALS = {
  TEACHER: "/exams",
  ADMIN: "/exams",
  STUDENT: "/sessions",
  PROCTOR: "/sessions",
} as const;

/**
 * Returns the default portal URL for a given user role.
 * Teachers/Admins go to /exams (Educator Portal).
 * Candidates/Students go to /sessions (Candidate Portal).
 */
export function getRolePortal(role?: string | null): string {
  if (role === "TEACHER" || role === "ADMIN") {
    return ROLE_PORTALS.TEACHER;
  }
  return ROLE_PORTALS.STUDENT;
}

/**
 * Validates whether a requested path is permissible for the user's role.
 */
export function isRoleAllowedRoute(role: string | null | undefined, path: string): boolean {
  if (!role) return false;
  if (role === "ADMIN") {
    return path.startsWith("/exams") || path.startsWith("/sessions") || path.startsWith("/my-exams");
  }
  if (role === "TEACHER") {
    return path.startsWith("/exams");
  }
  return path.startsWith("/sessions") || path.startsWith("/my-exams");
}

/**
 * Safely resolves the post-authentication redirect destination.
 * If returnUrl is provided and matches the user's role permissions, it redirects there.
 * Otherwise, directly directs the user to their designated role portal.
 */
export function resolvePostAuthRedirect(
  role: string | null | undefined,
  returnUrl: string | null
): string {
  const portal = getRolePortal(role);
  if (!returnUrl) return portal;

  // Prevent open redirects & protocol relative URLs
  if (!returnUrl.startsWith("/") || returnUrl.startsWith("//") || returnUrl.startsWith("/\\")) {
    return portal;
  }

  // Prevent routing back to auth pages or root landing
  if (returnUrl === "/" || returnUrl.startsWith("/login") || returnUrl.startsWith("/register")) {
    return portal;
  }

  // Ensure user cannot navigate to a portal unsuited for their role
  if (isRoleAllowedRoute(role, returnUrl)) {
    return returnUrl;
  }

  return portal;
}
