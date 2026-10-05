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
import { Loader2, AlertCircle } from "lucide-react";
import { CreateExamSchema } from "@/lib/validations";
import { ApiError } from "@/services/api";
import type { Exam } from "@/types/exam-types";

interface CreateExamDialogProps {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  onSubmit: (payload: {
    title: string;
    subject: string;
    description: string;
    durationMins: number;
  }) => Promise<Exam>;
  onSuccess?: (newExam: Exam) => void;
}

export function CreateExamDialog({
  open,
  onOpenChange,
  onSubmit,
  onSuccess,
}: CreateExamDialogProps) {
  const [title, setTitle] = useState("");
  const [subject, setSubject] = useState("");
  const [description, setDescription] = useState("");
  const [durationMins, setDurationMins] = useState("60");

  const [formErrors, setFormErrors] = useState<Record<string, string>>({});
  const [backendError, setBackendError] = useState<string | null>(null);
  const [submitting, setSubmitting] = useState(false);

  const resetForm = () => {
    setTitle("");
    setSubject("");
    setDescription("");
    setDurationMins("60");
    setFormErrors({});
    setBackendError(null);
  };

  const handleClose = () => {
    if (submitting) return;
    resetForm();
    onOpenChange(false);
  };

  const handleSubmit = async (e: React.FormEvent) => {
    e.preventDefault();
    setFormErrors({});
    setBackendError(null);

    const numDuration = parseInt(durationMins, 10) || 0;
    const validation = CreateExamSchema.safeParse({
      title,
      subject,
      description,
      durationMins: numDuration,
    });

    if (!validation.success) {
      const errors: Record<string, string> = {};
      validation.error.errors.forEach((err) => {
        if (err.path[0]) {
          errors[err.path[0] as string] = err.message;
        }
      });
      setFormErrors(errors);
      return;
    }

    setSubmitting(true);
    try {
      const created = await onSubmit({
        title: title.trim(),
        subject: subject.trim(),
        description: description.trim(),
        durationMins: numDuration,
      });
      resetForm();
      onOpenChange(false);
      onSuccess?.(created);
    } catch (err) {
      const message =
        err instanceof ApiError
          ? err.message
          : err instanceof Error
          ? err.message
          : "Failed to create assessment. Please verify your connection.";
      setBackendError(message);
    } finally {
      setSubmitting(false);
    }
  };

  return (
    <Dialog open={open} onOpenChange={(val) => !submitting && (val ? onOpenChange(true) : handleClose())}>
      <DialogContent className="sm:max-w-lg">
        <form onSubmit={handleSubmit}>
          <DialogHeader>
            <DialogTitle className="text-xl">Create New Assessment</DialogTitle>
            <DialogDescription>
              Set up initial metadata for your exam. New exams start as DRAFT and can be updated prior to publishing.
            </DialogDescription>
          </DialogHeader>

          {backendError && (
            <div className="mt-3 flex items-start gap-2.5 rounded-lg bg-destructive/10 border border-destructive/20 p-3 text-sm text-destructive font-medium">
              <AlertCircle className="size-4 shrink-0 mt-0.5" />
              <span>{backendError}</span>
            </div>
          )}

          <div className="flex flex-col gap-4 py-3">
            <div className="space-y-1.5">
              <Label htmlFor="create-exam-title" className="text-sm font-medium">
                Exam Title <span className="text-destructive">*</span>
              </Label>
              <Input
                id="create-exam-title"
                placeholder="e.g. Midterm General Physics"
                value={title}
                onChange={(e) => {
                  setTitle(e.target.value);
                  if (formErrors.title) {
                    setFormErrors((prev) => ({ ...prev, title: "" }));
                  }
                }}
                disabled={submitting}
                aria-invalid={!!formErrors.title}
              />
              {formErrors.title && <p className="text-xs text-destructive">{formErrors.title}</p>}
            </div>

            <div className="grid grid-cols-1 sm:grid-cols-2 gap-3">
              <div className="space-y-1.5">
                <Label htmlFor="create-exam-subject" className="text-sm font-medium">
                  Subject / Course <span className="text-destructive">*</span>
                </Label>
                <Input
                  id="create-exam-subject"
                  placeholder="e.g. Physics 101"
                  value={subject}
                  onChange={(e) => {
                    setSubject(e.target.value);
                    if (formErrors.subject) {
                      setFormErrors((prev) => ({ ...prev, subject: "" }));
                    }
                  }}
                  disabled={submitting}
                  aria-invalid={!!formErrors.subject}
                />
                {formErrors.subject && (
                  <p className="text-xs text-destructive">{formErrors.subject}</p>
                )}
              </div>

              <div className="space-y-1.5">
                <Label htmlFor="create-exam-duration" className="text-sm font-medium">
                  Duration (Minutes) <span className="text-destructive">*</span>
                </Label>
                <Input
                  id="create-exam-duration"
                  type="number"
                  min="1"
                  max="600"
                  value={durationMins}
                  onChange={(e) => {
                    setDurationMins(e.target.value);
                    if (formErrors.durationMins) {
                      setFormErrors((prev) => ({ ...prev, durationMins: "" }));
                    }
                  }}
                  disabled={submitting}
                  aria-invalid={!!formErrors.durationMins}
                />
                {formErrors.durationMins && (
                  <p className="text-xs text-destructive">{formErrors.durationMins}</p>
                )}
              </div>
            </div>

            <div className="space-y-1.5">
              <Label htmlFor="create-exam-desc" className="text-sm font-medium">
                Description & Instructions
              </Label>
              <Textarea
                id="create-exam-desc"
                rows={3}
                placeholder="State covered topics, instructions, and permitted accommodations..."
                value={description}
                onChange={(e) => setDescription(e.target.value)}
                disabled={submitting}
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
              Cancel
            </Button>
            <Button type="submit" disabled={submitting}>
              {submitting && <Loader2 className="mr-2 size-4 animate-spin" />}
              {submitting ? "Creating Draft..." : "Create Draft"}
            </Button>
          </DialogFooter>
        </form>
      </DialogContent>
    </Dialog>
  );
}
