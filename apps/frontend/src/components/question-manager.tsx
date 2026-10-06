"use client";

import { useState, useEffect } from "react";
import {
  FilePlus2,
  Mic,
  Plus,
  Trash2,
  Pencil,
  ArrowUp,
  ArrowDown,
  Type,
  Loader2,
  AlertCircle,
  CheckCircle2,
  Lock,
  Check,
} from "lucide-react";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Textarea } from "@/components/ui/textarea";
import { questionsApi, ApiError } from "@/services/api";
import { CreateQuestionSchema } from "@/lib/validations";
import type {
  Question,
  QuestionType,
  QuestionOption,
  ExamStatus,
} from "@/types/exam-types";

const fallbackQuestions: Question[] = [
  {
    id: "q-1",
    examId: "exam-1",
    questionNumber: 1,
    text: "Which structure controls what enters and leaves a cell?",
    type: "MCQ",
    points: 2,
    options: [
      { id: "a", optionKey: "A", optionText: "Cell wall", displayOrder: 1, isCorrect: false },
      { id: "b", optionKey: "B", optionText: "Cell membrane", displayOrder: 2, isCorrect: true },
      { id: "c", optionKey: "C", optionText: "Nucleus", displayOrder: 3, isCorrect: false },
    ],
  },
  {
    id: "q-2",
    examId: "exam-1",
    questionNumber: 2,
    text: "Explain how a plant uses sunlight to make food.",
    type: "ESSAY",
    points: 5,
    options: [],
  },
];

export interface QuestionManagerProps {
  examId?: string;
  examStatus?: ExamStatus;
  questions?: Question[];
  initialQuestions?: Question[];
  onQuestionsChange?: (questions: Question[]) => void;
  isReadOnly?: boolean;
}

