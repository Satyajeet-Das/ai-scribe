"use client";

import Link from "next/link";
import { useCallback, useEffect, useRef, useState } from "react";
import { AlertCircle, CheckCircle2, FileWarning, Info, Loader2, Plus, RefreshCw } from "lucide-react";
import { Button } from "@/components/ui/button";
import { Card } from "@/components/ui/card";
import { useExamDraft } from "@/hooks/use-exam-draft";
import { type DocumentError } from "@/lib/document-errors";
import { attentionFor, blankForm, questionLabel } from "@/lib/draft-form";
import { examDocumentsApi } from "@/services/exam-documents-api";
import type { Exam } from "@/types/exam-types";
import type { ApproveResult, DraftQuestion } from "@/types/exam-document";
import { ConfirmDialog } from "./confirm-dialog";
import { QuestionCard, type FocusTarget } from "./question-card";
import { QuestionEditor } from "./question-editor";

interface DraftWorkspaceProps {
  examId: string;
  documentId: string;
  /** The exam the questions would go into; approval needs it to still be a draft. */
  exam: Exam | undefined;
}

/** A question that does not exist yet, for the "add a question" editor. */
function blankQuestion(position: number): DraftQuestion {
  const f = blankForm();
  return {
    id: "new", position, numberLabel: f.numberLabel, type: f.type, instructions: "", text: "", points: null,
    options: [], tables: [], diagrams: [], sourcePages: [], confidence: "HIGH", warnings: [], origin: "TEACHER",
    edited: false, createdAt: "", updatedAt: "",
  };
}

