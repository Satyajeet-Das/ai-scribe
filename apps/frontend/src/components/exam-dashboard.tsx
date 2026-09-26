"use client";

import { useMemo, useState } from "react";
import {
  Archive,
  BookOpen,
  Check,
  ChevronRight,
  FilePlus2,
  Loader2,
  Search,
  UserCheck,
  Users,
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
  const [status, setStatus] = useState<"ALL" | ExamStatus>("ALL");
  const [createOpen, setCreateOpen] = useState(false);
  const [submitting, setSubmitting] = useState(false);

  // Form fields
  const [title, setTitle] = useState("");
  const [subject, setSubject] = useState("");
  const [description, setDescription] = useState("");
  const [durationMins, setDurationMins] = useState("60");

  // Keep items synced if prop changes
  const displayItems = exams.length > 0 ? exams : items;

  const filtered = useMemo(
    () =>
      displayItems.filter(
        (exam) =>
          (status === "ALL" || exam.status === status) &&
          `${exam.title} ${exam.subject}`.toLowerCase().includes(query.toLowerCase())
      ),
    [displayItems, query, status]
  );

  const handleCreate = async () => {
    if (!title.trim() || !subject.trim()) return;
    setSubmitting(true);
    try {
      const dur = Math.max(1, parseInt(durationMins, 10) || 60);
      if (onCreateExam) {
        await onCreateExam({
          title: title.trim(),
          subject: subject.trim(),
          description: description.trim(),
          durationMins: dur,
        });
      } else {
        const newExam: Exam = {
          id: crypto.randomUUID(),
          title: title.trim(),
          subject: subject.trim(),
          description: description.trim() || "No description provided.",
          durationMins: dur,
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
  };

  const handleArchive = async (id: string) => {
    if (onArchiveExam) {
      await onArchiveExam(id);
    } else {
      setItems((current) =>
        current.map((exam) => (exam.id === id ? { ...exam, status: "ARCHIVED" } : exam))
      );
    }
  };

  return (
    <section
      aria-labelledby="dashboard-title"
      className="mx-auto flex max-w-6xl flex-col gap-8 p-6 md:p-10"
    >
      <header className="flex flex-col justify-between gap-5 md:flex-row md:items-end">
        <div>
          <p className="mb-2 text-sm font-semibold uppercase tracking-[0.18em] text-primary">
            Educator Portal
          </p>
          <h1
            id="dashboard-title"
            className="text-balance text-3xl font-semibold tracking-tight md:text-4xl"
          >
            Assessment & Exam Workspace
          </h1>
          <p className="mt-2 max-w-xl text-muted-foreground">
            Create accessible exams, organize MCQ/Voice/Essay questions, and manage candidate
            assignments.
          </p>
        </div>

        <Dialog open={createOpen} onOpenChange={setCreateOpen}>
          <DialogTrigger render={<Button />}>
            <FilePlus2 data-icon="inline-start" />
            Create Exam
          </DialogTrigger>
          <DialogContent className="sm:max-w-lg">
            <DialogHeader>
              <DialogTitle>Create a New Exam</DialogTitle>
              <DialogDescription>
                Define the exam details. You can add questions and assign candidates once created.
              </DialogDescription>
            </DialogHeader>

            <div className="flex flex-col gap-4 py-2">
              <div className="flex flex-col gap-2">
                <Label htmlFor="exam-title">Exam Title *</Label>
                <Input
                  id="exam-title"
                  value={title}
                  onChange={(e) => setTitle(e.target.value)}
                  placeholder="e.g. Introductory Physics Midterm"
                  autoFocus
                />
              </div>

              <div className="grid grid-cols-2 gap-4">
                <div className="flex flex-col gap-2">
                  <Label htmlFor="exam-subject">Subject *</Label>
                  <Input
                    id="exam-subject"
                    value={subject}
                    onChange={(e) => setSubject(e.target.value)}
                    placeholder="e.g. Physics"
                  />
                </div>
                <div className="flex flex-col gap-2">
                  <Label htmlFor="exam-duration">Duration (minutes) *</Label>
                  <Input
                    id="exam-duration"
                    type="number"
                    min="1"
                    max="600"
                    value={durationMins}
                    onChange={(e) => setDurationMins(e.target.value)}
                  />
                </div>
              </div>

              <div className="flex flex-col gap-2">
                <Label htmlFor="exam-desc">Description</Label>
                <Textarea
                  id="exam-desc"
                  value={description}
                  onChange={(e) => setDescription(e.target.value)}
                  placeholder="Instructions or notes for students..."
                  rows={3}
                />
              </div>
            </div>

            <DialogFooter>
              <Button variant="outline" onClick={() => setCreateOpen(false)} disabled={submitting}>
                Cancel
              </Button>
              <Button
                onClick={handleCreate}
                disabled={!title.trim() || !subject.trim() || submitting}
              >
                {submitting && <Loader2 className="animate-spin" data-icon="inline-start" />}
                {submitting ? "Creating..." : "Create Draft"}
              </Button>
            </DialogFooter>
          </DialogContent>
        </Dialog>
      </header>

      {/* Metric Cards */}
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

      {/* Filter and Search Bar */}
      <div className="flex flex-col gap-3 border-b pb-5 md:flex-row">
        <div className="relative flex-1">
          <Search
            aria-hidden="true"
            className="absolute left-3 top-1/2 -translate-y-1/2 text-muted-foreground size-4"
          />
          <Input
            className="pl-9"
            aria-label="Search exams"
            placeholder="Search exams by title or subject..."
            value={query}
            onChange={(e) => setQuery(e.target.value)}
          />
        </div>
        <Select value={status} onValueChange={(val) => setStatus(val as typeof status)}>
          <SelectTrigger className="w-full md:w-48" aria-label="Filter by status">
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

      {/* Exam Cards Grid */}
      {loading ? (
        <div className="flex h-64 items-center justify-center">
          <Loader2 className="size-8 animate-spin text-primary" />
        </div>
      ) : filtered.length === 0 ? (
        <Card className="flex flex-col items-center justify-center p-12 text-center">
          <BookOpen className="size-12 text-muted-foreground/50 mb-3" />
          <h3 className="text-lg font-medium">No exams found</h3>
          <p className="text-sm text-muted-foreground mt-1 max-w-sm">
            {query || status !== "ALL"
              ? "Try changing your search query or status filter."
              : "Get started by creating your first exam draft using the button above."}
          </p>
        </Card>
      ) : (
        <div className="grid gap-4 md:grid-cols-2">
          {filtered.map((exam) => (
            <Card key={exam.id} className="transition-all hover:shadow-md border border-border">
              <CardHeader>
                <div className="flex items-start justify-between gap-3">
                  <div>
                    <CardDescription className="text-xs uppercase font-semibold tracking-wider text-muted-foreground">
                      {exam.subject} · {exam.durationMins} mins
                    </CardDescription>
                    <CardTitle className="mt-1 text-xl">{exam.title}</CardTitle>
                  </div>
                  <Badge variant="outline" className={statusStyles[exam.status]}>
                    {exam.status}
                  </Badge>
                </div>
              </CardHeader>
              <CardContent className="flex flex-col gap-5">
                <p className="min-h-10 text-sm leading-6 text-muted-foreground line-clamp-2">
                  {exam.description || "No description provided."}
                </p>

                <div className="flex items-center justify-between text-xs text-muted-foreground border-t pt-3">
                  <span>{exam.questions ?? 0} questions configured</span>
                  <span>{exam.candidates ?? 0} candidates assigned</span>
                </div>

                <div className="flex flex-wrap items-center gap-2 pt-1">
                  <Button variant="outline" size="sm" onClick={() => onOpenQuestions?.(exam)}>
                    Manage Questions
                    <ChevronRight data-icon="inline-end" />
                  </Button>

                  <Button variant="outline" size="sm" onClick={() => onOpenAssignments?.(exam)}>
                    <UserCheck data-icon="inline-start" />
                    Assign
                  </Button>

                  {exam.status === "DRAFT" && (
                    <Button size="sm" onClick={() => handlePublish(exam.id)}>
                      <Check data-icon="inline-start" />
                      Publish
                    </Button>
                  )}

                  {exam.status === "PUBLISHED" && (
                    <Button variant="outline" size="sm" onClick={() => handleArchive(exam.id)}>
                      <Archive data-icon="inline-start" />
                      Archive
                    </Button>
                  )}
                </div>
              </CardContent>
            </Card>
          ))}
        </div>
      )}
    </section>
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
  icon: typeof BookOpen;
}) {
  return (
    <Card className="border border-border">
      <CardContent className="flex items-center justify-between p-5">
        <div>
          <p className="text-xs font-semibold uppercase tracking-wider text-muted-foreground">
            {label}
          </p>
          <p className="mt-1 text-3xl font-semibold tracking-tight">{value}</p>
          <p className="mt-1 text-xs text-muted-foreground">{detail}</p>
        </div>
        <div className="rounded-xl bg-primary/10 p-3">
          <Icon aria-hidden="true" className="size-6 text-primary" />
        </div>
      </CardContent>
    </Card>
  );
}

export default ExamDashboard;
