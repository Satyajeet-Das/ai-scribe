"use client";

import Link from "next/link";
import { useEffect, useRef, useState } from "react";
import {
  AlertCircle,
  Ban,
  CheckCircle2,
  Clock,
  Download,
  FileText,
  Loader2,
  RotateCcw,
  Trash2,
  ClipboardCheck,
} from "lucide-react";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Progress, ProgressTrack, ProgressIndicator } from "@/components/ui/progress";
import { describeDocumentError, failureAdvice } from "@/lib/document-errors";
import { formatBytes } from "@/lib/document-validation";
import { examDocumentsApi } from "@/services/exam-documents-api";
import { isActiveStatus, type DocumentStatus, type ExamDocument } from "@/types/exam-document";
import { ConfirmDialog } from "./confirm-dialog";
import { STAGE_LABEL, STATUS_LABEL, STATUS_STYLE, announcementFor } from "./document-status";

const STATUS_ICON: Record<DocumentStatus, typeof Clock> = {
  QUEUED: Clock,
  PROCESSING: Loader2,
  REVIEW_REQUIRED: ClipboardCheck,
  COMPLETED: CheckCircle2,
  FAILED: AlertCircle,
  CANCELLED: Ban,
};

interface DocumentListProps {
  examId: string;
  documents: ExamDocument[];
  loading: boolean;
  /** A refresh failed; what is shown may be out of date. */
  refreshFailed?: string;
  /** Called after any action, so the list is refreshed from the server. */
  onChanged: () => void;
}

type Pending =
  | { kind: "delete"; doc: ExamDocument }
  | { kind: "reread"; doc: ExamDocument }
  | null;

/**
 * Tells a screen reader when a document changes state. It says nothing on the
 * first load and nothing while only progress moves, so a long extraction does
 * not produce a stream of announcements.
 */
function useStatusAnnouncement(documents: ExamDocument[]): string {
  const previous = useRef<Map<string, DocumentStatus> | null>(null);
  const [message, setMessage] = useState("");

  useEffect(() => {
    const now = new Map(documents.map((d) => [d.id, d.status] as const));
    const before = previous.current;
    previous.current = now;
    if (!before) return;
    const changed = documents.filter((d) => before.has(d.id) && before.get(d.id) !== d.status);
    if (changed.length > 0) {
      setMessage(changed.map((d) => announcementFor(d.filename, d.status)).join(" "));
    }
  }, [documents]);

  return message;
}

