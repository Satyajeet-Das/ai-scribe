"use client";

import { useMemo, useState } from "react";
import {
  BookOpen,
  Check,
  ChevronLeft,
  ChevronRight,
  FilePlus2,
  Plus,
  Search,
  SlidersHorizontal,
  UserCheck,
  Archive,
  RotateCcw,
  Edit,
  Trash2,
  Shield,
  Clock,
  Calendar,
  AlertCircle,
  CheckCircle2,
} from "lucide-react";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { Input } from "@/components/ui/input";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import { ExamCardsSkeleton, StatsSkeleton } from "@/components/ui/exam-skeleton";
import { CreateExamDialog } from "@/components/create-exam-dialog";
import { EditExamDialog } from "@/components/edit-exam-dialog";
import { ConfirmActionDialog, type ConfirmActionType } from "@/components/confirm-action-dialog";
import { useDebounce } from "@/hooks/use-debounce";
import { useAuthStore } from "@/store/auth-store";
import { DESIGN_TOKENS, DEFAULT_PAGE_SIZE } from "@/lib/constants";
import type { Exam, ExamStatus } from "@/types/exam-types";

const statusStyles: Record<ExamStatus, string> = {
  DRAFT: "bg-muted text-muted-foreground border-border",
  PUBLISHED:
    "bg-emerald-100 text-emerald-800 dark:bg-emerald-950 dark:text-emerald-200 border-emerald-300 dark:border-emerald-800 font-semibold",
  ARCHIVED:
    "bg-amber-100 text-amber-800 dark:bg-amber-950 dark:text-amber-200 border-amber-300 dark:border-amber-800",
};

interface ExamDashboardProps {
  exams?: Exam[];
  loading?: boolean;
  total?: number;
  onCreateExam?: (payload: {
    title: string;
    subject: string;
    description: string;
    durationMins: number;
  }) => Promise<Exam>;
  onUpdateExam?: (id: string, payload: Partial<Exam>) => Promise<Exam>;
  onPublishExam?: (id: string) => Promise<Exam>;
  onUnpublishExam?: (id: string) => Promise<Exam>;
  onArchiveExam?: (id: string) => Promise<Exam>;
  onDeleteExam?: (id: string) => Promise<void>;
  onOpenQuestions?: (exam: Exam) => void;
  onOpenAssignments?: (exam: Exam) => void;
  onSearchChange?: (query: string) => void;
  onStatusChange?: (status: ExamStatus | "ALL") => void;
}

