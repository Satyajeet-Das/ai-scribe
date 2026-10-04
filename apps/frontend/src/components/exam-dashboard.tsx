"use client";

import { useMemo, useState } from "react";
import {
  BookOpen,
  Calendar,
  Check,
  ChevronLeft,
  ChevronRight,
  Clock,
  FilePlus2,
  Filter,
  Loader2,
  Plus,
  Search,
  SlidersHorizontal,
  UserCheck,
  Users,
  Archive,
  AlertCircle,
} from "lucide-react";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
  DialogTrigger,
} from "@/components/ui/dialog";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import { Textarea } from "@/components/ui/textarea";
import { ExamCardsSkeleton, StatsSkeleton } from "@/components/ui/exam-skeleton";
import { useDebounce } from "@/hooks/use-debounce";
import { CreateExamSchema } from "@/lib/validations";
import { DESIGN_TOKENS, DEFAULT_PAGE_SIZE } from "@/lib/constants";
import type { Exam, ExamStatus } from "@/types/exam-types";

const fallbackExams: Exam[] = [
  {
    id: "exam-1",
    title: "Foundations of Biology",
    subject: "Biology",
    description: "Cells, systems, and the living world.",
    durationMins: 45,
    status: "PUBLISHED",
    createdBy: "teacher-1",
    publishedAt: "2025-02-14",
    createdAt: "2025-02-01",
    updatedAt: "2025-02-14",
    candidates: 24,
    questions: 18,
  },
  {
    id: "exam-2",
    title: "Algebraic Reasoning",
    subject: "Mathematics",
    description: "Linear equations and proportional thinking.",
    durationMins: 60,
    status: "DRAFT",
    createdBy: "teacher-1",
    createdAt: "2025-02-12",
    updatedAt: "2025-02-18",
    candidates: 0,
    questions: 12,
  },
  {
    id: "exam-3",
    title: "Modern World History",
    subject: "History",
    description: "A guided review of twentieth-century events.",
    durationMins: 40,
    status: "ARCHIVED",
    createdBy: "teacher-1",
    createdAt: "2024-11-02",
    updatedAt: "2024-12-01",
    candidates: 31,
    questions: 20,
  },
];

const statusStyles: Record<ExamStatus, string> = {
  DRAFT: "bg-muted text-muted-foreground",
  PUBLISHED:
    "bg-emerald-100 text-emerald-800 dark:bg-emerald-950 dark:text-emerald-200 border-emerald-300 dark:border-emerald-800",
  ARCHIVED:
    "bg-amber-100 text-amber-800 dark:bg-amber-950 dark:text-amber-200 border-amber-300 dark:border-amber-800",
};

interface ExamDashboardProps {
  exams?: Exam[];
  loading?: boolean;
  onCreateExam?: (payload: {
    title: string;
    subject: string;
    description: string;
    durationMins: number;
  }) => Promise<void>;
  onPublishExam?: (id: string) => Promise<void>;
  onArchiveExam?: (id: string) => Promise<void>;
  onOpenQuestions?: (exam: Exam) => void;
  onOpenAssignments?: (exam: Exam) => void;
}

