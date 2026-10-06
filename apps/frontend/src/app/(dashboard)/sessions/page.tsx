"use client";

import { useEffect, useState, useMemo } from "react";
import { useRouter } from "next/navigation";
import {
  BookOpen,
  Clock,
  Loader2,
  Play,
  ShieldAlert,
  Search,
} from "lucide-react";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";
import { Input } from "@/components/ui/input";
import { ProtectedRoute } from "@/components/auth/protected-route";
import { sessionsApi, examsApi } from "@/services/api";
import { useAuthStore } from "@/store/auth-store";
import { DESIGN_TOKENS } from "@/lib/constants";
import { useDebounce } from "@/hooks/use-debounce";
import type { Exam } from "@/types/exam-types";

interface AssignedExamItem {
  assignmentId: string;
  examId: string;
  title: string;
  subject: string;
  durationMins: number;
  status: "PENDING" | "IN_PROGRESS" | "SUBMITTED";
  sessionId?: string;
}

const mockAssignedExams: AssignedExamItem[] = [
  {
    assignmentId: "assign-bio-01",
    examId: "exam-1",
    title: "Foundations of Biology Midterm",
    subject: "Biology",
    durationMins: 45,
    status: "PENDING",
  },
  {
    assignmentId: "assign-math-02",
    examId: "exam-2",
    title: "Algebraic Reasoning and Functions",
    subject: "Mathematics",
    durationMins: 60,
    status: "PENDING",
  },
];

