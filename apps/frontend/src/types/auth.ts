/**
 * Authentication and Authorization Domain Types
 * Decoupled from specific auth providers (JWT, Clerk, OAuth, etc.)
 */

export type Role = "STUDENT" | "TEACHER" | "ADMIN" | "PROCTOR";

export interface User {
  id: string;
  email: string;
  firstName: string;
  lastName: string;
  role: Role;
  rollNo?: string;
}

export type UserResponse = User;

export interface StudentSearchResult {
  id: string;
  rollNo: string;
  roll_no?: string;
  name: string;
  email: string;
}

export interface LoginCredentials {
  email: string;
  password: string;
}

export interface RegisterPayload {
  email: string;
  password: string;
  firstName: string;
  lastName: string;
  role: Role;
  rollNo?: string;
}

export interface LoginResponse {
  accessToken: string;
  expiresIn: number;
  user: User;
}

export interface RegisterResponse {
  user: User;
}

export interface RefreshResponse {
  accessToken: string;
  expiresIn: number;
}

export interface LogoutResponse {
  message: string;
}

export interface ApiFieldError {
  field: string;
  error: string;
}

export interface ApiAction {
  type: string;
  message: string;
  value: string;
}

export interface ApiErrorResponse {
  code?: string;
  message: string;
  status?: number;
  override?: boolean;
  errors?: ApiFieldError[];
  action?: ApiAction;
}

export type AuthStatus = "idle" | "restoring" | "authenticated" | "unauthenticated";
