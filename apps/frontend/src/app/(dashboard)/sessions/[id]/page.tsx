"use client";

import { useEffect, useRef, useState, use, useCallback } from "react";
import Link from "next/link";
import { CheckCircle2, Home, Loader2, AlertTriangle, ShieldAlert, RotateCcw } from "lucide-react";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";
import { StudentExamRoom } from "@/components/student-exam-room";
import { VoiceAssistant, type VoiceActivity } from "@/components/voice/voice-assistant";
import type { VoiceSessionState } from "@/services/voice/protocol";
import { ProtectedRoute } from "@/components/auth/protected-route";
import { sessionsApi, examsApi, ApiError } from "@/services/api";
import type { Session, StudentQuestionResponse, SessionStatus } from "@/types/exam-types";

/** How long after the last keystroke a typed answer is saved. */
const TEXT_SAVE_DELAY_MS = 800;

export default function ActiveSessionRoomPage({
  params,
}: {
  params: Promise<{ id: string }> | { id: string };
}) {
  const resolvedParams = "then" in params ? use(params) : params;
  const sessionId = resolvedParams.id;

  const [session, setSession] = useState<Session | null>(null);
  const [currentQuestion, setCurrentQuestion] = useState<StudentQuestionResponse | null>(null);
  const [answers, setAnswers] = useState<Record<string, string>>({});
  const [saveStatus, setSaveStatus] = useState<"idle" | "unsaved" | "saving" | "saved" | "failed">(
    "idle"
  );
  const [saveError, setSaveError] = useState<string | null>(null);

  const [loading, setLoading] = useState(true);
  const [loadError, setLoadError] = useState<string | null>(null);
  const [navigating, setNavigating] = useState(false);
  const [submitting, setSubmitting] = useState(false);

  const [initialSeconds, setInitialSeconds] = useState(45 * 60);
  const [sessionStatus, setSessionStatus] = useState<SessionStatus>("IN_PROGRESS");
  const [submittedAt, setSubmittedAt] = useState<string | null>(null);

  const [examTitle, setExamTitle] = useState("Candidate Assessment");
  const [subject, setSubject] = useState("Subject");

  // Load Session and Hydrate State from Authoritative Backend
  const loadSession = useCallback(async () => {
    setLoading(true);
    setLoadError(null);

    try {
      const sess = await sessionsApi.getSession(sessionId);
      setSession(sess);
      setSessionStatus(sess.status);

      // Handle Terminal Submitted State
      if (sess.status === "SUBMITTED") {
        setSubmittedAt(sess.submittedAt ? sess.submittedAt.slice(11, 19) : "Completed");
        setLoading(false);
        return;
      }

      // Handle Terminal Expired State
      if (sess.status === "EXPIRED") {
        setLoading(false);
        return;
      }

      // Sync Countdown Timer
      if (sess.remainingSeconds !== undefined && sess.remainingSeconds !== null) {
        setInitialSeconds(Math.max(0, sess.remainingSeconds));
      } else if (sess.durationMins) {
        setInitialSeconds(sess.durationMins * 60);
      }

      // Fetch Exam Info if available
      if (sess.examId) {
        examsApi.getExam(sess.examId).then((ex) => {
          if (ex) {
            setExamTitle(ex.title);
            setSubject(ex.subject);
          }
        }).catch(() => {});
      }

      // Fetch Saved Answers
      try {
        const savedList = await sessionsApi.getAnswers(sessionId);
        if (savedList && savedList.length > 0) {
          const ansMap: Record<string, string> = {};
          for (const a of savedList) {
            ansMap[a.questionId] = a.selectedOptionId || a.textAnswer || "";
          }
          setAnswers(ansMap);
        }
      } catch {
        // Non-fatal if no answers saved yet
      }

      // Fetch Current Question
      try {
        const qResp = await sessionsApi.getCurrentQuestion(sessionId);
        setCurrentQuestion(qResp);
      } catch (err: unknown) {
        console.warn("Could not fetch current question from endpoint:", err);
      }
    } catch (err: unknown) {
      const msg = err instanceof Error ? err.message : "Failed to load examination session";
      setLoadError(msg);
    } finally {
      setLoading(false);
    }
  }, [sessionId]);

  useEffect(() => {
    loadSession();
  }, [loadSession]);

  // The question on screen, readable from callbacks without re-creating them.
  const shownQuestionIdRef = useRef<string | null>(null);
  useEffect(() => {
    shownQuestionIdRef.current = currentQuestion?.id ?? null;
  }, [currentQuestion]);

  // What the voice assistant is in the middle of. Changing question during any
  // of these would silently throw work away, so navigation waits for the
  // student to finish or cancel it.
  const voiceActivityRef = useRef<VoiceActivity>({
    recording: false,
    processing: false,
    awaitingConfirmation: false,
  });
  const [navigationNotice, setNavigationNotice] = useState<string | null>(null);

  const navigationBlocker = (): string | null => {
    const a = voiceActivityRef.current;
    if (a.recording) {
      return "A recording is in progress. Stop it with Enter, or choose Discard recording, before changing question.";
    }
    if (a.processing) {
      return "Your last recording is still being processed. Please wait a moment before changing question.";
    }
    if (a.awaitingConfirmation) {
      return "You have an answer waiting for confirmation. Choose Yes, save answer or No, cancel before changing question.";
    }
    return null;
  };

  const handleVoiceActivity = useCallback((activity: VoiceActivity) => {
    voiceActivityRef.current = activity;
    if (!activity.recording && !activity.processing && !activity.awaitingConfirmation) {
      setNavigationNotice(null);
    }
  }, []);

  // Typed answers. Text the server has not confirmed is kept here, tagged with
  // the question it belongs to, until a save succeeds. Saves run one at a time
  // so an older request can never overwrite newer text.
  const pendingTextRef = useRef<{ questionId: string; text: string } | null>(null);
  const textTimerRef = useRef<ReturnType<typeof setTimeout> | null>(null);
  const saveChainRef = useRef<Promise<boolean>>(Promise.resolve(true));

  /**
   * Saves any unconfirmed typed text. Resolves true when nothing is left
   * unsaved, and false when a save failed (the text is kept for a retry).
   */
  const flushText = useCallback((): Promise<boolean> => {
    if (textTimerRef.current !== null) {
      clearTimeout(textTimerRef.current);
      textTimerRef.current = null;
    }
    const run = async (): Promise<boolean> => {
      const pending = pendingTextRef.current;
      if (!pending) return true;
      const isShown = () => pending.questionId === shownQuestionIdRef.current;

      if (pending.text.trim() === "") {
        // The server rejects an empty answer, so a saved answer cannot be
        // cleared. Say so rather than showing a state that is not true.
        pendingTextRef.current = null;
        if (isShown()) {
          setSaveStatus("failed");
          setSaveError(
            "An answer that was already saved cannot be cleared. Type a new answer to replace it."
          );
        }
        return true;
      }

      pendingTextRef.current = null;
      if (isShown()) {
        setSaveStatus("saving");
        setSaveError(null);
      }
      try {
        await sessionsApi.submitAnswer(sessionId, pending.questionId, {
          textAnswer: pending.text,
        });
      } catch (err: unknown) {
        // Keep the text for a retry unless the student has typed something newer.
        if (!pendingTextRef.current) pendingTextRef.current = pending;
        if (isShown()) {
          setSaveStatus("failed");
          setSaveError(err instanceof Error ? err.message : "Failed to save answer");
        }
        return false;
      }
      if (isShown()) setSaveStatus(pendingTextRef.current ? "unsaved" : "saved");
      return pendingTextRef.current === null;
    };
    const next = saveChainRef.current.then(run);
    saveChainRef.current = next;
    return next;
  }, [sessionId]);

  const handleTextAnswerChange = (text: string) => {
    if (!currentQuestion) return;
    const questionId = currentQuestion.id;
    setAnswers((prev) => ({ ...prev, [questionId]: text }));
    pendingTextRef.current = { questionId, text };
    setSaveStatus("unsaved");
    setSaveError(null);
    if (textTimerRef.current !== null) clearTimeout(textTimerRef.current);
    textTimerRef.current = setTimeout(() => void flushText(), TEXT_SAVE_DELAY_MS);
  };

  // Leaving the page with an answer the server has not confirmed would lose it.
  useEffect(() => {
    if (saveStatus !== "unsaved" && saveStatus !== "saving" && saveStatus !== "failed") return;
    const warn = (e: BeforeUnloadEvent) => {
      e.preventDefault();
      e.returnValue = "";
    };
    window.addEventListener("beforeunload", warn);
    return () => window.removeEventListener("beforeunload", warn);
  }, [saveStatus]);

  // Do not leave a timer behind when the page goes away.
  useEffect(
    () => () => {
      if (textTimerRef.current !== null) clearTimeout(textTimerRef.current);
    },
    []
  );

  // Save selected option
  const handleSelectOption = async (optionId: string) => {
    if (!currentQuestion) return;
    const qId = currentQuestion.id;

    // Optimistic UI update
    setAnswers((prev) => ({ ...prev, [qId]: optionId }));
    setSaveStatus("saving");
    setSaveError(null);

    try {
      await sessionsApi.submitAnswer(sessionId, qId, {
        selectedOptionId: optionId,
        textAnswer: "",
      });
      setSaveStatus("saved");
    } catch (err: unknown) {
      setSaveStatus("failed");
      const msg = err instanceof Error ? err.message : "Failed to save answer";
      setSaveError(msg);
    }
  };

  // Retry saving current answer
  const handleRetrySave = () => {
    if (!currentQuestion) return;
    if (pendingTextRef.current?.questionId === currentQuestion.id) {
      void flushText();
      return;
    }
    const val = answers[currentQuestion.id];
    if (val && currentQuestion.type === "MCQ") {
      handleSelectOption(val);
    }
  };

  // Navigate to Next Question
  const handleNext = async () => {
    if (navigating || submitting) return;
    const blocker = navigationBlocker();
    if (blocker) {
      setNavigationNotice(blocker);
      return;
    }
    setNavigationNotice(null);
    // Never move away from an answer the server has not confirmed.
    if (!(await flushText())) return;
    setNavigating(true);

    try {
      await sessionsApi.nextQuestion(sessionId);
      const nextQ = await sessionsApi.getCurrentQuestion(sessionId);
      setCurrentQuestion(nextQ);
      setSaveStatus("idle");
      setSaveError(null);
    } catch (err: unknown) {
      console.error("Failed to advance question:", err);
    } finally {
      setNavigating(false);
    }
  };

  // Navigate to Previous Question
  const handlePrevious = async () => {
    if (navigating || submitting) return;
    const blocker = navigationBlocker();
    if (blocker) {
      setNavigationNotice(blocker);
      return;
    }
    setNavigationNotice(null);
    if (!(await flushText())) return;
    setNavigating(true);

    try {
      await sessionsApi.previousQuestion(sessionId);
      const prevQ = await sessionsApi.getCurrentQuestion(sessionId);
      setCurrentQuestion(prevQ);
      setSaveStatus("idle");
      setSaveError(null);
    } catch (err: unknown) {
      console.error("Failed to go to previous question:", err);
    } finally {
      setNavigating(false);
    }
  };

  // Handle Final Exam Submission
  const handleSubmit = async () => {
    if (submitting) return;
    // Submitting seals the exam, so every typed answer must be saved first.
    if (!(await flushText())) return;
    setSubmitting(true);

    try {
      const res = await sessionsApi.submitSession(sessionId);
      setSessionStatus("SUBMITTED");
      setSubmittedAt(
        res.submittedAt ? res.submittedAt.slice(11, 19) : new Date().toISOString().slice(11, 19)
      );
    } catch (err: unknown) {
      // If error indicates already submitted (Conflict 409), refresh authoritative state
      if (err instanceof ApiError && (err.isConflict() || err.status === 409)) {
        try {
          const sess = await sessionsApi.getSession(sessionId);
          if (sess.status === "SUBMITTED") {
            setSessionStatus("SUBMITTED");
            setSubmittedAt(sess.submittedAt ? sess.submittedAt.slice(11, 19) : "Completed");
            return;
          }
        } catch {
          // ignore
        }
      }

      console.error("Submission failed:", err);
      alert("Submission encountered a network issue. Please check your connection and retry.");
    } finally {
      setSubmitting(false);
    }
  };

  // Handle Session Expiration
  const handleExpire = async () => {
    try {
      const sess = await sessionsApi.getSession(sessionId);
      if (sess.status === "EXPIRED" || sess.status === "SUBMITTED") {
        setSessionStatus(sess.status);
      } else {
        setSessionStatus("EXPIRED");
      }
    } catch {
      setSessionStatus("EXPIRED");
    }
  };

  // Voice assistant: the backend reported the session's position (on connect,
  // on reconnect, and after voice navigation). It is authoritative.
  const handleVoiceSessionState = useCallback((state: VoiceSessionState) => {
    if (state.remainingSeconds !== undefined) {
      setInitialSeconds(Math.max(0, state.remainingSeconds));
    }
    const question = state.question;
    if (question) {
      if (shownQuestionIdRef.current !== question.id) {
        // The page moved without going through handleNext, so make sure typed
        // text for the question being left is saved.
        void flushText();
        setSaveStatus("idle");
        setSaveError(null);
      }
      setCurrentQuestion(question);
    }
  }, [flushText]);

  // Voice assistant: the backend confirmed an answer was persisted. The
  // selection shown on screen is re-read from the HTTP API rather than assumed.
  const handleVoiceAnswerSaved = useCallback(
    async (questionId: string) => {
      try {
        const saved = await sessionsApi.getQuestionAnswer(sessionId, questionId);
        setAnswers((prev) => ({
          ...prev,
          [questionId]: saved.selectedOptionId || saved.textAnswer || "",
        }));
        setSaveStatus("saved");
        setSaveError(null);
      } catch {
        // The answer is saved on the server; only the on-screen copy is stale.
      }
    },
    [sessionId]
  );

  // Loading State
  if (loading) {
    return (
      <ProtectedRoute allowedRoles={["STUDENT", "ADMIN", "PROCTOR"]}>
        <div className="flex min-h-screen items-center justify-center p-4 bg-background">
          <div className="flex flex-col items-center gap-3">
            <Loader2 className="size-8 animate-spin text-primary" />
            <p className="text-sm font-medium text-muted-foreground">Connecting to active assessment room...</p>
          </div>
        </div>
      </ProtectedRoute>
    );
  }

  // Load Error State
  if (loadError) {
    return (
      <ProtectedRoute allowedRoles={["STUDENT", "ADMIN", "PROCTOR"]}>
        <div className="flex min-h-screen items-center justify-center p-4 bg-background">
          <Card className="w-full max-w-md p-6 border-destructive/20 text-center">
            <CardHeader className="flex flex-col items-center">
              <ShieldAlert className="size-12 text-destructive mb-2" />
              <CardTitle className="text-xl font-bold">Failed to Load Session</CardTitle>
              <CardDescription className="text-sm text-muted-foreground mt-2">
                {loadError}
              </CardDescription>
            </CardHeader>
            <CardContent className="mt-4 flex flex-col gap-2">
              <Button onClick={loadSession} className="w-full gap-2 font-semibold">
                <RotateCcw className="size-4" />
                Retry Reconnection
              </Button>
              <Link href="/my-exams" className="w-full">
                <Button variant="outline" className="w-full font-semibold">
                  Return to My Assessments
                </Button>
              </Link>
            </CardContent>
          </Card>
        </div>
      </ProtectedRoute>
    );
  }

  // Terminal State: SUBMITTED
  if (sessionStatus === "SUBMITTED") {
    return (
      <ProtectedRoute allowedRoles={["STUDENT", "ADMIN", "PROCTOR"]}>
        <div className="flex min-h-screen items-center justify-center p-4 bg-background">
          <Card className="w-full max-w-md text-center p-6 shadow-md border" data-testid="exam-submitted-card">
            <CardHeader className="flex flex-col items-center">
              <div className="size-16 rounded-full bg-emerald-100 dark:bg-emerald-950 flex items-center justify-center mb-4">
                <CheckCircle2 className="size-10 text-emerald-600 dark:text-emerald-400" />
              </div>
              <CardTitle className="text-2xl font-bold">Exam Submitted Successfully</CardTitle>
              <CardDescription className="mt-2 text-sm text-muted-foreground" suppressHydrationWarning>
                Your answers have been permanently recorded. Submitted at{" "}
                <strong className="text-foreground">{submittedAt || "just now"}</strong>.
              </CardDescription>
            </CardHeader>
            <CardContent className="mt-6 flex flex-col gap-3">
              <div className="rounded-lg bg-muted p-3 text-xs text-muted-foreground">
                Total questions answered: <strong>{Object.keys(answers).length}</strong>
              </div>
              <Link href="/my-exams" className="w-full">
                <Button className="w-full font-semibold gap-2">
                  <Home className="size-4" />
                  Return to Candidate Dashboard
                </Button>
              </Link>
            </CardContent>
          </Card>
        </div>
      </ProtectedRoute>
    );
  }

  // Terminal State: EXPIRED
  if (sessionStatus === "EXPIRED") {
    return (
      <ProtectedRoute allowedRoles={["STUDENT", "ADMIN", "PROCTOR"]}>
        <div className="flex min-h-screen items-center justify-center p-4 bg-background">
          <Card className="w-full max-w-md text-center p-6 shadow-md border border-destructive/30" data-testid="exam-expired-card">
            <CardHeader className="flex flex-col items-center">
              <div className="size-16 rounded-full bg-amber-100 dark:bg-amber-950 flex items-center justify-center mb-4">
                <AlertTriangle className="size-10 text-amber-600 dark:text-amber-400" />
              </div>
              <CardTitle className="text-2xl font-bold">Exam Time Expired</CardTitle>
              <CardDescription className="mt-2 text-sm text-muted-foreground">
                The allotted time for this examination has ended. All confirmed answers have been sealed.
              </CardDescription>
            </CardHeader>
            <CardContent className="mt-6 flex flex-col gap-3">
              <Link href="/my-exams" className="w-full">
                <Button className="w-full font-semibold gap-2">
                  <Home className="size-4" />
                  Return to Candidate Dashboard
                </Button>
              </Link>
            </CardContent>
          </Card>
        </div>
      </ProtectedRoute>
    );
  }

  // Active State: IN_PROGRESS
  return (
    <ProtectedRoute allowedRoles={["STUDENT", "ADMIN", "PROCTOR"]}>
      <StudentExamRoom
        currentQuestion={currentQuestion}
        totalQuestions={currentQuestion?.totalQuestions || 1}
        initialSeconds={initialSeconds}
        answers={answers}
        saveStatus={saveStatus}
        saveError={saveError}
        onSelectOption={handleSelectOption}
        onTextAnswerChange={handleTextAnswerChange}
        onRetrySave={handleRetrySave}
        onNext={handleNext}
        onPrevious={handlePrevious}
        onSubmit={handleSubmit}
        navigating={navigating}
        submitting={submitting}
        examTitle={examTitle}
        subject={subject}
        onExpire={handleExpire}
        navigationNotice={navigationNotice}
        voiceSlot={
          <VoiceAssistant
            sessionId={sessionId}
            questionId={currentQuestion?.id ?? null}
            disabled={navigating || submitting}
            allowVoiceAnswer={currentQuestion ? currentQuestion.type !== "MCQ" : false}
            onActivityChange={handleVoiceActivity}
            onSessionState={handleVoiceSessionState}
            onAnswerSaved={handleVoiceAnswerSaved}
            onSessionEnded={loadSession}
          />
        }
      />
    </ProtectedRoute>
  );
}