export function DraftWorkspace({ examId, documentId, exam }: DraftWorkspaceProps) {
  const store = useExamDraft(examId, documentId);
  const { draft } = store;

  const [expanded, setExpanded] = useState<Set<string>>(new Set());
  const [dirty, setDirty] = useState<Set<string>>(new Set());
  const [onlyAttention, setOnlyAttention] = useState(false);
  const [adding, setAdding] = useState(false);
  const [addPosition, setAddPosition] = useState<number | null>(null);
  const [deleteTarget, setDeleteTarget] = useState<DraftQuestion | null>(null);
  const [deleting, setDeleting] = useState(false);
  const [approveOpen, setApproveOpen] = useState(false);
  const [approving, setApproving] = useState(false);
  const [approved, setApproved] = useState<ApproveResult | null>(null);
  const [problem, setProblem] = useState<DocumentError | null>(null);
  const [announcement, setAnnouncement] = useState("");
  const [focusReq, setFocusReq] = useState<{ id: string; target: FocusTarget; n: number } | null>(null);

  const approvingRef = useRef(false);
  const addButton = useRef<HTMLButtonElement>(null);
  const successHeading = useRef<HTMLHeadingElement>(null);
  const nonce = useRef(0);

  const say = useCallback((text: string) => setAnnouncement(text), []);
  const focusOn = (id: string, target: FocusTarget) => setFocusReq({ id, target, n: ++nonce.current });

  const setDirtyFor = useCallback((id: string, isDirty: boolean) => {
    setDirty((prev) => {
      if (prev.has(id) === isDirty) return prev; // no change: no re-render
      const next = new Set(prev);
      if (isDirty) next.add(id);
      else next.delete(id);
      return next;
    });
  }, []);

  // Warn before the tab is closed with changes that are not saved.
  const hasUnsaved = dirty.size > 0;
  useEffect(() => {
    if (!hasUnsaved) return;
    const warn = (e: BeforeUnloadEvent) => {
      e.preventDefault();
    };
    window.addEventListener("beforeunload", warn);
    return () => window.removeEventListener("beforeunload", warn);
  }, [hasUnsaved]);

  // ---- the original PDF, fetched once through the API and held in memory ----
  const pdfUrl = useRef<Promise<string> | null>(null);
  const createdUrl = useRef<string | null>(null);
  useEffect(
    () => () => {
      if (createdUrl.current) URL.revokeObjectURL(createdUrl.current);
    },
    []
  );
  const openSource = useCallback(
    async (page: number) => {
      // Open the tab now, while the click still counts as a user action, and point it at the PDF when it arrives.
      const tab = window.open("about:blank", "_blank");
      if (tab) tab.opener = null;
      try {
        pdfUrl.current ??= examDocumentsApi.getFile(examId, documentId).then((blob) => {
          const url = URL.createObjectURL(new Blob([blob], { type: "application/pdf" }));
          createdUrl.current = url;
          return url;
        });
        const url = await pdfUrl.current;
        if (tab) {
          tab.location.href = `${url}#page=${page}`;
          say(`Opened page ${page} of the original PDF in a new tab.`);
        } else {
          say("Your browser blocked the new tab. Allow pop-ups for this site to view the original PDF.");
        }
      } catch {
        pdfUrl.current = null;
        tab?.close();
        say("The original PDF could not be opened right now. Try again, or use Download PDF on the exam page.");
      }
    },
    [documentId, examId, say]
  );

  // ---- derived state ----
  const questions = draft?.questions ?? [];
  const readOnly = draft?.document.status === "COMPLETED" || approved !== null;
  const attentionTotal = questions.filter((q) => attentionFor(q).length > 0).length;
  const dirtyCount = questions.filter((q) => dirty.has(q.id)).length + (adding ? 1 : 0);
  const shown = onlyAttention ? questions.filter((q) => attentionFor(q).length > 0) : questions;
  const pageCount = draft?.document.pageCount ?? 1;
  const examIsDraft = exam?.status === "DRAFT";

  const blockers: string[] = [];
  if (readOnly) blockers.push("This draft has already been approved.");
  else {
    if (!exam) blockers.push("Loading the exam…");
    else if (!examIsDraft) blockers.push(`The exam is ${exam.status.toLowerCase()}, so questions can't be added. Revert it to draft first.`);
    if (questions.length === 0) blockers.push("There are no questions to add.");
    if (attentionTotal > 0) blockers.push(`${attentionTotal} ${attentionTotal === 1 ? "question needs" : "questions need"} attention.`);
    if (dirtyCount > 0) blockers.push("Save or discard your unsaved changes.");
    if (store.stale) blockers.push("Reload the draft first: it was changed somewhere else.");
  }
  const canApprove = blockers.length === 0 && store.pending === 0;

  // ---- actions ----
  function toggle(q: DraftQuestion) {
    const open = expanded.has(q.id);
    if (open && dirty.has(q.id)) {
      say(`Question ${questionLabel(q)} has unsaved changes. Save or discard them before closing.`);
      return;
    }
    setExpanded((prev) => {
      const next = new Set(prev);
      if (open) next.delete(q.id);
      else next.add(q.id);
      return next;
    });
  }

  async function move(q: DraftQuestion, delta: -1 | 1) {
    const res = await store.move(q.id, delta);
    if (res.ok) {
      if (res.value.from !== res.value.to) {
        say(`Question ${questionLabel(q)} moved ${delta < 0 ? "up" : "down"} to position ${res.value.to} of ${res.value.total}.`);
      }
      focusOn(q.id, delta < 0 ? "move-up" : "move-down");
    } else {
      setProblem(res.error);
    }
  }

  async function confirmDelete() {
    if (!deleteTarget || deleting) return;
    const target = deleteTarget;
    const index = questions.findIndex((q) => q.id === target.id);
    setDeleting(true);
    const res = await store.remove(target.id);
    setDeleting(false);
    setDeleteTarget(null);
    if (!res.ok) {
      setProblem(res.error);
      return;
    }
    setDirtyFor(target.id, false);
    const remaining = questions.filter((q) => q.id !== target.id);
    say(`Question ${questionLabel(target)} deleted from the draft. ${remaining.length} ${remaining.length === 1 ? "question" : "questions"} left.`);
    const neighbour = remaining[index] ?? remaining[index - 1];
    if (neighbour) focusOn(neighbour.id, "heading");
    else addButton.current?.focus();
  }

  async function saveQuestion(q: DraftQuestion, request: Parameters<typeof store.replace>[1]) {
    setProblem(null);
    return store.replace(q.id, request);
  }

  async function addQuestion(request: Parameters<typeof store.add>[0]) {
    setProblem(null);
    const res = await store.add(request, addPosition ?? undefined);
    if (res.ok) {
      setAdding(false);
      setAddPosition(null);
      say(`New question added at position ${res.value.position}.`);
      focusOn(res.value.id, "heading");
    }
    return res;
  }

  async function approve() {
    if (approvingRef.current) return;
    approvingRef.current = true;
    setApproving(true);
    setProblem(null);
    const res = await store.approve();
    approvingRef.current = false;
    setApproving(false);
    setApproveOpen(false);
    if (res.ok) {
      setApproved(res.value);
      say(`${res.value.questions.length} questions were added to the exam.`);
      requestAnimationFrame(() => successHeading.current?.focus());
    } else {
      setProblem(res.error);
      // The server may already have it (approved in another tab): look again.
      if (res.error.code === "ALREADY_APPROVED") void store.reload();
    }
  }

  // ---- states before there is a draft ----
  if (store.loading) {
    return (
      <p role="status" className="flex items-center gap-2 text-sm text-muted-foreground">
        <Loader2 className="size-4 animate-spin" aria-hidden="true" />
        Loading the extracted questions…
      </p>
    );
  }
  if (!draft) {
    return (
      <Card className="space-y-3 p-6" role="alert">
        <p className="flex items-center gap-2 font-medium text-destructive">
          <AlertCircle className="size-5" aria-hidden="true" />
          {store.loadError?.message ?? "The draft could not be loaded."}
        </p>
        <div>
          <Button variant="outline" onClick={() => void store.reload()}>
            <RefreshCw className="size-4" aria-hidden="true" />
            Try again
          </Button>
        </div>
      </Card>
    );
  }

  const unreadPages = (draft.document.pages ?? []).filter((p) => p.note || p.imagesSkipped > 0);
  const incompleteReading = unreadPages.length > 0 || draft.unassigned.length > 0;

  return (
    <div className="space-y-6">
      <div role="status" aria-live="polite" className="sr-only" data-testid="workspace-announcer">
        {announcement}
      </div>

      {approved ? (
        <Card className="space-y-2 border-emerald-300 bg-emerald-50 p-5 dark:border-emerald-800 dark:bg-emerald-950/40" role="region" aria-labelledby="approved-heading">
          <h2 id="approved-heading" ref={successHeading} tabIndex={-1} className="flex items-center gap-2 text-lg font-semibold outline-none">
            <CheckCircle2 className="size-5 text-emerald-600" aria-hidden="true" />
            {approved.questions.length} {approved.questions.length === 1 ? "question was" : "questions were"} added to the exam
          </h2>
          <p className="text-sm">
            They are at the end of “{exam?.title ?? "the exam"}”, in the order you reviewed them. The exam is still a draft: nothing
            has been published or assigned. Check the questions in the question studio, then publish the exam when you are ready.
          </p>
          <Link
            href={`/exams/${encodeURIComponent(examId)}`}
            className="inline-flex h-8 items-center rounded-lg bg-primary px-3 text-sm font-medium text-primary-foreground hover:bg-primary/80 focus-visible:outline-none focus-visible:ring-[3px] focus-visible:ring-ring/50"
          >
            Go to the exam
          </Link>
        </Card>
      ) : readOnly ? (
        <Card className="space-y-1 p-4" role="note">
          <p className="text-sm font-medium">This draft was approved{draft.document.approvedAt ? ` on ${new Date(draft.document.approvedAt).toLocaleDateString()}` : ""}.</p>
          <p className="text-sm text-muted-foreground">
            It is shown here for reference and can no longer be changed. To edit those questions, use the question studio on the exam.
          </p>
        </Card>
      ) : (
        <Card className="flex items-start gap-3 border-amber-300 bg-amber-50 p-4 dark:border-amber-800 dark:bg-amber-950/40" role="note">
          <FileWarning className="mt-0.5 size-5 shrink-0 text-amber-700 dark:text-amber-300" aria-hidden="true" />
          <div className="space-y-1 text-sm">
            <p className="font-semibold">This is a draft for you to check, not part of the exam.</p>
            <p>
              The questions below were read from your PDF by software, which can make mistakes. Nothing is added to the exam until you approve
              the draft, and nothing is published.
            </p>
          </div>
        </Card>
      )}

      {store.stale && (
        <div role="alert" className="flex flex-wrap items-center justify-between gap-3 rounded-lg border border-amber-300 bg-amber-50 p-3 text-sm text-amber-900 dark:bg-amber-950 dark:text-amber-200">
          <p>This draft was changed somewhere else (perhaps in another tab). Reload it before making more changes. Anything you have typed stays in the boxes.</p>
          <Button variant="outline" size="sm" onClick={() => void store.reload()}>
            <RefreshCw className="size-4" aria-hidden="true" />
            Reload the draft
          </Button>
        </div>
      )}

      {problem && (
        <div role="alert" className="space-y-2 rounded-lg border border-destructive/30 bg-destructive/10 p-3 text-sm text-destructive">
          <div className="flex items-start justify-between gap-3">
            <p className="flex items-start gap-2">
              <AlertCircle className="mt-0.5 size-4 shrink-0" aria-hidden="true" />
              {problem.message}
            </p>
            <button type="button" className="text-xs underline" onClick={() => setProblem(null)}>
              Dismiss
            </button>
          </div>
          {Object.keys(problem.fields).length > 0 && (
            <ul className="list-disc space-y-0.5 pl-9">
              {Object.entries(problem.fields).map(([field, message]) => (
                <li key={field}>
                  {humaniseField(field)}: {message}
                </li>
              ))}
            </ul>
          )}
        </div>
      )}

      {draft.documentInstructions && (
        <section aria-labelledby="doc-instructions-heading" className="rounded-lg border p-4">
          <h2 id="doc-instructions-heading" className="flex items-center gap-1.5 text-sm font-semibold">
            <Info className="size-4" aria-hidden="true" />
            Instructions for the whole paper
          </h2>
          <p className="mt-1 whitespace-pre-line text-sm">{draft.documentInstructions}</p>
          <p className="mt-2 text-xs text-muted-foreground">
            These are shown for your reference and are not added to the exam. Copy any you need into the exam description or a question&apos;s instructions.
          </p>
        </section>
      )}

      {incompleteReading && (
        <section aria-labelledby="unread-heading" className="space-y-2 rounded-lg border border-amber-300 bg-amber-50 p-4 text-sm text-amber-900 dark:border-amber-800 dark:bg-amber-950/40 dark:text-amber-200">
          <h2 id="unread-heading" className="flex items-center gap-1.5 font-semibold">
            <FileWarning className="size-4" aria-hidden="true" />
            Parts of the paper that may be missing
          </h2>
          <p>Compare this draft with the original, and add anything that is missing.</p>
          <ul className="list-disc space-y-1 pl-5">
            {unreadPages.map((p) => (
              <li key={`page-${p.number}`}>
                Page {p.number}: {p.note ?? `${p.imagesSkipped} ${p.imagesSkipped === 1 ? "picture" : "pictures"} could not be read.`}
              </li>
            ))}
            {draft.unassigned.map((u, i) => (
              <li key={`un-${i}`}>
                Page {u.page}: {u.reason}
                {u.text && <span className="block whitespace-pre-line text-xs opacity-80">“{u.text.length > 300 ? `${u.text.slice(0, 300)}…` : u.text}”</span>}
              </li>
            ))}
          </ul>
        </section>
      )}

      <section aria-labelledby="questions-heading" className="space-y-4">
        <div className="flex flex-wrap items-center justify-between gap-3">
          <div>
            <h2 id="questions-heading" className="text-lg font-semibold">
              Questions ({questions.length})
            </h2>
            {!readOnly && (
              <p className="text-sm text-muted-foreground" data-testid="attention-summary">
                {attentionTotal === 0
                  ? questions.length === 0 ? "No questions yet." : "Every question is ready for approval."
                  : `${attentionTotal} of ${questions.length} ${attentionTotal === 1 ? "needs" : "need"} attention.`}
              </p>
            )}
          </div>
          <div className="flex flex-wrap items-center gap-3">
            {!readOnly && attentionTotal > 0 && (
              <label className="flex items-center gap-2 text-sm">
                <input type="checkbox" className="size-4" checked={onlyAttention} onChange={(e) => setOnlyAttention(e.target.checked)} />
                Show only questions that need attention
              </label>
            )}
            {!readOnly && (
              <Button
                ref={addButton}
                variant="outline"
                size="sm"
                disabled={adding || store.stale}
                onClick={() => {
                  setAdding(true);
                  setAddPosition(questions.length + 1);
                }}
              >
                <Plus className="size-4" aria-hidden="true" />
                Add a question
              </Button>
            )}
          </div>
        </div>

        {adding && !readOnly && (
          <Card className="space-y-4 p-4" role="group" aria-label="New question">
            <h3 className="text-sm font-semibold">Add a question the reader missed</h3>
            <div className="max-w-xs">
              <label htmlFor="add-position" className="text-sm font-medium">
                Put it at position
              </label>
              <select
                id="add-position"
                className="mt-1 h-8 w-full rounded-lg border border-input bg-transparent px-2 text-sm"
                value={addPosition ?? questions.length + 1}
                onChange={(e) => setAddPosition(Number(e.target.value))}
              >
                {Array.from({ length: questions.length + 1 }, (_, i) => i + 1).map((p) => (
                  <option key={p} value={p}>
                    {p}
                    {p === questions.length + 1 ? " (last)" : p === 1 ? " (first)" : ""}
                  </option>
                ))}
              </select>
            </div>
            <QuestionEditor
              documentId={documentId}
              question={blankQuestion(questions.length + 1)}
              pageCount={pageCount}
              isNew
              locked={store.stale}
              onSave={(request) => addQuestion(request)}
              onCancel={() => {
                setAdding(false);
                setAddPosition(null);
                addButton.current?.focus();
              }}
              onOpenSource={openSource}
            />
          </Card>
        )}

        {shown.length === 0 && !adding ? (
          <p className="rounded-lg border border-dashed p-6 text-center text-sm text-muted-foreground">
            {onlyAttention ? "Nothing needs attention." : "There are no questions in this draft. Add the ones the reader missed."}
          </p>
        ) : (
          <ol className="space-y-3" aria-label="Draft questions">
            {shown.map((q) => (
              <QuestionCard
                key={q.id}
                documentId={documentId}
                question={q}
                index={questions.findIndex((x) => x.id === q.id)}
                total={questions.length}
                pageCount={pageCount}
                expanded={expanded.has(q.id)}
                readOnly={readOnly}
                locked={store.stale}
                focus={focusReq?.id === q.id ? focusReq.target : null}
                focusNonce={focusReq?.n ?? 0}
                onToggle={() => toggle(q)}
                onSave={(request) => saveQuestion(q, request)}
                onSaved={() => {
                  say(`Question ${questionLabel(q)} saved.`);
                  focusOn(q.id, "heading");
                }}
                onMove={(delta) => void move(q, delta)}
                onDelete={() => setDeleteTarget(q)}
                onDirtyChange={(d) => setDirtyFor(q.id, d)}
                onOpenSource={openSource}
              />
            ))}
          </ol>
        )}
      </section>

      {!readOnly && (
        <section aria-labelledby="approve-heading" className="space-y-3 rounded-lg border p-4">
          <h2 id="approve-heading" className="text-lg font-semibold">
            Add these questions to the exam
          </h2>
          <p className="text-sm text-muted-foreground">
            When every question is correct, approve the draft. That copies the questions into{" "}
            <strong>{exam ? `“${exam.title}”` : "the exam"}</strong>. It does not publish the exam.
          </p>
          {blockers.length > 0 && (
            <ul id="approve-blockers" className="list-disc space-y-0.5 pl-5 text-sm">
              {blockers.map((b) => (
                <li key={b}>{b}</li>
              ))}
            </ul>
          )}
          <Button onClick={() => setApproveOpen(true)} disabled={!canApprove || approving} aria-describedby={blockers.length > 0 ? "approve-blockers" : undefined}>
            <CheckCircle2 className="size-4" aria-hidden="true" />
            Approve and add to exam…
          </Button>
        </section>
      )}

      <ConfirmDialog
        open={approveOpen}
        onOpenChange={setApproveOpen}
        title={`Add ${questions.length} ${questions.length === 1 ? "question" : "questions"} to the exam?`}
        description={
          <ul className="list-disc space-y-1.5 pl-5">
            <li>
              The questions are added to the end of <strong>“{exam?.title}”</strong>
              {typeof exam?.questions === "number" ? `, after its ${exam.questions} existing ${exam.questions === 1 ? "question" : "questions"}` : ""}, in the order shown here.
            </li>
            <li>The exam stays a draft. Nothing is published and no student is assigned.</li>
            <li>You can still edit the questions afterwards in the question studio, but not in this draft.</li>
          </ul>
        }
        confirmLabel="Add questions to exam"
        busy={approving}
        onConfirm={() => void approve()}
      />
      <ConfirmDialog
        open={deleteTarget !== null}
        onOpenChange={(open) => !open && !deleting && setDeleteTarget(null)}
        title={`Delete question ${deleteTarget ? questionLabel(deleteTarget) : ""}?`}
        description={<p>It is removed from this draft only. It cannot be undone, but you can add a question again by hand.</p>}
        confirmLabel="Delete question"
        destructive
        busy={deleting}
        onConfirm={() => void confirmDelete()}
      />
    </div>
  );
}

/** "questions[2].points" becomes "Question 2, marks" for a person to read. */
function humaniseField(field: string): string {
  const m = /^questions\[(\d+)\]\.(.*)$/.exec(field);
  if (!m) return field;
  const what: Record<string, string> = {
    points: "marks", text: "question text", options: "options", type: "type",
  };
  const rest = what[m[2]] ?? m[2].replace(/^warnings\./, "warning ").replace(/^diagrams\[(\d+)\]\./, "diagram $1 ");
  return `Question ${m[1]}, ${rest}`;
}

