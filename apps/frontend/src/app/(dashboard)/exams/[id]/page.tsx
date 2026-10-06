"use client";

import { useEffect, useState, use } from "react";
import Link from "next/link";
import { useRouter } from "next/navigation";
import {
  ArrowLeft,
  BookOpen,
  Clock,
  Loader2,
  Calendar,
  Edit,
  CheckCircle2,
  RotateCcw,
  Archive,
  Trash2,
  AlertCircle,
  Shield,
  Users,
} from "lucide-react";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Card } from "@/components/ui/card";
import { QuestionManager } from "@/components/question-manager";
import { AssignmentManager } from "@/components/assignment-manager";
import { ProtectedRoute } from "@/components/auth/protected-route";
import { EditExamDialog } from "@/components/edit-exam-dialog";
import { ConfirmActionDialog, type ConfirmActionType } from "@/components/confirm-action-dialog";
import { examsApi, questionsApi, assignmentsApi, ApiError } from "@/services/api";
import { useAuthStore } from "@/store/auth-store";
import { DESIGN_TOKENS } from "@/lib/constants";
import type { Exam, Question, Assignment, ExamStatus } from "@/types/exam-types";

const statusStyles: Record<ExamStatus, string> = {
  DRAFT: "bg-muted text-muted-foreground border-border",
  PUBLISHED:
    "bg-emerald-100 text-emerald-800 dark:bg-emerald-950 dark:text-emerald-200 border-emerald-300 dark:border-emerald-800 font-semibold",
  ARCHIVED:
    "bg-amber-100 text-amber-800 dark:bg-amber-950 dark:text-amber-200 border-amber-300 dark:border-amber-800",
};

