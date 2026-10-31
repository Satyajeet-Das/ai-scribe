import { describe, it, expect } from "vitest";
import type { DraftQuestion } from "@/types/exam-document";
import {
  attentionFor,
  blankForm,
  notesFor,
  parsePoints,
  questionLabel,
  sameForm,
  toForm,
  toRequest,
  validateForm,
} from "../draft-form";

function q(over: Partial<DraftQuestion> = {}): DraftQuestion {
  return {
    id: "q1", position: 1, numberLabel: "1", type: "MCQ", instructions: "", text: "What is the SI unit of force?",
    points: 2,
    options: [
      { label: "A", text: "Newton", isCorrect: false },
      { label: "B", text: "Joule", isCorrect: false },
    ],
    tables: [], diagrams: [], sourcePages: [1], confidence: "HIGH", warnings: [], origin: "EXTRACTED", edited: false,
    createdAt: "2026-01-01T00:00:00Z", updatedAt: "2026-01-01T00:00:00Z", ...over,
  };
}

describe("toForm / toRequest", () => {
  it("round-trips a question without changing it", () => {
    const original = q({ tables: [{ page: 1, markdown: "| a |" }], diagrams: [{ page: 1, region: "left", description: "A box.", confirmed: true }] });
    const req = toRequest(toForm(original), original.warnings);
    expect(req).toEqual({
      numberLabel: "1", type: "MCQ", instructions: "", text: "What is the SI unit of force?", points: 2,
      options: [{ label: "A", text: "Newton", isCorrect: false }, { label: "B", text: "Joule", isCorrect: false }],
      tables: [{ page: 1, markdown: "| a |" }],
      diagrams: [{ page: 1, region: "left", description: "A box.", confirmed: true }],
      acknowledgedWarnings: [],
    });
  });

  it("shows unset marks as empty and sends them as null, never as zero", () => {
    const form = toForm(q({ points: null }));
    expect(form.points).toBe("");
    expect(toRequest(form, []).points).toBeNull();
  });

  it("keeps zero marks as zero", () => {
    expect(toRequest(toForm(q({ points: 0 })), []).points).toBe(0);
  });

  it("never sends options for a written question, but keeps them in the form", () => {
    const form = { ...toForm(q()), type: "ESSAY" as const };
    expect(form.options).toHaveLength(2);
    expect(toRequest(form, []).options).toEqual([]);
  });

  it("never sends the teacher's 'correct' choice for a question that is not multiple choice", () => {
    const form = { ...toForm(q()), type: "ESSAY" as const };
    form.options[0].isCorrect = true;
    expect(JSON.stringify(toRequest(form, []))).not.toContain("true");
  });

  it("trims what the teacher typed", () => {
    const form = { ...toForm(q()), text: "  Spaced out  ", numberLabel: " 3 " };
    const req = toRequest(form, []);
    expect(req.text).toBe("Spaced out");
    expect(req.numberLabel).toBe("3");
  });

  it("does not send a confirmed diagram whose description was cleared", () => {
    const form = toForm(q({ diagrams: [{ page: 1, region: "", description: "A box.", confirmed: true }] }));
    form.diagrams[0].description = "   ";
    expect(toRequest(form, []).diagrams[0].confirmed).toBe(false);
  });

  it("acknowledges only blocking warnings that exist", () => {
    const warnings = [
      { code: "UNVERIFIED_TEXT", message: "m", blocking: true, acknowledged: false },
      { code: "PAGE_UNKNOWN", message: "m", blocking: false, acknowledged: false },
    ];
    const form = { ...toForm(q({ warnings })), acknowledged: ["UNVERIFIED_TEXT", "PAGE_UNKNOWN", "MADE_UP"] };
    expect(toRequest(form, warnings).acknowledgedWarnings).toEqual(["UNVERIFIED_TEXT"]);
  });

  it("starts from the acknowledgements already saved", () => {
    const warnings = [{ code: "UNVERIFIED_TEXT", message: "m", blocking: true, acknowledged: true }];
    expect(toForm(q({ warnings })).acknowledged).toEqual(["UNVERIFIED_TEXT"]);
  });

  it("does not share its arrays with the saved question", () => {
    const saved = q();
    const form = toForm(saved);
    form.options[0].text = "changed";
    expect(saved.options[0].text).toBe("Newton");
  });
});