export function QuestionManager({
  examId = "exam-1",
  examStatus = "DRAFT",
  questions,
  initialQuestions,
  onQuestionsChange,
  isReadOnly: explicitReadOnly,
}: QuestionManagerProps) {
  const incomingQuestions = questions ?? initialQuestions ?? fallbackQuestions;
  const [items, setItems] = useState<Question[]>(incomingQuestions);
  const [selectedId, setSelectedId] = useState<string>(incomingQuestions[0]?.id ?? "");

  // Dialog states
  const [editorOpen, setEditorOpen] = useState(false);
  const [editingQuestion, setEditingQuestion] = useState<Question | null>(null);
  const [deleteConfirmQuestion, setDeleteConfirmQuestion] = useState<Question | null>(null);

  // Async states
  const [deleteLoading, setDeleteLoading] = useState(false);
  const [reordering, setReordering] = useState(false);
  const [feedback, setFeedback] = useState<{ type: "success" | "error"; message: string } | null>(
    null
  );

  const isReadOnly = explicitReadOnly ?? (examStatus !== "DRAFT");

  // Synchronize when parent passes updated questions
  const [prevQuestions, setPrevQuestions] = useState(questions);
  if (questions !== prevQuestions) {
    setPrevQuestions(questions);
    setItems(questions ?? []);
  }

  // Clear feedback after 5 seconds
  useEffect(() => {
    if (!feedback) return;
    const timer = setTimeout(() => setFeedback(null), 5000);
    return () => clearTimeout(timer);
  }, [feedback]);

  const updateItemsAndNotify = (newItems: Question[]) => {
    setItems(newItems);
    onQuestionsChange?.(newItems);
  };

  const selectedQuestion = items.find((q) => q.id === selectedId) || items[0];

  const totalPoints = items.reduce((acc, curr) => acc + (curr.points || 0), 0);

  // ---------------------------------------------------------------------------
  // Reordering Logic
  // ---------------------------------------------------------------------------
  const handleMoveQuestion = async (questionId: string, direction: "up" | "down") => {
    if (isReadOnly || reordering) return;

    const currentIndex = items.findIndex((q) => q.id === questionId);
    if (currentIndex === -1) return;
    const targetIndex = direction === "up" ? currentIndex - 1 : currentIndex + 1;
    if (targetIndex < 0 || targetIndex >= items.length) return;

    const reorderedList = [...items];
    const [moved] = reorderedList.splice(currentIndex, 1);
    reorderedList.splice(targetIndex, 0, moved);

    // Re-assign 1-based question numbers sequentially
    const updatedWithNumbers = reorderedList.map((q, idx) => ({
      ...q,
      questionNumber: idx + 1,
    }));

    // Optimistic UI update
    updateItemsAndNotify(updatedWithNumbers);
    setReordering(true);

    try {
      const qIds = updatedWithNumbers.map((q) => q.id);
      await questionsApi.reorderQuestions(examId, { questionIds: qIds });
      setFeedback({
        type: "success",
        message: `Question ${moved.questionNumber} moved ${direction} successfully.`,
      });
    } catch (err) {
      // Revert optimistic update on failure
      updateItemsAndNotify(items);
      const msg = err instanceof ApiError ? err.message : "Failed to reorder questions.";
      setFeedback({ type: "error", message: msg });
    } finally {
      setReordering(false);
    }
  };

  // ---------------------------------------------------------------------------
  // Delete Question Logic
  // ---------------------------------------------------------------------------
  const confirmDeleteQuestion = async () => {
    if (!deleteConfirmQuestion || isReadOnly) return;
    const targetId = deleteConfirmQuestion.id;
    const qNum = deleteConfirmQuestion.questionNumber;

    setDeleteLoading(true);
    try {
      await questionsApi.deleteQuestion(targetId);

      const remaining = items
        .filter((q) => q.id !== targetId)
        .map((q, idx) => ({ ...q, questionNumber: idx + 1 }));

      updateItemsAndNotify(remaining);

      // Shift selection
      if (selectedId === targetId) {
        setSelectedId(remaining[0]?.id ?? "");
      }

      setFeedback({
        type: "success",
        message: `Question ${qNum} deleted and remaining questions re-sequenced.`,
      });
      setDeleteConfirmQuestion(null);
    } catch (err) {
      const msg = err instanceof ApiError ? err.message : "Failed to delete question.";
      setFeedback({ type: "error", message: msg });
    } finally {
      setDeleteLoading(false);
    }
  };

  // ---------------------------------------------------------------------------
  // Open Add / Edit Dialogs
  // ---------------------------------------------------------------------------
  const handleOpenAdd = () => {
    if (isReadOnly) return;
    setEditingQuestion(null);
    setEditorOpen(true);
  };

  const handleOpenEdit = (q: Question) => {
    if (isReadOnly) return;
    setEditingQuestion(q);
    setEditorOpen(true);
  };

  return (
    <div className="space-y-4">
      {/* Read-Only Alert Banner for Non-Draft Exams */}
      {isReadOnly && (
        <div
          role="status"
          className="flex items-center gap-2.5 rounded-lg border border-amber-300/80 bg-amber-50 dark:border-amber-800 dark:bg-amber-950/40 p-3.5 text-xs text-amber-900 dark:text-amber-200"
        >
          <Lock className="size-4 shrink-0 text-amber-600 dark:text-amber-400" />
          <span>
            {examStatus === "ARCHIVED"
              ? "Archived Assessment (Read-Only) — questions cannot be modified."
              : "Published Assessment (Read-Only) — questions are locked to maintain assessment integrity."}
          </span>
        </div>
      )}

      {/* Dismissible Feedback Alert */}
      {feedback && (
        <div
          role="alert"
          className={`flex items-center justify-between gap-3 rounded-lg border p-3.5 text-xs font-medium transition-all ${
            feedback.type === "success"
              ? "bg-emerald-50 dark:bg-emerald-950/40 border-emerald-300 dark:border-emerald-800 text-emerald-900 dark:text-emerald-200"
              : "bg-destructive/10 border-destructive/20 text-destructive"
          }`}
        >
          <div className="flex items-center gap-2">
            {feedback.type === "success" ? (
              <CheckCircle2 className="size-4 text-emerald-600 dark:text-emerald-400 shrink-0" />
            ) : (
              <AlertCircle className="size-4 text-destructive shrink-0" />
            )}
            <span>{feedback.message}</span>
          </div>
          <button
            type="button"
            onClick={() => setFeedback(null)}
            className="text-xs opacity-70 hover:opacity-100 underline ml-4"
          >
            Dismiss
          </button>
        </div>
      )}

      {/* Main Studio Grid */}
      <div className="grid gap-6 md:grid-cols-[320px_1fr]">
        {/* Sidebar: Question List & Studio Header */}
        <aside className="flex flex-col gap-4">
          <div className="flex items-center justify-between border-b pb-3">
            <div>
              <h2 className="text-lg font-bold tracking-tight">Question Studio</h2>
              <p className="text-xs text-muted-foreground">
                {items.length} {items.length === 1 ? "question" : "questions"} ({totalPoints} pts total)
              </p>
            </div>

            {!isReadOnly && (
              <Button
                size="sm"
                className="font-semibold gap-1.5"
                onClick={handleOpenAdd}
                disabled={reordering}
                aria-label="Add a new question"
              >
                <Plus className="size-4" />
                Add Question
              </Button>
            )}
          </div>

          {/* List of Questions */}
          {items.length === 0 ? (
            <div className="rounded-lg border border-dashed p-6 text-center text-xs text-muted-foreground">
              No questions configured yet.
            </div>
          ) : (
            <div
              className="flex flex-col gap-2 max-h-[640px] overflow-y-auto pr-1"
              role="list"
              aria-label="Configured questions list"
            >
              {items.map((q, idx) => {
                const isSelected = selectedQuestion?.id === q.id;
                const isFirst = idx === 0;
                const isLast = idx === items.length - 1;

                return (
                  <div
                    key={q.id}
                    className={`group relative flex items-start justify-between gap-2 rounded-lg border p-3 text-left transition-all ${
                      isSelected
                        ? "border-primary bg-primary/5 shadow-xs"
                        : "border-border hover:bg-muted/50"
                    }`}
                  >
                    <button
                      type="button"
                      onClick={() => setSelectedId(q.id)}
                      className="flex-1 space-y-1 text-left outline-hidden"
                      aria-current={isSelected ? "true" : undefined}
                    >
                      <div className="flex items-center gap-2">
                        <span className="text-xs font-bold text-foreground">Q{q.questionNumber}</span>
                        <Badge variant="outline" className="text-[10px] py-0 px-1 font-medium">
                          {q.type}
                        </Badge>
                        <span className="text-[10px] text-muted-foreground font-medium">
                          {q.points} {q.points === 1 ? "pt" : "pts"}
                        </span>
                        {q.type === "MCQ" && (
                          <span className="text-[10px] text-muted-foreground">
                            • {q.options?.length ?? 0} opts
                          </span>
                        )}
                      </div>
                      <p className="text-xs text-foreground/90 line-clamp-2 font-normal leading-relaxed">
                        {q.text}
                      </p>
                    </button>

                    {/* Quick Reorder Controls */}
                    {!isReadOnly && items.length > 1 && (
                      <div className="flex flex-col items-center gap-0.5 shrink-0 opacity-80 group-hover:opacity-100 transition-opacity">
                        <button
                          type="button"
                          disabled={isFirst || reordering}
                          onClick={(e) => {
                            e.stopPropagation();
                            handleMoveQuestion(q.id, "up");
                          }}
                          className="rounded p-1 hover:bg-muted disabled:opacity-30 disabled:cursor-not-allowed text-muted-foreground hover:text-foreground"
                          aria-label={`Move question ${q.questionNumber} up`}
                          title="Move up"
                        >
                          <ArrowUp className="size-3.5" />
                        </button>
                        <button
                          type="button"
                          disabled={isLast || reordering}
                          onClick={(e) => {
                            e.stopPropagation();
                            handleMoveQuestion(q.id, "down");
                          }}
                          className="rounded p-1 hover:bg-muted disabled:opacity-30 disabled:cursor-not-allowed text-muted-foreground hover:text-foreground"
                          aria-label={`Move question ${q.questionNumber} down`}
                          title="Move down"
                        >
                          <ArrowDown className="size-3.5" />
                        </button>
                      </div>
                    )}
                  </div>
                );
              })}
            </div>
          )}
        </aside>

        {/* Main Panel: Selected Question Preview or Empty State */}
        <main>
          {selectedQuestion ? (
            <Card className="border border-border">
              <CardHeader className="flex flex-row items-start justify-between border-b pb-4 gap-4">
                <div className="space-y-1.5">
                  <div className="flex flex-wrap items-center gap-2">
                    <Badge variant="secondary" className="font-bold text-xs">
                      Question {selectedQuestion.questionNumber}
                    </Badge>
                    <Badge variant="outline">{selectedQuestion.type}</Badge>
                    <span className="text-xs text-muted-foreground font-medium">
                      {selectedQuestion.points} {selectedQuestion.points === 1 ? "point" : "points"}
                    </span>
                  </div>
                  <CardTitle className="mt-2">
                    <h3 className="text-base sm:text-lg font-bold leading-snug">
                      {selectedQuestion.text}
                    </h3>
                  </CardTitle>
                </div>

                {/* Question Actions Toolbar */}
                {!isReadOnly && (
                  <div className="flex items-center gap-1 shrink-0">
                    <Button
                      variant="outline"
                      size="sm"
                      onClick={() => handleOpenEdit(selectedQuestion)}
                      className="gap-1.5 text-xs h-8"
                      aria-label={`Edit question ${selectedQuestion.questionNumber}`}
                    >
                      <Pencil className="size-3.5" />
                      Edit
                    </Button>
                    <Button
                      variant="ghost"
                      size="icon"
                      className="size-8 text-muted-foreground hover:text-destructive hover:bg-destructive/10"
                      aria-label={`Delete question ${selectedQuestion.questionNumber}`}
                      onClick={() => setDeleteConfirmQuestion(selectedQuestion)}
                    >
                      <Trash2 className="size-4" />
                    </Button>
                  </div>
                )}
              </CardHeader>

              <CardContent className="pt-6 space-y-5">
                {/* MCQ Options Display */}
                {selectedQuestion.type === "MCQ" && (
                  <div className="space-y-3">
                    <div className="flex items-center justify-between">
                      <p className="text-xs font-semibold uppercase tracking-wider text-muted-foreground">
                        Options ({selectedQuestion.options?.length ?? 0}):
                      </p>
                      <span className="text-[11px] text-muted-foreground">
                        Emerald highlight indicates configured correct answer
                      </span>
                    </div>

                    <div className="grid gap-2.5">
                      {selectedQuestion.options && selectedQuestion.options.length > 0 ? (
                        selectedQuestion.options.map((opt) => {
                          const optionText =
                            opt.optionText ||
                            (opt as unknown as Record<string, string>).text ||
                            (opt as unknown as Record<string, string>).option_text ||
                            (opt as unknown as Record<string, string>).value ||
                            "";
                          const optionKey =
                            opt.optionKey ||
                            (opt as unknown as Record<string, string>).key ||
                            (opt as unknown as Record<string, string>).option_key ||
                            "";
                          const isCorrect = Boolean(
                            opt.isCorrect ??
                            (opt as unknown as Record<string, boolean>).is_correct
                          );

                          return (
                            <div
                              key={opt.id}
                              className={`flex items-center justify-between rounded-lg border p-3 text-sm transition-colors ${
                                isCorrect
                                  ? "border-emerald-500/60 bg-emerald-500/10 text-foreground dark:border-emerald-500/70 dark:bg-emerald-950/20"
                                  : "border-border bg-card text-foreground"
                              }`}
                            >
                              <div className="flex items-center gap-3 min-w-0">
                                <span
                                  className={`font-bold text-xs rounded-md px-2.5 py-1 shrink-0 ${
                                    isCorrect
                                      ? "bg-emerald-600 text-white"
                                      : "bg-muted text-foreground"
                                  }`}
                                >
                                  {optionKey}
                                </span>
                                <span className="font-medium text-foreground break-words">
                                  {optionText}
                                </span>
                              </div>

                              {isCorrect && (
                                <Badge
                                  variant="outline"
                                  className="text-xs text-emerald-700 dark:text-emerald-300 border-emerald-400 dark:border-emerald-700 bg-emerald-50 dark:bg-emerald-900/40 gap-1 shrink-0 ml-2 font-semibold"
                                >
                                  <Check className="size-3" />
                                  Correct Answer
                                </Badge>
                              )}
                            </div>
                          );
                        })
                      ) : (
                        <p className="text-xs text-muted-foreground italic">No options configured.</p>
                      )}
                    </div>
                  </div>
                )}

                {/* Essay Display */}
                {selectedQuestion.type === "ESSAY" && (
                  <div className="rounded-lg border border-dashed p-6 text-center text-sm text-muted-foreground">
                    <Type className="mx-auto size-8 text-muted-foreground/50 mb-2" />
                    <p className="font-medium text-foreground">Written / Essay Response Mode</p>
                    <p className="text-xs text-muted-foreground mt-1">
                      Candidates respond with rich free-form text. Real-time character/word counts
                      and dictation support will be enabled during examination.
                    </p>
                  </div>
                )}

                {/* Voice Display */}
                {selectedQuestion.type === "VOICE" && (
                  <div className="rounded-lg border border-dashed p-6 text-center text-sm text-muted-foreground">
                    <Mic className="mx-auto size-8 text-muted-foreground/50 mb-2" />
                    <p className="font-medium text-foreground">Voice Dictation Mode</p>
                    <p className="text-xs text-muted-foreground mt-1">
                      Candidates articulate their answers verbally. Audio responses are streamed
                      and converted to text with accessibility transcripts.
                    </p>
                  </div>
                )}
              </CardContent>
            </Card>
          ) : (
            <Card className="flex flex-col items-center justify-center p-12 text-center border-dashed">
              <FilePlus2 className="size-12 text-muted-foreground/40 mb-3" />
              <h3 className="text-lg font-semibold">No Questions Configured</h3>
              <p className="text-sm text-muted-foreground mt-1 max-w-sm">
                {isReadOnly
                  ? "This assessment does not contain any questions."
                  : "Add questions and configure MCQ options, prompt text, and scoring weights."}
              </p>
              {!isReadOnly && (
                <Button onClick={handleOpenAdd} className="mt-4 gap-2 font-semibold">
                  <Plus className="size-4" />
                  Add Your First Question
                </Button>
              )}
            </Card>
          )}
        </main>
      </div>

      {/* Question Editor Dialog (Add & Edit) */}
      {editorOpen && (
        <QuestionEditorDialog
          key={editingQuestion ? editingQuestion.id : "new-question"}
          open={editorOpen}
        onOpenChange={setEditorOpen}
        examId={examId}
        questionToEdit={editingQuestion}
        nextQuestionNumber={items.length + 1}
        onSaveSuccess={(savedQ, isEdit) => {
          if (isEdit) {
            const updated = items.map((q) => (q.id === savedQ.id ? savedQ : q));
            updateItemsAndNotify(updated);
            setFeedback({
              type: "success",
              message: `Question ${savedQ.questionNumber} updated successfully.`,
            });
          } else {
            const updated = [...items, savedQ];
            updateItemsAndNotify(updated);
            setSelectedId(savedQ.id);
            setFeedback({
              type: "success",
              message: `Question ${savedQ.questionNumber} added successfully.`,
            });
          }
        }}
      />
      )}

      {/* Delete Question Confirmation Dialog */}
      {deleteConfirmQuestion && (
        <Dialog
          open={Boolean(deleteConfirmQuestion)}
          onOpenChange={(open) => !open && setDeleteConfirmQuestion(null)}
        >
          <DialogContent className="sm:max-w-md">
            <DialogHeader>
              <DialogTitle className="flex items-center gap-2 text-destructive">
                <Trash2 className="size-5" />
                Delete Question {deleteConfirmQuestion.questionNumber}?
              </DialogTitle>
              <DialogDescription>
                Are you sure you want to delete this question? Associated multiple-choice options will
                be removed, and remaining questions will automatically be re-sequenced. This action
                cannot be undone.
              </DialogDescription>
            </DialogHeader>

            <div className="rounded-lg bg-muted p-3 text-xs text-foreground/90 font-medium line-clamp-2">
              &quot;{deleteConfirmQuestion.text}&quot;
            </div>

            <DialogFooter className="gap-2 sm:gap-0">
              <Button
                variant="outline"
                type="button"
                onClick={() => setDeleteConfirmQuestion(null)}
                disabled={deleteLoading}
              >
                Cancel
              </Button>
              <Button
                variant="destructive"
                type="button"
                onClick={confirmDeleteQuestion}
                disabled={deleteLoading}
              >
                {deleteLoading && <Loader2 className="mr-2 size-4 animate-spin" />}
                {deleteLoading ? "Deleting..." : "Delete Question"}
              </Button>
            </DialogFooter>
          </DialogContent>
        </Dialog>
      )}
    </div>
  );
}

