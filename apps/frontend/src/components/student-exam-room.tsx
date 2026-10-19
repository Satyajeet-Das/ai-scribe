"use client";

import { useEffect, useMemo, useRef, useState, type ReactNode } from "react";
import {
  Check,
  ChevronLeft,
  ChevronRight,
  Clock3,
  Eye,
  Highlighter,
  Loader2,
  AlertCircle,
  RotateCcw,
  Send,
  AlertTriangle,
  HelpCircle,
} from "lucide-react";
import {
  AlertDialog,
  AlertDialogAction,
  AlertDialogCancel,
  AlertDialogContent,
  AlertDialogDescription,
  AlertDialogFooter,
  AlertDialogHeader,
  AlertDialogTitle,
} from "@/components/ui/alert-dialog";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { RadioGroup, RadioGroupItem } from "@/components/ui/radio-group";
import { Switch } from "@/components/ui/switch";
import { Textarea } from "@/components/ui/textarea";
import {
  hasNoModifiers,
  isInsideLayer,
  isPlainShortcutEvent,
  isSurfaceTarget,
  isTextEntryTarget,
  shortcutLetter,
} from "@/lib/keyboard";
import type { StudentQuestionResponse, StudentOptionResponse, Question } from "@/types/exam-types";

export interface StudentExamRoomProps {
  currentQuestion: StudentQuestionResponse | null;
  totalQuestions?: number;
  initialSeconds?: number;
  answers: Record<string, string>;
  /** "unsaved" means typed text the server has not confirmed yet. */
  saveStatus?: "idle" | "unsaved" | "saving" | "saved" | "failed";
  saveError?: string | null;
  onSelectOption: (optionId: string) => void;
  onTextAnswerChange?: (text: string) => void;
  onRetrySave?: () => void;
  onNext: () => Promise<void> | void;
  onPrevious: () => Promise<void> | void;
  onSubmit: () => Promise<void> | void;
  onJumpToQuestion?: (index: number) => void;
  navigating?: boolean;
  submitting?: boolean;
  examTitle?: string;
  subject?: string;
  onExpire?: () => void;
  /** Optional voice assistant, rendered above the question. */
  voiceSlot?: ReactNode;
  /** Why the last attempt to change question was refused, if it was. */
  navigationNotice?: string | null;
  /** How many other questions have an answer whose save failed. */
  unsavedElsewhere?: number;
  /**
   * Asked when the submit dialog opens: what submitting now would throw away
   * (a recording in progress, an answer not yet confirmed), if anything.
   */
  getSubmitWarning?: () => string | null;

  // Backward compatibility with previous offline mock usage if any
  questions?: Question[];
  onAnswerChange?: (questionId: string, answer: string) => void;
}