export function DocumentList({ examId, documents, loading, refreshFailed, onChanged }: DocumentListProps) {
  const announcement = useStatusAnnouncement(documents);
  const [busy, setBusy] = useState<string | null>(null);
  const [message, setMessage] = useState<{ id: string; text: string } | null>(null);
  const [pending, setPending] = useState<Pending>(null);

  // One action at a time per list: a second click while one runs does nothing.
  async function run(doc: ExamDocument, action: () => Promise<unknown>, done?: string) {
    if (busy) return;
    setBusy(doc.id);
    setMessage(null);
    try {
      await action();
      if (done) setMessage({ id: doc.id, text: done });
    } catch (err) {
      setMessage({ id: doc.id, text: describeDocumentError(err).message });
    } finally {
      setBusy(null);
      setPending(null);
      onChanged();
    }
  }

  async function download(doc: ExamDocument) {
    await run(doc, async () => {
      const blob = await examDocumentsApi.getFile(examId, doc.id);
      const url = URL.createObjectURL(blob);
      const a = document.createElement("a");
      a.href = url;
      a.download = doc.filename;
      a.click();
      setTimeout(() => URL.revokeObjectURL(url), 10_000);
    });
  }

  if (loading) {
    return (
      <p className="flex items-center gap-2 text-sm text-muted-foreground" role="status">
        <Loader2 className="size-4 animate-spin" aria-hidden="true" />
        Loading uploaded papers…
      </p>
    );
  }

  return (
    <div className="space-y-3">
      <div role="status" aria-live="polite" className="sr-only" data-testid="status-announcer">
        {announcement}
      </div>

      {refreshFailed && (
        <p role="alert" className="rounded-md border border-amber-300 bg-amber-50 p-2 text-xs text-amber-900 dark:bg-amber-950 dark:text-amber-200">
          {refreshFailed} What is shown below may be out of date.
        </p>
      )}

      {documents.length === 0 ? (
        <p className="text-sm text-muted-foreground">No papers uploaded for this exam yet.</p>
      ) : (
        <ul className="space-y-3">
          {documents.map((doc) => (
            <DocumentItem
              key={doc.id}
              doc={doc}
              examId={examId}
              busy={busy === doc.id}
              locked={busy !== null}
              message={message?.id === doc.id ? message.text : undefined}
              onCancel={() => run(doc, () => examDocumentsApi.cancel(examId, doc.id), "Processing cancelled.")}
              onRetry={() => run(doc, () => examDocumentsApi.retry(examId, doc.id), "Processing started again.")}
              onReread={() => setPending({ kind: "reread", doc })}
              onDelete={() => setPending({ kind: "delete", doc })}
              onDownload={() => download(doc)}
            />
          ))}
        </ul>
      )}

      <ConfirmDialog
        open={pending?.kind === "delete"}
        onOpenChange={(open) => !open && setPending(null)}
        title="Delete this paper?"
        description={
          <p>
            “{pending?.doc.filename}” and its draft will be deleted. Questions that were already approved
            into the exam stay in the exam.
          </p>
        }
        confirmLabel="Delete paper"
        destructive
        busy={busy !== null}
        onConfirm={() => pending && run(pending.doc, () => examDocumentsApi.remove(examId, pending.doc.id))}
      />
      <ConfirmDialog
        open={pending?.kind === "reread"}
        onOpenChange={(open) => !open && setPending(null)}
        title="Read this paper again?"
        description={
          <p>
            The questions extracted from “{pending?.doc.filename}” and any corrections you have made to them
            will be thrown away, and the paper will be read again from the start.
          </p>
        }
        confirmLabel="Discard draft and read again"
        destructive
        busy={busy !== null}
        onConfirm={() =>
          pending &&
          run(
            pending.doc,
            () =>
              examDocumentsApi.retry(examId, pending.doc.id, {
                discardDraft: true,
                expectedDraftVersion: pending.doc.draftVersion,
              }),
            "Processing started again."
          )
        }
      />
    </div>
  );
}

interface ItemProps {
  doc: ExamDocument;
  examId: string;
  busy: boolean;
  locked: boolean;
  message?: string;
  onCancel: () => void;
  onRetry: () => void;
  onReread: () => void;
  onDelete: () => void;
  onDownload: () => void;
}

