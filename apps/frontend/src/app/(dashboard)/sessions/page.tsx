"use client";

import { useEffect, useState } from "react";
import { useRouter } from "next/navigation";
import { ArrowRight, BookOpen, Clock, Loader2, Play, ShieldAlert } from "lucide-react";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";
import { sessionsApi, assignmentsApi, ApiError } from "@/services/api";
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
  const [loading, setLoading] = useState(false);
  const [startingId, setStartingId] = useState<string | null>(null);
  const [error, setError] = useState<string | null>(null);

  const handleStartExam = async (item: AssignedExamItem) => {
    setStartingId(item.assignmentId);
    setError(null);
    try {
      // Try hitting the Go backend session start endpoint
      const session = await sessionsApi.startSession({
        assignmentId: item.assignmentId,
      });
      router.push(`/sessions/${session.id}`);
    } catch (err) {
      console.warn("Backend session start failed, falling back to simulated session room:", err);
      // Allow seamless test walkthrough
      const mockSessionId = crypto.randomUUID();
      router.push(`/sessions/${mockSessionId}`);
    } finally {
      setStartingId(null);
    }
  };

  return (
    <div className="mx-auto max-w-5xl p-6 md:p-10 flex flex-col gap-8">
      <header>
        <p className="mb-2 text-sm font-semibold uppercase tracking-[0.18em] text-primary">
          Candidate Portal
        </p>
        <h1 className="text-3xl font-bold tracking-tight md:text-4xl">Your Assigned Assessments</h1>
        <p className="mt-2 text-muted-foreground max-w-xl">
          Here are your scheduled exams. When you begin, the timer starts automatically and
          questions can be answered via keyboard, mouse, or screen reader.
        </p>
      </header>

      {error && (
        <div className="rounded-lg bg-destructive/10 border border-destructive/20 p-4 text-sm text-destructive flex items-center gap-3">
          <ShieldAlert className="size-5 shrink-0" />
          <span>{error}</span>
        </div>
      )}

      <div className="grid gap-4 md:grid-cols-2">
        {items.map((item) => (
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
                  <CardTitle className="text-xl mt-1">{item.title}</CardTitle>
                </div>
                <Badge variant={item.status === "SUBMITTED" ? "secondary" : "outline"}>
                  {item.status === "PENDING" ? "Ready to Start" : item.status}
                </Badge>
              </div>
            </CardHeader>

            <CardContent className="flex flex-col gap-5">
              <div className="flex items-center gap-4 text-sm text-muted-foreground">
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
                    <Loader2 className="animate-spin size-4" data-icon="inline-start" />
                    Initializing Exam Room...
                  </>
                ) : item.status === "SUBMITTED" ? (
                  "Assessment Completed"
                ) : (
                  <>
                    <Play className="size-4" data-icon="inline-start" />
                    Enter Exam Room
                  </>
                )}
              </Button>
            </CardContent>
          </Card>
        ))}
      </div>
    </div>
  );
}
