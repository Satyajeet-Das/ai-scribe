"use client";

import { use } from "react";
import { ProtectedRoute } from "@/components/auth/protected-route";
import { DraftReview } from "@/components/exam-documents/draft-review";
import { DESIGN_TOKENS } from "@/lib/constants";

export default function ReviewExtractedQuestionsPage({
  params,
}: {
  params: Promise<{ id: string; documentId: string }>;
}) {
  const { id, documentId } = use(params);
  return (
    <ProtectedRoute allowedRoles={["TEACHER", "ADMIN"]}>
      <div className="min-h-screen bg-background pb-16">
        <div className={DESIGN_TOKENS.layout.containerNarrow}>
          <div className="py-6 sm:py-8">
            <DraftReview examId={id} documentId={documentId} />
          </div>
        </div>
      </div>
    </ProtectedRoute>
  );
}
