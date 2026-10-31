"use client";

import { Card } from "@/components/ui/card";
import { useExamDocuments } from "@/hooks/use-exam-documents";
import type { ExamStatus } from "@/types/exam-types";
import { DocumentList } from "./document-list";
import { DocumentUpload } from "./document-upload";

interface ExamDocumentsPanelProps {
  examId: string;
  examStatus: ExamStatus | undefined;
}

/**
 * "Import from a PDF" on the exam page: upload a paper, watch it being read,
 * and open the draft to review. It never adds anything to the exam itself;
 * that happens only when the draft is approved on the review page.
 */
export function ExamDocumentsPanel({ examId, examStatus }: ExamDocumentsPanelProps) {
  const { documents, loading, error, reload } = useExamDocuments(examId);
  const isDraft = examStatus === "DRAFT";

  return (
    <section aria-labelledby="exam-documents-heading">
      <Card className="space-y-4 p-4 sm:p-6">
        <div className="space-y-1">
          <h2 id="exam-documents-heading" className="text-lg font-semibold tracking-tight">
            Import questions from a PDF
          </h2>
          <p className="text-sm text-muted-foreground">
            Upload an exam paper and the questions in it are read for you. You review and correct every
            question before anything is added to this exam, and nothing is published automatically.
          </p>
        </div>

        <DocumentUpload
          examId={examId}
          disabledReason={
            isDraft
              ? undefined
              : "Papers can only be uploaded while the exam is a draft. Revert the exam to draft to import questions."
          }
          onUploaded={reload}
        />

        <div className="border-t pt-4">
          <h3 className="mb-3 text-sm font-semibold">Uploaded papers</h3>
          <DocumentList
            examId={examId}
            documents={documents}
            loading={loading}
            refreshFailed={error?.message}
            onChanged={reload}
          />
        </div>
      </Card>
    </section>
  );
}
