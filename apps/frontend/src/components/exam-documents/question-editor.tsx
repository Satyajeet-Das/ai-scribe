"use client";

import { useEffect, useId, useMemo, useRef, useState } from "react";
import { AlertCircle, ExternalLink, Info, Loader2, Plus, Trash2 } from "lucide-react";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Textarea } from "@/components/ui/textarea";
import type { DocumentError } from "@/lib/document-errors";
import {
  LIMITS,
  notesFor,
  sameForm,
  toForm,
  toRequest,
  validateForm,
  type QuestionForm,
} from "@/lib/draft-form";
import { clearUnsavedEdit, loadUnsavedEdit, saveUnsavedEdit } from "@/lib/unsaved-edits";
import type { OpResult } from "@/hooks/use-exam-draft";
import type { DraftQuestion, DraftQuestionRequest } from "@/types/exam-document";

const SELECT_CLASS =
  "h-8 w-full rounded-lg border border-input bg-transparent px-2 text-sm outline-none focus-visible:border-ring focus-visible:ring-3 focus-visible:ring-ring/50 disabled:cursor-not-allowed disabled:opacity-50 aria-invalid:border-destructive dark:bg-input/30";

interface QuestionEditorProps {
  documentId: string;
  question: DraftQuestion;
  pageCount: number;
  /** A question being added: nothing to restore, and the buttons say so. */
  isNew?: boolean;
  readOnly?: boolean;
  /** Changes are refused (the draft is out of date, or something else is being saved). */
  locked?: boolean;
  onSave: (request: DraftQuestionRequest, form: QuestionForm) => Promise<OpResult<unknown>>;
  /** Called after a successful save. */
  onSaved?: () => void;
  onCancel?: () => void;
  onDirtyChange?: (dirty: boolean) => void;
  onOpenSource: (page: number) => void;
}

function nextLabel(form: QuestionForm): string {
  const used = new Set(form.options.map((o) => o.label.trim().toUpperCase()));
  for (let i = 0; i < 26; i++) {
    const l = String.fromCharCode(65 + i);
    if (!used.has(l)) return l;
  }
  return "";
}

/**
 * Edits one draft question. What is typed stays in this component until Save,
 * with a safety copy in session storage; Save sends the question's complete
 * state to the server, which has the last word on whether it is valid.
 *
 * Nothing is filled in on the teacher's behalf: marks start empty if the paper
 * did not print them, and no answer is marked correct unless the teacher does it.
 */
