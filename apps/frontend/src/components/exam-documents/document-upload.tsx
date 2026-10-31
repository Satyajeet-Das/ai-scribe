"use client";

import { useCallback, useId, useRef, useState } from "react";
import { AlertCircle, CheckCircle2, FileText, Loader2, Upload, X } from "lucide-react";
import { Button } from "@/components/ui/button";
import { checkDocumentFile, formatBytes, maxDocumentBytes, type FileCheck } from "@/lib/document-validation";
import { describeDocumentError, type DocumentError } from "@/lib/document-errors";
import { examDocumentsApi } from "@/services/exam-documents-api";
import type { ExamDocument } from "@/types/exam-document";

interface DocumentUploadProps {
  examId: string;
  /** Why uploading is unavailable (for example, the exam is published); disables the control. */
  disabledReason?: string;
  /**
   * Called when the list of documents should be refreshed: once the server has
   * queued the file, or has said it already has it (then there is no new document).
   */
  onUploaded: (doc?: ExamDocument) => void;
  /** Overrides the size limit shown and checked; defaults to the server's default. */
  maxBytes?: number;
}

/**
 * Chooses and uploads one exam PDF.
 *
 * The file is checked here only so a teacher hears about an unusable file
 * before a long upload; the server repeats every check. While a request is in
 * flight the controls are disabled and a ref guards against a second submit
 * from a double click or Enter. `fetch` cannot report upload progress, so
 * this shows that an upload is under way, not how far along it is.
 *
 * A failed upload can be retried. If the first attempt did reach the server,
 * the retry is answered with "already uploaded", which is reported as such and
 * refreshes the list, so a retry can never make a second document.
 */
