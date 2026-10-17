"use client";

import { useState } from "react";
import { Loader2, Lock, LockOpen } from "lucide-react";
import {
  AlertDialog,
  AlertDialogAction,
  AlertDialogCancel,
  AlertDialogContent,
  AlertDialogDescription,
  AlertDialogFooter,
  AlertDialogHeader,
  AlertDialogTitle,
} from "@/components/ui/alert-dialog";
import { Button } from "@/components/ui/button";
import { Label } from "@/components/ui/label";
import { Textarea } from "@/components/ui/textarea";
import { formatDateTime } from "@/components/response-review/status-badge";
import { formatMarks, type StudentMarking } from "@/hooks/use-student-marking";

const MIN_REASON = 3;
const MAX_REASON = 500;

/**
 * Finishing and reopening a paper. Finalising needs every question marked and
 * nothing unsaved; the buttons say why when they cannot be used.
 */
export function PaperActions({ marking }: { marking: StudentMarking }) {
  const sheet = marking.sheet;
  if (!sheet || !sheet.markingAllowed) return null;
  return sheet.paper.status === "FINAL" ? (
    <ReopenAction marking={marking} />
  ) : (
    <FinaliseAction marking={marking} />
  );
}

function FinaliseAction({ marking }: { marking: StudentMarking }) {
  const sheet = marking.sheet!;
  const [open, setOpen] = useState(false);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<string | null>(null);

  const unmarked = marking.unmarkedNumbers;
  const unsaved = marking.anyDirty || marking.anySaving;
  const blocked = unmarked.length > 0 ? "unmarked" : unsaved ? "unsaved" : null;
  const why =
    blocked === "unmarked"
      ? `Every question needs a mark first. Not marked: ${unmarked.join(", ")}.`
      : blocked === "unsaved"
        ? "Save your changes first."
        : null;

  const confirm = async () => {
    setBusy(true);
    setError(null);
    const result = await marking.finalise();
    setBusy(false);
    if (result.ok) setOpen(false);
    else setError(result.message);
  };

  return (
    <div className="ml-auto flex flex-col items-end gap-1">
      <Button
        type="button"
        onClick={() => {
          setError(null);
          setOpen(true);
        }}
        disabled={blocked !== null}
        aria-describedby={why ? "finalise-why" : undefined}
        className="gap-1.5"
      >
        <Lock className="size-4" aria-hidden="true" />
        Finalise paper
      </Button>
      {why && (
        <p id="finalise-why" className="max-w-xs text-right text-xs text-muted-foreground">
          {why}
        </p>
      )}

      <AlertDialog open={open} onOpenChange={(next) => !busy && setOpen(next)}>
        <AlertDialogContent className="max-w-md bg-background p-6">
          <AlertDialogHeader>
            <AlertDialogTitle className="text-xl font-bold">Finalise this paper?</AlertDialogTitle>
            <AlertDialogDescription className="pt-2 text-sm text-muted-foreground">
              Total{" "}
              <strong className="text-foreground">
                {formatMarks(sheet.paper.totalMarks)} / {formatMarks(sheet.paper.maxMarks)}
              </strong>{" "}
              across {sheet.paper.totalQuestions} questions. The marks are locked until the paper is
              reopened, and the reopening is recorded.
            </AlertDialogDescription>
          </AlertDialogHeader>
          {error && (
            <p role="alert" className="text-sm text-destructive">
              {error}
            </p>
          )}
          <AlertDialogFooter className="mt-4 flex flex-col-reverse gap-2 sm:flex-row">
            <AlertDialogCancel disabled={busy}>Keep marking</AlertDialogCancel>
            <AlertDialogAction
              onClick={(e) => {
                e.preventDefault();
                void confirm();
              }}
              disabled={busy}
              className="gap-2 font-semibold"
            >
              {busy && <Loader2 className="size-4 motion-safe:animate-spin" aria-hidden="true" />}
              Finalise
            </AlertDialogAction>
          </AlertDialogFooter>
        </AlertDialogContent>
      </AlertDialog>
    </div>
  );
}

function ReopenAction({ marking }: { marking: StudentMarking }) {
  const sheet = marking.sheet!;
  const [open, setOpen] = useState(false);
  const [busy, setBusy] = useState(false);
  const [reason, setReason] = useState("");
  const [error, setError] = useState<string | null>(null);

  const trimmed = reason.trim();
  const valid = trimmed.length >= MIN_REASON && trimmed.length <= MAX_REASON;
  const finalisedAt = formatDateTime(sheet.paper.finalisedAt);

  const confirm = async () => {
    if (!valid) return;
    setBusy(true);
    setError(null);
    const result = await marking.reopen(trimmed);
    setBusy(false);
    if (result.ok) {
      setOpen(false);
      setReason("");
    } else {
      setError(result.message);
    }
  };

  return (
    <div className="ml-auto flex flex-col items-end gap-1">
      <p className="flex items-center gap-1.5 text-sm text-muted-foreground">
        <Lock className="size-3.5" aria-hidden="true" />
        {finalisedAt ? `Finalised ${finalisedAt}` : "Finalised"}
      </p>
      <Button
        type="button"
        variant="outline"
        onClick={() => {
          setError(null);
          setOpen(true);
        }}
        className="gap-1.5"
      >
        <LockOpen className="size-4" aria-hidden="true" />
        Reopen for changes
      </Button>

      <AlertDialog open={open} onOpenChange={(next) => !busy && setOpen(next)}>
        <AlertDialogContent className="max-w-md bg-background p-6">
          <AlertDialogHeader>
            <AlertDialogTitle className="text-xl font-bold">Reopen this paper?</AlertDialogTitle>
            <AlertDialogDescription className="pt-2 text-sm text-muted-foreground">
              Marks can be changed again, and the paper must be finalised once more. What was final
              is kept in the record, together with your reason.
            </AlertDialogDescription>
          </AlertDialogHeader>
          <div className="flex flex-col gap-1.5">
            <Label htmlFor="reopen-reason">Reason for reopening</Label>
            <Textarea
              id="reopen-reason"
              value={reason}
              rows={3}
              maxLength={MAX_REASON}
              onChange={(e) => setReason(e.target.value)}
              aria-describedby="reopen-reason-hint"
            />
            <p id="reopen-reason-hint" className="text-xs text-muted-foreground">
              At least {MIN_REASON} characters.
            </p>
          </div>
          {error && (
            <p role="alert" className="text-sm text-destructive">
              {error}
            </p>
          )}
          <AlertDialogFooter className="mt-4 flex flex-col-reverse gap-2 sm:flex-row">
            <AlertDialogCancel disabled={busy}>Keep final</AlertDialogCancel>
            <AlertDialogAction
              onClick={(e) => {
                e.preventDefault();
                void confirm();
              }}
              disabled={busy || !valid}
              className="gap-2 font-semibold"
            >
              {busy && <Loader2 className="size-4 motion-safe:animate-spin" aria-hidden="true" />}
              Reopen
            </AlertDialogAction>
          </AlertDialogFooter>
        </AlertDialogContent>
      </AlertDialog>
    </div>
  );
}
