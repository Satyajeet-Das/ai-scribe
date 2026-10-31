"use client";

import { useEffect, useRef } from "react";
import { AlertTriangle, ArrowDown, ArrowUp, ChevronDown, ChevronUp, Trash2 } from "lucide-react";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { attentionFor, questionLabel, type QuestionForm } from "@/lib/draft-form";
import type { OpResult } from "@/hooks/use-exam-draft";
import type { DraftQuestion, DraftQuestionRequest } from "@/types/exam-document";
import { QuestionEditor } from "./question-editor";

/** What the workspace asks a card to focus after something changed. */
export type FocusTarget = "heading" | "move-up" | "move-down";

interface QuestionCardProps {
  documentId: string;
  question: DraftQuestion;
  index: number;
  total: number;
  pageCount: number;
  expanded: boolean;
  readOnly: boolean;
  /** The draft is out of date, or another change is being made. */
  locked: boolean;
  focus: FocusTarget | null;
  /** Changes on every focus request, so asking for the same target twice still moves focus. */
  focusNonce: number;
  onToggle: () => void;
  onSave: (request: DraftQuestionRequest, form: QuestionForm) => Promise<OpResult<unknown>>;
  onSaved: () => void;
  onMove: (delta: -1 | 1) => void;
  onDelete: () => void;
  onDirtyChange: (dirty: boolean) => void;
  onOpenSource: (page: number) => void;
}

const TYPE_LABEL = { MCQ: "Multiple choice", ESSAY: "Written answer" } as const;

/**
 * One question in the draft. Collapsed it shows what matters at a glance (its
 * number, type, marks and what is left to do); expanded it is the editor.
 * Moving a question is done with buttons, never drag-and-drop alone, so it
 * works from the keyboard and with a screen reader.
 */
export function QuestionCard({
  documentId,
  question,
  index,
  total,
  pageCount,
  expanded,
  readOnly,
  locked,
  focus,
  focusNonce,
  onToggle,
  onSave,
  onSaved,
  onMove,
  onDelete,
  onDirtyChange,
  onOpenSource,
}: QuestionCardProps) {
  const heading = useRef<HTMLHeadingElement>(null);
  const up = useRef<HTMLButtonElement>(null);
  const down = useRef<HTMLButtonElement>(null);
  const label = questionLabel(question);
  const attention = attentionFor(question);
  const editorId = `editor-${question.id}`;

  // Put the keyboard where the teacher would expect after an action.
  useEffect(() => {
    if (!focus) return;
    const target = focus === "move-up" ? up.current : focus === "move-down" ? down.current : heading.current;
    // A button that has become disabled (the question reached an end) cannot take focus.
    const usable = target instanceof HTMLButtonElement && target.disabled ? null : target;
    (usable ?? heading.current)?.focus();
  }, [focus, focusNonce]);

  return (
    <li id={`question-${question.id}`} className="rounded-lg border bg-card p-4" data-testid={`question-${question.id}`}>
      <div className="flex flex-wrap items-start justify-between gap-3">
        <div className="min-w-0 space-y-1">
          <h3 ref={heading} tabIndex={-1} className="flex flex-wrap items-center gap-2 text-sm font-semibold outline-none focus-visible:ring-3 focus-visible:ring-ring/50">
            <span>Question {label}</span>
            <Badge variant="outline">{TYPE_LABEL[question.type]}</Badge>
            <Badge variant="outline">{question.points === null ? "Marks not set" : `${question.points} ${question.points === 1 ? "mark" : "marks"}`}</Badge>
            {question.origin === "TEACHER" && <Badge variant="secondary">Added by you</Badge>}
          </h3>
          <p className="text-xs text-muted-foreground">
            Position {index + 1} of {total}
            {question.sourcePages.length > 0 && <> · page {question.sourcePages.join(", ")} of the paper</>}
          </p>
        </div>
        <div className="flex flex-wrap items-center gap-1">
          <Button
            ref={up}
            type="button"
            variant="outline"
            size="icon-sm"
            disabled={readOnly || locked || index === 0}
            onClick={() => onMove(-1)}
            aria-label={`Move question ${label} up`}
          >
            <ArrowUp className="size-4" aria-hidden="true" />
          </Button>
          <Button
            ref={down}
            type="button"
            variant="outline"
            size="icon-sm"
            disabled={readOnly || locked || index === total - 1}
            onClick={() => onMove(1)}
            aria-label={`Move question ${label} down`}
          >
            <ArrowDown className="size-4" aria-hidden="true" />
          </Button>
          {!readOnly && (
            <Button type="button" variant="ghost" size="icon-sm" disabled={locked} onClick={onDelete} aria-label={`Delete question ${label}`} className="text-destructive hover:text-destructive">
              <Trash2 className="size-4" aria-hidden="true" />
            </Button>
          )}
        </div>
      </div>

      {!expanded && (
        <p className="mt-2 line-clamp-3 whitespace-pre-line text-sm" data-testid="question-preview">
          {question.text || <span className="italic text-muted-foreground">No text</span>}
        </p>
      )}

      {!readOnly && attention.length > 0 && (
        <div className="mt-3 rounded-md border border-amber-300 bg-amber-50 p-2.5 text-sm text-amber-900 dark:border-amber-800 dark:bg-amber-950 dark:text-amber-200">
          <p className="flex items-center gap-1.5 font-semibold">
            <AlertTriangle className="size-4" aria-hidden="true" />
            Needs attention
            <span className="font-normal"> ({attention.length} {attention.length === 1 ? "thing" : "things"} to fix before approval)</span>
          </p>
          <ul className="mt-1 list-disc space-y-0.5 pl-5">
            {attention.map((a) => (
              <li key={a.key}>{a.message}</li>
            ))}
          </ul>
        </div>
      )}

      <div className="mt-3">
        <Button
          type="button"
          variant="outline"
          size="sm"
          onClick={onToggle}
          aria-expanded={expanded}
          aria-controls={editorId}
          aria-label={`${expanded ? "Close" : readOnly ? "View" : "Edit"} question ${label}`}
        >
          {expanded ? <ChevronUp className="size-4" aria-hidden="true" /> : <ChevronDown className="size-4" aria-hidden="true" />}
          {expanded ? "Close" : readOnly ? "View" : "Edit"}
        </Button>
      </div>

      {expanded && (
        <div id={editorId} className="mt-4 border-t pt-4">
          <QuestionEditor
            documentId={documentId}
            question={question}
            pageCount={pageCount}
            readOnly={readOnly}
            locked={locked}
            onSave={onSave}
            onSaved={onSaved}
            onDirtyChange={onDirtyChange}
            onOpenSource={onOpenSource}
          />
        </div>
      )}
    </li>
  );
}
