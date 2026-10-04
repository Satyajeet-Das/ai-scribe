"use client";

import { useEffect, useState, useMemo } from "react";
import { useRouter } from "next/navigation";
import {
  ArrowRight,
  BookOpen,
  Clock,
  Loader2,
  Play,
  ShieldAlert,
  Search,
  CheckCircle2,
} from "lucide-react";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";
import { Input } from "@/components/ui/input";
import { ProtectedRoute } from "@/components/auth/protected-route";
import { sessionsApi, assignmentsApi, ApiError } from "@/services/api";
import { DESIGN_TOKENS } from "@/lib/constants";
import { useDebounce } from "@/hooks/use-debounce";
import type { Assignment, Session } from "@/types/exam-types";

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
  const [items, setItems] = useState<AssignedExamItem[]>(mockAssignedExams);
  const [query, setQuery] = useState("");
  const debouncedQuery = useDebounce(query, 300);
  const [loading, setLoading] = useState(false);
  const [startingId, setStartingId] = useState<string | null>(null);
  const [error, setError] = useState<string | null>(null);

  const filteredItems = useMemo(() => {
    if (!debouncedQuery) return items;
    return items.filter(
      (item) =>
        item.title.toLowerCase().includes(debouncedQuery.toLowerCase()) ||
        item.subject.toLowerCase().includes(debouncedQuery.toLowerCase())
    );
  }, [items, debouncedQuery]);

  const handleStartExam = async (item: AssignedExamItem) => {
    setStartingId(item.assignmentId);
    setError(null);
    try {
      const session = await sessionsApi.startSession({
        assignmentId: item.assignmentId,
      });
      router.push(`/sessions/${session.id}`);
    } catch (err) {
      console.warn("Backend session start failed, falling back to simulated session room:", err);
      // Fallback allows full UI review
      const mockSessionId = crypto.randomUUID();
      router.push(`/sessions/${mockSessionId}`);
    } finally {
      setStartingId(null);
    }
  };

  return (
    <ProtectedRoute allowedRoles={["STUDENT", "ADMIN"]}>
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
            {filteredItems.length === 0 ? (
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
                        <Badge variant={item.status === "SUBMITTED" ? "secondary" : "outline"}>
                          {item.status === "PENDING" ? "Ready to Start" : item.status}
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
                            Initializing Exam Room...
                          </>
                        ) : item.status === "SUBMITTED" ? (
                          "Assessment Completed"
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