export function ExamDashboard({
  exams = fallbackExams,
  loading = false,
  onCreateExam,
  onPublishExam,
  onArchiveExam,
  onOpenQuestions,
  onOpenAssignments,
}: ExamDashboardProps) {
  const [items, setItems] = useState<Exam[]>(exams);
  const [query, setQuery] = useState("");
  const debouncedQuery = useDebounce(query, 300);
  const [status, setStatus] = useState<"ALL" | ExamStatus>("ALL");
  const [createOpen, setCreateOpen] = useState(false);
  const [submitting, setSubmitting] = useState(false);
  const [actionLoadingId, setActionLoadingId] = useState<string | null>(null);

  // Pagination state
  const [currentPage, setCurrentPage] = useState(1);
  const pageSize = DEFAULT_PAGE_SIZE;

  // Form fields & validation errors
  const [title, setTitle] = useState("");
  const [subject, setSubject] = useState("");
  const [description, setDescription] = useState("");
  const [durationMins, setDurationMins] = useState("60");
  const [formErrors, setFormErrors] = useState<Record<string, string>>({});

  // Keep items synced if prop changes
  const displayItems = exams.length > 0 ? exams : items;

  // Debounced search and status filter
  const filtered = useMemo(() => {
    return displayItems.filter((exam) => {
      const matchesStatus = status === "ALL" || exam.status === status;
      const matchesQuery =
        debouncedQuery === "" ||
        `${exam.title} ${exam.subject} ${exam.description || ""}`
          .toLowerCase()
          .includes(debouncedQuery.toLowerCase());
      return matchesStatus && matchesQuery;
    });
  }, [displayItems, debouncedQuery, status]);

  // Paginated slice
  const totalPages = Math.max(1, Math.ceil(filtered.length / pageSize));
  const safePage = Math.min(currentPage, totalPages);
  const paginatedExams = useMemo(() => {
    const start = (safePage - 1) * pageSize;
    return filtered.slice(start, start + pageSize);
  }, [filtered, safePage, pageSize]);

  const handleCreate = async () => {
    setFormErrors({});
    const numDuration = parseInt(durationMins, 10) || 0;

    const validation = CreateExamSchema.safeParse({
      title,
      subject,
      description,
      durationMins: numDuration,
    });

    if (!validation.success) {
      const errors: Record<string, string> = {};
      validation.error.errors.forEach((err) => {
        if (err.path[0]) errors[err.path[0] as string] = err.message;
      });
      setFormErrors(errors);
      return;
    }

    setSubmitting(true);
    try {
      if (onCreateExam) {
        await onCreateExam({
          title: title.trim(),
          subject: subject.trim(),
          description: description.trim(),
          durationMins: numDuration,
        });
      } else {
        const newExam: Exam = {
          id: crypto.randomUUID(),
          title: title.trim(),
          subject: subject.trim(),
          description: description.trim() || "No description provided.",
          durationMins: numDuration,
          status: "DRAFT",
          createdBy: "teacher-1",
          createdAt: new Date().toISOString(),
          updatedAt: new Date().toISOString(),
          candidates: 0,
          questions: 0,
        };
        setItems((current) => [newExam, ...current]);
      }

      setTitle("");
      setSubject("");
      setDescription("");
      setDurationMins("60");
      setCreateOpen(false);
    } finally {
      setSubmitting(false);
    }
  };

  const handlePublish = async (id: string) => {
    setActionLoadingId(id);
    try {
      if (onPublishExam) {
        await onPublishExam(id);
      } else {
        setItems((current) =>
          current.map((exam) =>
            exam.id === id
              ? { ...exam, status: "PUBLISHED", publishedAt: new Date().toISOString() }
              : exam
          )
        );
      }
    } finally {
      setActionLoadingId(null);
    }
  };

  const handleArchive = async (id: string) => {
    setActionLoadingId(id);
    try {
      if (onArchiveExam) {
        await onArchiveExam(id);
      } else {
        setItems((current) =>
          current.map((exam) => (exam.id === id ? { ...exam, status: "ARCHIVED" } : exam))
        );
      }
    } finally {
      setActionLoadingId(null);
    }
  };

  return (
    <div className="flex flex-col gap-6 w-full">
      {/* Page Header */}
      <header className="flex flex-col gap-4 sm:flex-row sm:items-center sm:justify-between border-b pb-6">
        <div>
          <div className="flex items-center gap-2">
            <h1 className={DESIGN_TOKENS.typography.h1}>Exam Management</h1>
            <Badge variant="outline" className="text-xs font-semibold">
              Educator
            </Badge>
          </div>
          <p className={DESIGN_TOKENS.typography.muted}>
            Author accessible assessments, configure voice accommodations, and monitor student
            assignments.
          </p>
        </div>

        {/* Create Exam Dialog */}
        <Dialog open={createOpen} onOpenChange={setCreateOpen}>
          <DialogTrigger
            render={
              <Button className="shrink-0 font-medium">
                <Plus className="mr-1.5 size-4" />
                Create New Exam
              </Button>
            }
          />
          <DialogContent className="sm:max-w-lg">
            <DialogHeader>
              <DialogTitle className="text-xl">Create New Assessment</DialogTitle>
              <DialogDescription>
                Set up initial metadata for your exam. Questions and accommodations can be added
                after saving.
              </DialogDescription>
            </DialogHeader>

            <div className="flex flex-col gap-4 py-2">
              <div className="space-y-1.5">
                <Label htmlFor="exam-title" className="text-sm font-medium">
                  Exam Title <span className="text-destructive">*</span>
                </Label>
                <Input
                  id="exam-title"
                  placeholder="e.g. Midterm General Physics"
                  value={title}
                  onChange={(e) => setTitle(e.target.value)}
                  aria-invalid={!!formErrors.title}
                />
                {formErrors.title && <p className="text-xs text-destructive">{formErrors.title}</p>}
              </div>

              <div className="grid grid-cols-2 gap-3">
                <div className="space-y-1.5">
                  <Label htmlFor="exam-subject" className="text-sm font-medium">
                    Subject / Course <span className="text-destructive">*</span>
                  </Label>
                  <Input
                    id="exam-subject"
                    placeholder="e.g. Physics 101"
                    value={subject}
                    onChange={(e) => setSubject(e.target.value)}
                    aria-invalid={!!formErrors.subject}
                  />
                  {formErrors.subject && (
                    <p className="text-xs text-destructive">{formErrors.subject}</p>
                  )}
                </div>

                <div className="space-y-1.5">
                  <Label htmlFor="exam-duration" className="text-sm font-medium">
                    Duration (Minutes) <span className="text-destructive">*</span>
                  </Label>
                  <Input
                    id="exam-duration"
                    type="number"
                    min="5"
                    max="360"
                    value={durationMins}
                    onChange={(e) => setDurationMins(e.target.value)}
                    aria-invalid={!!formErrors.durationMins}
                  />
                  {formErrors.durationMins && (
                    <p className="text-xs text-destructive">{formErrors.durationMins}</p>
                  )}
                </div>
              </div>

              <div className="space-y-1.5">
                <Label htmlFor="exam-desc" className="text-sm font-medium">
                  Description / Instructions
                </Label>
                <Textarea
                  id="exam-desc"
                  rows={3}
                  placeholder="Briefly state covered topics, rules, and allowed assistive materials..."
                  value={description}
                  onChange={(e) => setDescription(e.target.value)}
                  aria-invalid={!!formErrors.description}
                />
                {formErrors.description && (
                  <p className="text-xs text-destructive">{formErrors.description}</p>
                )}
              </div>
            </div>

            <DialogFooter className="gap-2 sm:gap-0">
              <Button
                variant="outline"
                type="button"
                onClick={() => {
                  setFormErrors({});
                  setCreateOpen(false);
                }}
              >
                Cancel
              </Button>
              <Button type="button" onClick={handleCreate} disabled={submitting}>
                {submitting && <Loader2 className="mr-2 size-4 animate-spin" />}
                {submitting ? "Saving..." : "Create Draft"}
              </Button>
            </DialogFooter>
          </DialogContent>
        </Dialog>
      </header>

      {/* Metric Cards or Loading Stats Skeleton */}
      {loading ? (
        <StatsSkeleton />
      ) : (
        <div className="grid gap-4 md:grid-cols-3">
          <StatCard
            label="Total Exams"
            value={displayItems.length}
            detail="Across all subjects"
            icon={BookOpen}
          />
          <StatCard
            label="Active Candidates"
            value={displayItems
              .filter((item) => item.status === "PUBLISHED")
              .reduce((sum, item) => sum + (item.candidates ?? 0), 0)}
            detail="Currently assigned"
            icon={Users}
          />
          <StatCard
            label="Drafts in Progress"
            value={displayItems.filter((item) => item.status === "DRAFT").length}
            detail="Ready for question review"
            icon={FilePlus2}
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
            aria-label="Search exams"
            placeholder="Search exams by title, subject, or description..."
            value={query}
            onChange={(e) => {
              setQuery(e.target.value);
              setCurrentPage(1);
            }}
          />
        </div>

        <div className="flex items-center gap-2">
          <Select
            value={status}
            onValueChange={(val) => {
              setStatus(val as typeof status);
              setCurrentPage(1);
            }}
          >
            <SelectTrigger className="w-full md:w-48" aria-label="Filter by status">
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
        <ExamCardsSkeleton count={pageSize} />
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
                    <div className="space-y-1">
                      <CardDescription className="text-xs uppercase font-semibold tracking-wider text-muted-foreground">
                        {exam.subject} · {exam.durationMins} mins
                      </CardDescription>
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

                  <div className="flex items-center justify-between text-xs text-muted-foreground border-t pt-3">
                    <span>{exam.questions ?? 0} questions configured</span>
                    <span>{exam.candidates ?? 0} candidates assigned</span>
                  </div>

                  <div className="flex flex-wrap items-center gap-2 pt-1 border-t">
                    <Button variant="outline" size="sm" onClick={() => onOpenQuestions?.(exam)}>
                      Questions
                      <ChevronRight className="ml-1 size-3.5" />
                    </Button>

                    <Button variant="outline" size="sm" onClick={() => onOpenAssignments?.(exam)}>
                      <UserCheck className="mr-1.5 size-3.5" />
                      Assign
                    </Button>

                    {exam.status === "DRAFT" && (
                      <Button
                        size="sm"
                        disabled={actionLoadingId === exam.id}
                        onClick={() => handlePublish(exam.id)}
                      >
                        {actionLoadingId === exam.id ? (
                          <Loader2 className="mr-1.5 size-3.5 animate-spin" />
                        ) : (
                          <Check className="mr-1.5 size-3.5" />
                        )}
                        Publish
                      </Button>
                    )}

                    {exam.status === "PUBLISHED" && (
                      <Button
                        variant="ghost"
                        size="sm"
                        disabled={actionLoadingId === exam.id}
                        className="text-muted-foreground hover:text-foreground"
                        onClick={() => handleArchive(exam.id)}
                      >
                        {actionLoadingId === exam.id ? (
                          <Loader2 className="mr-1.5 size-3.5 animate-spin" />
                        ) : (
                          <Archive className="mr-1.5 size-3.5" />
                        )}
                        Archive
                      </Button>
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
