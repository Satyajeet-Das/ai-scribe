import { describe, it, expect, vi, beforeEach, afterEach } from "vitest";
import { renderHook, act } from "@testing-library/react";
import { ApiError } from "@/services/api";
import type { ExamDocument } from "@/types/exam-document";

vi.mock("@/services/exam-documents-api", () => ({
  examDocumentsApi: { list: vi.fn() },
}));

import { examDocumentsApi } from "@/services/exam-documents-api";
import { useExamDocuments, POLL_FAST_MS, POLL_SLOW_MS } from "../use-exam-documents";

const list = vi.mocked(examDocumentsApi.list);

function doc(over: Partial<ExamDocument> = {}): ExamDocument {
  return {
    id: "d1", examId: "e1", filename: "paper.pdf", sizeBytes: 1000, pageCount: 3, status: "QUEUED",
    progress: { done: 0, total: 3 }, attempt: 1, draftVersion: 0, uploadedBy: "u1",
    createdAt: "2026-01-01T00:00:00Z", updatedAt: "2026-01-01T00:00:00Z", ...over,
  };
}
const ok = (...documents: ExamDocument[]) => ({ documents, total: documents.length });

async function flush() {
  await act(async () => {
    await vi.advanceTimersByTimeAsync(0);
  });
}
async function advance(ms: number) {
  await act(async () => {
    await vi.advanceTimersByTimeAsync(ms);
  });
}

