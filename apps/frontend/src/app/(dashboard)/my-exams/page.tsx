"use client";

import { useEffect, useState, useMemo } from "react";
import { useRouter } from "next/navigation";
import {
  BookOpen,
  Clock,
  Loader2,
  Play,
  Search,
  AlertCircle,
  RotateCcw,
  Calendar,
  CheckCircle2,
  FileText,
} from "lucide-react";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";
import { Input } from "@/components/ui/input";
import { ProtectedRoute } from "@/components/auth/protected-route";
import { assignmentsApi, sessionsApi } from "@/services/api";
import { useAuthStore } from "@/store/auth-store";
import { DESIGN_TOKENS } from "@/lib/constants";
import { useDebounce } from "@/hooks/use-debounce";
import { ExamInstructionsDialog } from "@/components/exam-instructions-dialog";
import type { StudentAssignedExam, Session } from "@/types/exam-types";

export default function MyExamsPage() {
  const router = useRouter();
  const user = useAuthStore((s) => s.user);
  const [exams, setExams] = useState<StudentAssignedExam[]>([]);
  const [sessionsMap, setSessionsMap] = useState<Record<string, Session>>({});
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const [searchQuery, setSearchQuery] = useState("");
  const debouncedQuery = useDebounce(searchQuery.trim().toLowerCase(), 250);
  const [subjectFilter, setSubjectFilter] = useState<string>("ALL");

  // Instructions Dialog State
  const [selectedExam, setSelectedExam] = useState<StudentAssignedExam | null>(null);
  const [instructionsOpen, setInstructionsOpen] = useState(false);
  const [startingExam, setStartingExam] = useState(false);

  const fetchAssignedExams = async () => {
    if (!user?.id) {
      setLoading(false);
      return;
    }

    setLoading(true);
    setError(null);

    try {
      // 1. Fetch assigned exams & active sessions in parallel
      const [examsRes, sessionsRes] = await Promise.allSettled([
        assignmentsApi.getMyAssignedExams({ limit: 100 }),
        sessionsApi?.getSessions
          ? sessionsApi.getSessions({ limit: 100 })
          : Promise.resolve({ sessions: [], total: 0 }),
      ]);

      if (examsRes.status === "fulfilled") {
        setExams(examsRes.value.data || []);
      } else {
        // Fallback: try fetching by student ID
        try {
          const studentAssignments = await assignmentsApi.getStudentAssignments(user.id);
          const mapped: StudentAssignedExam[] = studentAssignments
            .filter((a) => a.status === "ASSIGNED" || a.status === "ACTIVE")
            .map((a) => ({
              assignmentId: a.id,
              examId: a.examId,
              title: `Assessment (${a.examId.slice(0, 8)})`,
              subject: "Assigned Subject",
              durationMins: 60,
              examStatus: "PUBLISHED",
              assignedAt: a.assignedAt,
              status: a.status,
            }));
          setExams(mapped);
        } catch {
          const primaryMsg =
            examsRes.reason instanceof Error
              ? examsRes.reason.message
              : "Failed to load assigned assessments";
          setError(primaryMsg);
        }
      }

      if (sessionsRes.status === "fulfilled") {
        const sList = sessionsRes.value.sessions || [];
        const sMap: Record<string, Session> = {};
        for (const s of sList) {
          if (s.examId) {
            // Keep the active or latest session
            if (!sMap[s.examId] || s.status === "IN_PROGRESS") {
              sMap[s.examId] = s;
            }
          }
        }
        setSessionsMap(sMap);
      }
    } catch (err: unknown) {
      const msg = err instanceof Error ? err.message : "Failed to load assigned assessments";
      setError(msg);
    } finally {
      setLoading(false);
    }
  };

  useEffect(() => {
    fetchAssignedExams();
  }, [user?.id]);

  // Extract unique subjects for filtering
  const subjects = useMemo(() => {
    const set = new Set<string>();
    exams.forEach((e) => {
      if (e.subject) set.add(e.subject);
    });
    return Array.from(set);
  }, [exams]);

  // Filtered exams
  const filteredExams = useMemo(() => {
    return exams.filter((exam) => {
      if (subjectFilter !== "ALL" && exam.subject !== subjectFilter) {
        return false;
      }
      if (debouncedQuery !== "") {
        const matchTitle = exam.title.toLowerCase().includes(debouncedQuery);
        const matchSubject = exam.subject.toLowerCase().includes(debouncedQuery);
        const matchDesc = (exam.description || "").toLowerCase().includes(debouncedQuery);
        if (!matchTitle && !matchSubject && !matchDesc) return false;
      }
      return true;
    });
  }, [exams, subjectFilter, debouncedQuery]);

  const handleOpenInstructions = (exam: StudentAssignedExam) => {
    setSelectedExam(exam);
    setInstructionsOpen(true);
  };

  const handleConfirmStart = async () => {
    if (!selectedExam) return;

    // If session already in progress, resume directly!
    const existingSession = sessionsMap[selectedExam.examId];
    if (existingSession && existingSession.status === "IN_PROGRESS") {
      setInstructionsOpen(false);
      router.push(`/sessions/${existingSession.id}`);
      return;
    }

    setStartingExam(true);
    try {
      const sess = await sessionsApi.startSession({
        assignmentId: selectedExam.assignmentId,
        examId: selectedExam.examId,
      });
      setInstructionsOpen(false);
      router.push(`/sessions/${sess.id}`);
    } catch (err: unknown) {
      const msg = err instanceof Error ? err.message : "Failed to enter assessment room";
      alert(msg);
    } finally {
      setStartingExam(false);
    }
  };

  return (
    <ProtectedRoute allowedRoles={["STUDENT", "ADMIN"]}>
      <div className="min-h-screen bg-background pb-16">
        <div className={DESIGN_TOKENS.layout.container}>
          <div className="py-6 sm:py-8 flex flex-col gap-6">
            {/* Page Header */}
            <div className="flex flex-col sm:flex-row sm:items-center sm:justify-between gap-4 border-b pb-6">
              <div>
                <div className="flex items-center gap-2.5">
                  <h1 className="text-2xl sm:text-3xl font-extrabold tracking-tight text-foreground">
                    My Assessments
                  </h1>
                  {user?.rollNo && (
                    <Badge variant="outline" className="font-mono text-xs">
                      {user.rollNo}
                    </Badge>
                  )}
                  <Badge variant="secondary" className="font-mono text-xs">
                    {exams.length} Available
                  </Badge>
                </div>
                <p className="text-sm text-muted-foreground mt-1.5 max-w-2xl">
                  View and launch examinations assigned to you. Countdown timers commence once an assessment room is confirmed.
                </p>
              </div>

              <Button
                variant="outline"
                size="sm"
                onClick={fetchAssignedExams}
                disabled={loading}
                className="gap-2 self-start sm:self-auto"
              >
                <RotateCcw className={`size-3.5 ${loading ? "animate-spin" : ""}`} />
                <span>Refresh</span>
              </Button>
            </div>

            {/* Error Banner */}
            {error && (
              <div
                role="alert"
                className="rounded-lg bg-destructive/10 border border-destructive/20 p-4 text-sm text-destructive flex items-center justify-between gap-3"
              >
                <div className="flex items-center gap-2.5">
                  <AlertCircle className="size-5 shrink-0" />
                  <span>{error}</span>
                </div>
                <Button
                  size="sm"
                  variant="outline"
                  onClick={fetchAssignedExams}
                  className="h-8 gap-1 text-xs shrink-0"
                >
                  <RotateCcw className="size-3.5" />
                  Retry
                </Button>
              </div>
            )}

            {/* Search & Subject Filter Bar */}
            {exams.length > 0 && (
              <div className="flex flex-col sm:flex-row items-stretch sm:items-center justify-between gap-3">
                <div className="relative flex-1 max-w-md">
                  <Search className="absolute left-3 top-1/2 size-4 -translate-y-1/2 text-muted-foreground pointer-events-none" />
                  <Input
                    placeholder="Search by title, subject, or description..."
                    value={searchQuery}
                    onChange={(e) => setSearchQuery(e.target.value)}
                    className="pl-9 h-9 text-sm"
                  />
                </div>

                {subjects.length > 1 && (
                  <div className="flex items-center gap-1.5 overflow-x-auto pb-1 sm:pb-0">
                    <button
                      type="button"
                      onClick={() => setSubjectFilter("ALL")}
                      className={`px-3 py-1 rounded-md text-xs font-medium transition-colors whitespace-nowrap ${
                        subjectFilter === "ALL"
                          ? "bg-primary text-primary-foreground font-semibold shadow-xs"
                          : "border bg-card text-muted-foreground hover:text-foreground"
                      }`}
                    >
                      All Subjects
                    </button>
                    {subjects.map((sub) => (
                      <button
                        key={sub}
                        type="button"
                        onClick={() => setSubjectFilter(sub)}
                        className={`px-3 py-1 rounded-md text-xs font-medium transition-colors whitespace-nowrap ${
                          subjectFilter === sub
                            ? "bg-primary text-primary-foreground font-semibold shadow-xs"
                            : "border bg-card text-muted-foreground hover:text-foreground"
                        }`}
                      >
                        {sub}
                      </button>
                    ))}
                  </div>
                )}
              </div>
            )}

            {/* Main Content Area */}
            {loading ? (
              <div className="grid gap-4 sm:grid-cols-2 lg:grid-cols-3">
                {[1, 2, 3].map((n) => (
                  <Card key={n} className="border border-border p-6 space-y-4 animate-pulse">
                    <div className="h-5 w-2/3 bg-muted rounded" />
                    <div className="h-4 w-1/3 bg-muted rounded" />
                    <div className="h-8 bg-muted rounded mt-6" />
                  </Card>
                ))}
              </div>
            ) : exams.length === 0 ? (
              /* Clean Empty State */
              <Card className="border border-dashed border-border py-16 px-6 text-center">
                <BookOpen className="mx-auto size-12 text-muted-foreground/40 mb-3" />
                <h2 className="text-lg font-bold text-foreground">No Assigned Assessments</h2>
                <p className="text-sm text-muted-foreground mt-1 max-w-md mx-auto">
                  You currently have no pending examinations assigned to your candidate profile. When your educators assign tests, they will appear here.
                </p>
                <div className="mt-6 flex justify-center gap-3">
                  <Button
                    size="sm"
                    variant="outline"
                    onClick={fetchAssignedExams}
                    className="gap-1.5"
                  >
                    <RotateCcw className="size-3.5" />
                    Check Again
                  </Button>
                </div>
              </Card>
            ) : filteredExams.length === 0 ? (
              /* Search Empty State */
              <Card className="border border-border p-10 text-center text-sm text-muted-foreground">
                <p className="font-semibold text-foreground">No assessments found</p>
                <p className="mt-1 text-xs">
                  No assigned assessments matched &ldquo;{searchQuery}&rdquo;.
                </p>
                <Button
                  size="sm"
                  variant="ghost"
                  onClick={() => {
                    setSearchQuery("");
                    setSubjectFilter("ALL");
                  }}
                  className="mt-4 text-xs"
                >
                  Clear Filters
                </Button>
              </Card>
            ) : (
              /* Exams Grid */
              <div className="grid gap-4 sm:grid-cols-2 lg:grid-cols-3" data-testid="assigned-exams-grid">
                {filteredExams.map((exam) => {
                  const existingSession = sessionsMap[exam.examId];
                  const isInProgress = existingSession?.status === "IN_PROGRESS";
                  const isSubmitted = existingSession?.status === "SUBMITTED";

                  return (
                    <Card
                      key={exam.assignmentId || exam.examId}
                      className="border border-border flex flex-col justify-between hover:border-primary/40 hover:shadow-xs transition-all"
                    >
                      <CardHeader className="pb-3">
                        <div className="flex items-center justify-between gap-2">
                          <Badge variant="outline" className="text-xs font-medium">
                            {exam.subject}
                          </Badge>
                          {isInProgress ? (
                            <Badge
                              variant="default"
                              className="text-[10px] bg-amber-600 hover:bg-amber-600 text-white font-semibold"
                            >
                              IN PROGRESS
                            </Badge>
                          ) : isSubmitted ? (
                            <Badge
                              variant="default"
                              className="text-[10px] bg-emerald-600 hover:bg-emerald-600 text-white font-semibold"
                            >
                              SUBMITTED
                            </Badge>
                          ) : (
                            <Badge
                              variant="default"
                              className="text-[10px] bg-primary text-primary-foreground font-semibold"
                            >
                              ASSIGNED
                            </Badge>
                          )}
                        </div>
                        <CardTitle className="text-base font-bold mt-2 line-clamp-1">
                          {exam.title}
                        </CardTitle>
                        {exam.description && (
                          <CardDescription className="text-xs line-clamp-2 mt-1">
                            {exam.description}
                          </CardDescription>
                        )}
                      </CardHeader>

                      <CardContent className="pt-0 space-y-4">
                        <div className="grid grid-cols-2 gap-2 text-xs text-muted-foreground border-t border-b py-2.5">
                          <span className="flex items-center gap-1.5">
                            <Clock className="size-3.5 text-primary" />
                            {exam.durationMins} minutes
                          </span>
                          <span className="flex items-center gap-1.5" suppressHydrationWarning>
                            <Calendar className="size-3.5 text-muted-foreground" />
                            {exam.assignedAt ? exam.assignedAt.slice(0, 10) : "Recent"}
                          </span>
                        </div>

                        {isInProgress ? (
                          <Button
                            size="sm"
                            onClick={() => handleOpenInstructions(exam)}
                            className="w-full gap-1.5 font-semibold bg-amber-600 hover:bg-amber-700 text-white shadow-xs"
                            data-testid={`resume-assessment-${exam.examId}`}
                          >
                            <RotateCcw className="size-3.5" />
                            Resume Assessment
                          </Button>
                        ) : isSubmitted ? (
                          <Button
                            size="sm"
                            variant="secondary"
                            disabled
                            className="w-full gap-1.5 font-semibold cursor-not-allowed opacity-80"
                          >
                            <CheckCircle2 className="size-3.5 text-emerald-600" />
                            Assessment Completed
                          </Button>
                        ) : (
                          <Button
                            size="sm"
                            onClick={() => handleOpenInstructions(exam)}
                            aria-label={`Launch candidate assessment for ${exam.title}`}
                            className="w-full gap-1.5 font-semibold shadow-xs"
                            data-testid={`start-assessment-${exam.examId}`}
                          >
                            <Play className="size-3.5 fill-current" />
                            Start Assessment
                          </Button>
                        )}
                      </CardContent>
                    </Card>
                  );
                })}
              </div>
            )}
          </div>
        </div>
      </div>

      {/* Instructions & Confirmation Dialog */}
      <ExamInstructionsDialog
        open={instructionsOpen}
        onOpenChange={setInstructionsOpen}
        exam={selectedExam}
        sessionStatus={selectedExam ? sessionsMap[selectedExam.examId]?.status : undefined}
        sessionId={selectedExam ? sessionsMap[selectedExam.examId]?.id : undefined}
        onConfirm={handleConfirmStart}
        loading={startingExam}
      />
    </ProtectedRoute>
  );
}
