import { describe, it, expect, vi, beforeEach } from "vitest";
import { renderHook, act, waitFor } from "@testing-library/react";
import { ApiError } from "@/services/api";
import type { DraftQuestion, DraftQuestionRequest, ExamDocumentDraft } from "@/types/exam-document";

vi.mock("@/services/exam-documents-api", () => ({
  examDocumentsApi: {
    getDraft: vi.fn(), replaceQuestion: vi.fn(), addQuestion: vi.fn(),
    deleteQuestion: vi.fn(), reorder: vi.fn(), approve: vi.fn(),
  },
}));

import { examDocumentsApi } from "@/services/exam-documents-api";
import { useExamDraft } from "../use-exam-draft";

const api = vi.mocked(examDocumentsApi);

function question(id: string, position: number, text = `Question ${id}`): DraftQuestion {
  return {
    id, position, numberLabel: String(position), type: "ESSAY", instructions: "", text, points: 5, options: [],
    tables: [], diagrams: [], sourcePages: [1], confidence: "HIGH", warnings: [], origin: "EXTRACTED", edited: false,
    createdAt: "2026-01-01T00:00:00Z", updatedAt: "2026-01-01T00:00:00Z",
  };
}

function draft(version: number, ...ids: string[]): ExamDocumentDraft {
  return {
    document: { id: "d1", examId: "e1", draftVersion: version, status: "REVIEW_REQUIRED", pageCount: 3 } as ExamDocumentDraft["document"],
    draftVersion: version, documentInstructions: "", unassigned: [],
    questions: ids.map((id, i) => question(id, i + 1)),
  };
}

const request = (text: string): DraftQuestionRequest => ({
  numberLabel: "1", type: "ESSAY", instructions: "", text, points: 5, options: [], tables: [], diagrams: [], acknowledgedWarnings: [],
});

function deferred<T>() {
  let resolve!: (v: T) => void;
  let reject!: (e: unknown) => void;
  const promise = new Promise<T>((res, rej) => ((resolve = res), (reject = rej)));
  return { promise, resolve, reject };
}

async function ready(...ids: string[]) {
  api.getDraft.mockResolvedValueOnce(draft(1, ...(ids.length ? ids : ["a", "b", "c"])));
  const hook = renderHook(() => useExamDraft("e1", "d1"));
  await waitFor(() => expect(hook.result.current.loading).toBe(false));
  return hook;
}