describe("parsePoints", () => {
  it.each([["5", 5], [" 12 ", 12], ["0", 0]])("reads %j as %j", (raw, want) => expect(parsePoints(raw)).toBe(want));
  it.each([[""], ["2.5"], ["-1"], ["abc"], ["1e3"], ["١٢"]])("rejects %j", (raw) => expect(parsePoints(raw)).toBeNull());
});

describe("sameForm", () => {
  it("is true for equal forms, whatever the order of acknowledgements", () => {
    const a = { ...toForm(q()), acknowledged: ["A", "B"] };
    const b = { ...toForm(q()), acknowledged: ["B", "A"] };
    expect(sameForm(a, b)).toBe(true);
  });
  it("sees any real edit", () => {
    const a = toForm(q());
    expect(sameForm(a, { ...a, text: a.text + "?" })).toBe(false);
    expect(sameForm(a, { ...a, options: [...a.options, { label: "C", text: "x", isCorrect: false }] })).toBe(false);
  });
});

describe("validateForm", () => {
  const ok = () => toForm(q());

  it("accepts a good question", () => {
    expect(validateForm(ok(), 3)).toEqual({});
  });

  it("accepts a draft that is not complete yet, as the server does (completeness is checked at approval)", () => {
    const form = { ...ok(), points: "", options: [{ label: "A", text: "Only one", isCorrect: false }] };
    expect(validateForm(form, 3)).toEqual({});
  });

  it("needs question text", () => {
    expect(validateForm({ ...ok(), text: "  a " }, 3).text).toMatch(/question text/i);
  });

  it.each([["2.5"], ["-1"], ["many"]])("refuses marks %j", (points) => {
    expect(validateForm({ ...ok(), points }, 3).points).toMatch(/whole number/i);
  });

  it("refuses marks over the limit", () => {
    expect(validateForm({ ...ok(), points: "1001" }, 3).points).toMatch(/at most 1000/);
  });

  it("checks option labels: required, letters and digits only, unique without regard to case", () => {
    const form = ok();
    form.options = [
      { label: "", text: "x", isCorrect: false },
      { label: "A)", text: "x", isCorrect: false },
      { label: "b", text: "x", isCorrect: false },
      { label: "B", text: "x", isCorrect: false },
    ];
    const e = validateForm(form, 3);
    expect(e["options[0].label"]).toMatch(/label/i);
    expect(e["options[1].label"]).toMatch(/letters and digits/i);
    expect(e["options[2].label"]).toBeUndefined();
    expect(e["options[3].label"]).toMatch(/used by another option/i);
  });

  it("needs text for each option, and allows only one correct answer", () => {
    const form = ok();
    form.options = [
      { label: "A", text: "", isCorrect: true },
      { label: "B", text: "y", isCorrect: true },
    ];
    const e = validateForm(form, 3);
    expect(e["options[0].text"]).toMatch(/text of this option/i);
    expect(e.options).toMatch(/only one option/i);
  });

  it("ignores the options of a written question", () => {
    const form = { ...ok(), type: "ESSAY" as const };
    form.options = [{ label: "", text: "", isCorrect: false }];
    expect(validateForm(form, 3)).toEqual({});
  });

  it("checks table and diagram pages against the document", () => {
    const form = ok();
    form.tables = [{ page: 9, markdown: "| a |" }, { page: 1, markdown: "  " }];
    form.diagrams = [{ page: 0, region: "", description: "", confirmed: true }];
    const e = validateForm(form, 3);
    expect(e["tables[0].page"]).toMatch(/between 1 and 3/);
    expect(e["tables[1].markdown"]).toMatch(/enter the table/i);
    expect(e["diagrams[0].page"]).toMatch(/between 1 and 3/);
    expect(e["diagrams[0].confirmed"]).toMatch(/describe the diagram/i);
  });

  it("counts characters, not bytes, so accented text and maths are not penalised", () => {
    const form = { ...ok(), text: "é".repeat(8000) };
    expect(validateForm(form, 3).text).toBeUndefined();
    expect(validateForm({ ...form, text: "é".repeat(8001) }, 3).text).toMatch(/under 8000/);
  });
});