export function StudentExamRoom({
  currentQuestion,
  totalQuestions: propTotalQuestions,
  initialSeconds = 45 * 60,
  answers,
  saveStatus = "idle",
  saveError = null,
  onSelectOption,
  onTextAnswerChange,
  onRetrySave,
  onNext,
  onPrevious,
  onSubmit,
  onJumpToQuestion,
  navigating = false,
  submitting = false,
  examTitle = "Online Examination",
  subject = "Assessment",
  onExpire,
  voiceSlot,
  navigationNotice = null,
  unsavedElsewhere = 0,
  getSubmitWarning,
  // backward compatibility props
  questions,
  onAnswerChange,
}: StudentExamRoomProps) {
  const [seconds, setSeconds] = useState(initialSeconds);
  const [largeText, setLargeText] = useState(false);
  const [contrast, setContrast] = useState(false);
  const [submitDialogOpen, setSubmitDialogOpen] = useState(false);
  const [submitWarning, setSubmitWarning] = useState<string | null>(null);
  const [boundaryNote, setBoundaryNote] = useState<{ seq: number; text: string } | null>(null);
  const headingRef = useRef<HTMLHeadingElement>(null);
  const shownQuestionRef = useRef<string | null>(null);

  const openSubmitDialog = () => {
    setSubmitWarning(getSubmitWarning?.() ?? null);
    setSubmitDialogOpen(true);
  };

  // Time running out is reported once, however often the page re-renders. A
  // reconnect that brings more time arms it again.
  const onExpireRef = useRef(onExpire);
  useEffect(() => {
    onExpireRef.current = onExpire;
  }, [onExpire]);
  const expiredRef = useRef(false);

  // Sync initial seconds if it changes on reconnect
  useEffect(() => {
    setSeconds(initialSeconds);
    if (initialSeconds > 0) expiredRef.current = false;
  }, [initialSeconds]);

  // Countdown timer
  useEffect(() => {
    if (seconds <= 0) {
      if (!expiredRef.current) {
        expiredRef.current = true;
        onExpireRef.current?.();
      }
      return;
    }
    const timer = window.setTimeout(() => setSeconds((prev) => Math.max(0, prev - 1)), 1000);
    return () => window.clearTimeout(timer);
  }, [seconds]);

  // Keyboard navigation shortcuts.
  //  - Alt+N / Alt+P work unless the student is typing or a dialog is open.
  //  - Left / Right arrows work only when focus is on the page itself. On any
  //    control they keep their own meaning: moving a text cursor, or moving
  //    between the options of a multiple-choice question.
  useEffect(() => {
    function handleKeyDown(e: KeyboardEvent) {
      if (!isPlainShortcutEvent(e) || submitDialogOpen) return;

      if (e.key === "ArrowRight" || e.key === "ArrowLeft") {
        if (!hasNoModifiers(e) || !isSurfaceTarget(e.target)) return;
        e.preventDefault();
        if (navigating || submitting || !currentQuestion) return;
        const forward = e.key === "ArrowRight";
        if (forward ? currentQuestion?.isLast : currentQuestion?.isFirst) {
          setBoundaryNote((prev) => ({
            seq: (prev?.seq ?? 0) + 1,
            text: forward ? "This is the last question." : "This is the first question.",
          }));
          return;
        }
        if (forward) onNext();
        else onPrevious();
        return;
      }

      if (isTextEntryTarget(e.target) || isInsideLayer(e.target)) return;
      if (!e.altKey || e.ctrlKey || e.metaKey) return;
      const key = shortcutLetter(e);
      if (key !== "n" && key !== "p") return;
      e.preventDefault();
      if (navigating || submitting || !currentQuestion) return;
      if (key === "n" && !currentQuestion.isLast) onNext();
      else if (key === "p" && !currentQuestion.isFirst) onPrevious();
    }

    window.addEventListener("keydown", handleKeyDown);
    return () => window.removeEventListener("keydown", handleKeyDown);
  }, [currentQuestion, navigating, submitting, submitDialogOpen, onNext, onPrevious]);

  const formattedTime = useMemo(() => {
    const m = Math.floor(seconds / 60);
    const s = seconds % 60;
    return `${String(m).padStart(2, "0")}:${String(s).padStart(2, "0")}`;
  }, [seconds]);

  const totalQuestions = propTotalQuestions || currentQuestion?.totalQuestions || questions?.length || 1;
  const currentIndex = currentQuestion?.currentIndex || 1;
  const answeredCount = Object.keys(answers).length;
  const unansweredCount = Math.max(0, totalQuestions - answeredCount);
  const progressPercent = Math.min(100, Math.round((answeredCount / totalQuestions) * 100));

  // Determine active question data (support both StudentQuestionResponse and legacy Question)
  const activeQuestionText = currentQuestion?.text || (questions && questions[currentIndex - 1]?.text) || "Loading question...";
  const activeQuestionPoints = currentQuestion?.points ?? (questions && questions[currentIndex - 1]?.points) ?? 1;
  const activeQuestionType = currentQuestion?.type || (questions && questions[currentIndex - 1]?.type) || "MCQ";
  const activeQuestionId = currentQuestion?.id || (questions && questions[currentIndex - 1]?.id) || "";
  const activeOptions = currentQuestion?.options || (questions && questions[currentIndex - 1]?.options) || [];

  const isFirst = currentQuestion ? currentQuestion.isFirst : currentIndex <= 1;
  const isLast = currentQuestion ? currentQuestion.isLast : currentIndex >= totalQuestions;

  const currentAnswerValue = answers[activeQuestionId] || "";

  // When the question changes, move focus to its heading so a screen reader
  // reads the new question and the arrow keys work straight away. Not on the
  // first load, which would pull focus away from the page's own start.
  useEffect(() => {
    if (!activeQuestionId) return;
    if (shownQuestionRef.current !== null && shownQuestionRef.current !== activeQuestionId) {
      headingRef.current?.focus();
    }
    shownQuestionRef.current = activeQuestionId;
  }, [activeQuestionId]);

  const handleOptionSelect = (optionId: string) => {
    if (navigating || submitting) return;
    onSelectOption(optionId);
    if (onAnswerChange && activeQuestionId) {
      onAnswerChange(activeQuestionId, optionId);
    }
  };

  const handleTextChange = (val: string) => {
    if (onTextAnswerChange) {
      onTextAnswerChange(val);
    }
    if (onAnswerChange && activeQuestionId) {
      onAnswerChange(activeQuestionId, val);
    }
  };

  return (
    <div
      className={`${largeText ? "text-lg" : ""} ${contrast ? "bg-foreground text-background" : "bg-background text-foreground"} min-h-screen flex flex-col`}
    >
      {/* Header */}
      <header className="sticky top-0 z-20 border-b bg-background/95 backdrop-blur shadow-xs">
        <div className="mx-auto flex max-w-6xl flex-wrap items-center justify-between gap-4 px-4 py-3 sm:px-6">
          <div className="flex items-center gap-3">
            <div>
              <span className="text-xs font-medium text-muted-foreground uppercase tracking-wider">
                {subject}
              </span>
              <h1 className="text-lg font-bold sm:text-xl line-clamp-1">{examTitle}</h1>
            </div>
          </div>

          <div className="flex items-center gap-3">
            {/* Countdown Badge */}
            <Badge
              variant={seconds < 300 ? "destructive" : "secondary"}
              className={`px-3 py-1.5 text-sm font-semibold tabular-nums gap-1.5 ${
                seconds < 300 ? "animate-pulse" : ""
              }`}
              aria-label={`Time remaining: ${formattedTime}`}
            >
              <Clock3 className="size-4 shrink-0" />
              <span>{formattedTime}</span>
            </Badge>

            {/* Submit Button Trigger */}
            <Button
              variant="default"
              size="sm"
              onClick={openSubmitDialog}
              className="gap-2 font-semibold shadow-xs"
              data-testid="submit-exam-button"
            >
              <Send className="size-3.5 fill-current" />
              <span>Submit Exam</span>
            </Button>
          </div>
        </div>
      </header>

      {/* Main Workspace */}
      <div className="mx-auto flex-1 w-full max-w-6xl grid gap-6 p-4 sm:p-6 md:grid-cols-[240px_1fr]">
        {/* Left Sidebar / Question Navigator */}
        <aside className="flex flex-col gap-5">
          {/* Accessibility Settings */}
          <Card className="bg-background shadow-xs border">
            <CardHeader className="py-3 px-4">
              <CardTitle className="text-xs font-semibold uppercase tracking-wider text-muted-foreground">
                Display Options
              </CardTitle>
            </CardHeader>
            <CardContent className="flex flex-col gap-3.5 px-4 pb-4">
              <ToggleRow
                label="Large text"
                icon={Eye}
                checked={largeText}
                onCheckedChange={setLargeText}
              />
              <ToggleRow
                label="High contrast"
                icon={Highlighter}
                checked={contrast}
                onCheckedChange={setContrast}
              />
            </CardContent>
          </Card>

          {/* Question Grid Navigator */}
          <Card className="bg-background shadow-xs border flex-1">
            <CardHeader className="py-3 px-4">
              <div className="flex items-center justify-between">
                <CardTitle className="text-xs font-semibold uppercase tracking-wider text-muted-foreground">
                  Questions Navigator
                </CardTitle>
                <span className="text-xs font-semibold text-primary">
                  {answeredCount}/{totalQuestions}
                </span>
              </div>
            </CardHeader>
            <CardContent className="px-4 pb-4 space-y-4">
              <nav
                aria-label="Question navigation grid"
                className="grid grid-cols-5 gap-2 sm:grid-cols-4"
              >
                {Array.from({ length: totalQuestions }).map((_, qIdx) => {
                  const qNum = qIdx + 1;
                  const isCurrent = currentIndex === qNum;
                  // If question ID is known for this index, check answers
                  const isAnswered =
                    currentQuestion && currentQuestion.questionNumber === qNum
                      ? !!currentAnswerValue
                      : false;

                  const tileClass = `grid size-9 place-items-center rounded-lg border text-xs font-semibold transition-all ${
                    isCurrent
                      ? "border-primary bg-primary text-primary-foreground shadow-xs font-bold scale-105"
                      : isAnswered
                      ? "border-primary/50 bg-primary/10 text-primary"
                      : "bg-background text-muted-foreground"
                  }`;
                  const label = `Question ${qNum}${isAnswered ? ", Answered" : ", Unanswered"}`;

                  // Without a way to jump, a tile is a status marker, not a
                  // button that appears to work and does nothing.
                  if (!onJumpToQuestion) {
                    return (
                      <span
                        key={qIdx}
                        role="img"
                        aria-label={label}
                        aria-current={isCurrent ? "step" : undefined}
                        className={tileClass}
                      >
                        {qNum}
                      </span>
                    );
                  }
                  return (
                    <button
                      key={qIdx}
                      type="button"
                      aria-label={label}
                      aria-current={isCurrent ? "step" : undefined}
                      disabled={navigating || submitting}
                      onClick={() => onJumpToQuestion(qIdx)}
                      className={`${tileClass} hover:bg-muted focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring`}
                    >
                      {qNum}
                    </button>
                  );
                })}
              </nav>

              <div className="border-t pt-3 space-y-1.5 text-[11px] text-muted-foreground">
                <div className="flex items-center gap-2">
                  <span className="size-2.5 rounded-sm bg-primary" />
                  <span>Current Question</span>
                </div>
                <div className="flex items-center gap-2">
                  <span className="size-2.5 rounded-sm bg-primary/20 border border-primary/40" />
                  <span>Answered Question</span>
                </div>
                <div className="flex items-center gap-2">
                  <span className="size-2.5 rounded-sm bg-muted border" />
                  <span>Unanswered</span>
                </div>
              </div>
            </CardContent>
          </Card>
        </aside>

        {/* Main Examination Question Area */}
        <main className="flex flex-col gap-4">
          {voiceSlot}
          {navigationNotice && (
            <div
              role="alert"
              data-testid="navigation-notice"
              className="rounded-lg border border-amber-500/40 bg-amber-500/10 p-3 text-sm text-amber-900 dark:text-amber-200 flex items-start gap-2"
            >
              <AlertTriangle className="size-4 shrink-0 mt-0.5" aria-hidden="true" />
              <span>{navigationNotice}</span>
            </div>
          )}
          {unsavedElsewhere > 0 && (
            <div
              role="alert"
              data-testid="unsaved-elsewhere-notice"
              className="rounded-lg border border-destructive/30 bg-destructive/10 p-3 text-sm text-destructive flex items-center justify-between gap-3"
            >
              <span className="flex items-start gap-2">
                <AlertTriangle className="size-4 shrink-0 mt-0.5" aria-hidden="true" />
                {unsavedElsewhere === 1
                  ? "Your answer to another question has not been saved yet. It is kept on this device."
                  : `Your answers to ${unsavedElsewhere} other questions have not been saved yet. They are kept on this device.`}
              </span>
              {onRetrySave && (
                <Button
                  size="xs"
                  variant="destructive"
                  onClick={onRetrySave}
                  className="gap-1 font-semibold shrink-0"
                  data-testid="retry-elsewhere-button"
                >
                  <RotateCcw className="size-3" aria-hidden="true" />
                  Retry saving
                </Button>
              )}
            </div>
          )}
          <div className="sr-only" role="status" aria-live="polite" data-testid="boundary-status">
            {boundaryNote && <span key={boundaryNote.seq}>{boundaryNote.text}</span>}
          </div>
          <Card className="min-h-[500px] flex flex-col justify-between bg-background shadow-xs border">
            <div>
              <CardHeader className="border-b pb-4">
                <div className="flex flex-wrap items-center justify-between gap-3">
                  <div className="flex items-center gap-2">
                    <Badge variant="outline" className="font-semibold text-xs">
                      Question {currentIndex} of {totalQuestions}
                    </Badge>
                    <Badge variant="secondary" className="text-xs">
                      {activeQuestionPoints} {activeQuestionPoints === 1 ? "point" : "points"}
                    </Badge>
                  </div>

                  {/* Real-time Save Status Indicator */}
                  <div
                    className="flex items-center gap-1.5 text-xs font-medium"
                    role="status"
                    aria-live="polite"
                    data-testid="answer-save-status"
                  >
                    {saveStatus === "saving" && (
                      <span className="flex items-center gap-1.5 text-amber-600 dark:text-amber-400">
                        <Loader2 className="size-3.5 animate-spin" />
                        Saving answer...
                      </span>
                    )}
                    {saveStatus === "unsaved" && (
                      <span className="flex items-center gap-1.5 text-amber-600 dark:text-amber-400">
                        <AlertCircle className="size-3.5" />
                        Not saved yet
                      </span>
                    )}
                    {saveStatus === "saved" && (
                      <span className="flex items-center gap-1.5 text-emerald-600 dark:text-emerald-400">
                        <Check className="size-3.5 text-emerald-600 stroke-[3]" />
                        Answer saved
                      </span>
                    )}
                    {saveStatus === "failed" && (
                      <span className="flex items-center gap-1.5 text-destructive font-semibold">
                        <AlertCircle className="size-3.5" />
                        Failed to save
                        {onRetrySave && (
                          <button
                            type="button"
                            onClick={onRetrySave}
                            className="underline ml-1 hover:text-destructive/80 font-bold"
                            data-testid="retry-save-button"
                          >
                            Retry
                          </button>
                        )}
                      </span>
                    )}
                    {saveStatus === "idle" && currentAnswerValue && (
                      <span className="flex items-center gap-1.5 text-muted-foreground">
                        <Check className="size-3.5 text-muted-foreground" />
                        Recorded
                      </span>
                    )}
                  </div>
                </div>

                {/* Question Prompt */}
                <h2
                  ref={headingRef}
                  tabIndex={-1}
                  className="mt-4 text-xl sm:text-2xl font-bold leading-relaxed text-foreground outline-none focus-visible:ring-2 focus-visible:ring-ring rounded-sm"
                >
                  <span className="sr-only">
                    Question {currentIndex} of {totalQuestions}.{" "}
                  </span>
                  {activeQuestionText}
                </h2>
              </CardHeader>

              {/* Options & Response Input */}
              <CardContent className="pt-6">
                {/* Save Error Banner */}
                {saveStatus === "failed" && (
                  <div className="mb-6 rounded-lg bg-destructive/10 border border-destructive/20 p-3 text-xs text-destructive flex items-center justify-between gap-3">
                    <div className="flex items-center gap-2">
                      <AlertTriangle className="size-4 shrink-0" />
                      <span>{saveError || "Failed to persist answer. Your response is saved locally."}</span>
                    </div>
                    {onRetrySave && (
                      <Button size="xs" variant="destructive" onClick={onRetrySave} className="gap-1 font-semibold" data-testid="retry-banner-button">
                        <RotateCcw className="size-3" />
                        Retry
                      </Button>
                    )}
                  </div>
                )}

                {activeQuestionType === "MCQ" ? (
                  <RadioGroup
                    value={currentAnswerValue}
                    onValueChange={handleOptionSelect}
                    aria-label="Answer choices"
                    className="flex flex-col gap-3"
                    disabled={navigating || submitting}
                  >
                    {activeOptions.map((opt: StudentOptionResponse | any) => {
                      const isSelected = currentAnswerValue === opt.id;
                      return (
                        <label
                          key={opt.id}
                          htmlFor={`option-${opt.id}`}
                          data-testid={`option-label-${opt.id}`}
                          className={`flex cursor-pointer items-center gap-4 rounded-xl border p-4 transition-all focus-within:ring-2 focus-within:ring-ring ${
                            isSelected
                              ? "border-primary bg-primary/5 shadow-xs font-medium"
                              : "border-border hover:bg-muted/50 hover:border-border/80"
                          }`}
                        >
                          <RadioGroupItem
                            value={opt.id}
                            id={`option-${opt.id}`}
                            className="size-4"
                          />
                          <span
                            className={`grid size-7 place-items-center rounded-md text-xs font-bold ${
                              isSelected
                                ? "bg-primary text-primary-foreground"
                                : "bg-muted text-muted-foreground"
                            }`}
                          >
                            {opt.optionKey}
                          </span>
                          <span className="flex-1 text-sm sm:text-base leading-snug">
                            {opt.optionText}
                          </span>
                        </label>
                      );
                    })}
                  </RadioGroup>
                ) : (
                  <div className="flex flex-col gap-3">
                    <label htmlFor="written-response" className="text-sm font-semibold text-foreground">
                      Your Response
                    </label>
                    <Textarea
                      id="written-response"
                      value={currentAnswerValue}
                      onChange={(e) => handleTextChange(e.target.value)}
                      placeholder="Type your response here..."
                      className="min-h-48 text-base leading-relaxed"
                      disabled={navigating || submitting}
                    />
                    <p className="text-xs text-muted-foreground">
                      Your response is saved a moment after you stop typing. The status above shows
                      when it has been saved.
                    </p>
                  </div>
                )}
              </CardContent>
            </div>

            {/* Bottom Navigation Toolbar */}
            <div className="border-t p-4 sm:px-6 flex items-center justify-between gap-4 bg-muted/20">
              <Button
                variant="outline"
                size="sm"
                onClick={onPrevious}
                disabled={isFirst || navigating || submitting}
                className="gap-1.5 font-semibold"
                data-testid="prev-question-button"
              >
                <ChevronLeft className="size-4" />
                <span>Previous</span>
              </Button>

              {/* Progress Summary Center */}
              <div className="hidden sm:flex items-center gap-3">
                <div className="w-32 bg-muted rounded-full h-2 overflow-hidden">
                  <div
                    className="bg-primary h-full transition-all duration-300"
                    style={{ width: `${progressPercent}%` }}
                  />
                </div>
                <span className="text-xs text-muted-foreground font-medium">
                  {progressPercent}% answered
                </span>
              </div>

              {isLast ? (
                <Button
                  size="sm"
                  variant="default"
                  onClick={openSubmitDialog}
                  disabled={navigating || submitting}
                  className="gap-1.5 font-semibold bg-emerald-600 hover:bg-emerald-700 text-white"
                  data-testid="last-question-review-button"
                >
                  <span>Review & Submit</span>
                  <Send className="size-3.5 fill-current" />
                </Button>
              ) : (
                <Button
                  size="sm"
                  onClick={onNext}
                  disabled={navigating || submitting}
                  className="gap-1.5 font-semibold"
                  data-testid="next-question-button"
                >
                  {navigating ? (
                    <>
                      <Loader2 className="size-4 animate-spin" />
                      <span>Loading...</span>
                    </>
                  ) : (
                    <>
                      <span>Next</span>
                      <ChevronRight className="size-4" />
                    </>
                  )}
                </Button>
              )}
            </div>
          </Card>
        </main>
      </div>

      {/* Submit Confirmation Dialog */}
      <AlertDialog open={submitDialogOpen} onOpenChange={setSubmitDialogOpen}>
        <AlertDialogContent className="max-w-md bg-background p-6">
          <AlertDialogHeader>
            <AlertDialogTitle className="text-xl font-bold flex items-center gap-2">
              <HelpCircle className="size-5 text-primary" />
              Submit Exam Confirmation
            </AlertDialogTitle>
            <AlertDialogDescription className="pt-2 text-sm text-muted-foreground">
              You have answered <strong className="text-foreground">{answeredCount}</strong> of{" "}
              <strong className="text-foreground">{totalQuestions}</strong> questions.
            </AlertDialogDescription>
            {unansweredCount > 0 && (
              <div className="mt-2 rounded-lg bg-amber-500/10 border border-amber-500/20 p-3 text-xs text-amber-900 dark:text-amber-200 flex items-start gap-2 text-left">
                <AlertTriangle className="size-4 shrink-0 text-amber-600 mt-0.5" />
                <span>
                  Warning: You still have <strong data-testid="unanswered-count">{unansweredCount}</strong> unanswered question(s). You cannot return after submission.
                </span>
              </div>
            )}
            {submitWarning && (
              <div
                role="alert"
                data-testid="submit-voice-warning"
                className="mt-2 rounded-lg bg-destructive/10 border border-destructive/20 p-3 text-xs text-destructive flex items-start gap-2 text-left"
              >
                <AlertTriangle className="size-4 shrink-0 mt-0.5" aria-hidden="true" />
                <span>{submitWarning}</span>
              </div>
            )}
            <p className="mt-2 text-xs text-muted-foreground border-t pt-2 text-left">
              Submissions are final. Your active timer will stop, and responses will be locked.
            </p>
          </AlertDialogHeader>

          <AlertDialogFooter className="mt-4 flex flex-col-reverse sm:flex-row gap-2">
            <AlertDialogCancel disabled={submitting}>Keep Working</AlertDialogCancel>
            <AlertDialogAction
              onClick={async (e) => {
                e.preventDefault();
                await onSubmit();
              }}
              disabled={submitting}
              className="gap-2 bg-primary font-semibold"
              data-testid="confirm-submit-dialog-button"
            >
              {submitting ? (
                <>
                  <Loader2 className="size-4 animate-spin" />
                  <span>Submitting...</span>
                </>
              ) : (
                <>
                  <Send className="size-3.5 fill-current" />
                  <span>{submitWarning ? "Submit anyway" : "Confirm & Submit"}</span>
                </>
              )}
            </AlertDialogAction>
          </AlertDialogFooter>
        </AlertDialogContent>
      </AlertDialog>
    </div>
  );
}

function ToggleRow({
  label,
  icon: Icon,
  checked,
  onCheckedChange,
}: {
  label: string;
  icon: typeof Eye;
  checked: boolean;
  onCheckedChange: (value: boolean) => void;
}) {
  return (
    <label className="flex items-center justify-between gap-3 text-xs font-medium cursor-pointer">
      <span className="flex items-center gap-2 text-muted-foreground">
        <Icon className="size-4 shrink-0" aria-hidden="true" />
        {label}
      </span>
      <Switch checked={checked} onCheckedChange={onCheckedChange} aria-label={label} />
    </label>
  );
}

export default StudentExamRoom;