export function DocumentUpload({ examId, disabledReason, onUploaded, maxBytes }: DocumentUploadProps) {
  const limit = maxBytes ?? maxDocumentBytes();
  const inputId = useId();
  const hintId = useId();
  const problemId = useId();

  const [file, setFile] = useState<File | null>(null);
  const [check, setCheck] = useState<FileCheck | null>(null);
  const [uploading, setUploading] = useState(false);
  const [error, setError] = useState<DocumentError | null>(null);
  const [status, setStatus] = useState("");
  const [dragging, setDragging] = useState(false);
  // Changing the key gives a fresh <input>, the only reliable way to clear a chosen file.
  const [inputKey, setInputKey] = useState(0);

  const inFlight = useRef(false);
  const controller = useRef<AbortController | null>(null);
  const disabled = Boolean(disabledReason);

  const choose = useCallback(
    (picked: File | null) => {
      setError(null);
      setStatus("");
      if (!picked) {
        setFile(null);
        setCheck(null);
        return;
      }
      const result = checkDocumentFile(picked, limit);
      setFile(picked);
      setCheck(result);
      if (!result.ok) setStatus("");
    },
    [limit]
  );

  const clear = useCallback(() => {
    choose(null);
    setInputKey((k) => k + 1);
  }, [choose]);

  const submit = useCallback(async () => {
    if (!file || !check?.ok || inFlight.current || disabled) return;
    inFlight.current = true;
    const ctl = new AbortController();
    controller.current = ctl;
    setUploading(true);
    setError(null);
    setStatus(`Uploading ${file.name}…`);
    try {
      const doc = await examDocumentsApi.upload(examId, file, ctl.signal);
      clear();
      // After clear(), which resets the status along with the selection.
      setStatus(`Uploaded ${file.name}. Reading has started; its progress is shown below.`);
      onUploaded(doc);
    } catch (err) {
      if (ctl.signal.aborted) {
        setStatus("");
        setError({ kind: "network", message: "Upload cancelled.", retryable: true, fields: {} });
      } else {
        const described = describeDocumentError(err);
        setStatus("");
        if (described.code === "DOCUMENT_ALREADY_UPLOADED") {
          // An earlier attempt got through: the file is already there. Drop the
          // selection, keep the explanation, and refresh so it shows up.
          clear();
          setError(described);
          onUploaded();
        } else {
          setError(described);
        }
      }
    } finally {
      inFlight.current = false;
      controller.current = null;
      setUploading(false);
    }
  }, [check, clear, disabled, examId, file, onUploaded]);

  const cancelUpload = () => controller.current?.abort();

  const onDrop = (e: React.DragEvent) => {
    e.preventDefault();
    setDragging(false);
    if (uploading || disabled) return;
    const dropped = e.dataTransfer.files;
    if (dropped.length > 1) {
      choose(null);
      setError({ kind: "validation", message: "Drop one PDF at a time.", retryable: false, fields: {} });
      return;
    }
    if (dropped[0]) choose(dropped[0]);
  };

  const invalid = Boolean(check && !check.ok);
  const problem = invalid ? check!.message : error?.message;
  const canSubmit = Boolean(file && check?.ok) && !uploading && !disabled;

  return (
    <div className="space-y-3">
      <div
        onDragOver={(e) => {
          e.preventDefault();
          if (!uploading && !disabled) setDragging(true);
        }}
        onDragLeave={() => setDragging(false)}
        onDrop={onDrop}
        className={`rounded-lg border-2 border-dashed p-4 transition-colors ${
          dragging ? "border-primary bg-primary/5" : "border-border"
        } ${disabled ? "opacity-60" : ""}`}
      >
        <label htmlFor={inputId} className="block text-sm font-medium">
          Exam paper (PDF)
        </label>
        <p id={hintId} className="mt-1 text-xs text-muted-foreground">
          PDF only, up to {formatBytes(limit)}. You can also drop a file here. Nothing is added to the exam
          until you have reviewed and approved it.
        </p>
        <input
          key={inputKey}
          id={inputId}
          type="file"
          accept="application/pdf,.pdf"
          disabled={uploading || disabled}
          aria-describedby={`${hintId}${problem ? ` ${problemId}` : ""}`}
          aria-invalid={invalid || undefined}
          onChange={(e) => choose(e.target.files?.[0] ?? null)}
          className="mt-3 block w-full text-sm file:mr-3 file:rounded-md file:border file:border-border file:bg-background file:px-3 file:py-1.5 file:text-sm file:font-medium hover:file:bg-muted disabled:cursor-not-allowed"
        />
      </div>

      {disabledReason && (
        <p className="text-sm text-muted-foreground">{disabledReason}</p>
      )}

      {file && (
        <div className="flex items-center justify-between gap-3 rounded-lg border bg-muted/30 p-3">
          <div className="flex min-w-0 items-center gap-2">
            <FileText className="size-4 shrink-0 text-primary" aria-hidden="true" />
            <span className="truncate text-sm font-medium" data-testid="chosen-file-name">
              {file.name}
            </span>
            <span className="shrink-0 text-xs text-muted-foreground">{formatBytes(file.size)}</span>
          </div>
          {!uploading && (
            <Button type="button" variant="ghost" size="sm" onClick={clear} aria-label={`Remove ${file.name}`}>
              <X className="size-4" aria-hidden="true" />
              Remove
            </Button>
          )}
        </div>
      )}

      {problem && (
        <div
          id={problemId}
          role="alert"
          className="flex items-start gap-2 rounded-lg border border-destructive/30 bg-destructive/10 p-3 text-sm text-destructive"
        >
          <AlertCircle className="mt-0.5 size-4 shrink-0" aria-hidden="true" />
          <div className="space-y-2">
            <p>{problem}</p>
            {error?.retryable && file && check?.ok && !uploading && (
              <Button type="button" size="sm" variant="outline" onClick={submit}>
                Try again
              </Button>
            )}
          </div>
        </div>
      )}

      <div className="flex flex-wrap items-center gap-2">
        <Button type="button" onClick={submit} disabled={!canSubmit} aria-busy={uploading}>
          {uploading ? (
            <Loader2 className="size-4 animate-spin" aria-hidden="true" />
          ) : (
            <Upload className="size-4" aria-hidden="true" />
          )}
          {uploading ? "Uploading…" : "Upload paper"}
        </Button>
        {uploading && (
          <Button type="button" variant="outline" onClick={cancelUpload}>
            Cancel upload
          </Button>
        )}
      </div>

      {/* One polite region for the whole component, so changes are announced once. */}
      <p role="status" aria-live="polite" className="flex items-center gap-1.5 text-sm text-muted-foreground">
        {status.startsWith("Uploaded") && (
          <CheckCircle2 className="size-4 text-emerald-600" aria-hidden="true" />
        )}
        {status}
      </p>
    </div>
  );
}