describe("attentionFor", () => {
  it("has nothing to say about a complete question", () => {
    expect(attentionFor(q())).toEqual([]);
  });

  it("finds missing marks", () => {
    expect(attentionFor(q({ points: null })).map((a) => a.key)).toContain("points");
  });

  it("does not treat zero marks as missing", () => {
    expect(attentionFor(q({ points: 0 }))).toEqual([]);
  });

  it("finds a multiple-choice question with too few options", () => {
    expect(attentionFor(q({ options: [{ label: "A", text: "x", isCorrect: false }] })).map((a) => a.key)).toContain("options");
  });

  it("finds a written question that somehow has options", () => {
    expect(attentionFor(q({ type: "ESSAY" })).map((a) => a.key)).toContain("options");
  });

  it("distinguishes a diagram with no description from one not yet confirmed", () => {
    const blank = attentionFor(q({ diagrams: [{ page: 2, region: "", description: "", confirmed: false }] }));
    expect(blank[0].message).toMatch(/has no description/);
    const unconfirmed = attentionFor(q({ diagrams: [{ page: 2, region: "", description: "A box.", confirmed: false }] }));
    expect(unconfirmed[0].message).toMatch(/confirm/i);
    expect(attentionFor(q({ diagrams: [{ page: 2, region: "", description: "A box.", confirmed: true }] }))).toEqual([]);
  });

  it("flags a blocking warning until it is acknowledged, and ignores non-blocking ones", () => {
    const warnings = [
      { code: "UNVERIFIED_TEXT", message: "m", blocking: true, acknowledged: false },
      { code: "PAGE_UNKNOWN", message: "m", blocking: false, acknowledged: false },
    ];
    expect(attentionFor(q({ warnings })).map((a) => a.key)).toEqual(["warning-UNVERIFIED_TEXT"]);
    expect(attentionFor(q({ warnings: [{ ...warnings[0], acknowledged: true }, warnings[1]] }))).toEqual([]);
  });

  it("reports everything at once", () => {
    const all = attentionFor(q({ points: null, options: [], text: "" }));
    expect(all.map((a) => a.key)).toEqual(expect.arrayContaining(["text", "points", "options"]));
  });
});

describe("notesFor", () => {
  it("lists non-blocking warnings and a low-confidence note, never blocking ones", () => {
    const notes = notesFor(
      q({
        confidence: "LOW",
        warnings: [
          { code: "A", message: "Page was a guess.", blocking: false, acknowledged: false },
          { code: "B", message: "Blocking one.", blocking: true, acknowledged: false },
        ],
      })
    );
    expect(notes[0]).toMatch(/not confident/i);
    expect(notes).toContain("Page was a guess.");
    expect(notes).not.toContain("Blocking one.");
  });
});

describe("questionLabel / blankForm", () => {
  it("prefers the number printed on the paper, and falls back to the position", () => {
    expect(questionLabel({ numberLabel: "2(b)", position: 5 })).toBe("2(b)");
    expect(questionLabel({ numberLabel: "  ", position: 5 })).toBe("5");
  });
  it("starts a new question as a written one with nothing invented", () => {
    const b = blankForm();
    expect(b).toMatchObject({ type: "ESSAY", text: "", points: "", options: [], tables: [], diagrams: [] });
  });
});
