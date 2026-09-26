"use client";

import { useEffect, useState, use } from "react";
import { useRouter } from "next/navigation";
import Link from "next/link";
import { ArrowLeft, BookOpen, Clock, Loader2 } from "lucide-react";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { QuestionManager } from "@/components/question-manager";
import { AssignmentManager } from "@/components/assignment-manager";
import { examsApi, questionsApi, assignmentsApi } from "@/services/api";
import type { Exam, Question, Assignment } from "@/types/exam-types";

export default function ExamDetailPage({ params }: { params: Promise<{ id: string }> }) {
  const resolvedParams = use(params);
  const examId = resolvedParams.id;
  const router = useRouter();

  const [exam, setExam] = useState<Exam | null>(null);
  const [questions, setQuestions] = useState<Question[]>([]);
  const [assignments, setAssignments] = useState<Assignment[]>([]);
  const [loading, setLoading] = useState(true);

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
          setExam({
            id: examId,
            title: "Sample Assessment",
            subject: "General Science",
            description: "Midterm assessment questions and configurations.",
            durationMins: 45,
            status: "DRAFT",
            createdBy: "teacher-1",
            createdAt: new Date().toISOString(),
            updatedAt: new Date().toISOString(),
          });
        }

        if (questionsData.status === "fulfilled") {
          setQuestions(questionsData.value);
        }

        if (assignmentsData.status === "fulfilled") {
          setAssignments(assignmentsData.value);
        }
      } finally {
        setLoading(false);
      }
    }

    loadData();
  }, [examId]);

  if (loading) {
    return (
      <div className="flex h-96 items-center justify-center">
        <Loader2 className="size-8 animate-spin text-primary" />
      </div>
    );
  }

  return (
    <div className="mx-auto max-w-6xl p-6 md:p-10 flex flex-col gap-8">
      {/* Top Header */}
      <div>
        <Link
          href="/exams"
          className="inline-flex items-center gap-2 text-sm text-muted-foreground hover:text-foreground mb-4 transition-colors"
        >
          <ArrowLeft className="size-4" />
          Back to all exams
        </Link>

        {exam && (
          <div className="flex flex-col md:flex-row md:items-center justify-between gap-4 border-b pb-6">
            <div>
              <div className="flex items-center gap-3">
                <h1 className="text-3xl font-bold tracking-tight">{exam.title}</h1>
                <Badge variant="outline">{exam.status}</Badge>
              </div>
              <p className="mt-1 text-muted-foreground">{exam.description}</p>
            </div>

            <div className="flex items-center gap-4 text-sm text-muted-foreground">
              <span className="flex items-center gap-1.5">
                <BookOpen className="size-4" />
                {exam.subject}
              </span>
              <span className="flex items-center gap-1.5">
                <Clock className="size-4" />
                {exam.durationMins} minutes
              </span>
            </div>
          </div>
        )}
      </div>

      {/* Question Studio */}
      <QuestionManager examId={examId} questions={questions} />

      {/* Candidate Assignments Section */}
      <div className="mt-4">
        <AssignmentManager examId={examId} assignments={assignments} />
      </div>
    </div>
  );
}
