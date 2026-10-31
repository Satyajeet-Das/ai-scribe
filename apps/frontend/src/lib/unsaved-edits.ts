import type { QuestionForm } from "@/lib/draft-form";

/**
 * Unsaved edits to a draft question, kept in the browser tab's session storage
 * so a reload, an accidental navigation or a failed save does not lose what the
 * teacher typed. It is a safety copy, never the source of truth: the server's
 * draft is. Every access can fail (private windows, blocked storage, full
 * quota), and then there simply is no safety copy.
 */

interface Stored {
  /** The question's `updatedAt` when editing began. A newer saved version makes this copy stale. */
  base: string;
  form: QuestionForm;
}

function storageKey(documentId: string, questionId: string): string {
  return `scribe:draft-edit:${documentId}:${questionId}`;
}

export function saveUnsavedEdit(documentId: string, questionId: string, base: string, form: QuestionForm): void {
  try {
    sessionStorage.setItem(storageKey(documentId, questionId), JSON.stringify({ base, form } satisfies Stored));
  } catch {
    // No safety copy; editing carries on.
  }
}

/** The kept edit, only if it was begun on the version that is still current. */
export function loadUnsavedEdit(documentId: string, questionId: string, base: string): QuestionForm | null {
  try {
    const raw = sessionStorage.getItem(storageKey(documentId, questionId));
    if (!raw) return null;
    const parsed = JSON.parse(raw) as Partial<Stored>;
    if (parsed.base !== base || !parsed.form || typeof parsed.form.text !== "string") {
      sessionStorage.removeItem(storageKey(documentId, questionId));
      return null;
    }
    return parsed.form;
  } catch {
    return null;
  }
}

export function clearUnsavedEdit(documentId: string, questionId: string): void {
  try {
    sessionStorage.removeItem(storageKey(documentId, questionId));
  } catch {
    // Nothing to clear.
  }
}
