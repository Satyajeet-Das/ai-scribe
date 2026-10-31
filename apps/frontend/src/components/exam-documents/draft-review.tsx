"use client";

import Link from "next/link";
import { useCallback } from "react";
import { AlertCircle, ArrowLeft, Ban, Clock, Loader2 } from "lucide-react";
import { Card } from "@/components/ui/card";
import { useExamDocuments } from "@/hooks/use-exam-documents";
import { useRemoteResource } from "@/hooks/use-remote-resource";
import { failureAdvice } from "@/lib/document-errors";
import { examsApi } from "@/services/api";
import { DraftWorkspace } from "./draft-workspace";
import { STAGE_LABEL, STATUS_LABEL } from "./document-status";

interface DraftReviewProps {
  examId: string;
  documentId: string;
}

/**
 * Review of one document's extracted questions. It watches the document, and
 * shows the editor only once the server says a draft exists. While the paper is
 * still being read it shows where that has got to; if reading failed it says so.
 */
export function DraftReview({ examId, documentId }: DraftReviewProps) {
  const docs = useExamDocuments(examId);
  // The hook drops an answer that arrives after the exam changed or the page closed.
  const loadExam = useCallback(() => examsApi.getExam(examId), [examId]);
  const exam = useRemoteResource(`exam:${examId}`, loadExam);
  const doc = docs.documents.find((d) => d.id === documentId);

  const back = (
    <Link
      href={`/exams/${encodeURIComponent(examId)}`}
      className="mb-4 inline-flex items-center gap-1.5 text-sm text-muted-foreground transition-colors hover:text-foreground"
    >
      <ArrowLeft className="size-4" aria-hidden="true" />
      Back to the exam
    </Link>
  );

  let body: React.ReactNode;
  if (docs.loading) {
    body = (
      <p role="status" className="flex items-center gap-2 text-sm text-muted-foreground">
        <Loader2 className="size-4 animate-spin" aria-hidden="true" />
        Loading…
      </p>
    );
  } else if (!doc) {
    body = (
      <Card className="space-y-2 p-6" role="alert">
        <p className="flex items-center gap-2 font-medium">
          <AlertCircle className="size-5 text-destructive" aria-hidden="true" />
          {docs.error && docs.error.kind !== "notFound" ? docs.error.message : "This paper could not be found. It may have been deleted."}
        </p>
      </Card>
    );
  } else if (doc.status === "QUEUED" || doc.status === "PROCESSING") {
    body = (
      <Card className="space-y-2 p-6" role="status" aria-live="polite">
        <p className="flex items-center gap-2 font-medium">
          <Clock className="size-5" aria-hidden="true" />
          {STATUS_LABEL[doc.status]}
          {doc.stage ? `: ${STAGE_LABEL[doc.stage].toLowerCase()}…` : "…"}
        </p>
        <p className="text-sm text-muted-foreground">
          The questions are not ready to review yet. You can wait here, or leave and come back from the exam page; this page will show them as soon
          as they are ready.
        </p>
      </Card>
    );
  } else if (doc.status === "FAILED" || doc.status === "CANCELLED") {
    body = (
      <Card className="space-y-2 p-6" role="alert">
        <p className="flex items-center gap-2 font-medium">
          {doc.status === "FAILED" ? <AlertCircle className="size-5 text-destructive" aria-hidden="true" /> : <Ban className="size-5" aria-hidden="true" />}
          {doc.status === "FAILED" ? "This paper could not be read." : "Reading this paper was cancelled."}
        </p>
        {doc.status === "FAILED" && <p className="text-sm">{doc.failureMessage}</p>}
        <p className="text-sm text-muted-foreground">{doc.status === "FAILED" ? failureAdvice(doc.failureCode) : "There is no draft to review."} Go back to the exam to try again.</p>
      </Card>
    );
  } else {
    body = <DraftWorkspace examId={examId} documentId={documentId} exam={exam.data} />;
  }

  return (
    <div>
      {back}
      <header className="mb-6 space-y-1 border-b pb-4">
        <h1 className="text-2xl font-extrabold tracking-tight sm:text-3xl">Review extracted questions</h1>
        <p className="text-sm text-muted-foreground">
          {doc ? <>From “{doc.filename}”, {doc.pageCount} {doc.pageCount === 1 ? "page" : "pages"}</> : "Exam paper"}
          {exam.data ? <> · for “{exam.data.title}”</> : null}
        </p>
      </header>
      {docs.error && doc && (
        <p role="alert" className="mb-4 rounded-md border border-amber-300 bg-amber-50 p-2 text-xs text-amber-900 dark:bg-amber-950 dark:text-amber-200">
          {docs.error.message} What is shown may be out of date.
        </p>
      )}
      {body}
    </div>
  );
}
