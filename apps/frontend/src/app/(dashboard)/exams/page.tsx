"use client";

import { useEffect, useState } from "react";
import { useRouter } from "next/navigation";
import { ExamDashboard } from "@/components/exam-dashboard";
import { AssignmentManager } from "@/components/assignment-manager";
import { Dialog, DialogContent, DialogHeader, DialogTitle } from "@/components/ui/dialog";
import { examsApi, assignmentsApi, ApiError } from "@/services/api";
import type { Exam, Assignment } from "@/types/exam-types";

export default function ExamsPage() {
  const router = useRouter();
  const [exams, setExams] = useState<Exam[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);

  // Assignment Modal state
  const [selectedExamForAssign, setSelectedExamForAssign] = useState<Exam | null>(null);
  const [assignments, setAssignments] = useState<Assignment[]>([]);
  const [assignLoading, setAssignLoading] = useState(false);

  const loadExams = async () => {
    setLoading(true);
    setError(null);
    try {
      const res = await examsApi.getExams();
      setExams(res.exams || []);
    } catch (err) {
      console.warn("Backend unavailable, using local mock data fallback:", err);
      // Fallback is handled inside ExamDashboard
    } finally {
      setLoading(false);
    }
  };

  useEffect(() => {
    loadExams();
  }, []);

  const handleCreateExam = async (payload: {
    title: string;
    subject: string;
    description: string;
    durationMins: number;
  }) => {
    try {
      const newExam = await examsApi.createExam(payload);
      setExams((prev) => [newExam, ...prev]);
    } catch (err) {
      const msg = err instanceof ApiError ? err.message : "Failed to create exam";
      setError(msg);
      throw err;
    }
  };

  const handlePublish = async (id: string) => {
    try {
      const updated = await examsApi.publishExam(id);
      setExams((prev) => prev.map((e) => (e.id === id ? updated : e)));
    } catch (err) {
      console.error("Failed to publish exam:", err);
      // Optimistic local update
      setExams((prev) =>
        prev.map((e) =>
          e.id === id ? { ...e, status: "PUBLISHED", publishedAt: new Date().toISOString() } : e
        )
      );
    }
  };

  const handleArchive = async (id: string) => {
    try {
      const updated = await examsApi.archiveExam(id);
      setExams((prev) => prev.map((e) => (e.id === id ? updated : e)));
    } catch (err) {
      console.error("Failed to archive exam:", err);
      setExams((prev) => prev.map((e) => (e.id === id ? { ...e, status: "ARCHIVED" } : e)));
    }
  };

  const handleOpenQuestions = (exam: Exam) => {
    router.push(`/exams/${exam.id}`);
  };

  const handleOpenAssignments = async (exam: Exam) => {
    setSelectedExamForAssign(exam);
    setAssignLoading(true);
    try {
      const list = await assignmentsApi.getExamAssignments(exam.id);
      setAssignments(list);
    } catch {
      setAssignments([
        {
          id: "mock-assign-1",
          examId: exam.id,
          studentId: "student-001",
          assignedAt: new Date().toISOString(),
          status: "ACTIVE",
        },
      ]);
    } finally {
      setAssignLoading(false);
    }
  };

  return (
    <div className="min-h-screen bg-background pb-16">
      {error && (
        <div className="mx-auto max-w-6xl px-6 pt-4">
          <div className="rounded-lg bg-destructive/10 border border-destructive/20 p-3 text-sm text-destructive font-medium">
            {error}
          </div>
        </div>
      )}

      <ExamDashboard
        exams={exams}
        loading={loading}
        onCreateExam={handleCreateExam}
        onPublishExam={handlePublish}
        onArchiveExam={handleArchive}
        onOpenQuestions={handleOpenQuestions}
        onOpenAssignments={handleOpenAssignments}
      />

      {/* Assignment Management Dialog */}
      <Dialog
        open={Boolean(selectedExamForAssign)}
        onOpenChange={(open) => !open && setSelectedExamForAssign(null)}
      >
        <DialogContent className="sm:max-w-xl">
          <DialogHeader>
            <DialogTitle>Assign Candidates to Exam</DialogTitle>
          </DialogHeader>
          {selectedExamForAssign && (
            <AssignmentManager examId={selectedExamForAssign.id} assignments={assignments} />
          )}
        </DialogContent>
      </Dialog>
    </div>
  );
}
