import { describe, it, expect, beforeEach } from "vitest";
import { useAuthStore } from "../auth-store";

describe("useAuthStore", () => {
  beforeEach(() => {
    // Reset store state between tests
    useAuthStore.setState({
      user: null,
      accessToken: null,
      isAuthenticated: false,
      isLoading: false,
      isHydrated: true,
      error: null,
    });
    localStorage.clear();
  });

  it("initializes with unauthenticated state", () => {
    const state = useAuthStore.getState();
    expect(state.isAuthenticated).toBe(false);
    expect(state.user).toBeNull();
    expect(state.accessToken).toBeNull();
  });

  it("updates user and sets isAuthenticated via setUser", () => {
    useAuthStore.getState().setUser({
      id: "u-123",
      email: "teacher@test.com",
      firstName: "Jane",
      lastName: "Doe",
      role: "TEACHER",
    });

    const state = useAuthStore.getState();
    expect(state.isAuthenticated).toBe(true);
    expect(state.user?.email).toBe("teacher@test.com");
    expect(state.user?.role).toBe("TEACHER");
  });

  it("sets access token via setAccessToken", () => {
    useAuthStore.getState().setAccessToken("jwt_test_token_123");
    expect(useAuthStore.getState().accessToken).toBe("jwt_test_token_123");
  });

  it("evaluates hasRole accurately", () => {
    useAuthStore.getState().setUser({
      id: "u-1",
      email: "student@test.com",
      firstName: "Alex",
      lastName: "Smith",
      role: "STUDENT",
    });

    expect(useAuthStore.getState().hasRole(["STUDENT"])).toBe(true);
    expect(useAuthStore.getState().hasRole(["STUDENT", "ADMIN"])).toBe(true);
    expect(useAuthStore.getState().hasRole(["TEACHER"])).toBe(false);
  });

  it("resets state and clears tokens on logout", () => {
    useAuthStore.getState().setUser({
      id: "u-1",
      email: "user@test.com",
      firstName: "Test",
      lastName: "User",
      role: "TEACHER",
    });
    useAuthStore.getState().setAccessToken("sample_token");

    useAuthStore.getState().logout();

    const state = useAuthStore.getState();
    expect(state.isAuthenticated).toBe(false);
    expect(state.user).toBeNull();
    expect(state.accessToken).toBeNull();
  });
});
