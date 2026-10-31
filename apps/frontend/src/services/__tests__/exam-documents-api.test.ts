import { describe, it, expect, vi, beforeEach, afterEach } from "vitest";
import { examDocumentsApi } from "../exam-documents-api";
import { ApiError } from "../api";

function jsonResponse(body: unknown, status = 200): Response {
  return new Response(JSON.stringify(body), {
    status,
    headers: { "content-type": "application/json" },
  });
}

describe("examDocumentsApi", () => {
  const fetchMock = vi.fn();

  beforeEach(() => {
    fetchMock.mockReset();
    vi.stubGlobal("fetch", fetchMock);
  });

  afterEach(() => {
    vi.unstubAllGlobals();
  });

  it("uploads the file as multipart form data in the field 'file'", async () => {
    fetchMock.mockResolvedValue(jsonResponse({ id: "d1", status: "QUEUED" }, 202));
    const file = new File(["%PDF-1.4"], "Paper 1.pdf", { type: "application/pdf" });

    const doc = await examDocumentsApi.upload("exam-1", file);

    expect(doc.status).toBe("QUEUED");
    const [url, init] = fetchMock.mock.calls[0];
    expect(String(url)).toMatch(/\/exams\/exam-1\/documents$/);
    expect(init.method).toBe("POST");
    expect(init.body).toBeInstanceOf(FormData);
    expect((init.body as FormData).get("file")).toBeInstanceOf(File);
    expect(((init.body as FormData).get("file") as File).name).toBe("Paper 1.pdf");
    // The browser must add the multipart boundary itself.
    expect(new Headers(init.headers).has("Content-Type")).toBe(false);
    expect(init.cache).toBe("no-store");
  });

  it("allows an upload far longer than the default timeout", async () => {
    vi.useFakeTimers();
    try {
      // A request that finishes after 40 s: past the 15 s default, within the upload limit.
      fetchMock.mockImplementation(
        (_url: string, init: RequestInit) =>
          new Promise((resolve, reject) => {
            init.signal?.addEventListener("abort", () => reject(new DOMException("aborted", "AbortError")));
            setTimeout(() => resolve(jsonResponse({ id: "d1" }, 202)), 40_000);
          })
      );
      const pending = examDocumentsApi.upload("exam-1", new File(["x"], "a.pdf"));
      await vi.advanceTimersByTimeAsync(41_000);
      await expect(pending).resolves.toMatchObject({ id: "d1" });
    } finally {
      vi.useRealTimers();
    }
  });

  it("cancels an upload when its signal is aborted", async () => {
    fetchMock.mockImplementation(
      (_url: string, init: RequestInit) =>
        new Promise((_resolve, reject) => {
          init.signal?.addEventListener("abort", () => reject(new DOMException("aborted", "AbortError")));
        })
    );
    const controller = new AbortController();
    const pending = examDocumentsApi.upload("exam-1", new File(["x"], "a.pdf"), controller.signal);
    controller.abort();
    await expect(pending).rejects.toMatchObject({ status: 0 });
  });

  it("surfaces the server's error code and field errors", async () => {
    fetchMock.mockResolvedValue(
      jsonResponse(
        { code: "INVALID_PDF", message: "This PDF is password protected.", status: 422, override: true, errors: [{ field: "file", error: "ENCRYPTED" }] },
        422
      )
    );
    const err = await examDocumentsApi.upload("exam-1", new File(["x"], "a.pdf")).catch((e) => e);
    expect(err).toBeInstanceOf(ApiError);
    expect(err.status).toBe(422);
    expect(err.code).toBe("INVALID_PDF");
    expect(err.errors?.[0]).toEqual({ field: "file", error: "ENCRYPTED" });
  });

  it("reads documents without letting the browser cache exam content", async () => {
    fetchMock.mockResolvedValue(jsonResponse({ documents: [], total: 0 }));
    await examDocumentsApi.list("exam-1");
    await examDocumentsApi.get("exam-1", "d1");
    await examDocumentsApi.getDraft("exam-1", "d1");
    for (const [, init] of fetchMock.mock.calls) expect(init.cache).toBe("no-store");
    expect(String(fetchMock.mock.calls[1][0])).toMatch(/\/exams\/exam-1\/documents\/d1$/);
    expect(String(fetchMock.mock.calls[2][0])).toMatch(/\/exams\/exam-1\/documents\/d1\/draft$/);
  });

  it("encodes identifiers placed in the path", async () => {
    fetchMock.mockResolvedValue(jsonResponse({}));
    await examDocumentsApi.get("a/b", "c d");
    expect(String(fetchMock.mock.calls[0][0])).toMatch(/\/exams\/a%2Fb\/documents\/c%20d$/);
  });

  it("fetches the original PDF as a blob", async () => {
    fetchMock.mockResolvedValue(new Response("%PDF-1.4", { status: 200, headers: { "content-type": "application/pdf" } }));
    const blob = await examDocumentsApi.getFile("exam-1", "d1");
    expect(blob.size).toBe(8);
    expect(blob.type).toBe("application/pdf");
    expect(String(fetchMock.mock.calls[0][0])).toMatch(/\/documents\/d1\/file$/);
  });

  it("sends the version with every draft change", async () => {
    fetchMock.mockResolvedValue(jsonResponse({ question: {}, draftVersion: 4 }));
    const question = {
      numberLabel: "1", type: "ESSAY" as const, instructions: "", text: "Explain.", points: 5,
      options: [], tables: [], diagrams: [], acknowledgedWarnings: [],
    };

    await examDocumentsApi.replaceQuestion("e", "d", "q1", { expectedDraftVersion: 3, question });
    await examDocumentsApi.addQuestion("e", "d", { expectedDraftVersion: 4, position: 2, question });
    await examDocumentsApi.deleteQuestion("e", "d", "q1", 5);
    await examDocumentsApi.reorder("e", "d", { expectedDraftVersion: 6, questionIds: ["a", "b"] });
    await examDocumentsApi.approve("e", "d", { expectedDraftVersion: 7 });

    const calls = fetchMock.mock.calls.map(([url, init]) => ({ url: String(url), method: init.method, body: init.body }));
    expect(calls[0]).toMatchObject({ method: "PUT" });
    expect(calls[0].url).toMatch(/\/draft\/questions\/q1$/);
    expect(JSON.parse(calls[0].body).expectedDraftVersion).toBe(3);
    expect(calls[1]).toMatchObject({ method: "POST" });
    expect(JSON.parse(calls[1].body)).toMatchObject({ expectedDraftVersion: 4, position: 2 });
    expect(calls[2]).toMatchObject({ method: "DELETE" });
    expect(calls[2].url).toMatch(/\/draft\/questions\/q1\?expectedDraftVersion=5$/);
    expect(calls[3]).toMatchObject({ method: "PUT" });
    expect(calls[3].url).toMatch(/\/draft\/reorder$/);
    expect(JSON.parse(calls[3].body)).toEqual({ expectedDraftVersion: 6, questionIds: ["a", "b"] });
    expect(calls[4]).toMatchObject({ method: "POST" });
    expect(calls[4].url).toMatch(/\/approve$/);
    expect(JSON.parse(calls[4].body)).toEqual({ expectedDraftVersion: 7 });
  });

  it("cancels, retries and deletes through the document's own routes", async () => {
    fetchMock.mockResolvedValue(jsonResponse({ id: "d1" }));
    await examDocumentsApi.cancel("e", "d1");
    await examDocumentsApi.retry("e", "d1", { discardDraft: true, expectedDraftVersion: 2 });
    fetchMock.mockResolvedValue(new Response(null, { status: 204 }));
    await examDocumentsApi.remove("e", "d1");

    expect(String(fetchMock.mock.calls[0][0])).toMatch(/\/documents\/d1\/cancel$/);
    expect(String(fetchMock.mock.calls[1][0])).toMatch(/\/documents\/d1\/retry$/);
    expect(JSON.parse(fetchMock.mock.calls[1][1].body)).toEqual({ discardDraft: true, expectedDraftVersion: 2 });
    expect(fetchMock.mock.calls[2][1].method).toBe("DELETE");
  });
});