export default function ExamDetailPage({ params }: { params: Promise<{ id: string }> }) {
  const resolvedParams = use(params);
  const examId = resolvedParams.id;
  const router = useRouter();
  const { user } = useAuthStore();
  const isAdmin = user?.role === "ADMIN";

  const [exam, setExam] = useState<Exam | null>(null);
  const [questions, setQuestions] = useState<Question[]>([]);
  const [assignments, setAssignments] = useState<Assignment[]>([]);
  const [loading, setLoading] = useState(true);

  // Lifecycle & Dialog states
  const [editOpen, setEditOpen] = useState(false);
  const [confirmAction, setConfirmAction] = useState<ConfirmActionType | null>(null);
  const [actionLoading, setActionLoading] = useState(false);
  const [feedback, setFeedback] = useState<{
    type: "success" | "error";
    message: string;
  } | null>(null);

  const showFeedback = (type: "success" | "error", message: string) => {
    setFeedback({ type, message });
    setTimeout(() => {
      setFeedback(null);
    }, 6000);
  };

  useEffect(() => {
    async function loadData() {
      setLoading(true);
      try {
        const [examData, questionsData, assignmentsData] = await Promise.allSettled([
          examsApi.getExam(examId),
          questionsApi.getQuestions(examId),
          assignmentsApi.getExamAssignments(examId),
        ]);

        if (examData.status === "fulfilled") {
          setExam(examData.value);
        } else {
          setExam(null);
        }

        if (questionsData.status === "fulfilled") {
          setQuestions(questionsData.value || []);
        } else {
          setQuestions([]);
        }

        if (assignmentsData.status === "fulfilled") {
          setAssignments(assignmentsData.value || []);
        } else {
          setAssignments([]);
        }
      } finally {
        setLoading(false);
      }
    }

    loadData();
  }, [examId]);

  const handleReloadAssignments = async () => {
    try {
      const [assignmentsData, examData] = await Promise.allSettled([
        assignmentsApi.getExamAssignments(examId),
        examsApi.getExam(examId),
      ]);
      if (assignmentsData.status === "fulfilled") {
        setAssignments(assignmentsData.value);
      }
      if (examData.status === "fulfilled") {
        setExam(examData.value);
      }
    } catch {
      // ignore
    }
  };

  const handleUpdateExam = async (id: string, payload: Partial<Exam>): Promise<Exam> => {
    try {
      const updated = await examsApi.updateExam(id, payload);
      setExam(updated);
      showFeedback("success", "Assessment details updated successfully.");
      return updated;
    } catch (err) {
      const msg = err instanceof ApiError ? err.message : "Failed to update exam";
      showFeedback("error", msg);
      throw err;
    }
  };

  const handleConfirmAction = async () => {
    if (!confirmAction || !exam) return;

    setActionLoading(true);
    try {
      if (confirmAction === "publish") {
        const updated = await examsApi.publishExam(exam.id);
        setExam(updated);
        showFeedback("success", `Assessment "${exam.title}" published successfully.`);
      } else if (confirmAction === "unpublish") {
        const updated = await examsApi.unpublishExam(exam.id);
        setExam(updated);
        showFeedback("success", `Assessment "${exam.title}" reverted to draft.`);
      } else if (confirmAction === "archive") {
        const updated = await examsApi.archiveExam(exam.id);
        setExam(updated);
        showFeedback("success", `Assessment "${exam.title}" archived.`);
      } else if (confirmAction === "delete") {
        await examsApi.deleteExam(exam.id);
        router.push("/exams");
        return;
      }
      setConfirmAction(null);
    } catch (err: unknown) {
      const msg = err instanceof ApiError ? err.message : "Action could not be completed.";
      showFeedback("error", msg);
    } finally {
      setActionLoading(false);
    }
  };

  if (loading) {
    return (
      <ProtectedRoute allowedRoles={["TEACHER", "ADMIN"]}>
        <div className="flex h-96 items-center justify-center">
          <Loader2 className="size-8 animate-spin text-primary" />
        </div>
      </ProtectedRoute>
    );
  }

  if (!exam) {
    return (
      <ProtectedRoute allowedRoles={["TEACHER", "ADMIN"]}>
        <div className="min-h-screen bg-background pb-16">
          <div className={DESIGN_TOKENS.layout.container}>
            <div className="py-8">
              <Link
                href="/exams"
                className="mb-6 inline-flex items-center gap-1.5 text-sm text-muted-foreground hover:text-foreground transition-colors"
              >
                <ArrowLeft className="size-4" />
                Back to all exams
              </Link>
              <Card className="flex flex-col items-center justify-center p-12 text-center border-dashed">
                <BookOpen className="size-12 text-muted-foreground/40 mb-3" />
                <h2 className="text-xl font-bold">Assessment Not Found</h2>
                <p className="text-sm text-muted-foreground mt-1 max-w-sm">
                  The requested assessment could not be found or you do not have permission to view it.
                </p>
                <Link href="/exams" className="mt-6">
                  <Button size="sm">Return to Assessments</Button>
                </Link>
              </Card>
            </div>
          </div>
        </div>
      </ProtectedRoute>
    );
  }

  return (
    <ProtectedRoute allowedRoles={["TEACHER", "ADMIN"]}>
      <div className="min-h-screen bg-background pb-16">
        <div className={DESIGN_TOKENS.layout.container}>
          <div className="py-6 sm:py-8 flex flex-col gap-8">
            {/* Top Navigation & Breadcrumbs */}
            <div>
              <Link
                href="/exams"
                className="mb-4 inline-flex items-center gap-1.5 text-sm text-muted-foreground hover:text-foreground transition-colors"
              >
                <ArrowLeft className="size-4" />
                Back to all exams
              </Link>

              {/* Feedback alert */}
              {feedback && (
                <div
                  role="alert"
                  className={`mb-4 flex items-center justify-between gap-3 rounded-lg border p-4 text-sm font-medium transition-all ${
                    feedback.type === "success"
                      ? "bg-emerald-50 dark:bg-emerald-950/40 border-emerald-300 dark:border-emerald-800 text-emerald-900 dark:text-emerald-200"
                      : "bg-destructive/10 border-destructive/20 text-destructive"
                  }`}
                >
                  <div className="flex items-center gap-2">
                    {feedback.type === "success" ? (
                      <CheckCircle2 className="size-4 text-emerald-600 dark:text-emerald-400 shrink-0" />
                    ) : (
                      <AlertCircle className="size-4 text-destructive shrink-0" />
                    )}
                    <span>{feedback.message}</span>
                  </div>
                  <button
                    type="button"
                    onClick={() => setFeedback(null)}
                    className="text-xs opacity-70 hover:opacity-100 ml-4 underline"
                  >
                    Dismiss
                  </button>
                </div>
              )}

              {/* Exam Header */}
              {exam && (
                <div className="flex flex-col gap-4 border-b pb-6">
                  <div className="flex flex-col md:flex-row md:items-start justify-between gap-4">
                    <div className="space-y-1">
                      <div className="flex flex-wrap items-center gap-3">
                        <h1 className={DESIGN_TOKENS.typography.h1}>{exam.title}</h1>
                        <Badge
                          variant="outline"
                          className={`${statusStyles[exam.status]} px-2.5 py-0.5 text-xs font-semibold`}
                        >
                          {exam.status}
                        </Badge>
                        {isAdmin && (
                          <Badge variant="secondary" className="text-xs">
                            <Shield className="mr-1 size-3 text-primary" />
                            Admin View
                          </Badge>
                        )}
                      </div>
                      <p className="mt-1 text-sm text-muted-foreground max-w-3xl">
                        {exam.description || "No description provided."}
                      </p>
                    </div>

                    {/* Action Toolbar */}
                    <div className="flex flex-wrap items-center gap-2 shrink-0">
                      {exam.status === "DRAFT" && (
                        <>
                          <Button
                            variant="outline"
                            size="sm"
                            onClick={() => setEditOpen(true)}
                            className="gap-1.5"
                          >
                            <Edit className="size-4" />
                            Edit Exam
                          </Button>
                          <Button
                            size="sm"
                            onClick={() => setConfirmAction("publish")}
                            className="gap-1.5 bg-emerald-600 hover:bg-emerald-700 text-white"
                          >
                            <CheckCircle2 className="size-4" />
                            Publish
                          </Button>
                          <Button
                            variant="outline"
                            size="sm"
                            onClick={() => setConfirmAction("delete")}
                            className="gap-1.5 text-destructive hover:bg-destructive/10 hover:text-destructive"
                          >
                            <Trash2 className="size-4" />
                            Delete
                          </Button>
                        </>
                      )}

                      {exam.status === "PUBLISHED" && (
                        <>
                          <Button
                            variant="outline"
                            size="sm"
                            onClick={() => setEditOpen(true)}
                            className="gap-1.5"
                          >
                            <Edit className="size-4" />
                            Edit Details
                          </Button>
                          <Button
                            variant="outline"
                            size="sm"
                            onClick={() => setConfirmAction("unpublish")}
                            className="gap-1.5 border-amber-300/80 bg-amber-50/50 text-amber-800 hover:bg-amber-100 hover:text-amber-950 focus-visible:ring-amber-500/30 active:bg-amber-200/60 dark:border-amber-800/80 dark:bg-amber-950/30 dark:text-amber-300 dark:hover:bg-amber-900/50 dark:hover:text-amber-100"
                          >
                            <RotateCcw className="size-4" />
                            Revert to Draft
                          </Button>
                          <Button
                            variant="outline"
                            size="sm"
                            onClick={() => setConfirmAction("archive")}
                            className="gap-1.5 text-muted-foreground hover:text-foreground"
                          >
                            <Archive className="size-4" />
                            Archive
                          </Button>
                        </>
                      )}

                      {exam.status === "ARCHIVED" && (
                        <div className="flex items-center gap-2 rounded-md bg-amber-50 dark:bg-amber-950/40 border border-amber-200 dark:border-amber-800 px-3 py-1.5 text-xs text-amber-800 dark:text-amber-300">
                          <Archive className="size-3.5" />
                          Archived (Read-Only)
                        </div>
                      )}
                    </div>
                  </div>

                  {/* Metadata Row */}
                  <div className="flex flex-wrap items-center gap-6 pt-2 text-xs font-medium text-muted-foreground border-t">
                    <span className="flex items-center gap-1.5">
                      <BookOpen className="size-4 text-primary" />
                      Subject: <strong className="text-foreground">{exam.subject}</strong>
                    </span>
                    <span className="flex items-center gap-1.5">
                      <Clock className="size-4 text-primary" />
                      Duration: <strong className="text-foreground">{exam.durationMins} minutes</strong>
                    </span>
                    <span className="flex items-center gap-1.5">
                      <Users className="size-4 text-primary" />
                      Assigned: <strong className="text-foreground">{exam.assignedCount ?? assignments.filter(a => a.status === "ASSIGNED" || a.status === "ACTIVE").length} students</strong>
                    </span>
                    {exam.createdAt && (
                      <span className="flex items-center gap-1.5">
                        <Calendar className="size-4" />
                        Created: {new Date(exam.createdAt).toLocaleDateString()}
                      </span>
                    )}
                    {exam.publishedAt && (
                      <span className="flex items-center gap-1.5">
                        <CheckCircle2 className="size-4 text-emerald-600" />
                        Published: {new Date(exam.publishedAt).toLocaleDateString()}
                      </span>
                    )}
                  </div>
                </div>
              )}
            </div>

            {/* Question Studio */}
            <QuestionManager
              examId={examId}
              examStatus={exam?.status}
              questions={questions}
              onQuestionsChange={(updated) => setQuestions(updated)}
            />

            {/* Candidate Assignments Section */}
            <div className="mt-4">
              <AssignmentManager
                examId={examId}
                assignments={assignments}
                onAssignmentChanged={handleReloadAssignments}
              />
            </div>
          </div>
        </div>

        {/* Edit Exam Dialog */}
        {exam && (
          <EditExamDialog
            exam={exam}
            open={editOpen}
            onOpenChange={setEditOpen}
            onSubmit={async (id, payload) => {
              return await handleUpdateExam(id, payload);
            }}
          />
        )}

        {/* Confirmation Dialog */}
        {exam && (
          <ConfirmActionDialog
            action={confirmAction}
            examTitle={exam.title}
            open={Boolean(confirmAction)}
            onOpenChange={(open) => !open && setConfirmAction(null)}
            onConfirm={handleConfirmAction}
            loading={actionLoading}
          />
        )}
      </div>
    </ProtectedRoute>
  );
}
