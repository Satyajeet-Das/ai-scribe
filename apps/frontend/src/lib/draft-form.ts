import type {
  DraftOption,
  DraftQuestion,
  DraftQuestionRequest,
  DraftQuestionType,
  DraftWarning,
} from "@/types/exam-document";

/**
 * The editable shape of a draft question, and the rules around it.
 *
 * The validation here mirrors the server's (apps/backend/internal/document/
 * validate.go) so a teacher hears about a problem at once. The server repeats
 * every check and is the authority: a form that passes here can still be refused.
 */

export interface FormTable {
  page: number;
  markdown: string;
}

export interface FormDiagram {
  page: number;
  region: string;
  description: string;
  confirmed: boolean;
}

export interface QuestionForm {
  numberLabel: string;
  type: DraftQuestionType;
  instructions: string;
  text: string;
  /** Kept as typed; empty means "not set yet". */
  points: string;
  /** Kept while the type is ESSAY, so switching back does not lose them; not sent then. */
  options: DraftOption[];
  tables: FormTable[];
  diagrams: FormDiagram[];
  /** Codes of blocking warnings the teacher says they have checked. */
  acknowledged: string[];
}

export const LIMITS = {
  label: 30,
  text: 8000,
  instructions: 4000,
  options: 12,
  optionKey: 10,
  optionText: 2000,
  tables: 10,
  tableMarkdown: 8000,
  diagrams: 10,
  region: 200,
  description: 2000,
  points: 1000,
} as const;

export function toForm(q: DraftQuestion): QuestionForm {
  return {
    numberLabel: q.numberLabel,
    type: q.type,
    instructions: q.instructions,
    text: q.text,
    points: q.points === null ? "" : String(q.points),
    options: q.options.map((o) => ({ ...o })),
    tables: q.tables.map((t) => ({ ...t })),
    diagrams: q.diagrams.map((d) => ({
      page: d.page,
      region: d.region,
      description: d.description,
      confirmed: d.confirmed,
    })),
    acknowledged: q.warnings.filter((w) => w.blocking && w.acknowledged).map((w) => w.code),
  };
}

/** The point count the form holds, or null when it is empty or not a whole number. */
export function parsePoints(raw: string): number | null {
  const t = raw.trim();
  if (!/^\d+$/.test(t)) return null;
  return Number(t);
}

export function toRequest(
  form: QuestionForm,
  warnings: Array<Pick<DraftWarning, "code" | "blocking">>
): DraftQuestionRequest {
  const blocking = new Set(warnings.filter((w) => w.blocking).map((w) => w.code));
  return {
    numberLabel: form.numberLabel.trim(),
    type: form.type,
    instructions: form.instructions.trim(),
    text: form.text.trim(),
    points: parsePoints(form.points),
    // A written question has no options; the server refuses them.
    options:
      form.type === "ESSAY"
        ? []
        : form.options.map((o) => ({ label: o.label.trim(), text: o.text.trim(), isCorrect: o.isCorrect })),
    tables: form.tables.map((t) => ({ page: t.page, markdown: t.markdown.trim() })),
    diagrams: form.diagrams.map((d) => ({
      page: d.page,
      region: d.region.trim(),
      description: d.description.trim(),
      confirmed: d.confirmed && d.description.trim() !== "",
    })),
    // Only warnings that exist and block can be acknowledged.
    acknowledgedWarnings: form.acknowledged.filter((c) => blocking.has(c)),
  };
}

function normalised(f: QuestionForm): string {
  // Order-independent for the acknowledged codes; everything else compares as typed.
  return JSON.stringify({ ...f, acknowledged: [...f.acknowledged].sort() });
}

export function sameForm(a: QuestionForm, b: QuestionForm): boolean {
  return normalised(a) === normalised(b);
}