function DocumentItem({ doc, examId, busy, locked, message, onCancel, onRetry, onReread, onDelete, onDownload }: ItemProps) {
  const Icon = STATUS_ICON[doc.status];
  const reviewHref = `/exams/${encodeURIComponent(examId)}/documents/${encodeURIComponent(doc.id)}`;
  // Numbers are shown only where the server's numbers mean something: the
  // "finding the questions" step counts requests; the others report 0 of N.
  const showProgress = doc.status === "PROCESSING" && doc.stage === "EXTRACTING" && doc.progress.total > 0;

  return (
    <li className="rounded-lg border bg-card p-4" data-testid={`document-${doc.id}`}>
      <div className="flex flex-wrap items-start justify-between gap-3">
        <div className="min-w-0 space-y-1">
          <h3 className="flex items-center gap-2 text-sm font-semibold">
            <FileText className="size-4 shrink-0 text-primary" aria-hidden="true" />
            <span className="truncate">{doc.filename}</span>
          </h3>
          <p className="text-xs text-muted-foreground">
            {formatBytes(doc.sizeBytes)} · {doc.pageCount} {doc.pageCount === 1 ? "page" : "pages"} · uploaded{" "}
            {new Date(doc.createdAt).toLocaleString()}
          </p>
        </div>
        <Badge variant="outline" className={`${STATUS_STYLE[doc.status]} gap-1 px-2.5 py-0.5 text-xs font-semibold`}>
          <Icon className={`size-3.5 ${doc.status === "PROCESSING" ? "animate-spin" : ""}`} aria-hidden="true" />
          {STATUS_LABEL[doc.status]}
        </Badge>
      </div>

      {doc.status === "QUEUED" && (
        <p className="mt-2 text-sm text-muted-foreground">Waiting its turn. You can leave this page; it will carry on.</p>
      )}

      {doc.status === "PROCESSING" && (
        <div className="mt-2 space-y-1.5">
          <p className="text-sm">{doc.stage ? STAGE_LABEL[doc.stage] : "Working on it"}…</p>
          {showProgress && (
            <Progress value={(doc.progress.done / doc.progress.total) * 100} aria-label="Reading progress">
              <ProgressTrack>
                <ProgressIndicator />
              </ProgressTrack>
              <p className="mt-1 text-xs text-muted-foreground">
                Part {doc.progress.done} of {doc.progress.total} done
              </p>
            </Progress>
          )}
        </div>
      )}

      {doc.status === "FAILED" && (
        <div className="mt-2 space-y-1 text-sm">
          <p className="font-medium text-destructive">
            {doc.failureMessage || "Something went wrong while reading this paper."}
          </p>
          <p className="text-muted-foreground">{failureAdvice(doc.failureCode)}</p>
        </div>
      )}

      {doc.status === "COMPLETED" && doc.approvedAt && (
        <p className="mt-2 text-sm text-muted-foreground">
          Approved into the exam on {new Date(doc.approvedAt).toLocaleDateString()}.
        </p>
      )}

      {message && (
        <p className="mt-2 text-sm" role="status">
          {message}
        </p>
      )}

      <div className="mt-3 flex flex-wrap items-center gap-2">
        {doc.status === "REVIEW_REQUIRED" && (
          <Link
            href={reviewHref}
            className="inline-flex h-8 items-center gap-1.5 rounded-lg bg-primary px-3 text-sm font-medium text-primary-foreground hover:bg-primary/80 focus-visible:outline-none focus-visible:ring-[3px] focus-visible:ring-ring/50"
          >
            <ClipboardCheck className="size-4" aria-hidden="true" />
            Review questions
          </Link>
        )}
        {doc.status === "COMPLETED" && (
          <Link
            href={reviewHref}
            className="inline-flex h-8 items-center gap-1.5 rounded-lg border border-input bg-background px-3 text-sm font-medium hover:bg-muted focus-visible:outline-none focus-visible:ring-[3px] focus-visible:ring-ring/50"
          >
            View approved draft
            <span className="sr-only"> of {doc.filename}</span>
          </Link>
        )}
        {isActiveStatus(doc.status) && (
          <Button variant="outline" size="sm" onClick={onCancel} disabled={locked} aria-busy={busy} aria-label={`Cancel processing of ${doc.filename}`}>
            <Ban className="size-4" aria-hidden="true" />
            Cancel
          </Button>
        )}
        {(doc.status === "FAILED" || doc.status === "CANCELLED") && (
          <Button variant="outline" size="sm" onClick={onRetry} disabled={locked} aria-busy={busy} aria-label={`Try again with ${doc.filename}`}>
            <RotateCcw className="size-4" aria-hidden="true" />
            Try again
          </Button>
        )}
        {doc.status === "REVIEW_REQUIRED" && (
          <Button variant="ghost" size="sm" onClick={onReread} disabled={locked} aria-label={`Read ${doc.filename} again`}>
            <RotateCcw className="size-4" aria-hidden="true" />
            Read again
          </Button>
        )}
        <Button variant="ghost" size="sm" onClick={onDownload} disabled={locked} aria-label={`Download PDF of ${doc.filename}`}>
          <Download className="size-4" aria-hidden="true" />
          Download PDF
        </Button>
        <Button
          variant="ghost"
          size="sm"
          onClick={onDelete}
          disabled={locked}
          aria-label={`Delete ${doc.filename}`}
          className="text-destructive hover:text-destructive"
        >
          <Trash2 className="size-4" aria-hidden="true" />
          Delete
        </Button>
      </div>
    </li>
  );
}
