"use client";

import { useState } from "react";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Textarea } from "@/components/ui/textarea";
import { Badge } from "@/components/ui/badge";
import { Loader2, AlertCircle, Info, Lock } from "lucide-react";
import { UpdateExamSchema } from "@/lib/validations";
import { ApiError } from "@/services/api";
import type { Exam } from "@/types/exam-types";

interface EditExamDialogProps {
  exam: Exam | null;
  open: boolean;
  onOpenChange: (open: boolean) => void;
  onSubmit: (id: string, payload: Partial<Exam>) => Promise<Exam>;
  onSuccess?: (updatedExam: Exam) => void;
}

export function EditExamDialog({
  exam,
  open,
  onOpenChange,
  onSubmit,
  onSuccess,
}: EditExamDialogProps) {
  if (!exam) return null;

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="sm:max-w-lg">
        {open && (
          <EditExamFormContent
            key={exam.id}
            exam={exam}
            onOpenChange={onOpenChange}
            onSubmit={onSubmit}
            onSuccess={onSuccess}
          />
        )}
      </DialogContent>
    </Dialog>
  );
}

function EditExamFormContent({
  exam,
  onOpenChange,
  onSubmit,
  onSuccess,
}: {
  exam: Exam;
  onOpenChange: (open: boolean) => void;
  onSubmit: (id: string, payload: Partial<Exam>) => Promise<Exam>;
  onSuccess?: (updatedExam: Exam) => void;
}) {
  const [title, setTitle] = useState(exam.title);
  const [subject, setSubject] = useState(exam.subject);
  const [description, setDescription] = useState(exam.description || "");
  const [durationMins, setDurationMins] = useState(String(exam.durationMins));

  const [formErrors, setFormErrors] = useState<Record<string, string>>({});
  const [backendError, setBackendError] = useState<string | null>(null);
  const [submitting, setSubmitting] = useState(false);

  const isPublished = exam.status === "PUBLISHED";
  const isArchived = exam.status === "ARCHIVED";

  const handleClose = () => {
    if (submitting) return;
    onOpenChange(false);
  };

  const handleSubmit = async (e: React.FormEvent) => {
    e.preventDefault();
    if (isArchived) return;

    setBackendError(null);
    setFormErrors({});

    const durationNum = parseInt(durationMins, 10);
    const parseResult = UpdateExamSchema.safeParse({
      title,
      subject,
      description,
      durationMins: isNaN(durationNum) ? undefined : durationNum,
    });

    if (!parseResult.success) {
      const errors: Record<string, string> = {};
      for (const issue of parseResult.error.issues) {
        const path = issue.path[0];
        if (typeof path === "string") {
          errors[path] = issue.message;
        }
      }
      setFormErrors(errors);
      return;
    }

    const payload: Partial<Exam> = {
      title: parseResult.data.title,
      subject: parseResult.data.subject,
      description: parseResult.data.description,
    };

    if (!isPublished && parseResult.data.durationMins !== undefined) {
      payload.durationMins = parseResult.data.durationMins;
    }

    setSubmitting(true);
    try {
      const updated = await onSubmit(exam.id, payload);
      onSuccess?.(updated);
      onOpenChange(false);
    } catch (err) {
      const message =
        err instanceof ApiError
          ? err.message
          : err instanceof Error
          ? err.message
          : "Failed to update assessment.";
      setBackendError(message);
    } finally {
      setSubmitting(false);
    }
  };

  return (
    <form onSubmit={handleSubmit}>
      <DialogHeader>
        <div className="flex items-center gap-2">
          <DialogTitle className="text-xl">Edit Assessment</DialogTitle>
          <Badge variant="outline" className="text-xs">
            {exam.status}
          </Badge>
        </div>
        <DialogDescription>
          {isPublished
            ? "Update assessment details. Structural settings (duration) are locked for published exams."
            : isArchived
            ? "Archived assessments are read-only and cannot be modified."
            : "Update assessment metadata, course title, duration, and instructions."}
        </DialogDescription>
      </DialogHeader>

      {isPublished && (
        <div className="mt-3 flex items-start gap-2.5 rounded-lg bg-amber-500/10 border border-amber-500/20 p-3 text-xs text-amber-700 dark:text-amber-300 font-medium leading-relaxed">
          <Info className="size-4 shrink-0 mt-0.5" />
          <span>
            Assessment is <strong>Published</strong>: Duration is locked to safeguard running and scheduled exam attempts. Description and metadata can still be updated.
          </span>
        </div>
      )}

      {isArchived && (
        <div className="mt-3 flex items-start gap-2.5 rounded-lg bg-muted p-3 text-xs text-muted-foreground font-medium">
          <Lock className="size-4 shrink-0 mt-0.5" />
          <span>This assessment is archived and permanently immutable.</span>
        </div>
      )}

      {backendError && (
        <div className="mt-3 flex items-start gap-2.5 rounded-lg bg-destructive/10 border border-destructive/20 p-3 text-sm text-destructive font-medium">
          <AlertCircle className="size-4 shrink-0 mt-0.5" />
          <span>{backendError}</span>
        </div>
      )}

      <div className="flex flex-col gap-4 py-3">
        {/* Title */}
        <div className="space-y-1.5">
          <Label htmlFor="edit-exam-title" className="text-sm font-medium">
            Exam Title <span className="text-destructive">*</span>
          </Label>
          <Input
            id="edit-exam-title"
            value={title}
            onChange={(e) => setTitle(e.target.value)}
            disabled={submitting || isArchived}
            placeholder="e.g. Midterm General Physics"
            aria-invalid={!!formErrors.title}
          />
          {formErrors.title && (
            <p className="text-xs text-destructive">{formErrors.title}</p>
          )}
        </div>

        <div className="grid grid-cols-1 sm:grid-cols-2 gap-3">
          {/* Subject */}
          <div className="space-y-1.5">
            <Label htmlFor="edit-exam-subject" className="text-sm font-medium">
              Subject / Course <span className="text-destructive">*</span>
            </Label>
            <Input
              id="edit-exam-subject"
              value={subject}
              onChange={(e) => setSubject(e.target.value)}
              disabled={submitting || isArchived}
              placeholder="e.g. Physics 101"
              aria-invalid={!!formErrors.subject}
            />
            {formErrors.subject && (
              <p className="text-xs text-destructive">{formErrors.subject}</p>
            )}
          </div>

          {/* Duration */}
          <div className="space-y-1.5">
            <Label htmlFor="edit-exam-duration" className="text-sm font-medium">
              Duration (Minutes) <span className="text-destructive">*</span>
            </Label>
            <Input
              id="edit-exam-duration"
              type="number"
              min={1}
              max={600}
              value={durationMins}
              onChange={(e) => setDurationMins(e.target.value)}
              disabled={submitting || isPublished || isArchived}
              aria-invalid={!!formErrors.durationMins}
            />
            {formErrors.durationMins && (
              <p className="text-xs text-destructive">{formErrors.durationMins}</p>
            )}
          </div>
        </div>

        {/* Description */}
        <div className="space-y-1.5">
          <Label htmlFor="edit-exam-desc" className="text-sm font-medium">
            Description & Instructions
          </Label>
          <Textarea
            id="edit-exam-desc"
            rows={3}
            value={description}
            onChange={(e) => setDescription(e.target.value)}
            disabled={submitting || isArchived}
            aria-invalid={!!formErrors.description}
          />
          {formErrors.description && (
            <p className="text-xs text-destructive">{formErrors.description}</p>
          )}
        </div>
      </div>

      <DialogFooter className="gap-2 sm:gap-0 pt-2">
        <Button
          type="button"
          variant="outline"
          onClick={handleClose}
          disabled={submitting}
        >
          {isArchived ? "Close" : "Cancel"}
        </Button>
        {!isArchived && (
          <Button type="submit" disabled={submitting}>
            {submitting && <Loader2 className="mr-2 size-4 animate-spin" />}
            {submitting ? "Saving Changes..." : "Save Changes"}
          </Button>
        )}
      </DialogFooter>
    </form>
  );
}
