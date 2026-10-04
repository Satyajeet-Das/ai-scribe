import { describe, it, expect, beforeEach, afterEach, vi } from "vitest";
import { useAuthStore } from "../auth-store";
import { authApi, clearStoredToken } from "@/services/api";

describe("useAuthStore", () => {
  beforeEach(() => {
    clearStoredToken();
    localStorage.clear();
    useAuthStore.setState({
      user: null,
      accessToken: null,
      isAuthenticated: false,
      status: "idle",
      isLoading: false,
      isHydrated: true,
      sessionExpired: false,
      error: null,
    });
    vi.restoreAllMocks();
  });

  afterEach(() => {
    vi.restoreAllMocks();
  });

  it("initializes with unauthenticated state", () => {
    const state = useAuthStore.getState();
    expect(state.isAuthenticated).toBe(false);
    expect(state.user).toBeNull();
    expect(state.accessToken).toBeNull();
    expect(state.status).toBe("idle");
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
    expect(state.status).toBe("authenticated");
  });

  it("sets access token via setAccessToken", () => {
    useAuthStore.getState().setAccessToken("jwt_test_token_123");
    expect(useAuthStore.getState().accessToken).toBe("jwt_test_token_123");
  });

  it("evaluates hasRole accurately across all roles", () => {
    useAuthStore.getState().setUser({
      id: "u-1",
      email: "student@test.com",
      firstName: "Alex",
      lastName: "Smith",
      role: "STUDENT",
    });

    expect(useAuthStore.getState().hasRole(["STUDENT"])).toBe(true);
    expect(useAuthStore.getState().hasRole(["STUDENT", "ADMIN"])).toBe(true);
    expect(useAuthStore.getState().hasRole(["TEACHER", "PROCTOR"])).toBe(false);

    useAuthStore.getState().setUser({
      id: "u-2",
      email: "proctor@test.com",
      firstName: "Pat",
      lastName: "Proctor",
      role: "PROCTOR",
    });
    expect(useAuthStore.getState().hasRole(["PROCTOR"])).toBe(true);
    expect(useAuthStore.getState().hasRole(["STUDENT"])).toBe(false);
  });

  it("resets state and clears tokens on logout", async () => {
    useAuthStore.getState().setUser({
      id: "u-1",
      email: "user@test.com",
      firstName: "Test",
      lastName: "User",
      role: "TEACHER",
    });
    useAuthStore.getState().setAccessToken("sample_token");

    vi.spyOn(authApi, "logout").mockResolvedValue({ message: "logged out" });

    await useAuthStore.getState().logout();

    const state = useAuthStore.getState();
    expect(state.isAuthenticated).toBe(false);
    expect(state.user).toBeNull();
    expect(state.accessToken).toBeNull();
    expect(state.status).toBe("unauthenticated");
  });

  it("restores active session successfully via authApi.getMe()", async () => {
    useAuthStore.getState().setAccessToken("valid_stored_token");

    vi.spyOn(authApi, "getMe").mockResolvedValue({
      id: "u-restored",
      email: "restored@test.com",
      firstName: "Elena",
      lastName: "Rostova",
      role: "TEACHER",
    });

    const user = await useAuthStore.getState().restoreSession();

    expect(user?.email).toBe("restored@test.com");
    const state = useAuthStore.getState();
    expect(state.isAuthenticated).toBe(true);
    expect(state.status).toBe("authenticated");
    expect(state.sessionExpired).toBe(false);
  });

  it("recovers expired session via silent refresh during restoreSession", async () => {
    useAuthStore.getState().setAccessToken("expired_token");

    vi.spyOn(authApi, "getMe")
      .mockRejectedValueOnce(new Error("Unauthorized"))
      .mockResolvedValueOnce({
        id: "u-refreshed",
        email: "refreshed@test.com",
        firstName: "Max",
        lastName: "Power",
        role: "STUDENT",
      });

    vi.spyOn(authApi, "refresh").mockResolvedValue({
      accessToken: "new_fresh_token",
      expiresIn: 900,
    });

    const user = await useAuthStore.getState().restoreSession();

    expect(user?.email).toBe("refreshed@test.com");
    const state = useAuthStore.getState();
    expect(state.isAuthenticated).toBe(true);
    expect(state.accessToken).toBe("new_fresh_token");
    expect(state.status).toBe("authenticated");
  });

  it("marks session expired when restoreSession refresh fails", async () => {
    useAuthStore.getState().setAccessToken("dead_token");

    vi.spyOn(authApi, "getMe").mockRejectedValue(new Error("Unauthorized"));
    vi.spyOn(authApi, "refresh").mockRejectedValue(new Error("Refresh failed"));

    const user = await useAuthStore.getState().restoreSession();

    expect(user).toBeNull();
    const state = useAuthStore.getState();
    expect(state.isAuthenticated).toBe(false);
    expect(state.status).toBe("unauthenticated");
    expect(state.sessionExpired).toBe(true);
    expect(state.accessToken).toBeNull();
  });
});
