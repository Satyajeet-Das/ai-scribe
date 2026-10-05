"use client";

import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import { Button } from "@/components/ui/button";
import { Loader2, AlertTriangle, CheckCircle2, RotateCcw, Archive, Trash2 } from "lucide-react";

export type ConfirmActionType = "publish" | "unpublish" | "archive" | "delete";

interface ConfirmActionDialogProps {
  action: ConfirmActionType | null;
  examTitle: string;
  open: boolean;
  onOpenChange: (open: boolean) => void;
  onConfirm: () => Promise<void>;
  loading: boolean;
}

const ACTION_CONFIG: Record<
  ConfirmActionType,
  {
    title: string;
    description: (title: string) => string;
    confirmText: string;
    icon: typeof AlertTriangle;
    buttonVariant: "default" | "destructive" | "outline";
    iconColor: string;
  }
> = {
  publish: {
    title: "Publish Assessment",
    description: (title) =>
      `Are you sure you want to publish "${title}"? Publishing will make this exam ready for student allocation and freeze structural timing settings.`,
    confirmText: "Publish Assessment",
    icon: CheckCircle2,
    buttonVariant: "default",
    iconColor: "text-emerald-600 dark:text-emerald-400",
  },
  unpublish: {
    title: "Revert Assessment to Draft",
    description: (title) =>
      `Are you sure you want to revert "${title}" to draft? This allows full editing of all parameters. Note: This action is only permitted if no students have started sessions or active assignments.`,
    confirmText: "Revert to Draft",
    icon: RotateCcw,
    buttonVariant: "default",
    iconColor: "text-amber-600 dark:text-amber-400",
  },
  archive: {
    title: "Archive Assessment",
    description: (title) =>
      `Are you sure you want to archive "${title}"? Archived assessments are permanently frozen and cannot be edited, published, or reopened.`,
    confirmText: "Archive Assessment",
    icon: Archive,
    buttonVariant: "default",
    iconColor: "text-amber-600 dark:text-amber-400",
  },
  delete: {
    title: "Delete Assessment",
    description: (title) =>
      `Are you sure you want to delete "${title}"? This will soft-delete the exam. Assessments with existing student assignments or sessions cannot be deleted and must be archived instead.`,
    confirmText: "Delete Assessment",
    icon: Trash2,
    buttonVariant: "destructive",
    iconColor: "text-destructive",
  },
};

export function ConfirmActionDialog({
  action,
  examTitle,
  open,
  onOpenChange,
  onConfirm,
  loading,
}: ConfirmActionDialogProps) {
  if (!action) return null;

  const config = ACTION_CONFIG[action];
  const Icon = config.icon;

  const handleConfirm = async () => {
    try {
      await onConfirm();
      onOpenChange(false);
    } catch {
      // Error handling is handled in caller, dialog remains open for retry or cancellation
    }
  };

  return (
    <Dialog open={open} onOpenChange={(val) => !loading && onOpenChange(val)}>
      <DialogContent className="sm:max-w-md">
        <DialogHeader className="gap-2">
          <div className="flex items-center gap-3">
            <div className={`p-2 rounded-full bg-muted ${config.iconColor}`}>
              <Icon className="size-5" />
            </div>
            <DialogTitle className="text-lg font-semibold">{config.title}</DialogTitle>
          </div>
          <DialogDescription className="text-sm leading-relaxed text-muted-foreground pt-1">
            {config.description(examTitle)}
          </DialogDescription>
        </DialogHeader>

        <DialogFooter className="gap-2 sm:gap-0 pt-3">
          <Button
            type="button"
            variant="outline"
            disabled={loading}
            onClick={() => onOpenChange(false)}
          >
            Cancel
          </Button>
          <Button
            type="button"
            variant={config.buttonVariant}
            disabled={loading}
            onClick={handleConfirm}
          >
            {loading && <Loader2 className="mr-2 size-4 animate-spin" />}
            {loading ? "Processing..." : config.confirmText}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