export function ExamDashboard({
  exams = [],
  loading = false,
  total,
  onCreateExam,
  onUpdateExam,
  onPublishExam,
  onUnpublishExam,
  onArchiveExam,
  onDeleteExam,
  onOpenQuestions,
  onOpenAssignments,
  onSearchChange,
  onStatusChange,
}: ExamDashboardProps) {
  const { user } = useAuthStore();
  const isAdmin = user?.role === "ADMIN";

  // Search & Filter state
  const [query, setQuery] = useState("");
  const debouncedQuery = useDebounce(query, 300);
  const [status, setStatus] = useState<"ALL" | ExamStatus>("ALL");

  // Pagination state
  const [currentPage, setCurrentPage] = useState(1);
  const pageSize = DEFAULT_PAGE_SIZE;

  // Dialog States
  const [createOpen, setCreateOpen] = useState(false);
  const [editingExam, setEditingExam] = useState<Exam | null>(null);

  // Confirm Action State
  const [confirmAction, setConfirmAction] = useState<ConfirmActionType | null>(null);
  const [targetExam, setTargetExam] = useState<Exam | null>(null);
  const [actionLoading, setActionLoading] = useState(false);

  // Feedback notifications
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

  // Debounced search and status filter
  const filtered = useMemo(() => {
    return exams.filter((exam) => {
      const matchesStatus = status === "ALL" || exam.status === status;
      const matchesQuery =
        debouncedQuery === "" ||
        `${exam.title} ${exam.subject} ${exam.description || ""}`
          .toLowerCase()
          .includes(debouncedQuery.toLowerCase());
      return matchesStatus && matchesQuery;
    });
  }, [exams, debouncedQuery, status]);

  // Paginated slice
  const effectiveTotal = total ?? filtered.length;
  const totalPages = Math.max(1, Math.ceil(filtered.length / pageSize));
  const safePage = Math.min(currentPage, totalPages);
  const paginatedExams = useMemo(() => {
    const start = (safePage - 1) * pageSize;
    return filtered.slice(start, start + pageSize);
  }, [filtered, safePage, pageSize]);

  // Handle action confirmation
  const handleConfirmAction = async () => {
    if (!confirmAction || !targetExam) return;

    setActionLoading(true);
    try {
      if (confirmAction === "publish" && onPublishExam) {
        await onPublishExam(targetExam.id);
        showFeedback("success", `Assessment "${targetExam.title}" published successfully.`);
      } else if (confirmAction === "unpublish" && onUnpublishExam) {
        await onUnpublishExam(targetExam.id);
        showFeedback("success", `Assessment "${targetExam.title}" reverted to draft.`);
      } else if (confirmAction === "archive" && onArchiveExam) {
        await onArchiveExam(targetExam.id);
        showFeedback("success", `Assessment "${targetExam.title}" archived.`);
      } else if (confirmAction === "delete" && onDeleteExam) {
        await onDeleteExam(targetExam.id);
        showFeedback("success", `Assessment "${targetExam.title}" deleted.`);
      }
      setTargetExam(null);
      setConfirmAction(null);
    } catch (err: unknown) {
      const msg = err instanceof Error ? err.message : "Action could not be completed.";
      showFeedback("error", msg);
      throw err;
    } finally {
      setActionLoading(false);
    }
  };

  const openConfirm = (exam: Exam, action: ConfirmActionType) => {
    setTargetExam(exam);
    setConfirmAction(action);
  };

  return (
    <div className="flex flex-col gap-6 w-full">
      {/* Page Header */}
      <header className="flex flex-col gap-4 sm:flex-row sm:items-center sm:justify-between border-b pb-6">
        <div>
          <div className="flex items-center gap-2">
            <h1 className={DESIGN_TOKENS.typography.h1}>Exam Management</h1>
            <Badge variant="outline" className="text-xs font-semibold">
              {isAdmin ? (
                <span className="flex items-center gap-1 text-primary">
                  <Shield className="size-3" />
                  Administrator
                </span>
              ) : (
                "Educator"
              )}
            </Badge>
          </div>
          <p className={DESIGN_TOKENS.typography.muted}>
            Author accessible assessments, enforce lifecycle states, and allocate student testing accommodations.
          </p>
        </div>

        <Button className="shrink-0 font-medium" onClick={() => setCreateOpen(true)}>
          <Plus className="mr-1.5 size-4" />
          Create New Exam
        </Button>
      </header>

      {/* Action Feedback Banner */}
      {feedback && (
        <div
          role="alert"
          className={`flex items-start gap-3 rounded-lg border p-4 text-sm font-medium transition-all ${
            feedback.type === "success"
              ? "bg-emerald-50 dark:bg-emerald-950/40 border-emerald-300 dark:border-emerald-800 text-emerald-900 dark:text-emerald-200"
              : "bg-destructive/10 border-destructive/20 text-destructive"
          }`}
        >
          {feedback.type === "success" ? (
            <CheckCircle2 className="size-5 shrink-0 text-emerald-600 dark:text-emerald-400 mt-0.5" />
          ) : (
            <AlertCircle className="size-5 shrink-0 text-destructive mt-0.5" />
          )}
          <div className="flex-1">
            <p>{feedback.message}</p>
          </div>
          <Button
            variant="ghost"
            size="sm"
            className="size-7 p-0 -mr-1.5 -mt-1 text-muted-foreground hover:text-foreground"
            onClick={() => setFeedback(null)}
          >
            ×
          </Button>
        </div>
      )}

      {/* Metric Cards or Loading Stats Skeleton */}
      {loading ? (
        <StatsSkeleton />
      ) : (
        <div className="grid gap-4 grid-cols-2 lg:grid-cols-4">
          <StatCard
            label="Total Assessments"
            value={effectiveTotal}
            detail="Managed in catalog"
            icon={BookOpen}
          />
          <StatCard
            label="Published Exams"
            value={exams.filter((item) => item.status === "PUBLISHED").length}
            detail="Active for allocation"
            icon={CheckCircle2}
          />
          <StatCard
            label="Drafts in Progress"
            value={exams.filter((item) => item.status === "DRAFT").length}
            detail="Editable drafts"
            icon={FilePlus2}
          />
          <StatCard
            label="Archived Records"
            value={exams.filter((item) => item.status === "ARCHIVED").length}
            detail="Permanently frozen"
            icon={Archive}
          />
        </div>
      )}

      {/* Filter and Search Bar */}
      <div className="flex flex-col gap-3 border-b pb-5 md:flex-row md:items-center">
        <div className="relative flex-1">
          <Search
            aria-hidden="true"
            className="absolute left-3 top-1/2 -translate-y-1/2 text-muted-foreground size-4"
          />
          <Input
            className="pl-9"
            aria-label="Search assessments"
            placeholder="Search exams by title, subject, or description..."
            value={query}
            onChange={(e) => {
              setQuery(e.target.value);
              setCurrentPage(1);
              onSearchChange?.(e.target.value);
            }}
          />
        </div>

        <div className="flex items-center gap-2">
          <Select
            value={status}
            onValueChange={(val) => {
              const newStatus = val as typeof status;
              setStatus(newStatus);
              setCurrentPage(1);
              onStatusChange?.(newStatus);
            }}
          >
            <SelectTrigger className="w-full md:w-48" aria-label="Filter by lifecycle status">
              <SlidersHorizontal className="size-4 mr-2 text-muted-foreground" />
              <SelectValue placeholder="All statuses" />
            </SelectTrigger>
            <SelectContent>
              <SelectItem value="ALL">All Statuses</SelectItem>
              <SelectItem value="DRAFT">Draft</SelectItem>
              <SelectItem value="PUBLISHED">Published</SelectItem>
              <SelectItem value="ARCHIVED">Archived</SelectItem>
            </SelectContent>
          </Select>
        </div>
      </div>

      {/* Exam Cards Grid or Skeleton */}
      {loading ? (
        <div data-testid="exam-loading-skeleton">
          <ExamCardsSkeleton count={pageSize} />
        </div>
      ) : filtered.length === 0 ? (
        <Card className="flex flex-col items-center justify-center p-12 text-center border-dashed">
          <div className="size-16 rounded-full bg-muted flex items-center justify-center mb-3">
            <BookOpen className="size-8 text-muted-foreground/60" />
          </div>
          <h3 className="text-lg font-semibold">No assessments found</h3>
          <p className="text-sm text-muted-foreground mt-1 max-w-sm">
            {query || status !== "ALL"
              ? "No exams match your search criteria. Try clearing filters or using another keyword."
              : "You haven't created any exams yet. Get started by drafting your first accessible exam."}
          </p>
          {query || status !== "ALL" ? (
            <Button
              variant="outline"
              size="sm"
              className="mt-4"
              onClick={() => {
                setQuery("");
                setStatus("ALL");
                setCurrentPage(1);
              }}
            >
              Reset Filters
            </Button>
          ) : (
            <Button size="sm" className="mt-4" onClick={() => setCreateOpen(true)}>
              <Plus className="mr-1.5 size-4" />
              Create First Exam
            </Button>
          )}
        </Card>
      ) : (
        <div className="flex flex-col gap-5">
          <div className="grid gap-4 md:grid-cols-2">
            {paginatedExams.map((exam) => (
              <Card
                key={exam.id}
                className="transition-all hover:shadow-md border border-border flex flex-col justify-between"
              >
                <CardHeader className="pb-3">
                  <div className="flex items-start justify-between gap-3">
                    <div className="space-y-1.5">
                      <div className="flex items-center gap-2 text-xs uppercase font-semibold tracking-wider text-muted-foreground">
                        <span>{exam.subject}</span>
                        <span>·</span>
                        <span className="flex items-center gap-1 font-normal lowercase">
                          <Clock className="size-3" />
                          {exam.durationMins} mins
                        </span>
                      </div>
                      <CardTitle className="text-xl font-bold">{exam.title}</CardTitle>
                    </div>
                    <Badge variant="outline" className={statusStyles[exam.status]}>
                      {exam.status}
                    </Badge>
                  </div>
                </CardHeader>

                <CardContent className="flex flex-col gap-4 flex-1 justify-between">
                  <p className="min-h-10 text-sm leading-6 text-muted-foreground line-clamp-2">
                    {exam.description || "No description provided for this assessment."}
                  </p>

                  <div className="flex flex-col gap-1.5 text-xs text-muted-foreground border-t pt-3">
                    <div className="flex items-center justify-between">
                      <span className="flex items-center gap-1">
                        <Calendar className="size-3" />
                        Created {new Date(exam.createdAt).toLocaleDateString()}
                      </span>
                      {exam.publishedAt && (
                        <span className="text-emerald-700 dark:text-emerald-400">
                          Published {new Date(exam.publishedAt).toLocaleDateString()}
                        </span>
                      )}
                    </div>
                    {isAdmin && (
                      <span className="text-[11px] text-muted-foreground/75 truncate">
                        Owner: {exam.createdBy}
                      </span>
                    )}
                  </div>

                  {/* Contextual Action Toolbar based on Lifecycle Rules */}
                  <div className="flex flex-wrap items-center gap-2 pt-2 border-t">
                    {/* Navigation Actions */}
                    <Button variant="outline" size="sm" onClick={() => onOpenQuestions?.(exam)}>
                      Questions
                      <ChevronRight className="ml-1 size-3.5" />
                    </Button>

                    <Button variant="outline" size="sm" onClick={() => onOpenAssignments?.(exam)}>
                      <UserCheck className="mr-1.5 size-3.5" />
                      Assign
                    </Button>

                    {/* DRAFT STATE: Edit, Publish, Delete */}
                    {exam.status === "DRAFT" && (
                      <>
                        <Button
                          variant="outline"
                          size="sm"
                          onClick={() => setEditingExam(exam)}
                        >
                          <Edit className="mr-1.5 size-3.5" />
                          Edit
                        </Button>

                        <Button
                          size="sm"
                          onClick={() => openConfirm(exam, "publish")}
                        >
                          <Check className="mr-1.5 size-3.5" />
                          Publish
                        </Button>

                        <Button
                          variant="ghost"
                          size="sm"
                          aria-label="Delete assessment"
                          className="text-muted-foreground hover:text-destructive"
                          onClick={() => openConfirm(exam, "delete")}
                        >
                          <Trash2 className="size-3.5" />
                          <span className="sr-only">Delete</span>
                        </Button>
                      </>
                    )}

                    {/* PUBLISHED STATE: Edit (non-structural), Unpublish, Archive */}
                    {exam.status === "PUBLISHED" && (
                      <>
                        <Button
                          variant="outline"
                          size="sm"
                          onClick={() => setEditingExam(exam)}
                        >
                          <Edit className="mr-1.5 size-3.5" />
                          Edit Info
                        </Button>

                        <Button
                          variant="outline"
                          size="sm"
                          aria-label="Revert to draft"
                          onClick={() => openConfirm(exam, "unpublish")}
                          className="text-amber-700 dark:text-amber-300 hover:bg-amber-50 dark:hover:bg-amber-950/40"
                        >
                          <RotateCcw className="mr-1.5 size-3.5" />
                          Unpublish
                        </Button>

                        <Button
                          variant="ghost"
                          size="sm"
                          className="text-muted-foreground hover:text-foreground"
                          onClick={() => openConfirm(exam, "archive")}
                        >
                          <Archive className="mr-1.5 size-3.5" />
                          Archive
                        </Button>
                      </>
                    )}

                    {/* ARCHIVED STATE: Read-only badge */}
                    {exam.status === "ARCHIVED" && (
                      <span className="inline-flex items-center gap-1.5 text-xs text-amber-700 dark:text-amber-300 font-medium py-1 px-2.5 rounded bg-amber-50 dark:bg-amber-950/40 border border-amber-200 dark:border-amber-800">
                        <Archive className="size-3.5" />
                        Archived (Read-Only)
                      </span>
                    )}
                  </div>
                </CardContent>
              </Card>
            ))}
          </div>

          {/* Pagination Controls */}
          {totalPages > 1 && (
            <div className="flex flex-col sm:flex-row items-center justify-between gap-3 border-t pt-4">
              <span className="text-xs text-muted-foreground">
                Showing <strong>{(safePage - 1) * pageSize + 1}</strong> to{" "}
                <strong>{Math.min(safePage * pageSize, filtered.length)}</strong> of{" "}
                <strong>{filtered.length}</strong> exams
              </span>

              <div className="flex items-center gap-2">
                <Button
                  variant="outline"
                  size="sm"
                  disabled={safePage <= 1}
                  onClick={() => setCurrentPage((p) => Math.max(1, p - 1))}
                  aria-label="Previous page"
                >
                  <ChevronLeft className="size-4 mr-1" />
                  Previous
                </Button>

                <span className="text-xs font-medium px-2">
                  Page {safePage} of {totalPages}
                </span>

                <Button
                  variant="outline"
                  size="sm"
                  disabled={safePage >= totalPages}
                  onClick={() => setCurrentPage((p) => Math.min(totalPages, p + 1))}
                  aria-label="Next page"
                >
                  Next
                  <ChevronRight className="size-4 ml-1" />
                </Button>
              </div>
            </div>
          )}
        </div>
      )}

      {/* Create Exam Dialog */}
      <CreateExamDialog
        open={createOpen}
        onOpenChange={setCreateOpen}
        onSubmit={async (payload) => {
          if (!onCreateExam) {
            throw new Error("Create exam handler is not configured");
          }
          const created = await onCreateExam(payload);
          showFeedback("success", `Assessment "${created.title}" drafted successfully.`);
          return created;
        }}
      />

      {/* Edit Exam Dialog */}
      <EditExamDialog
        exam={editingExam}
        open={Boolean(editingExam)}
        onOpenChange={(val) => !val && setEditingExam(null)}
        onSubmit={async (id, payload) => {
          if (!onUpdateExam) {
            throw new Error("Update exam handler is not configured");
          }
          const updated = await onUpdateExam(id, payload);
          showFeedback("success", `Assessment "${updated.title}" updated successfully.`);
          return updated;
        }}
      />

      {/* Confirmation Action Dialog (Publish, Unpublish, Archive, Delete) */}
      <ConfirmActionDialog
        action={confirmAction}
        examTitle={targetExam?.title || "Assessment"}
        open={Boolean(confirmAction && targetExam)}
        onOpenChange={(val) => {
          if (!val && !actionLoading) {
            setConfirmAction(null);
            setTargetExam(null);
          }
        }}
        onConfirm={handleConfirmAction}
        loading={actionLoading}
      />
    </div>
  );
}

function StatCard({
  label,
  value,
  detail,
  icon: Icon,
}: {
  label: string;
  value: number;
  detail: string;
  icon: React.ElementType;
}) {
  return (
    <Card className="border border-border">
      <CardHeader className="flex flex-row items-center justify-between space-y-0 pb-2">
        <CardTitle className="text-sm font-medium text-muted-foreground">{label}</CardTitle>
        <div className="flex size-8 items-center justify-center rounded-lg bg-muted text-muted-foreground">
          <Icon className="size-4" />
        </div>
      </CardHeader>
      <CardContent>
        <div className="text-2xl font-bold">{value}</div>
        <p className="text-xs text-muted-foreground mt-1">{detail}</p>
      </CardContent>
    </Card>
  );
}

export default ExamDashboard;