export default function StudentSessionsPage() {
  const router = useRouter();
  const user = useAuthStore((s) => s.user);
  const [items, setItems] = useState<AssignedExamItem[]>([]);
  const [loading, setLoading] = useState(true);
  const [query, setQuery] = useState("");
  const debouncedQuery = useDebounce(query, 300);
  const [startingId, setStartingId] = useState<string | null>(null);
  const [error, setError] = useState<string | null>(null);

  useEffect(() => {
    if (!user?.id) {
      setLoading(false);
      return;
    }
    let ignore = false;
    setLoading(true);

    Promise.allSettled([
      examsApi.getExams({ status: "PUBLISHED" }),
      sessionsApi.getSessions({ limit: 100 }),
    ])
      .then(([examsOutcome, sessionsOutcome]) => {
        if (ignore) return;

        let examList: Exam[] = [];
        if (examsOutcome.status === "fulfilled") {
          const res = examsOutcome.value;
          examList = Array.isArray(res)
            ? res
            : res.exams || (res as { data?: Exam[] }).data || [];
        }

        const sessionByExam = new Map<string, { id: string; status: string }>();
        if (sessionsOutcome.status === "fulfilled") {
          const res = sessionsOutcome.value;
          const sList = Array.isArray(res)
            ? res
            : (res as { sessions?: typeof res.sessions }).sessions || [];
          for (const s of sList) {
            if (s.examId) {
              sessionByExam.set(s.examId, { id: s.id, status: s.status });
            }
          }
        }

        if (examList.length > 0) {
          const mapped: AssignedExamItem[] = examList.map((exam) => {
            const existing = sessionByExam.get(exam.id);
            let status: "PENDING" | "IN_PROGRESS" | "SUBMITTED" = "PENDING";
            if (existing) {
              if (existing.status === "IN_PROGRESS") {
                status = "IN_PROGRESS";
              } else if (existing.status === "SUBMITTED" || existing.status === "EXPIRED") {
                status = "SUBMITTED";
              }
            }

            return {
              assignmentId: exam.id,
              examId: exam.id,
              title: exam.title,
              subject: exam.subject,
              durationMins: exam.durationMins,
              status,
              sessionId: existing?.id,
            };
          });
          setItems(mapped);
        } else {
          setItems([]);
        }
      })
      .catch((err) => {
        if (!ignore) {
          console.warn("Failed to load assessments or sessions, falling back to mock:", err);
          setItems(mockAssignedExams);
        }
      })
      .finally(() => {
        if (!ignore) {
          setLoading(false);
        }
      });

    return () => {
      ignore = true;
    };
  }, [user?.id]);

  const filteredItems = useMemo(() => {
    if (!debouncedQuery) return items;
    return items.filter(
      (item) =>
        item.title.toLowerCase().includes(debouncedQuery.toLowerCase()) ||
        item.subject.toLowerCase().includes(debouncedQuery.toLowerCase())
    );
  }, [items, debouncedQuery]);

  const handleStartExam = async (item: AssignedExamItem) => {
    // If we already have the active in-progress session ID, navigate directly to it!
    if (item.status === "IN_PROGRESS" && item.sessionId) {
      router.push(`/sessions/${item.sessionId}`);
      return;
    }

    setStartingId(item.examId);
    setError(null);
    try {
      const session = await sessionsApi.startSession({
        examId: item.examId,
      });
      router.push(`/sessions/${session.id}`);
    } catch (err: unknown) {
      const msg = err instanceof Error ? err.message : "Failed to enter exam room";
      setError(msg);
    } finally {
      setStartingId(null);
    }
  };

  return (
    <ProtectedRoute allowedRoles={["STUDENT", "ADMIN", "PROCTOR"]}>
      <div className="min-h-screen bg-background pb-16">
        <div className={DESIGN_TOKENS.layout.container}>
          <div className={`${DESIGN_TOKENS.layout.pageSection} flex flex-col gap-6`}>
            {/* Header */}
            <header className="border-b pb-6">
              <span className="text-xs font-semibold uppercase tracking-wider text-primary">
                Candidate Assessment Portal
              </span>
              <h1 className={`${DESIGN_TOKENS.typography.h1} mt-1`}>Your Scheduled Assessments</h1>
              <p className={`${DESIGN_TOKENS.typography.muted} mt-2 max-w-2xl`}>
                Select an assigned assessment to launch the accessible exam room. The countdown
                timer begins once you enter, and questions support voice narration, speech-to-text
                dictation, and keyboard shortcuts.
              </p>
            </header>

            {/* Error Banner */}
            {error && (
              <div className="rounded-lg bg-destructive/10 border border-destructive/20 p-4 text-sm text-destructive flex items-center gap-3">
                <ShieldAlert className="size-5 shrink-0" />
                <span>{error}</span>
              </div>
            )}

            {/* Search Bar */}
            <div className="relative max-w-md">
              <Search
                aria-hidden="true"
                className="absolute left-3 top-1/2 -translate-y-1/2 text-muted-foreground size-4"
              />
              <Input
                className="pl-9"
                aria-label="Search assigned assessments"
                placeholder="Search assessments by subject or title..."
                value={query}
                onChange={(e) => setQuery(e.target.value)}
              />
            </div>

            {/* Assessment Cards Grid */}
            {loading ? (
              <div className="flex flex-col items-center justify-center p-12 text-center">
                <Loader2 className="size-8 animate-spin text-primary mb-3" />
                <p className="text-sm text-muted-foreground">Loading your assigned assessments...</p>
              </div>
            ) : filteredItems.length === 0 ? (
              <Card className="flex flex-col items-center justify-center p-12 text-center border-dashed">
                <div className="size-16 rounded-full bg-muted flex items-center justify-center mb-3">
                  <BookOpen className="size-8 text-muted-foreground/60" />
                </div>
                <h3 className="text-lg font-semibold">No assigned exams</h3>
                <p className="text-sm text-muted-foreground mt-1 max-w-sm">
                  {query
                    ? "No assessments matched your search. Try clearing your query."
                    : "You currently have no pending exam assignments from your educators."}
                </p>
                {query && (
                  <Button variant="outline" size="sm" className="mt-4" onClick={() => setQuery("")}>
                    Clear Search
                  </Button>
                )}
              </Card>
            ) : (
              <div className="grid gap-4 md:grid-cols-2">
                {filteredItems.map((item) => (
                  <Card
                    key={item.assignmentId}
                    className="flex flex-col justify-between border border-border hover:shadow-md transition-shadow"
                  >
                    <CardHeader>
                      <div className="flex items-start justify-between gap-3">
                        <div>
                          <CardDescription className="text-xs uppercase font-semibold tracking-wider text-muted-foreground">
                            {item.subject}
                          </CardDescription>
                          <CardTitle className="text-xl mt-1 font-bold">{item.title}</CardTitle>
                        </div>
                        <Badge
                          variant={
                            item.status === "SUBMITTED"
                              ? "secondary"
                              : item.status === "IN_PROGRESS"
                              ? "default"
                              : "outline"
                          }
                          className={
                            item.status === "IN_PROGRESS"
                              ? "bg-emerald-600 hover:bg-emerald-600 text-white dark:bg-emerald-700"
                              : undefined
                          }
                        >
                          {item.status === "IN_PROGRESS"
                            ? "In Progress"
                            : item.status === "PENDING"
                            ? "Ready to Start"
                            : "Assessment Completed"}
                        </Badge>
                      </div>
                    </CardHeader>

                    <CardContent className="flex flex-col gap-5">
                      <div className="flex items-center gap-4 text-xs font-medium text-muted-foreground">
                        <span className="flex items-center gap-1.5">
                          <Clock className="size-4" />
                          {item.durationMins} minutes allocated
                        </span>
                        <span className="flex items-center gap-1.5">
                          <BookOpen className="size-4" />
                          Voice & Text enabled
                        </span>
                      </div>

                      <Button
                        size="lg"
                        className="w-full font-semibold"
                        disabled={startingId === item.assignmentId || item.status === "SUBMITTED"}
                        onClick={() => handleStartExam(item)}
                      >
                        {startingId === item.assignmentId ? (
                          <>
                            <Loader2 className="animate-spin size-4 mr-2" />
                            {item.status === "IN_PROGRESS"
                              ? "Reconnecting Exam Room..."
                              : "Initializing Exam Room..."}
                          </>
                        ) : item.status === "SUBMITTED" ? (
                          "Assessment Completed"
                        ) : item.status === "IN_PROGRESS" ? (
                          <>
                            <Play className="size-4 mr-2 fill-current" />
                            Resume Exam Room
                          </>
                        ) : (
                          <>
                            <Play className="size-4 mr-2" />
                            Enter Exam Room
                          </>
                        )}
                      </Button>
                    </CardContent>
                  </Card>
                ))}
              </div>
            )}
          </div>
        </div>
      </div>
    </ProtectedRoute>
  );
}
