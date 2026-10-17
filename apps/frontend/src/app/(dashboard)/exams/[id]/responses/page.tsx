"use client";

import { use } from "react";
import { ProtectedRoute } from "@/components/auth/protected-route";
import { ResponseOverview } from "@/components/response-review/response-overview";
import { DESIGN_TOKENS } from "@/lib/constants";

export default function ExamResponsesPage({ params }: { params: Promise<{ id: string }> }) {
  const { id } = use(params);
  return (
    <ProtectedRoute allowedRoles={["TEACHER", "ADMIN"]}>
      <div className="min-h-screen bg-background pb-16">
        <div className={DESIGN_TOKENS.layout.container}>
          <div className="py-6 sm:py-8">
            <ResponseOverview key={id} examId={id} />
          </div>
        </div>
      </div>
    </ProtectedRoute>
  );
}
