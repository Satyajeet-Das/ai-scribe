import { describe, it, expect } from "vitest";
import { parseMarksInput } from "../validations";

describe("parseMarksInput", () => {
  it("treats an empty field as no mark", () => {
    expect(parseMarksInput("", 10)).toEqual({ ok: true, value: null });
    expect(parseMarksInput("   ", 10)).toEqual({ ok: true, value: null });
  });

  it.each([
    ["0", 0],
    ["7", 7],
    ["7.5", 7.5],
    ["10", 10],
    ["10.0", 10],
    [" 3.5 ", 3.5],
  ])("accepts %j", (raw, value) => {
    expect(parseMarksInput(raw, 10)).toEqual({ ok: true, value });
  });

  it.each(["7.25", "1e1", ".5", "abc", "7,5", "7.", "0x10", "1 000"])(
    "refuses %j with a plain explanation",
    (raw) => {
      const r = parseMarksInput(raw, 10);
      expect(r.ok).toBe(false);
      if (!r.ok) expect(r.message).toMatch(/whole number or a half mark/);
    }
  );

  it("refuses a negative mark", () => {
    expect(parseMarksInput("-1", 10)).toEqual({ ok: false, message: "Marks cannot be negative." });
  });

  it("refuses more than the question is worth, and allows exactly the maximum", () => {
    expect(parseMarksInput("10.5", 10)).toEqual({
      ok: false,
      message: "Marks cannot be more than 10.",
    });
    expect(parseMarksInput("2.5", 2)).toMatchObject({ ok: false });
    expect(parseMarksInput("2", 2)).toEqual({ ok: true, value: 2 });
  });
});