/** Problems with the form, keyed by the same field paths the server uses. */
export function validateForm(form: QuestionForm, pageCount: number): Record<string, string> {
  const e: Record<string, string> = {};
  const text = form.text.trim();
  if ([...text].length < 2) e.text = "Enter the question text (at least 2 characters).";
  else if ([...form.text].length > LIMITS.text) e.text = `Keep the question under ${LIMITS.text} characters.`;
  if ([...form.numberLabel].length > LIMITS.label) e.numberLabel = `Keep the number under ${LIMITS.label} characters.`;
  if ([...form.instructions].length > LIMITS.instructions) {
    e.instructions = `Keep the instructions under ${LIMITS.instructions} characters.`;
  }

  const rawPoints = form.points.trim();
  if (rawPoints !== "") {
    const p = parsePoints(rawPoints);
    if (p === null) e.points = "Marks must be a whole number, like 2 or 5.";
    else if (p > LIMITS.points) e.points = `Marks can be at most ${LIMITS.points}.`;
  }

  if (form.type === "MCQ") {
    if (form.options.length > LIMITS.options) e.options = `A question can have at most ${LIMITS.options} options.`;
    const seen = new Set<string>();
    let correct = 0;
    form.options.forEach((o, i) => {
      const key = o.label.trim().toUpperCase();
      if (key === "") e[`options[${i}].label`] = "Give this option a label, like A.";
      else if ([...key].length > LIMITS.optionKey) e[`options[${i}].label`] = `Keep the label under ${LIMITS.optionKey} characters.`;
      else if (!/^[A-Z0-9]+$/.test(key)) e[`options[${i}].label`] = "Use letters and digits only, like A or 2.";
      else if (seen.has(key)) e[`options[${i}].label`] = `The label ${key} is used by another option.`;
      seen.add(key);
      if (o.text.trim() === "") e[`options[${i}].text`] = "Enter the text of this option.";
      else if ([...o.text].length > LIMITS.optionText) e[`options[${i}].text`] = `Keep the option under ${LIMITS.optionText} characters.`;
      if (o.isCorrect) correct++;
    });
    if (correct > 1 && !e.options) e.options = "Only one option can be marked correct.";
  }

  if (form.tables.length > LIMITS.tables) e.tables = `A question can have at most ${LIMITS.tables} tables.`;
  form.tables.forEach((t, i) => {
    if (t.page < 1 || t.page > pageCount) e[`tables[${i}].page`] = `Choose a page between 1 and ${pageCount}.`;
    if (t.markdown.trim() === "") e[`tables[${i}].markdown`] = "Enter the table, or remove it.";
    else if ([...t.markdown].length > LIMITS.tableMarkdown) e[`tables[${i}].markdown`] = `Keep the table under ${LIMITS.tableMarkdown} characters.`;
  });

  if (form.diagrams.length > LIMITS.diagrams) e.diagrams = `A question can have at most ${LIMITS.diagrams} diagrams.`;
  form.diagrams.forEach((d, i) => {
    if (d.page < 1 || d.page > pageCount) e[`diagrams[${i}].page`] = `Choose a page between 1 and ${pageCount}.`;
    if ([...d.region].length > LIMITS.region) e[`diagrams[${i}].region`] = `Keep this under ${LIMITS.region} characters.`;
    if ([...d.description].length > LIMITS.description) e[`diagrams[${i}].description`] = `Keep the description under ${LIMITS.description} characters.`;
    if (d.confirmed && d.description.trim() === "") e[`diagrams[${i}].confirmed`] = "Describe the diagram before confirming it.";
  });
  return e;
}

/** One reason a saved question is not ready to be approved. */
export interface Attention {
  /** A stable key, for list rendering and tests. */
  key: string;
  message: string;
}

/**
 * What stops a saved question being approved, from its saved state. These are
 * the checks the server makes at approval (validate.go problemsForApproval), so
 * a teacher can find what is left to do without opening every question.
 */
export function attentionFor(q: DraftQuestion): Attention[] {
  const out: Attention[] = [];
  if ([...q.text.trim()].length < 2) out.push({ key: "text", message: "The question has no text." });
  if (q.points === null) out.push({ key: "points", message: "Marks are not set." });
  if (q.type === "MCQ") {
    if (q.options.length < 2) out.push({ key: "options", message: "A multiple-choice question needs at least 2 options." });
  } else if (q.options.length > 0) {
    out.push({ key: "options", message: "A written question cannot have options." });
  }
  q.diagrams.forEach((d, i) => {
    if (d.description.trim() === "") {
      out.push({ key: `diagram-${i}`, message: `The diagram on page ${d.page} has no description.` });
    } else if (!d.confirmed) {
      out.push({ key: `diagram-${i}`, message: `Check and confirm the description of the diagram on page ${d.page}.` });
    }
  });
  for (const w of q.warnings) {
    if (w.blocking && !w.acknowledged) {
      out.push({ key: `warning-${w.code}`, message: "A warning needs your check before approval." });
    }
  }
  return out;
}

/** Warnings and low confidence that do not stop approval but deserve a look. */
export function notesFor(q: DraftQuestion): string[] {
  const notes = q.warnings.filter((w) => !w.blocking).map((w) => w.message);
  if (q.confidence === "LOW") notes.unshift("The reader was not confident about this question. Compare it with the original.");
  return notes;
}

export function questionLabel(q: Pick<DraftQuestion, "numberLabel" | "position">): string {
  return q.numberLabel.trim() !== "" ? q.numberLabel.trim() : String(q.position);
}

/** A blank question to start from when adding one the reader missed. */
export function blankForm(): QuestionForm {
  return {
    numberLabel: "",
    type: "ESSAY",
    instructions: "",
    text: "",
    points: "",
    options: [],
    tables: [],
    diagrams: [],
    acknowledged: [],
  };
}
