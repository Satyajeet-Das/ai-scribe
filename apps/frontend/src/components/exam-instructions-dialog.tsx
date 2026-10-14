"use client";

import { Clock, AlertTriangle, ShieldCheck, FileText, CheckCircle2, Play, RotateCcw } from "lucide-react";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import { Button } from "@/components/ui/button";
import { Badge } from "@/components/ui/badge";
import type { SessionStatus } from "@/types/exam-types";

interface ExamInstructionsDialogProps {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  exam: {
    id?: string;
    examId?: string;
    title: string;
    subject: string;
    durationMins: number;
    description?: string;
  } | null;
  sessionStatus?: SessionStatus;
  sessionId?: string;
  onConfirm: () => void | Promise<void>;
  loading?: boolean;
}

export function ExamInstructionsDialog({
  open,
  onOpenChange,
  exam,
  sessionStatus,
  sessionId,
  onConfirm,
  loading = false,
}: ExamInstructionsDialogProps) {
  if (!exam) return null;

  const isCompleted = sessionStatus === "SUBMITTED";
  const isInProgress = sessionStatus === "IN_PROGRESS";
  const isExpired = sessionStatus === "EXPIRED";

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="max-w-md sm:max-w-lg p-6 bg-background">
        <DialogHeader className="text-left space-y-2">
          <div className="flex items-center justify-between gap-2">
            <Badge variant="outline" className="text-xs font-medium">
              {exam.subject}
            </Badge>
            {isInProgress ? (
              <Badge variant="default" className="bg-amber-600 hover:bg-amber-600 text-white text-[10px]">
                ACTIVE SESSION
              </Badge>
            ) : isCompleted ? (
              <Badge variant="default" className="bg-emerald-600 hover:bg-emerald-600 text-white text-[10px]">
                COMPLETED
              </Badge>
            ) : isExpired ? (
              <Badge variant="destructive" className="text-[10px]">
                EXPIRED
              </Badge>
            ) : (
              <Badge variant="secondary" className="text-[10px]">
                READY TO START
              </Badge>
            )}
          </div>
          <DialogTitle className="text-xl font-bold">{exam.title}</DialogTitle>
          {exam.description && (
            <DialogDescription className="text-sm text-muted-foreground line-clamp-2">
              {exam.description}
            </DialogDescription>
          )}
        </DialogHeader>

        {/* Instructions & Guidelines */}
        <div className="space-y-4 py-3 text-sm">
          {isInProgress ? (
            <div className="rounded-lg bg-amber-500/10 border border-amber-500/20 p-3.5 flex items-start gap-3">
              <RotateCcw className="size-5 text-amber-600 dark:text-amber-400 shrink-0 mt-0.5" />
              <div className="space-y-1">
                <p className="font-semibold text-amber-900 dark:text-amber-200">
                  You have an active session in progress
                </p>
                <p className="text-xs text-amber-800/90 dark:text-amber-300/90">
                  Clicking &ldquo;Resume Exam&rdquo; will reconnect you directly to your session with your previously confirmed answers restored.
                </p>
              </div>
            </div>
          ) : isCompleted ? (
            <div className="rounded-lg bg-emerald-500/10 border border-emerald-500/20 p-3.5 flex items-start gap-3">
              <CheckCircle2 className="size-5 text-emerald-600 dark:text-emerald-400 shrink-0 mt-0.5" />
              <div className="space-y-1">
                <p className="font-semibold text-emerald-900 dark:text-emerald-200">
                  Assessment Completed
                </p>
                <p className="text-xs text-emerald-800/90 dark:text-emerald-300/90">
                  You have already submitted this exam. Submitted exams are sealed and immutable.
                </p>
              </div>
            </div>
          ) : isExpired ? (
            <div className="rounded-lg bg-destructive/10 border border-destructive/20 p-3.5 flex items-start gap-3">
              <AlertTriangle className="size-5 text-destructive shrink-0 mt-0.5" />
              <div className="space-y-1">
                <p className="font-semibold text-destructive">
                  Session Expired
                </p>
                <p className="text-xs text-muted-foreground">
                  The allotted time for this examination has passed.
                </p>
              </div>
            </div>
          ) : (
            <div className="rounded-lg bg-muted/60 border p-3.5 space-y-2.5">
              <div className="flex items-center gap-2 font-semibold text-foreground">
                <Clock className="size-4 text-primary" />
                <span>Allotted Duration: {exam.durationMins} minutes</span>
              </div>
              <ul className="space-y-2 text-xs text-muted-foreground list-disc pl-4">
                <li>The countdown timer starts as soon as you confirm and launch.</li>
                <li>Each answer you select is saved to the server automatically in real time.</li>
                <li>You can navigate back and forth between questions before submitting.</li>
                <li>If your connection drops, refreshing the page will seamlessly restore your active exam.</li>
                <li>Once you submit, your responses become permanent and cannot be edited.</li>
              </ul>
            </div>
          )}
        </div>

        <DialogFooter className="flex flex-col-reverse sm:flex-row gap-2 sm:justify-end">
          <Button
            type="button"
            variant="outline"
            onClick={() => onOpenChange(false)}
            disabled={loading}
          >
            Cancel
          </Button>

          {isCompleted ? (
            <Button disabled variant="secondary">
              Already Submitted
            </Button>
          ) : isExpired ? (
            <Button disabled variant="secondary">
              Exam Expired
            </Button>
          ) : isInProgress ? (
            <Button
              onClick={onConfirm}
              disabled={loading}
              className="gap-2 bg-amber-600 hover:bg-amber-700 text-white"
            >
              <RotateCcw className="size-4" />
              {loading ? "Reconnecting..." : "Resume Exam"}
            </Button>
          ) : (
            <Button
              onClick={onConfirm}
              disabled={loading}
              className="gap-2"
            >
              <Play className="size-4 fill-current" />
              {loading ? "Starting Assessment..." : "Confirm & Start Exam"}
            </Button>
          )}
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
