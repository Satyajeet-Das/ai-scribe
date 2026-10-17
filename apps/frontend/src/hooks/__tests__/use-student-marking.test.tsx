import { act, renderHook, waitFor } from "@testing-library/react";
import { beforeEach, describe, expect, it, vi } from "vitest";
import { ApiError } from "@/services/api";
import { responseReviewApi } from "@/services/response-review-api";
import { useStudentMarking } from "../use-student-marking";
import {
  finalSheet,
  markingQuestion,
  markingSheet,
} from "@/components/response-review/__tests__/marking-fixtures";

vi.mock("@/services/response-review-api", () => ({
  responseReviewApi: {
    getMarking: vi.fn(),
    saveQuestionMarks: vi.fn(),
    finalise: vi.fn(),
    reopen: vi.fn(),
  },
}));

const api = vi.mocked(responseReviewApi);

async function ready(sheet = markingSheet()) {
  api.getMarking.mockResolvedValue(sheet);
  const hook = renderHook(() => useStudentMarking("exam-1", "s-1"));
  await waitFor(() => expect(hook.result.current.sheet).toBeDefined());
  return hook;
}

const q2 = () => markingSheet().questions[1];

describe("useStudentMarking", () => {
  beforeEach(() => {
    Object.values(api).forEach((fn) => fn.mockReset());
  });

  it("starts with the saved values and nothing unsaved", async () => {
    const { result } = await ready();
    expect(result.current.draftFor(q2())).toEqual({ marks: "", comment: "" });
    expect(result.current.anyDirty).toBe(false);
    expect(result.current.unmarkedNumbers).toEqual([2]);
  });

  it("tracks what has been typed, and clears it when the text matches the saved value again", async () => {
    const { result } = await ready();
    act(() => result.current.setMarks(q2(), "7.5"));
    expect(result.current.isDirty(q2())).toBe(true);
    expect(result.current.anyDirty).toBe(true);
    act(() => result.current.setMarks(q2(), ""));
    expect(result.current.anyDirty).toBe(false);
  });

  it("puts the automatic mark in the field without saving it", async () => {
    const sheet = markingSheet();
    const { result } = await ready(sheet);
    act(() => result.current.applyAutoMark(sheet.questions[0]));
    expect(result.current.draftFor(sheet.questions[0]).marks).toBe("2");
    expect(api.saveQuestionMarks).not.toHaveBeenCalled();
  });

  it("saves with the attempt and version it read, then shows the server's totals", async () => {
    const { result } = await ready();
    const saved = markingQuestion({
      teacherMarks: 7.5,
      effectiveMarks: 7.5,
      markSource: "TEACHER",
      comment: "clear",
      version: 1,
    });
    api.saveQuestionMarks.mockResolvedValue({
      question: saved,
      paper: { ...markingSheet().paper, version: 1, totalMarks: 9.5, markedCount: 3 },
    });

    act(() => {
      result.current.setMarks(q2(), " 7.5 ");
      result.current.setComment(q2(), "  clear ");
    });
    let outcome;
    await act(async () => {
      outcome = await result.current.save("q2");
    });

    expect(outcome).toEqual({ ok: true });
    expect(api.saveQuestionMarks).toHaveBeenCalledWith("exam-1", "s-1", "q2", {
      sessionId: "sess-1",
      marks: 7.5,
      comment: "clear",
      expectedVersion: 0,
    });
    expect(result.current.sheet?.paper.totalMarks).toBe(9.5);
    expect(result.current.unmarkedNumbers).toEqual([]);
    expect(result.current.anyDirty).toBe(false);
    expect(result.current.statusOf("q2").state).toBe("saved");
  });

  it("sends null to clear a mark and a comment", async () => {
    const marked = markingSheet();
    marked.questions[1] = markingQuestion({
      teacherMarks: 4,
      effectiveMarks: 4,
      comment: "ok",
      version: 2,
      markSource: "TEACHER",
    });
    const { result } = await ready(marked);
    api.saveQuestionMarks.mockResolvedValue({
      question: markingQuestion({ version: 3 }),
      paper: marked.paper,
    });
    act(() => {
      result.current.setMarks(marked.questions[1], "");
      result.current.setComment(marked.questions[1], "");
    });
    await act(async () => {
      await result.current.save("q2");
    });
    expect(api.saveQuestionMarks.mock.calls[0][3]).toMatchObject({
      marks: null,
      comment: null,
      expectedVersion: 2,
    });
  });

  it("does not call the server for a mark it can see is wrong, and says why", async () => {
    const { result } = await ready();
    act(() => result.current.setMarks(q2(), "11"));
    let outcome;
    await act(async () => {
      outcome = await result.current.save("q2");
    });
    expect(outcome).toEqual({ ok: false, message: "Marks cannot be more than 10." });
    expect(api.saveQuestionMarks).not.toHaveBeenCalled();
    expect(result.current.statusOf("q2")).toMatchObject({ state: "error" });
    expect(result.current.isDirty(q2())).toBe(true);
  });

  it("keeps the typed text and reports a conflict when the mark changed elsewhere", async () => {
    const { result } = await ready();
    api.saveQuestionMarks.mockRejectedValue(new ApiError("changed", 409, "STALE_VERSION"));
    act(() => result.current.setMarks(q2(), "6"));
    await act(async () => {
      await result.current.save("q2");
    });
    expect(result.current.statusOf("q2").state).toBe("conflict");
    expect(result.current.draftFor(q2()).marks).toBe("6");

    const latest = markingSheet();
    latest.questions[1] = markingQuestion({
      teacherMarks: 9,
      effectiveMarks: 9,
      version: 4,
      markSource: "TEACHER",
    });
    api.getMarking.mockResolvedValue(latest);
    await act(async () => {
      await result.current.loadLatest("q2");
    });
    expect(result.current.sheet?.questions[1].teacherMarks).toBe(9);
    expect(result.current.draftFor(latest.questions[1]).marks).toBe("9");
    expect(result.current.statusOf("q2").state).toBe("idle");
  });

  it("reloads when the paper was finalised or another attempt is now shown", async () => {
    const { result } = await ready();
    api.saveQuestionMarks.mockRejectedValue(
      new ApiError("This paper is finalised.", 409, "PAPER_FINALISED")
    );
    api.getMarking.mockResolvedValue(finalSheet());
    act(() => result.current.setMarks(q2(), "6"));
    await act(async () => {
      await result.current.save("q2");
    });
    expect(result.current.sheet?.paper.status).toBe("FINAL");
    expect(result.current.statusOf("q2")).toMatchObject({
      state: "error",
      message: "This paper is finalised.",
    });
  });

  it("will not finalise with unsaved changes", async () => {
    const { result } = await ready();
    act(() => result.current.setMarks(q2(), "6"));
    let outcome;
    await act(async () => {
      outcome = await result.current.finalise();
    });
    expect(outcome).toMatchObject({ ok: false });
    expect(api.finalise).not.toHaveBeenCalled();
  });

  it("finalises with the paper version it holds and shows the frozen result", async () => {
    const { result } = await ready();
    api.finalise.mockResolvedValue(finalSheet());
    await act(async () => {
      expect(await result.current.finalise()).toEqual({ ok: true });
    });
    expect(api.finalise).toHaveBeenCalledWith("exam-1", "s-1", {
      sessionId: "sess-1",
      expectedPaperVersion: 0,
    });
    expect(result.current.sheet?.paper.status).toBe("FINAL");
  });

  it("reports why finalising failed, with the server's wording", async () => {
    const { result } = await ready();
    api.finalise.mockRejectedValue(new ApiError("Missing: 2", 409, "PAPER_INCOMPLETE"));
    await act(async () => {
      expect(await result.current.finalise()).toEqual({ ok: false, message: "Missing: 2" });
    });
  });

  it("reopens with a trimmed reason", async () => {
    const { result } = await ready(finalSheet());
    api.reopen.mockResolvedValue(markingSheet());
    await act(async () => {
      expect(await result.current.reopen("  appeal upheld ")).toEqual({ ok: true });
    });
    expect(api.reopen).toHaveBeenCalledWith("exam-1", "s-1", {
      sessionId: "sess-1",
      reason: "appeal upheld",
    });
    expect(result.current.sheet?.paper.status).toBe("DRAFT");
  });

  it("warns before the page is left with unsaved marks, and only then", async () => {
    const { result } = await ready();
    const fire = () => {
      const e = new Event("beforeunload", { cancelable: true });
      window.dispatchEvent(e);
      return e.defaultPrevented;
    };
    expect(fire()).toBe(false);
    act(() => result.current.setMarks(q2(), "3"));
    expect(fire()).toBe(true);
  });
});