export function QuestionEditor({
  documentId,
  question,
  pageCount,
  isNew,
  readOnly,
  locked,
  onSave,
  onSaved,
  onCancel,
  onDirtyChange,
  onOpenSource,
}: QuestionEditorProps) {
  const uid = useId();
  const id = (name: string) => `${uid}-${name}`;
  const base = useMemo(() => toForm(question), [question]);

  // null means "no edits": show the saved question. A kept edit from an earlier visit is picked up once.
  const [edited, setEdited] = useState<QuestionForm | null>(() =>
    isNew ? null : loadUnsavedEdit(documentId, question.id, question.updatedAt)
  );
  const [restoredNote, setRestoredNote] = useState(edited !== null);
  const [saving, setSaving] = useState(false);
  const [showErrors, setShowErrors] = useState(false);
  const [failure, setFailure] = useState<DocumentError | null>(null);
  const root = useRef<HTMLFormElement>(null);

  const form = edited ?? base;
  const dirty = edited !== null && !sameForm(edited, base);
  const localErrors = useMemo(() => validateForm(form, pageCount), [form, pageCount]);
  // Server field errors are shown until the field is edited again.
  const serverErrors = failure?.fields ?? {};
  const errorFor = (path: string): string | undefined =>
    (showErrors ? localErrors[path] : undefined) ?? serverErrors[path];
  const notes = notesFor(question);
  const blockingWarnings = question.warnings.filter((w) => w.blocking);

  // Told only when dirtiness changes, not whenever the parent re-renders with a new callback.
  const dirtyCallback = useRef(onDirtyChange);
  useEffect(() => {
    dirtyCallback.current = onDirtyChange;
  });
  useEffect(() => {
    dirtyCallback.current?.(dirty);
  }, [dirty]);

  // A safety copy of what is typed, removed as soon as it is saved or discarded.
  useEffect(() => {
    if (isNew) return;
    if (dirty && edited) saveUnsavedEdit(documentId, question.id, question.updatedAt, edited);
    else clearUnsavedEdit(documentId, question.id);
  }, [dirty, documentId, edited, isNew, question.id, question.updatedAt]);

  function update(patch: Partial<QuestionForm>) {
    setFailure(null);
    setEdited({ ...form, ...patch });
  }

  function focusFirstProblem(errors: Record<string, string>) {
    const first = Object.keys(errors)[0];
    if (!first) return;
    const fields = Array.from(root.current?.querySelectorAll<HTMLElement>("[data-field]") ?? []);
    fields.find((el) => el.dataset.field === first)?.focus();
  }

  async function submit(e?: React.FormEvent) {
    e?.preventDefault();
    if (saving || readOnly || locked) return;
    setShowErrors(true);
    if (Object.keys(localErrors).length > 0) {
      focusFirstProblem(localErrors);
      return;
    }
    setSaving(true);
    setFailure(null);
    const result = await onSave(toRequest(form, question.warnings), form);
    setSaving(false);
    if (result.ok) {
      setEdited(null);
      setShowErrors(false);
      setRestoredNote(false);
      onSaved?.();
    } else {
      setFailure(result.error);
      // Put the cursor on the first field the server named, if it named any.
      requestAnimationFrame(() => focusFirstProblem(result.error.fields));
    }
  }

  function discard() {
    setEdited(null);
    setFailure(null);
    setShowErrors(false);
    setRestoredNote(false);
    onCancel?.();
  }

  const disabled = Boolean(readOnly) || saving;

  const fieldProps = (path: string, extra: Record<string, unknown> = {}) => {
    const err = errorFor(path);
    return {
      "data-field": path,
      "aria-invalid": err ? true : undefined,
      "aria-describedby": err ? id(`err-${path}`) : undefined,
      disabled,
      ...extra,
    };
  };
  const errorText = (path: string) => {
    const err = errorFor(path);
    return err ? (
      <p id={id(`err-${path}`)} className="mt-1 text-xs text-destructive">
        {err}
      </p>
    ) : null;
  };

  const sourcePages = question.sourcePages;

  return (
    <form ref={root} onSubmit={submit} noValidate className="space-y-5" aria-label={`Edit question ${question.numberLabel || question.position}`}>
      {restoredNote && dirty && (
        <p role="status" className="rounded-md border border-amber-300 bg-amber-50 p-2 text-xs text-amber-900 dark:bg-amber-950 dark:text-amber-200">
          Your earlier unsaved changes to this question were restored.
        </p>
      )}

      {(sourcePages.length > 0 || question.origin === "TEACHER") && (
        <div className="flex flex-wrap items-center gap-2 text-xs text-muted-foreground">
          {question.origin === "TEACHER" ? (
            <Badge variant="outline">Added by you</Badge>
          ) : (
            <>
              <span>
                Found on {sourcePages.length === 1 ? "page" : "pages"} {sourcePages.join(", ")} of the paper.
              </span>
              {sourcePages.map((p) => (
                <Button key={p} type="button" variant="outline" size="xs" onClick={() => onOpenSource(p)}>
                  <ExternalLink className="size-3" aria-hidden="true" />
                  View page {p} in the original PDF
                </Button>
              ))}
            </>
          )}
        </div>
      )}

      {(notes.length > 0 || blockingWarnings.length > 0) && (
        <section aria-label="Things to check" className="space-y-2 rounded-lg border bg-muted/30 p-3">
          <h4 className="flex items-center gap-1.5 text-sm font-semibold">
            <Info className="size-4" aria-hidden="true" />
            Things to check
          </h4>
          <ul className="space-y-2 text-sm">
            {blockingWarnings.map((w) => (
              <li key={w.code} className="flex items-start gap-2">
                <input
                  type="checkbox"
                  id={id(`ack-${w.code}`)}
                  data-field={`warnings.${w.code}`}
                  className="mt-1 size-4"
                  checked={form.acknowledged.includes(w.code)}
                  disabled={disabled}
                  onChange={(e) =>
                    update({
                      acknowledged: e.target.checked
                        ? [...form.acknowledged, w.code]
                        : form.acknowledged.filter((c) => c !== w.code),
                    })
                  }
                />
                <label htmlFor={id(`ack-${w.code}`)}>
                  <span className="font-semibold">Needs your check: </span>
                  {w.message}
                  <span className="block text-xs text-muted-foreground">Tick this once you have compared it with the original.</span>
                </label>
              </li>
            ))}
            {notes.map((n) => (
              <li key={n}>
                <span className="font-semibold">Note: </span>
                {n}
              </li>
            ))}
          </ul>
        </section>
      )}

      <div className="grid gap-4 sm:grid-cols-[8rem_1fr_8rem]">
        <div>
          <Label htmlFor={id("number")}>Number on the paper</Label>
          <Input id={id("number")} value={form.numberLabel} onChange={(e) => update({ numberLabel: e.target.value })} {...fieldProps("numberLabel")} />
          {errorText("numberLabel")}
        </div>
        <div>
          <Label htmlFor={id("type")}>Question type</Label>
          <select
            id={id("type")}
            className={SELECT_CLASS}
            value={form.type}
            onChange={(e) => update({ type: e.target.value as QuestionForm["type"] })}
            {...fieldProps("type")}
          >
            <option value="MCQ">Multiple choice</option>
            <option value="ESSAY">Written answer</option>
          </select>
          {form.type === "ESSAY" && form.options.length > 0 && (
            <p className="mt-1 text-xs text-muted-foreground">
              Options are kept here in case you switch back, but are not saved for a written question.
            </p>
          )}
        </div>
        <div>
          <Label htmlFor={id("points")}>Marks</Label>
          <Input
            id={id("points")}
            inputMode="numeric"
            value={form.points}
            placeholder="Not set"
            onChange={(e) => update({ points: e.target.value })}
            {...fieldProps("points")}
          />
          {errorText("points")}
        </div>
      </div>

      <div>
        <Label htmlFor={id("instructions")}>Instructions for this question (optional)</Label>
        <Textarea id={id("instructions")} rows={2} value={form.instructions} onChange={(e) => update({ instructions: e.target.value })} {...fieldProps("instructions")} />
        {errorText("instructions")}
      </div>

      <div>
        <Label htmlFor={id("text")}>Question text</Label>
        <Textarea id={id("text")} rows={4} value={form.text} onChange={(e) => update({ text: e.target.value })} {...fieldProps("text")} />
        {errorText("text")}
        <p className="mt-1 text-xs text-muted-foreground">
          Equations and symbols are kept as plain text, exactly as read. Compare fractions, powers and special
          symbols with the original.
        </p>
      </div>

      {form.type === "MCQ" && (
        <fieldset className="space-y-3" aria-describedby={id("options-help")}>
          <legend className="text-sm font-medium">Options</legend>
          <p id={id("options-help")} className="text-xs text-muted-foreground">
            Marking an answer as correct is optional, and only you can do it: the reader never chooses one.
          </p>
          {errorText("options")}
          {form.options.map((o, i) => (
            <div key={i} className="grid items-start gap-2 sm:grid-cols-[5rem_1fr_auto_auto]">
              <div>
                <Input
                  aria-label={`Option ${i + 1} label`}
                  value={o.label}
                  onChange={(e) => update({ options: form.options.map((x, j) => (j === i ? { ...x, label: e.target.value } : x)) })}
                  {...fieldProps(`options[${i}].label`)}
                />
                {errorText(`options[${i}].label`)}
              </div>
              <div>
                <Input
                  aria-label={`Option ${i + 1} text`}
                  value={o.text}
                  onChange={(e) => update({ options: form.options.map((x, j) => (j === i ? { ...x, text: e.target.value } : x)) })}
                  {...fieldProps(`options[${i}].text`)}
                />
                {errorText(`options[${i}].text`)}
              </div>
              <label className="flex h-8 items-center gap-1.5 text-sm">
                <input
                  type="checkbox"
                  className="size-4"
                  aria-label={`Correct answer: option ${o.label || i + 1}`}
                  checked={o.isCorrect}
                  disabled={disabled}
                  onChange={(e) =>
                    update({ options: form.options.map((x, j) => ({ ...x, isCorrect: j === i ? e.target.checked : e.target.checked ? false : x.isCorrect })) })
                  }
                />
                Correct
              </label>
              <Button
                type="button"
                variant="ghost"
                size="sm"
                disabled={disabled}
                aria-label={`Remove option ${o.label || i + 1}`}
                onClick={() => update({ options: form.options.filter((_, j) => j !== i) })}
              >
                <Trash2 className="size-4" aria-hidden="true" />
                Remove
              </Button>
            </div>
          ))}
          {form.options.length < LIMITS.options && (
            <Button
              type="button"
              variant="outline"
              size="sm"
              disabled={disabled}
              onClick={() => update({ options: [...form.options, { label: nextLabel(form), text: "", isCorrect: false }] })}
            >
              <Plus className="size-4" aria-hidden="true" />
              Add option
            </Button>
          )}
        </fieldset>
      )}

      <fieldset className="space-y-3">
        <legend className="text-sm font-medium">Tables</legend>
        {form.tables.length === 0 && <p className="text-xs text-muted-foreground">No tables.</p>}
        {form.tables.map((t, i) => (
          <div key={i} className="space-y-2 rounded-lg border p-3">
            <div className="grid gap-2 sm:grid-cols-[9rem_1fr]">
              <div>
                <Label htmlFor={id(`table-page-${i}`)}>Table {i + 1} is on page</Label>
                <select
                  id={id(`table-page-${i}`)}
                  className={SELECT_CLASS}
                  value={t.page}
                  onChange={(e) => update({ tables: form.tables.map((x, j) => (j === i ? { ...x, page: Number(e.target.value) } : x)) })}
                  {...fieldProps(`tables[${i}].page`)}
                >
                  {Array.from({ length: pageCount }, (_, p) => p + 1).map((p) => (
                    <option key={p} value={p}>
                      {p}
                    </option>
                  ))}
                </select>
              </div>
              <div>
                <Label htmlFor={id(`table-md-${i}`)}>Table {i + 1} (one row per line, columns separated by |)</Label>
                <Textarea
                  id={id(`table-md-${i}`)}
                  rows={4}
                  className="font-mono text-xs"
                  value={t.markdown}
                  onChange={(e) => update({ tables: form.tables.map((x, j) => (j === i ? { ...x, markdown: e.target.value } : x)) })}
                  {...fieldProps(`tables[${i}].markdown`)}
                />
                {errorText(`tables[${i}].markdown`)}
              </div>
            </div>
            <Button type="button" variant="ghost" size="sm" disabled={disabled} onClick={() => update({ tables: form.tables.filter((_, j) => j !== i) })}>
              <Trash2 className="size-4" aria-hidden="true" />
              Remove table {i + 1}
            </Button>
          </div>
        ))}
        {form.tables.length < LIMITS.tables && (
          <Button type="button" variant="outline" size="sm" disabled={disabled} onClick={() => update({ tables: [...form.tables, { page: sourcePages[0] ?? 1, markdown: "" }] })}>
            <Plus className="size-4" aria-hidden="true" />
            Add table
          </Button>
        )}
      </fieldset>

      <fieldset className="space-y-3">
        <legend className="text-sm font-medium">Diagrams and figures</legend>
        <p className="text-xs text-muted-foreground">
          Pictures are not copied into the exam. What a candidate gets is your description of the figure, so it must be
          accurate: it will be read aloud.
        </p>
        {form.diagrams.length === 0 && <p className="text-xs text-muted-foreground">No diagrams.</p>}
        {form.diagrams.map((d, i) => {
          const saved = question.diagrams[i];
          const fromReader = saved?.descriptionSource === "MODEL" && saved.description === d.description;
          return (
            <div key={i} className="space-y-2 rounded-lg border p-3">
              <div className="grid gap-2 sm:grid-cols-[9rem_1fr]">
                <div>
                  <Label htmlFor={id(`dg-page-${i}`)}>Diagram {i + 1} is on page</Label>
                  <select
                    id={id(`dg-page-${i}`)}
                    className={SELECT_CLASS}
                    value={d.page}
                    onChange={(e) => update({ diagrams: form.diagrams.map((x, j) => (j === i ? { ...x, page: Number(e.target.value) } : x)) })}
                    {...fieldProps(`diagrams[${i}].page`)}
                  >
                    {Array.from({ length: pageCount }, (_, p) => p + 1).map((p) => (
                      <option key={p} value={p}>
                        {p}
                      </option>
                    ))}
                  </select>
                </div>
                <div>
                  <Label htmlFor={id(`dg-region-${i}`)}>Where on the page (optional)</Label>
                  <Input id={id(`dg-region-${i}`)} value={d.region} onChange={(e) => update({ diagrams: form.diagrams.map((x, j) => (j === i ? { ...x, region: e.target.value } : x)) })} {...fieldProps(`diagrams[${i}].region`)} />
                </div>
              </div>
              <div>
                <Label htmlFor={id(`dg-desc-${i}`)}>Description of diagram {i + 1}</Label>
                <Textarea
                  id={id(`dg-desc-${i}`)}
                  rows={3}
                  value={d.description}
                  onChange={(e) => update({ diagrams: form.diagrams.map((x, j) => (j === i ? { ...x, description: e.target.value, confirmed: false } : x)) })}
                  {...fieldProps(`diagrams[${i}].description`)}
                />
                {errorText(`diagrams[${i}].description`)}
                {fromReader && (
                  <p className="mt-1 text-xs text-muted-foreground">This description was written by the reader from what it could see. Check it against the figure.</p>
                )}
              </div>
              <label className="flex items-start gap-2 text-sm">
                <input
                  type="checkbox"
                  className="mt-1 size-4"
                  data-field={`diagrams[${i}].confirmed`}
                  checked={d.confirmed}
                  disabled={disabled || d.description.trim() === ""}
                  onChange={(e) => update({ diagrams: form.diagrams.map((x, j) => (j === i ? { ...x, confirmed: e.target.checked } : x)) })}
                />
                <span>I have checked this description against the original figure.</span>
              </label>
              {errorText(`diagrams[${i}].confirmed`)}
              <Button type="button" variant="ghost" size="sm" disabled={disabled} onClick={() => update({ diagrams: form.diagrams.filter((_, j) => j !== i) })}>
                <Trash2 className="size-4" aria-hidden="true" />
                Remove diagram {i + 1}
              </Button>
            </div>
          );
        })}
        {form.diagrams.length < LIMITS.diagrams && (
          <Button type="button" variant="outline" size="sm" disabled={disabled} onClick={() => update({ diagrams: [...form.diagrams, { page: sourcePages[0] ?? 1, region: "", description: "", confirmed: false }] })}>
            <Plus className="size-4" aria-hidden="true" />
            Add diagram
          </Button>
        )}
      </fieldset>

      {failure && Object.keys(failure.fields).length === 0 && (
        <div role="alert" className="flex items-start gap-2 rounded-lg border border-destructive/30 bg-destructive/10 p-3 text-sm text-destructive">
          <AlertCircle className="mt-0.5 size-4 shrink-0" aria-hidden="true" />
          <div className="space-y-2">
            <p>{failure.message}</p>
            <p className="text-xs">Your changes are still here. Nothing was lost.</p>
          </div>
        </div>
      )}
      {failure && Object.keys(failure.fields).length > 0 && (
        <p role="alert" className="text-sm text-destructive">
          The server could not accept these changes. See the highlighted fields.
        </p>
      )}

      {!readOnly && (
        <div className="flex flex-wrap items-center gap-2 border-t pt-4">
          <Button type="submit" disabled={saving || locked || (!isNew && !dirty)} aria-busy={saving}>
            {saving && <Loader2 className="size-4 animate-spin" aria-hidden="true" />}
            {isNew ? "Add question" : saving ? "Saving…" : failure?.retryable ? "Try saving again" : "Save changes"}
          </Button>
          {(dirty || isNew) && (
            <Button type="button" variant="outline" onClick={discard} disabled={saving}>
              {isNew ? "Cancel" : "Discard changes"}
            </Button>
          )}
          {dirty && !saving && <span className="text-xs text-muted-foreground">You have unsaved changes.</span>}
          {locked && <span className="text-xs text-muted-foreground">Reload the draft to keep editing.</span>}
        </div>
      )}
    </form>
  );
}
