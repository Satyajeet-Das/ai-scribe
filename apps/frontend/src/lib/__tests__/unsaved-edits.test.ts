import { describe, it, expect, afterEach, vi } from "vitest";
import { blankForm } from "../draft-form";
import { clearUnsavedEdit, loadUnsavedEdit, saveUnsavedEdit } from "../unsaved-edits";

describe("unsaved edits", () => {
  afterEach(() => {
    vi.unstubAllGlobals();
    sessionStorage.clear();
  });

  const form = { ...blankForm(), text: "Half-typed question" };

  it("keeps an edit and gives it back for the same version", () => {
    saveUnsavedEdit("d1", "q1", "v1", form);
    expect(loadUnsavedEdit("d1", "q1", "v1")).toEqual(form);
  });

  it("keeps edits of different questions and documents apart", () => {
    saveUnsavedEdit("d1", "q1", "v1", form);
    expect(loadUnsavedEdit("d1", "q2", "v1")).toBeNull();
    expect(loadUnsavedEdit("d2", "q1", "v1")).toBeNull();
  });

  it("will not bring back an edit begun on an older version than the one saved since", () => {
    saveUnsavedEdit("d1", "q1", "old", form);
    expect(loadUnsavedEdit("d1", "q1", "new")).toBeNull();
    expect(loadUnsavedEdit("d1", "q1", "old")).toBeNull(); // and it has been discarded
  });

  it("forgets an edit once it is saved or discarded", () => {
    saveUnsavedEdit("d1", "q1", "v1", form);
    clearUnsavedEdit("d1", "q1");
    expect(loadUnsavedEdit("d1", "q1", "v1")).toBeNull();
  });

  it("ignores damaged data", () => {
    sessionStorage.setItem("scribe:draft-edit:d1:q1", "{not json");
    expect(loadUnsavedEdit("d1", "q1", "v1")).toBeNull();
    sessionStorage.setItem("scribe:draft-edit:d1:q2", JSON.stringify({ base: "v1", form: { nope: true } }));
    expect(loadUnsavedEdit("d1", "q2", "v1")).toBeNull();
  });

  it("carries on when storage is unavailable", () => {
    const blocked = () => {
      throw new DOMException("blocked", "SecurityError");
    };
    vi.stubGlobal("sessionStorage", { setItem: blocked, getItem: blocked, removeItem: blocked });
    expect(() => saveUnsavedEdit("d1", "q1", "v1", form)).not.toThrow();
    expect(loadUnsavedEdit("d1", "q1", "v1")).toBeNull();
    expect(() => clearUnsavedEdit("d1", "q1")).not.toThrow();
  });
});