describe("useExamDraft", () => {
  beforeEach(() => Object.values(api).forEach((f) => f.mockReset()));

  it("loads the draft", async () => {
    const { result } = await ready();
    expect(result.current.draft?.questions.map((q) => q.id)).toEqual(["a", "b", "c"]);
    expect(result.current.stale).toBe(false);
    expect(api.getDraft).toHaveBeenCalledWith("e1", "d1", expect.any(AbortSignal));
  });

  it("reports a load failure, and recovers on reload", async () => {
    api.getDraft.mockRejectedValueOnce(new ApiError("boom", 500)).mockResolvedValueOnce(draft(1, "a"));
    const { result } = renderHook(() => useExamDraft("e1", "d1"));
    await waitFor(() => expect(result.current.loadError?.kind).toBe("server"));
    expect(result.current.draft).toBeUndefined();

    await act(async () => {
      await result.current.reload();
    });
    expect(result.current.loadError).toBeUndefined();
    expect(result.current.draft?.questions).toHaveLength(1);
  });

  describe("saving a question", () => {
    it("sends the current version and shows what the server saved", async () => {
      const { result } = await ready();
      api.replaceQuestion.mockResolvedValue({ question: question("b", 2, "Corrected"), draftVersion: 2 });

      let out: Awaited<ReturnType<typeof result.current.replace>>;
      await act(async () => {
        out = await result.current.replace("b", request("Corrected"));
      });

      expect(out!.ok).toBe(true);
      expect(api.replaceQuestion).toHaveBeenCalledWith("e1", "d1", "b", { expectedDraftVersion: 1, question: request("Corrected") });
      expect(result.current.draft?.draftVersion).toBe(2);
      expect(result.current.draft?.questions[1].text).toBe("Corrected");
      expect(result.current.draft?.questions[0].text).toBe("Question a");
    });

    it("sends two quick saves one after the other, the second with the version the first produced", async () => {
      const { result } = await ready();
      const first = deferred<{ question: DraftQuestion; draftVersion: number }>();
      api.replaceQuestion
        .mockReturnValueOnce(first.promise)
        .mockResolvedValueOnce({ question: question("b", 2, "Second"), draftVersion: 3 });

      let p1: Promise<unknown>, p2: Promise<unknown>;
      act(() => {
        p1 = result.current.replace("a", request("First"));
        p2 = result.current.replace("b", request("Second"));
      });
      await waitFor(() => expect(api.replaceQuestion).toHaveBeenCalledTimes(1));
      // Give the queue every chance to start the second one early; it must not.
      await act(async () => {
        await new Promise((r) => setTimeout(r, 20));
      });
      expect(api.replaceQuestion).toHaveBeenCalledTimes(1); // the second waits its turn
      expect(result.current.pending).toBe(2);

      await act(async () => {
        first.resolve({ question: question("a", 1, "First"), draftVersion: 2 });
        await Promise.all([p1, p2]);
      });
      expect(api.replaceQuestion.mock.calls.map((c) => c[3].expectedDraftVersion)).toEqual([1, 2]);
      expect(result.current.draft?.draftVersion).toBe(3);
      expect(result.current.draft?.questions.map((q) => q.text)).toEqual(["First", "Second", "Question c"]);
      expect(result.current.pending).toBe(0);
    });

    it("lets a reload that was asked for later win over a slower earlier save", async () => {
      const { result } = await ready();
      const slow = deferred<{ question: DraftQuestion; draftVersion: number }>();
      api.replaceQuestion.mockReturnValueOnce(slow.promise);
      api.getDraft.mockResolvedValueOnce({ ...draft(5, "a", "b", "c"), questions: [question("a", 1, "Latest from server"), question("b", 2), question("c", 3)] });

      let save!: Promise<unknown>, reload!: Promise<unknown>;
      act(() => {
        save = result.current.replace("a", request("Mine"));
        reload = result.current.reload();
      });
      await act(async () => {
        slow.resolve({ question: question("a", 1, "Mine"), draftVersion: 2 });
        await Promise.all([save, reload]);
      });
      // The save finished first, then the reload: the newest word is the reload's.
      expect(result.current.draft?.questions[0].text).toBe("Latest from server");
      expect(result.current.draft?.draftVersion).toBe(5);
    });

    it("leaves the draft exactly as it was when a save fails, and says why", async () => {
      const { result } = await ready();
      api.replaceQuestion.mockRejectedValueOnce(new ApiError("down", 0));
      let out: Awaited<ReturnType<typeof result.current.replace>>;
      await act(async () => {
        out = await result.current.replace("a", request("Lost?"));
      });
      const failed = out!;
      expect(failed.ok).toBe(false);
      expect(!failed.ok && failed.error.kind).toBe("network");
      expect(!failed.ok && failed.error.retryable).toBe(true);
      expect(result.current.draft?.draftVersion).toBe(1);
      expect(result.current.draft?.questions[0].text).toBe("Question a");
      expect(result.current.stale).toBe(false);
    });

    it("returns the server's field errors so they can be shown beside the fields", async () => {
      const { result } = await ready();
      api.replaceQuestion.mockRejectedValueOnce(
        new ApiError("bad", 400, "VALIDATION_ERROR", true, [{ field: "points", error: "must be between 0 and 1000" }])
      );
      let out: Awaited<ReturnType<typeof result.current.replace>>;
      await act(async () => {
        out = await result.current.replace("a", request("x"));
      });
      expect(!out!.ok && out!.error.fields).toEqual({ points: "must be between 0 and 1000" });
    });
  });

  describe("when the draft was changed somewhere else", () => {
    async function goStale() {
      const hook = await ready();
      api.replaceQuestion.mockRejectedValueOnce(new ApiError("stale", 409, "STALE_VERSION", true));
      await act(async () => {
        await hook.result.current.replace("a", request("x"));
      });
      return hook;
    }

    it("marks the draft stale", async () => {
      const { result } = await goStale();
      expect(result.current.stale).toBe(true);
    });

    it("refuses further changes without asking the server, until reloaded", async () => {
      const { result } = await goStale();
      api.replaceQuestion.mockClear();
      let out: Awaited<ReturnType<typeof result.current.replace>>;
      await act(async () => {
        out = await result.current.replace("a", request("again"));
        await result.current.remove("b");
        await result.current.move("b", -1);
        await result.current.add(request("new"));
        await result.current.approve();
      });
      expect(out!.ok).toBe(false);
      expect(api.replaceQuestion).not.toHaveBeenCalled();
      expect(api.deleteQuestion).not.toHaveBeenCalled();
      expect(api.reorder).not.toHaveBeenCalled();
      expect(api.addQuestion).not.toHaveBeenCalled();
      expect(api.approve).not.toHaveBeenCalled();
    });

    it("works again after a reload, with the new version", async () => {
      const { result } = await goStale();
      api.getDraft.mockResolvedValueOnce(draft(9, "a", "b", "c"));
      await act(async () => {
        await result.current.reload();
      });
      expect(result.current.stale).toBe(false);
      api.replaceQuestion.mockResolvedValue({ question: question("a", 1), draftVersion: 10 });
      await act(async () => {
        await result.current.replace("a", request("fine"));
      });
      expect(api.replaceQuestion.mock.calls.at(-1)![3].expectedDraftVersion).toBe(9);
    });
  });

  describe("adding", () => {
    it("takes the whole draft again, because positions after the new question change", async () => {
      const { result } = await ready();
      api.addQuestion.mockResolvedValue({ question: question("n", 2, "Added"), draftVersion: 2 });
      api.getDraft.mockResolvedValueOnce({
        ...draft(2, "a", "n", "b", "c"),
        questions: [question("a", 1), question("n", 2, "Added"), question("b", 3), question("c", 4)],
      });

      let out: Awaited<ReturnType<typeof result.current.add>>;
      await act(async () => {
        out = await result.current.add(request("Added"), 2);
      });
      expect(api.addQuestion).toHaveBeenCalledWith("e1", "d1", { expectedDraftVersion: 1, position: 2, question: request("Added") });
      expect(out!.ok && out!.value.position).toBe(2);
      expect(result.current.draft?.questions.map((q) => q.id)).toEqual(["a", "n", "b", "c"]);
    });

    it("marks the draft stale if the question was added but the refresh failed", async () => {
      const { result } = await ready();
      api.addQuestion.mockResolvedValue({ question: question("n", 4, "Added"), draftVersion: 2 });
      api.getDraft.mockRejectedValueOnce(new ApiError("down", 0));
      let out: Awaited<ReturnType<typeof result.current.add>>;
      await act(async () => {
        out = await result.current.add(request("Added"));
      });
      expect(out!.ok).toBe(true); // the server has it; the teacher is not told it failed
      expect(result.current.stale).toBe(true);
    });
  });

  it("removes a question and shows the draft the server returns", async () => {
    const { result } = await ready();
    api.deleteQuestion.mockResolvedValue(draft(2, "a", "c"));
    await act(async () => {
      await result.current.remove("b");
    });
    expect(api.deleteQuestion).toHaveBeenCalledWith("e1", "d1", "b", 1);
    expect(result.current.draft?.questions.map((q) => q.id)).toEqual(["a", "c"]);
  });

  describe("reordering", () => {
    it("moves a question down by sending every id in the new order", async () => {
      const { result } = await ready();
      api.reorder.mockResolvedValue(draft(2, "b", "a", "c"));
      let out: Awaited<ReturnType<typeof result.current.move>>;
      await act(async () => {
        out = await result.current.move("a", 1);
      });
      expect(api.reorder).toHaveBeenCalledWith("e1", "d1", { expectedDraftVersion: 1, questionIds: ["b", "a", "c"] });
      expect(out!.ok && out!.value).toEqual({ from: 1, to: 2, total: 3 });
      expect(result.current.draft?.questions.map((q) => q.id)).toEqual(["b", "a", "c"]);
    });

    it("does nothing, and asks nothing, at either end", async () => {
      const { result } = await ready();
      await act(async () => {
        const up = await result.current.move("a", -1);
        const down = await result.current.move("c", 1);
        expect(up.ok && up.value.from === up.value.to).toBe(true);
        expect(down.ok && down.value.from === down.value.to).toBe(true);
      });
      expect(api.reorder).not.toHaveBeenCalled();
    });

    it("moves two steps correctly when asked twice in a row", async () => {
      const { result } = await ready();
      api.reorder
        .mockResolvedValueOnce(draft(2, "b", "a", "c"))
        .mockResolvedValueOnce(draft(3, "b", "c", "a"));
      await act(async () => {
        await Promise.all([result.current.move("a", 1), result.current.move("a", 1)]);
      });
      expect(api.reorder.mock.calls.map((c) => c[2])).toEqual([
        { expectedDraftVersion: 1, questionIds: ["b", "a", "c"] },
        { expectedDraftVersion: 2, questionIds: ["b", "c", "a"] },
      ]);
    });
  });

  describe("approving", () => {
    it("sends the version and shows the document as completed", async () => {
      const { result } = await ready();
      const document = { ...draft(2).document, status: "COMPLETED" as const, draftVersion: 2 };
      api.approve.mockResolvedValue({ document, questions: [] });
      await act(async () => {
        const out = await result.current.approve();
        expect(out.ok).toBe(true);
      });
      expect(api.approve).toHaveBeenCalledWith("e1", "d1", { expectedDraftVersion: 1 });
      expect(result.current.draft?.document.status).toBe("COMPLETED");
    });

    it("reports what is missing when the server refuses, and changes nothing", async () => {
      const { result } = await ready();
      api.approve.mockRejectedValue(
        new ApiError("incomplete", 409, "DRAFT_INCOMPLETE", true, [{ field: "questions[2].points", error: "Set the marks for this question." }])
      );
      await act(async () => {
        const out = await result.current.approve();
        expect(!out.ok && out.error.code).toBe("DRAFT_INCOMPLETE");
        expect(!out.ok && out.error.fields["questions[2].points"]).toMatch(/marks/);
      });
      expect(result.current.draft?.document.status).toBe("REVIEW_REQUIRED");
    });
  });

  describe("leaving the page", () => {
    it("drops a response that arrives after unmount, and cancels the load", async () => {
      const slow = deferred<ExamDocumentDraft>();
      api.getDraft.mockReturnValueOnce(slow.promise);
      const { result, unmount } = renderHook(() => useExamDraft("e1", "d1"));
      await waitFor(() => expect(api.getDraft).toHaveBeenCalledTimes(1));
      const signal = api.getDraft.mock.calls[0][2] as AbortSignal;
      unmount();
      expect(signal.aborted).toBe(true);
      await act(async () => {
        slow.resolve(draft(1, "a"));
      });
      expect(result.current.draft).toBeUndefined();
    });

    it("does not show one document's draft under another", async () => {
      api.getDraft.mockResolvedValueOnce(draft(1, "a"));
      const { result, rerender } = renderHook(({ doc }) => useExamDraft("e1", doc), { initialProps: { doc: "d1" } });
      await waitFor(() => expect(result.current.draft).toBeDefined());

      const slow = deferred<ExamDocumentDraft>();
      api.getDraft.mockReturnValueOnce(slow.promise);
      rerender({ doc: "d2" });
      expect(result.current.draft).toBeUndefined();
      expect(result.current.loading).toBe(true);
      await act(async () => {
        slow.resolve({ ...draft(1, "z"), document: { ...draft(1).document, id: "d2" } });
      });
      await waitFor(() => expect(result.current.draft?.questions[0].id).toBe("z"));
    });
  });
});