// -----------------------------------------------------------------------------
// Question Editor Dialog Component (Add & Edit Modes)
// -----------------------------------------------------------------------------
interface QuestionEditorDialogProps {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  examId: string;
  questionToEdit: Question | null;
  nextQuestionNumber: number;
  onSaveSuccess: (savedQuestion: Question, isEdit: boolean) => void;
}

function QuestionEditorDialog({
  open,
  onOpenChange,
  examId,
  questionToEdit,
  nextQuestionNumber,
  onSaveSuccess,
}: QuestionEditorDialogProps) {
  const isEdit = Boolean(questionToEdit);

  const [type, setType] = useState<QuestionType>(questionToEdit?.type || "MCQ");
  const [text, setText] = useState(questionToEdit?.text || "");
  const [points, setPoints] = useState(questionToEdit ? String(questionToEdit.points) : "2");
  const [saving, setSaving] = useState(false);
  const [formErrors, setFormErrors] = useState<Record<string, string>>({});

  const [options, setOptions] = useState<QuestionOption[]>(() => {
    if (questionToEdit?.type === "MCQ" && questionToEdit.options && questionToEdit.options.length > 0) {
      return questionToEdit.options.map((opt, i) => ({
        id: opt.id || crypto.randomUUID(),
        optionKey:
          opt.optionKey ||
          (opt as unknown as Record<string, string>).key ||
          (opt as unknown as Record<string, string>).option_key ||
          String.fromCharCode(65 + i),
        optionText:
          opt.optionText ||
          (opt as unknown as Record<string, string>).text ||
          (opt as unknown as Record<string, string>).option_text ||
          (opt as unknown as Record<string, string>).value ||
          "",
        displayOrder:
          opt.displayOrder ||
          (opt as unknown as Record<string, number>).order ||
          (opt as unknown as Record<string, number>).display_order ||
          i + 1,
        isCorrect: Boolean(
          opt.isCorrect ??
          (opt as unknown as Record<string, boolean>).is_correct
        ),
      }));
    }
    return [
      {
        id: crypto.randomUUID(),
        optionKey: "A",
        optionText: "",
        displayOrder: 1,
        isCorrect: true,
      },
      {
        id: crypto.randomUUID(),
        optionKey: "B",
        optionText: "",
        displayOrder: 2,
        isCorrect: false,
      },
    ];
  });

  const validate = () => {
    setFormErrors({});
    const numPoints = Math.max(1, parseInt(points, 10) || 1);

    const validation = CreateQuestionSchema.safeParse({
      text: text.trim(),
      type,
      points: numPoints,
      options: type === "MCQ" ? options : undefined,
    });

    if (!validation.success) {
      const errs: Record<string, string> = {};
      validation.error.errors.forEach((err) => {
        const key = err.path.join(".");
        errs[key || "general"] = err.message;
      });
      setFormErrors(errs);
      return false;
    }

    if (type === "MCQ") {
      const emptyOption = options.some((o) => !o.optionText.trim());
      if (emptyOption) {
        setFormErrors((prev) => ({
          ...prev,
          options: "All option text fields must be filled out.",
        }));
        return false;
      }

      const hasCorrect = options.some((o) => o.isCorrect);
      if (!hasCorrect) {
        setFormErrors((prev) => ({
          ...prev,
          options: "Please designate one option as the correct answer.",
        }));
        return false;
      }
    }

    return true;
  };

  const handleSave = async () => {
    if (!validate()) return;

    setSaving(true);
    const numPoints = Math.max(1, parseInt(points, 10) || 1);

    try {
      if (isEdit && questionToEdit) {
        // 1. Update question properties
        const updatedQ = await questionsApi.updateQuestion(questionToEdit.id, {
          text: text.trim(),
          points: numPoints,
        });

        // 2. Synchronize MCQ options if MCQ
        let finalOptions = questionToEdit.options || [];
        if (type === "MCQ") {
          // Identify additions, updates, removals
          const existingIds = new Set(questionToEdit.options?.map((o) => o.id) || []);
          const currentIds = new Set(options.map((o) => o.id));

          // Removed options
          for (const oldOpt of questionToEdit.options || []) {
            if (!currentIds.has(oldOpt.id)) {
              try {
                await questionsApi.deleteOption(questionToEdit.id, oldOpt.id);
              } catch {
                // Ignore delete errors
              }
            }
          }

          // Added and updated options
          const syncedOptions: QuestionOption[] = [];
          for (const opt of options) {
            if (existingIds.has(opt.id)) {
              // Existing option: update
              try {
                const updatedOpt = await questionsApi.updateOption(questionToEdit.id, opt.id, {
                  optionKey: opt.optionKey,
                  optionText: opt.optionText.trim(),
                  displayOrder: opt.displayOrder,
                  isCorrect: opt.isCorrect,
                });
                syncedOptions.push(updatedOpt);
              } catch {
                syncedOptions.push(opt);
              }
            } else {
              // New option: create
              try {
                const createdOpt = await questionsApi.createOption(questionToEdit.id, {
                  optionKey: opt.optionKey,
                  optionText: opt.optionText.trim(),
                  displayOrder: opt.displayOrder,
                  isCorrect: opt.isCorrect,
                });
                syncedOptions.push(createdOpt);
              } catch {
                syncedOptions.push(opt);
              }
            }
          }

          // If correct option is set, explicitly ensure backend setCorrectOption is synced
          const correctOpt = syncedOptions.find((o) => o.isCorrect);
          if (correctOpt && correctOpt.id) {
            try {
              await questionsApi.setCorrectOption(questionToEdit.id, correctOpt.id);
            } catch {
              // ignore
            }
          }

          finalOptions = syncedOptions;
        }

        const fullUpdated: Question = {
          ...updatedQ,
          options: finalOptions,
        };

        onSaveSuccess(fullUpdated, true);
        onOpenChange(false);
      } else {
        // Create question atomically with options
        const created = await questionsApi.createQuestion(examId, {
          questionNumber: nextQuestionNumber,
          text: text.trim(),
          type,
          points: numPoints,
          options:
            type === "MCQ"
              ? options.map((opt, i) => ({
                  optionKey: opt.optionKey,
                  optionText: opt.optionText.trim(),
                  displayOrder: opt.displayOrder || i + 1,
                  isCorrect: opt.isCorrect,
                }))
              : undefined,
        });

        onSaveSuccess(created, false);
        onOpenChange(false);
      }
    } catch (err) {
      const msg =
        err instanceof ApiError
          ? err.message
          : `Failed to ${isEdit ? "update" : "create"} question. Please try again.`;
      setFormErrors({ general: msg });
    } finally {
      setSaving(false);
    }
  };

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="max-h-[90vh] overflow-y-auto sm:max-w-lg">
        <DialogHeader>
          <DialogTitle>
            {isEdit
              ? `Edit Question ${questionToEdit?.questionNumber ?? ""}`
              : "Add a New Question"}
          </DialogTitle>
          <DialogDescription>
            Configure question prompt, response mode, and scoring criteria.
          </DialogDescription>
        </DialogHeader>

        <div className="flex flex-col gap-4 py-2">
          {formErrors.general && (
            <div
              role="alert"
              className="flex items-center gap-2 rounded-lg bg-destructive/10 p-2.5 text-xs text-destructive font-medium"
            >
              <AlertCircle className="size-4 shrink-0" />
              <span>{formErrors.general}</span>
            </div>
          )}

          <div className="grid gap-3 sm:grid-cols-2">
            <div className="space-y-1.5">
              <Label className="text-sm font-medium">Question Type</Label>
              <Select
                value={type}
                onValueChange={(val) => setType(val as QuestionType)}
                disabled={isEdit} // Prevent changing question type in edit to preserve DB integrity
              >
                <SelectTrigger aria-label="Select question type">
                  <SelectValue />
                </SelectTrigger>
                <SelectContent>
                  <SelectItem value="MCQ">Multiple Choice (MCQ)</SelectItem>
                  <SelectItem value="ESSAY">Essay / Written</SelectItem>
                  <SelectItem value="VOICE">Voice Dictation Only</SelectItem>
                </SelectContent>
              </Select>
            </div>

            <div className="space-y-1.5">
              <Label htmlFor="question-points" className="text-sm font-medium">
                Points
              </Label>
              <Input
                id="question-points"
                type="number"
                min="1"
                max="100"
                value={points}
                onChange={(e) => setPoints(e.target.value)}
                aria-invalid={!!formErrors.points}
              />
              {formErrors.points && <p className="text-xs text-destructive">{formErrors.points}</p>}
            </div>
          </div>

          <div className="space-y-1.5">
            <Label htmlFor="question-prompt" className="text-sm font-medium">
              Question Prompt <span className="text-destructive">*</span>
            </Label>
            <Textarea
              id="question-prompt"
              rows={3}
              placeholder="e.g. Which cellular structure is responsible for ATP synthesis?"
              value={text}
              onChange={(e) => setText(e.target.value)}
              aria-invalid={!!formErrors.text}
            />
            {formErrors.text && <p className="text-xs text-destructive">{formErrors.text}</p>}
          </div>

          {/* MCQ Options Builder */}
          {type === "MCQ" && (
            <div className="space-y-3 border-t pt-3">
              <div className="flex items-center justify-between">
                <div>
                  <Label className="text-sm font-medium">Answer Options</Label>
                  <p className="text-[11px] text-muted-foreground">
                    Click key button (e.g. A, B) to designate correct answer.
                  </p>
                </div>

                {options.length < 6 && (
                  <Button
                    type="button"
                    variant="outline"
                    size="sm"
                    className="text-xs h-7"
                    onClick={() => {
                      const keys = ["A", "B", "C", "D", "E", "F"];
                      const nextKey = keys[options.length] || `Opt ${options.length + 1}`;
                      setOptions([
                        ...options,
                        {
                          id: crypto.randomUUID(),
                          optionKey: nextKey,
                          optionText: "",
                          displayOrder: options.length + 1,
                          isCorrect: false,
                        },
                      ]);
                    }}
                  >
                    <Plus className="mr-1 size-3" />
                    Add Option
                  </Button>
                )}
              </div>

              {formErrors.options && (
                <p className="text-xs text-destructive font-medium">{formErrors.options}</p>
              )}

              <div className="space-y-2.5">
                {options.map((opt) => (
                  <div key={opt.id} className="flex items-center gap-2">
                    <Button
                      type="button"
                      variant={opt.isCorrect ? "default" : "outline"}
                      size="sm"
                      className={`size-8 p-0 font-bold shrink-0 ${
                        opt.isCorrect
                          ? "bg-emerald-600 hover:bg-emerald-700 text-white"
                          : "text-muted-foreground hover:text-foreground"
                      }`}
                      title={opt.isCorrect ? "Marked as correct answer" : "Click to mark as correct answer"}
                      aria-label={`Option ${opt.optionKey} ${
                        opt.isCorrect ? "(Correct answer)" : "(Click to set correct)"
                      }`}
                      onClick={() => {
                        setOptions(
                          options.map((o) => ({
                            ...o,
                            isCorrect: o.id === opt.id,
                          }))
                        );
                      }}
                    >
                      {opt.optionKey}
                    </Button>

                    <Input
                      placeholder={`Option ${opt.optionKey} text...`}
                      value={opt.optionText || ""}
                      onChange={(e) => {
                        const val = e.target.value;
                        setOptions(
                          options.map((o) =>
                            o.id === opt.id ? { ...o, optionText: val } : o
                          )
                        );
                      }}
                      aria-label={`Option ${opt.optionKey} text`}
                    />

                    {options.length > 2 && (
                      <Button
                        type="button"
                        variant="ghost"
                        size="icon"
                        className="size-8 text-muted-foreground hover:text-destructive shrink-0"
                        aria-label={`Remove option ${opt.optionKey}`}
                        onClick={() => {
                          const remaining = options.filter((o) => o.id !== opt.id);
                          // If removed option was the correct one, make the first option correct
                          if (opt.isCorrect && remaining.length > 0) {
                            remaining[0].isCorrect = true;
                          }
                          setOptions(remaining);
                        }}
                      >
                        <Trash2 className="size-4" />
                      </Button>
                    )}
                  </div>
                ))}
              </div>
            </div>
          )}
        </div>

        <DialogFooter className="gap-2 sm:gap-0">
          <Button
            variant="outline"
            type="button"
            onClick={() => onOpenChange(false)}
            disabled={saving}
          >
            Cancel
          </Button>
          <Button type="button" onClick={handleSave} disabled={saving}>
            {saving && <Loader2 className="mr-2 size-4 animate-spin" />}
            {saving ? (isEdit ? "Updating..." : "Saving...") : isEdit ? "Update Question" : "Save Question"}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}

export default QuestionManager;
