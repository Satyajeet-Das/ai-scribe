import { describe, it, expect, vi, beforeEach, afterEach } from "vitest";
import { responseReviewApi } from "../response-review-api";
import { ApiError } from "../api";

function jsonResponse(body: unknown, status = 200): Response {
  return new Response(JSON.stringify(body), {
    status,
    headers: { "content-type": "application/json" },
  });
}

describe("responseReviewApi", () => {
  const fetchMock = vi.fn();

  beforeEach(() => {
    fetchMock.mockReset();
    vi.stubGlobal("fetch", fetchMock);
  });

  afterEach(() => {
    vi.unstubAllGlobals();
  });

  it("requests the roster for an exam without empty parameters", async () => {
    fetchMock.mockResolvedValue(jsonResponse({ data: [] }));
    await responseReviewApi.listStudents("exam-1");
    const [url, init] = fetchMock.mock.calls[0];
    expect(String(url)).toMatch(/\/exams\/exam-1\/responses$/);
    expect(init.method ?? "GET").toBe("GET");
    expect(init.cache).toBe("no-store");
  });

  it("sends only the filters that are set", async () => {
    fetchMock.mockResolvedValue(jsonResponse({ data: [] }));
    await responseReviewApi.listStudents("exam-1", {
      search: "  ana  ",
      status: "SUBMITTED",
      limit: 20,
      offset: 40,
      sort: "rollNo",
      order: "desc",
    });
    const url = new URL(String(fetchMock.mock.calls[0][0]));
    expect(url.searchParams.get("search")).toBe("ana");
    expect(url.searchParams.get("status")).toBe("SUBMITTED");
    expect(url.searchParams.get("limit")).toBe("20");
    expect(url.searchParams.get("offset")).toBe("40");
    expect(url.searchParams.get("sort")).toBe("rollNo");
    expect(url.searchParams.get("order")).toBe("desc");

    fetchMock.mockClear();
    fetchMock.mockResolvedValue(jsonResponse({ data: [] }));
    await responseReviewApi.listStudents("exam-1", { search: "   ", offset: 0 });
    const bare = new URL(String(fetchMock.mock.calls[0][0]));
    expect([...bare.searchParams.keys()]).toEqual([]);
  });

  it("encodes identifiers in the path", async () => {
    fetchMock.mockResolvedValue(jsonResponse({}));
    await responseReviewApi.getStudentResponses("a/b", "c d");
    expect(String(fetchMock.mock.calls[0][0])).toMatch(/\/exams\/a%2Fb\/responses\/c%20d$/);
  });

  it("only ever issues GET requests", async () => {
    fetchMock.mockResolvedValue(jsonResponse({}));
    await responseReviewApi.listStudents("e");
    await responseReviewApi.getStudentResponses("e", "s");
    for (const [, init] of fetchMock.mock.calls) {
      expect((init.method ?? "GET").toUpperCase()).toBe("GET");
      expect(init.body).toBeUndefined();
    }
  });

  it("cancels the underlying request when the caller aborts", async () => {
    fetchMock.mockImplementation(
      (_url: string, init: RequestInit) =>
        new Promise((_resolve, reject) => {
          init.signal?.addEventListener("abort", () =>
            reject(Object.assign(new Error("aborted"), { name: "AbortError" }))
          );
        })
    );
    const controller = new AbortController();
    const pending = responseReviewApi.getStudentResponses("e", "s", controller.signal);
    controller.abort();
    await expect(pending).rejects.toBeInstanceOf(ApiError);
  });

  it("surfaces server errors as ApiError with the status", async () => {
    fetchMock.mockResolvedValue(
      jsonResponse({ code: "FORBIDDEN", message: "Forbidden", override: true }, 403)
    );
    await expect(responseReviewApi.listStudents("e")).rejects.toMatchObject({
      status: 403,
      message: "Forbidden",
    });
  });
  describe("getRecordingAudio", () => {
    it("fetches the audio as a blob from the recording's own path, never cached", async () => {
      fetchMock.mockResolvedValue(
        new Response(new Uint8Array([82, 73, 70, 70]), {
          status: 200,
          headers: { "content-type": "audio/wav" },
        })
      );
      const audio = await responseReviewApi.getRecordingAudio("exam 1", "s/1", "rec-1");

      // (The test runtime has two Blob classes, so check what matters: its content.)
      expect(audio.type).toBe("audio/wav");
      expect(audio.size).toBe(4);
      const [url, init] = fetchMock.mock.calls[0];
      expect(String(url)).toMatch(/\/exams\/exam%201\/responses\/s%2F1\/recordings\/rec-1\/audio$/);
      expect(init.cache).toBe("no-store");
      expect(init.credentials).toBe("include");
      expect(String(url)).not.toMatch(/token|access/i); // credentials travel in a header, not the URL
    });

    it("reports a removed recording as 410 with the server's code", async () => {
      fetchMock.mockResolvedValue(
        jsonResponse({ code: "RECORDING_REMOVED", message: "removed", override: true }, 410)
      );
      await expect(responseReviewApi.getRecordingAudio("e", "s", "r")).rejects.toMatchObject({
        status: 410,
        code: "RECORDING_REMOVED",
      });
    });

    it("reports a temporary failure as 503", async () => {
      fetchMock.mockResolvedValue(jsonResponse({ code: "AUDIO_UNAVAILABLE", message: "later" }, 503));
      await expect(responseReviewApi.getRecordingAudio("e", "s", "r")).rejects.toMatchObject({
        status: 503,
      });
    });
  });
});
