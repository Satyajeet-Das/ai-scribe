import { describe, it, expect, beforeEach, afterEach, vi } from "vitest";
import {
  apiFetch,
  getStoredToken,
  setStoredToken,
  clearStoredToken,
  registerAuthFailureHandler,
  ApiError,
  sessionsApi,
} from "../api";

describe("API Client & Token Management", () => {
  const originalFetch = globalThis.fetch;

  beforeEach(() => {
    clearStoredToken();
    localStorage.clear();
    vi.restoreAllMocks();
  });

  afterEach(() => {
    globalThis.fetch = originalFetch;
  });

  it("stores and retrieves token from memory and localStorage", () => {
    expect(getStoredToken()).toBeNull();

    setStoredToken("test_token_123");
    expect(getStoredToken()).toBe("test_token_123");
    expect(localStorage.getItem("ai_scribe_access_token")).toBe("test_token_123");

    clearStoredToken();
    expect(getStoredToken()).toBeNull();
    expect(localStorage.getItem("ai_scribe_access_token")).toBeNull();
  });

  it("injects Authorization Bearer header when token is present", async () => {
    setStoredToken("valid_bearer_token");

    const mockFetch = vi.fn().mockResolvedValue({
      ok: true,
      status: 200,
      headers: new Headers({ "content-type": "application/json" }),
      json: async () => ({ success: true }),
    });
    globalThis.fetch = mockFetch;

    const res = await apiFetch<{ success: boolean }>("/test-endpoint");
    expect(res.success).toBe(true);

    expect(mockFetch).toHaveBeenCalledTimes(1);
    const options = mockFetch.mock.calls[0][1];
    const headers = options.headers as Headers;
    expect(headers.get("Authorization")).toBe("Bearer valid_bearer_token");
    expect(options.credentials).toBe("include");
  });

  it("normalizes backend HTTPError structure into ApiError instance", async () => {
    const errorBody = {
      code: "UNAUTHORIZED",
      message: "Invalid email or password",
      status: 401,
      override: true,
      errors: [{ field: "email", error: "email is invalid" }],
    };

    globalThis.fetch = vi.fn().mockResolvedValue({
      ok: false,
      status: 401,
      headers: new Headers({ "content-type": "application/json" }),
      json: async () => errorBody,
    });

    try {
      await apiFetch("/auth/login", { method: "POST" });
      expect.fail("Should have thrown ApiError");
    } catch (err) {
      expect(err).toBeInstanceOf(ApiError);
      const apiErr = err as ApiError;
      expect(apiErr.status).toBe(401);
      expect(apiErr.code).toBe("UNAUTHORIZED");
      expect(apiErr.message).toBe("Invalid email or password");
      expect(apiErr.override).toBe(true);
      expect(apiErr.errors).toEqual([{ field: "email", error: "email is invalid" }]);
      expect(apiErr.isUnauthorized()).toBe(true);
    }
  });

  it("handles 401 with successful silent token refresh and retries original request", async () => {
    setStoredToken("expired_token");

    let callCount = 0;
    const mockFetch = vi.fn().mockImplementation((url: string) => {
      callCount++;
      if (url.includes("/auth/refresh")) {
        return Promise.resolve({
          ok: true,
          status: 200,
          headers: new Headers({ "content-type": "application/json" }),
          json: async () => ({
            accessToken: "new_refreshed_token_456",
            expiresIn: 900,
          }),
        });
      }

      if (url.includes("/data")) {
        if (callCount === 1) {
          // First attempt with expired token fails with 401
          return Promise.resolve({
            ok: false,
            status: 401,
            headers: new Headers({ "content-type": "application/json" }),
            json: async () => ({ code: "TOKEN_EXPIRED", message: "token is expired" }),
          });
        }
        // Second attempt with new refreshed token succeeds
        return Promise.resolve({
          ok: true,
          status: 200,
          headers: new Headers({ "content-type": "application/json" }),
          json: async () => ({ payload: "retried_successfully" }),
        });
      }

      return Promise.reject(new Error("Unexpected endpoint"));
    });

    globalThis.fetch = mockFetch;

    const result = await apiFetch<{ payload: string }>("/data");

    expect(result.payload).toBe("retried_successfully");
    expect(getStoredToken()).toBe("new_refreshed_token_456");
    expect(mockFetch).toHaveBeenCalledTimes(3); // 1. initial /data (401), 2. /auth/refresh (200), 3. retried /data (200)
  });

  it("prevents multiple simultaneous refresh requests when concurrent requests 401", async () => {
    setStoredToken("stale_token");

    let refreshCallCount = 0;

    const mockFetch = vi.fn().mockImplementation(async (url: string, init?: RequestInit) => {
      if (url.includes("/auth/refresh")) {
        refreshCallCount++;
        // Small delay to simulate network latency
        await new Promise((r) => setTimeout(r, 50));
        return {
          ok: true,
          status: 200,
          headers: new Headers({ "content-type": "application/json" }),
          json: async () => ({
            accessToken: "rotated_single_refresh_token",
            expiresIn: 900,
          }),
        };
      }

      const headers = init?.headers as Headers;
      const authHeader = headers?.get("Authorization");

      if (authHeader === "Bearer rotated_single_refresh_token") {
        return {
          ok: true,
          status: 200,
          headers: new Headers({ "content-type": "application/json" }),
          json: async () => ({ url, success: true }),
        };
      }

      // Initial requests with stale token fail with 401
      return {
        ok: false,
        status: 401,
        headers: new Headers({ "content-type": "application/json" }),
        json: async () => ({ code: "UNAUTHORIZED", message: "Token expired" }),
      };
    });

    globalThis.fetch = mockFetch;

    // Fire 5 concurrent requests simultaneously
    const requests = [
      apiFetch("/resource-1"),
      apiFetch("/resource-2"),
      apiFetch("/resource-3"),
      apiFetch("/resource-4"),
      apiFetch("/resource-5"),
    ];

    const results = await Promise.all(requests);

    // CRITICAL: Exactly ONE refresh call should have been made due to the mutex!
    expect(refreshCallCount).toBe(1);

    // All 5 requests should have retried and succeeded
    expect(results).toHaveLength(5);
    expect(getStoredToken()).toBe("rotated_single_refresh_token");
  });

  it("handles refresh failure by clearing tokens and invoking auth failure handler", async () => {
    setStoredToken("invalid_session_token");

    const failureSpy = vi.fn();
    const unregister = registerAuthFailureHandler(failureSpy);

    const mockFetch = vi.fn().mockImplementation((url: string) => {
      if (url.includes("/auth/refresh")) {
        return Promise.resolve({
          ok: false,
          status: 401,
          headers: new Headers({ "content-type": "application/json" }),
          json: async () => ({ code: "REFRESH_TOKEN_EXPIRED", message: "Session expired" }),
        });
      }

      return Promise.resolve({
        ok: false,
        status: 401,
        headers: new Headers({ "content-type": "application/json" }),
        json: async () => ({ code: "UNAUTHORIZED", message: "Unauthorized" }),
      });
    });

    globalThis.fetch = mockFetch;

    await expect(apiFetch("/protected-resource")).rejects.toThrow(ApiError);

    // Tokens should be cleared
    expect(getStoredToken()).toBeNull();
    // Auth failure callback should be fired
    expect(failureSpy).toHaveBeenCalledTimes(1);

    unregister();
  });

  it("supports timeout / cancellation without leaving dangling promises", async () => {
    const mockFetch = vi.fn().mockImplementation((_url: string, init?: RequestInit) => {
      return new Promise((_resolve, reject) => {
        const signal = init?.signal;
        if (signal) {
          signal.addEventListener("abort", () => {
            const abortErr = new Error("Request was cancelled or timed out");
            abortErr.name = "AbortError";
            reject(abortErr);
          });
        }
      });
    });

    globalThis.fetch = mockFetch;

    await expect(apiFetch("/slow-resource", { timeoutMs: 50 })).rejects.toThrow(
      "Request was cancelled or timed out"
    );
  });

  describe("Sessions API (Sprint 2 Runtime)", () => {
    it("calls POST /sessions when starting session", async () => {
      const mockFetch = vi.fn().mockResolvedValue({
        ok: true,
        status: 201,
        headers: new Headers({ "content-type": "application/json" }),
        json: async () => ({ id: "sess-1", status: "IN_PROGRESS" }),
      });
      globalThis.fetch = mockFetch;

      const res = await sessionsApi.startSession({ assignmentId: "assign-123" });
      expect(res.id).toBe("sess-1");

      const [url, init] = mockFetch.mock.calls[0];
      expect(url).toContain("/sessions");
      expect(init.method).toBe("POST");
      expect(JSON.parse(init.body as string)).toEqual({ assignmentId: "assign-123" });
    });

    it("calls POST /sessions/:id/next and /previous for question navigation", async () => {
      const mockFetch = vi.fn().mockResolvedValue({
        ok: true,
        status: 200,
        headers: new Headers({ "content-type": "application/json" }),
        json: async () => ({ id: "sess-1" }),
      });
      globalThis.fetch = mockFetch;

      await sessionsApi.nextQuestion("sess-1");
      expect(mockFetch.mock.calls[0][0]).toContain("/sessions/sess-1/next");
      expect(mockFetch.mock.calls[0][1].method).toBe("POST");

      await sessionsApi.previousQuestion("sess-1");
      expect(mockFetch.mock.calls[1][0]).toContain("/sessions/sess-1/previous");
      expect(mockFetch.mock.calls[1][1].method).toBe("POST");
    });

    it("calls PUT /sessions/:session_id/questions/:question_id/answer for submitting answers", async () => {
      const mockFetch = vi.fn().mockResolvedValue({
        ok: true,
        status: 200,
        headers: new Headers({ "content-type": "application/json" }),
        json: async () => ({ id: "ans-1", textAnswer: "Photosynthesis" }),
      });
      globalThis.fetch = mockFetch;

      const res = await sessionsApi.submitAnswer("sess-1", "q-42", {
        selectedOptionId: "opt-b",
        textAnswer: "Photosynthesis",
      });
      expect(res.id).toBe("ans-1");

      const [url, init] = mockFetch.mock.calls[0];
      expect(url).toContain("/sessions/sess-1/questions/q-42/answer");
      expect(init.method).toBe("PUT");
      expect(JSON.parse(init.body as string)).toEqual({
        selectedOptionId: "opt-b",
        textAnswer: "Photosynthesis",
      });
    });

    it("calls GET /sessions/:id/answers and handles direct array or wrapped envelope", async () => {
      const mockFetch = vi.fn().mockResolvedValue({
        ok: true,
        status: 200,
        headers: new Headers({ "content-type": "application/json" }),
        json: async () => [
          { id: "ans-1", questionId: "q-1", textAnswer: "A" },
          { id: "ans-2", questionId: "q-2", textAnswer: "B" },
        ],
      });
      globalThis.fetch = mockFetch;

      const answers = await sessionsApi.getAnswers("sess-1");
      expect(answers).toHaveLength(2);
      expect(mockFetch.mock.calls[0][0]).toContain("/sessions/sess-1/answers");
    });
  });
});
