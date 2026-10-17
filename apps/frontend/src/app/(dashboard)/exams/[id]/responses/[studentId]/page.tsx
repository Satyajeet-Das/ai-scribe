"use client";

import { use } from "react";
import { ProtectedRoute } from "@/components/auth/protected-route";
import { StudentResponseReview } from "@/components/response-review/student-response-review";
import { DESIGN_TOKENS } from "@/lib/constants";

export default function StudentResponsesPage({
  params,
}: {
  params: Promise<{ id: string; studentId: string }>;
}) {
  const { id, studentId } = use(params);
  return (
    <ProtectedRoute allowedRoles={["TEACHER", "ADMIN"]}>
      <div className="min-h-screen bg-background pb-16">
        <div className={DESIGN_TOKENS.layout.container}>
          <div className="py-6 sm:py-8">
            <StudentResponseReview examId={id} studentId={studentId} />
          </div>
        </div>
      </div>
    </ProtectedRoute>
  );
}
