"use client";

import { useEffect, useState } from "react";
import { useRouter } from "next/navigation";
import { ExamDashboard } from "@/components/exam-dashboard";
import { AssignmentManager } from "@/components/assignment-manager";
import { ProtectedRoute } from "@/components/auth/protected-route";
import {
  Dialog,
  DialogContent,
  DialogHeader,
  DialogTitle,
  DialogDescription,
} from "@/components/ui/dialog";
import { examsApi, assignmentsApi, ApiError } from "@/services/api";
import { DESIGN_TOKENS } from "@/lib/constants";
import type { Exam, Assignment } from "@/types/exam-types";

export default function ExamsPage() {
  const router = useRouter();
  const [exams, setExams] = useState<Exam[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);

  // Assignment Modal state
  const [selectedExamForAssign, setSelectedExamForAssign] = useState<Exam | null>(null);
  const [assignments, setAssignments] = useState<Assignment[]>([]);
  const fetchExams = async () => {
    try {
      const res = await examsApi.getExams();
      setExams(res.exams || []);
    } catch (err) {
      console.warn("Backend unavailable, using local mock data fallback:", err);
    } finally {
      setLoading(false);
    }
  };

  useEffect(() => {
    fetchExams();
  }, []);

  const handleCreateExam = async (payload: {
    title: string;
    subject: string;
    description: string;
    durationMins: number;
  }): Promise<Exam> => {
    try {
      const newExam = await examsApi.createExam(payload);
      setExams((prev) => [newExam, ...prev]);
      setError(null);
      return newExam;
    } catch (err) {
      const msg = err instanceof ApiError ? err.message : "Failed to create exam";
      setError(msg);
      throw err;
    }
  };

  const handleUpdateExam = async (id: string, payload: Partial<Exam>): Promise<Exam> => {
    try {
      const updated = await examsApi.updateExam(id, payload);
      setExams((prev) => prev.map((e) => (e.id === id ? updated : e)));
      setError(null);
      return updated;
    } catch (err) {
      const msg = err instanceof ApiError ? err.message : "Failed to update exam";
      setError(msg);
      throw err;
    }
  };

  const handlePublish = async (id: string): Promise<Exam> => {
    try {
      const updated = await examsApi.publishExam(id);
      setExams((prev) => prev.map((e) => (e.id === id ? updated : e)));
      setError(null);
      return updated;
    } catch (err) {
      const msg = err instanceof ApiError ? err.message : "Failed to publish exam";
      setError(msg);
      throw err;
    }
  };

  const handleUnpublish = async (id: string): Promise<Exam> => {
    try {
      const updated = await examsApi.unpublishExam(id);
      setExams((prev) => prev.map((e) => (e.id === id ? updated : e)));
      setError(null);
      return updated;
    } catch (err) {
      const msg = err instanceof ApiError ? err.message : "Failed to revert exam to draft";
      setError(msg);
      throw err;
    }
  };

  const handleArchive = async (id: string): Promise<Exam> => {
    try {
      const updated = await examsApi.archiveExam(id);
      setExams((prev) => prev.map((e) => (e.id === id ? updated : e)));
      setError(null);
      return updated;
    } catch (err) {
      const msg = err instanceof ApiError ? err.message : "Failed to archive exam";
      setError(msg);
      throw err;
    }
  };

  const handleDelete = async (id: string): Promise<void> => {
    try {
      await examsApi.deleteExam(id);
      setExams((prev) => prev.filter((e) => e.id !== id));
      setError(null);
    } catch (err) {
      const msg = err instanceof ApiError ? err.message : "Failed to delete exam";
      setError(msg);
      throw err;
    }
  };

  const handleOpenQuestions = (exam: Exam) => {
    router.push(`/exams/${exam.id}`);
  };

  const handleOpenAssignments = async (exam: Exam) => {
    setSelectedExamForAssign(exam);
    try {
      const list = await assignmentsApi.getExamAssignments(exam.id);
      setAssignments(list);
    } catch {
      setAssignments([]);
    }
  };

  const handleAssignmentChanged = async () => {
    if (selectedExamForAssign) {
      try {
        const list = await assignmentsApi.getExamAssignments(selectedExamForAssign.id);
        setAssignments(list);
      } catch {
        // ignore
      }
    }
    fetchExams();
  };

  return (
    <ProtectedRoute allowedRoles={["TEACHER", "ADMIN"]}>
      <div className="min-h-screen bg-background pb-16">
        {error && (
          <div className={`${DESIGN_TOKENS.layout.container} pt-4`}>
            <div className="rounded-lg bg-destructive/10 border border-destructive/20 p-3 text-sm text-destructive font-medium flex items-center justify-between">
              <span>{error}</span>
              <button
                type="button"
                onClick={() => setError(null)}
                className="text-xs underline hover:no-underline ml-4"
              >
                Dismiss
              </button>
            </div>
          </div>
        )}

        <div className={DESIGN_TOKENS.layout.container}>
          <div className={DESIGN_TOKENS.layout.pageSection}>
            <ExamDashboard
              exams={exams}
              loading={loading}
              onCreateExam={handleCreateExam}
              onUpdateExam={handleUpdateExam}
              onPublishExam={handlePublish}
              onUnpublishExam={handleUnpublish}
              onArchiveExam={handleArchive}
              onDeleteExam={handleDelete}
              onOpenQuestions={handleOpenQuestions}
              onOpenAssignments={handleOpenAssignments}
            />
          </div>
        </div>

        {/* Assignment Management Dialog */}
        <Dialog
          open={Boolean(selectedExamForAssign)}
          onOpenChange={(open) => !open && setSelectedExamForAssign(null)}
        >
          <DialogContent className="sm:max-w-xl">
            <DialogHeader>
              <DialogTitle>Assign Candidates to Exam</DialogTitle>
              <DialogDescription>
                Allocate specific students to take this assessment in voice-first mode.
              </DialogDescription>
            </DialogHeader>
            {selectedExamForAssign && (
              <AssignmentManager
                examId={selectedExamForAssign.id}
                assignments={assignments}
                onAssignmentChanged={handleAssignmentChanged}
              />
            )}
          </DialogContent>
        </Dialog>
      </div>
    </ProtectedRoute>
  );
}