describe("useExamDocuments", () => {
  beforeEach(() => {
    vi.useFakeTimers();
    list.mockReset();
    Object.defineProperty(document, "hidden", { value: false, configurable: true });
  });
  afterEach(() => {
    vi.useRealTimers();
  });

  it("loads the documents once and does not keep asking when nothing is being worked on", async () => {
    list.mockResolvedValue(ok(doc({ status: "REVIEW_REQUIRED" })));
    const { result } = renderHook(() => useExamDocuments("e1"));
    expect(result.current.loading).toBe(true);
    await flush();
    expect(result.current.loading).toBe(false);
    expect(result.current.documents).toHaveLength(1);

    await advance(60_000);
    expect(list).toHaveBeenCalledTimes(1);
  });

  it("keeps checking while a document is queued or processing, and stops when it finishes", async () => {
    list
      .mockResolvedValueOnce(ok(doc({ status: "QUEUED" })))
      .mockResolvedValueOnce(ok(doc({ status: "PROCESSING", stage: "EXTRACTING", progress: { done: 1, total: 3 } })))
      .mockResolvedValue(ok(doc({ status: "REVIEW_REQUIRED", draftVersion: 1 })));
    const { result } = renderHook(() => useExamDocuments("e1"));
    await flush();
    expect(result.current.documents[0].status).toBe("QUEUED");

    await advance(POLL_FAST_MS);
    expect(result.current.documents[0].status).toBe("PROCESSING");

    await advance(POLL_FAST_MS);
    expect(result.current.documents[0].status).toBe("REVIEW_REQUIRED");
    expect(list).toHaveBeenCalledTimes(3);

    await advance(120_000);
    expect(list).toHaveBeenCalledTimes(3);
  });

  it("slows down while nothing changes and speeds up again when something does", async () => {
    const queued = ok(doc({ status: "QUEUED" }));
    list.mockResolvedValue(queued);
    renderHook(() => useExamDocuments("e1"));
    await flush();
    expect(list).toHaveBeenCalledTimes(1);

    // First look found a change (nothing before it): next is fast.
    await advance(POLL_FAST_MS);
    expect(list).toHaveBeenCalledTimes(2);
    // Same answer: the wait grows (3 s, 4.5 s ...) and never passes the slow limit.
    await advance(POLL_FAST_MS);
    expect(list).toHaveBeenCalledTimes(2);
    await advance(1_000);
    expect(list).toHaveBeenCalledTimes(3);

    const waits: number[] = [];
    let last = Date.now();
    list.mockImplementation(async () => {
      waits.push(Date.now() - last);
      last = Date.now();
      return queued;
    });
    await advance(120_000);
    expect(Math.max(...waits)).toBeLessThanOrEqual(POLL_SLOW_MS);
    expect(Math.max(...waits)).toBeGreaterThan(POLL_FAST_MS);
  });

  it("never has two requests in flight", async () => {
    let inFlight = 0;
    let maxInFlight = 0;
    list.mockImplementation(async () => {
      inFlight++;
      maxInFlight = Math.max(maxInFlight, inFlight);
      await new Promise((r) => setTimeout(r, 5_000)); // slower than the poll interval
      inFlight--;
      return ok(doc({ status: "PROCESSING" }));
    });
    renderHook(() => useExamDocuments("e1"));
    await advance(60_000);
    expect(maxInFlight).toBe(1);
  });

  it("does not ask while the tab is hidden, and asks at once when it is shown again", async () => {
    list.mockResolvedValue(ok(doc({ status: "PROCESSING" })));
    renderHook(() => useExamDocuments("e1"));
    await flush();
    expect(list).toHaveBeenCalledTimes(1);

    Object.defineProperty(document, "hidden", { value: true, configurable: true });
    await advance(60_000);
    expect(list).toHaveBeenCalledTimes(1);

    Object.defineProperty(document, "hidden", { value: false, configurable: true });
    await act(async () => {
      document.dispatchEvent(new Event("visibilitychange"));
      await vi.advanceTimersByTimeAsync(0);
    });
    expect(list).toHaveBeenCalledTimes(2);
  });

  it("stops everything when it unmounts, and drops a late answer", async () => {
    let resolve!: (v: ReturnType<typeof ok>) => void;
    list.mockImplementation(() => new Promise((r) => (resolve = r)));
    const { result, unmount } = renderHook(() => useExamDocuments("e1"));
    await flush();
    const signal = list.mock.calls[0][1] as AbortSignal;

    unmount();
    expect(signal.aborted).toBe(true);
    await act(async () => {
      resolve(ok(doc({ status: "PROCESSING" })));
      await vi.advanceTimersByTimeAsync(60_000);
    });
    expect(list).toHaveBeenCalledTimes(1);
    expect(result.current.documents).toHaveLength(0);
  });

  it("clears its timer on unmount while waiting between checks", async () => {
    list.mockResolvedValue(ok(doc({ status: "PROCESSING" })));
    const { unmount } = renderHook(() => useExamDocuments("e1"));
    await flush();
    expect(vi.getTimerCount()).toBeGreaterThan(0);
    unmount();
    expect(vi.getTimerCount()).toBe(0);
  });

  it("keeps the documents it had, says the data may be old, and retries after a network failure", async () => {
    list
      .mockResolvedValueOnce(ok(doc({ status: "PROCESSING" })))
      .mockRejectedValueOnce(new ApiError("down", 0))
      .mockResolvedValue(ok(doc({ status: "REVIEW_REQUIRED" })));
    const { result } = renderHook(() => useExamDocuments("e1"));
    await flush();
    await advance(POLL_FAST_MS * 2);
    expect(result.current.error?.kind).toBe("network");
    expect(result.current.documents).toHaveLength(1);

    await advance(10_000);
    expect(result.current.error).toBeUndefined();
    expect(result.current.documents[0].status).toBe("REVIEW_REQUIRED");
  });

  it("does not retry a refusal such as a missing exam", async () => {
    list.mockRejectedValue(new ApiError("nope", 404));
    const { result } = renderHook(() => useExamDocuments("e1"));
    await flush();
    expect(result.current.error?.kind).toBe("notFound");
    await advance(120_000);
    expect(list).toHaveBeenCalledTimes(1);
  });

  it("reload checks again and restarts a loop that had gone idle", async () => {
    list
      .mockResolvedValueOnce(ok())
      .mockResolvedValueOnce(ok(doc({ status: "QUEUED" })))
      .mockResolvedValue(ok(doc({ status: "REVIEW_REQUIRED" })));
    const { result } = renderHook(() => useExamDocuments("e1"));
    await flush();
    await advance(30_000);
    expect(list).toHaveBeenCalledTimes(1); // idle

    act(() => result.current.reload());
    await flush();
    expect(result.current.documents[0].status).toBe("QUEUED");
    await advance(POLL_FAST_MS);
    expect(result.current.documents[0].status).toBe("REVIEW_REQUIRED");
  });

  it("never shows one exam's documents under another", async () => {
    list.mockImplementation(async (id: string) => ok(doc({ id: `doc-of-${id}`, examId: id, status: "REVIEW_REQUIRED" })));
    const { result, rerender } = renderHook(({ id }) => useExamDocuments(id), { initialProps: { id: "e1" } });
    await flush();
    expect(result.current.documents[0].id).toBe("doc-of-e1");

    rerender({ id: "e2" });
    expect(result.current.documents).toHaveLength(0);
    expect(result.current.loading).toBe(true);
    await flush();
    expect(result.current.documents[0].id).toBe("doc-of-e2");
  });
});
