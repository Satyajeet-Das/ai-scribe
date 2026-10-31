import type { DocumentStage, DocumentStatus } from "@/types/exam-document";

/** What each state means to a teacher, in plain words. */
export const STATUS_LABEL: Record<DocumentStatus, string> = {
  QUEUED: "Waiting to start",
  PROCESSING: "Reading the paper",
  REVIEW_REQUIRED: "Ready for review",
  COMPLETED: "Approved",
  FAILED: "Failed",
  CANCELLED: "Cancelled",
};

export const STAGE_LABEL: Record<DocumentStage, string> = {
  READING_PAGES: "Reading the pages",
  EXTRACTING: "Finding the questions",
  VALIDATING: "Checking the result",
  SAVING: "Saving the draft",
};

export const STATUS_STYLE: Record<DocumentStatus, string> = {
  QUEUED: "bg-muted text-muted-foreground border-border",
  PROCESSING: "bg-blue-50 text-blue-900 border-blue-300 dark:bg-blue-950 dark:text-blue-200 dark:border-blue-800",
  REVIEW_REQUIRED: "bg-amber-50 text-amber-900 border-amber-300 dark:bg-amber-950 dark:text-amber-200 dark:border-amber-800",
  COMPLETED: "bg-emerald-50 text-emerald-900 border-emerald-300 dark:bg-emerald-950 dark:text-emerald-200 dark:border-emerald-800",
  FAILED: "bg-destructive/10 text-destructive border-destructive/30",
  CANCELLED: "bg-muted text-muted-foreground border-border",
};

/** A sentence for a screen reader when a document changes state. */
export function announcementFor(filename: string, status: DocumentStatus): string {
  switch (status) {
    case "QUEUED":
      return `${filename}: waiting to start.`;
    case "PROCESSING":
      return `${filename}: reading has started.`;
    case "REVIEW_REQUIRED":
      return `${filename}: ready for review.`;
    case "COMPLETED":
      return `${filename}: approved.`;
    case "FAILED":
      return `${filename}: failed.`;
    case "CANCELLED":
      return `${filename}: cancelled.`;
  }
}
