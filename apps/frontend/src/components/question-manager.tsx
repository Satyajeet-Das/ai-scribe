"use client";

import { useState } from "react";
import {
  Check,
  ChevronDown,
  FilePlus2,
  Mic,
  Plus,
  Trash2,
  Type,
  Loader2,
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
import { RadioGroup, RadioGroupItem } from "@/components/ui/radio-group";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import { Textarea } from "@/components/ui/textarea";
import { questionsApi } from "@/services/api";
import { CreateQuestionSchema } from "@/lib/validations";
import type { Question, QuestionType, QuestionOption } from "@/types/exam-types";

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

export function QuestionManager({
  examId = "exam-1",
  questions = fallbackQuestions,
}: {
  examId?: string;
  questions?: Question[];
}) {
  const [items, setItems] = useState<Question[]>(
    questions.length > 0 ? questions : fallbackQuestions
  );
  const [open, setOpen] = useState(false);
  const [selected, setSelected] = useState<string>(items[0]?.id ?? "");
  const [type, setType] = useState<QuestionType>("MCQ");
  const [text, setText] = useState("");
  const [points, setPoints] = useState("2");
  const [saving, setSaving] = useState(false);
  const [formErrors, setFormErrors] = useState<Record<string, string>>({});

  const [options, setOptions] = useState<QuestionOption[]>([
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
  ]);

  const validate = () => {
    setFormErrors({});
    const numPoints = Math.max(1, parseInt(points, 10) || 1);

    const validation = CreateQuestionSchema.safeParse({
      text,
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

    return true;
  };

  const handleAddQuestion = async () => {
    if (!validate()) return;

    setSaving(true);
    const numPoints = Math.max(1, parseInt(points, 10) || 1);

    try {
      // Attempt backend API call
      const created = await questionsApi.createQuestion(examId, {
        questionNumber: items.length + 1,
        text: text.trim(),
        type,
        points: numPoints,
      });

      let savedOptions: QuestionOption[] = [];
      if (type === "MCQ" && created.id) {
        for (const opt of options) {
          try {
            const savedOpt = await questionsApi.createOption(created.id, {
              optionKey: opt.optionKey,
              optionText: opt.optionText.trim(),
              displayOrder: opt.displayOrder,
              isCorrect: opt.isCorrect,
            });
            savedOptions.push(savedOpt);
          } catch {
            savedOptions.push(opt);
          }
        }
      }

      const fullQuestion: Question = {
        ...created,
        options: savedOptions.length > 0 ? savedOptions : options,
      };

      setItems((curr) => [...curr, fullQuestion]);
      setSelected(fullQuestion.id);
    } catch {
      // Local optimistic fallback
      const localQuestion: Question = {
        id: crypto.randomUUID(),
        examId,
        questionNumber: items.length + 1,
        text: text.trim(),
        type,
        points: numPoints,
        options: type === "MCQ" ? options : [],
      };
      setItems((curr) => [...curr, localQuestion]);
      setSelected(localQuestion.id);
    } finally {
      setSaving(false);
      setText("");
      setFormErrors({});
      setOpen(false);
    }
  };

  const removeQuestion = async (id: string) => {
    try {
      await questionsApi.deleteQuestion(id);
    } catch {
      // ignore
    }
    setItems((curr) => {
      const updated = curr.filter((item) => item.id !== id);
      return updated.map((item, index) => ({ ...item, questionNumber: index + 1 }));
    });
    if (selected === id) {
      setSelected(items.find((item) => item.id !== id)?.id || "");
    }
  };

  const selectedQuestion = items.find((q) => q.id === selected) || items[0];

  return (
    <div className="grid gap-6 md:grid-cols-[300px_1fr]">
      {/* Sidebar List */}
      <aside className="flex flex-col gap-4">
        <div className="flex items-center justify-between">
          <div>
            <h2 className="text-xl font-bold tracking-tight">Question Studio</h2>
            <p className="text-xs text-muted-foreground">{items.length} questions configured</p>
          </div>

          <Dialog open={open} onOpenChange={setOpen}>
            <DialogTrigger
              render={
                <Button size="sm" className="font-semibold">
                  <Plus className="mr-1 size-4" />
                  Add
                </Button>
              }
            />
            <DialogContent className="max-h-[90vh] overflow-y-auto sm:max-w-lg">
              <DialogHeader>
                <DialogTitle>Add a New Question</DialogTitle>
                <DialogDescription>
                  Configure question prompt, response mode, and scoring criteria.
                </DialogDescription>
              </DialogHeader>

              <div className="flex flex-col gap-4 py-2">
                {formErrors.general && (
                  <div className="flex items-center gap-2 rounded-lg bg-destructive/10 p-2.5 text-xs text-destructive">
                    <AlertCircle className="size-4 shrink-0" />
                    <span>{formErrors.general}</span>
                  </div>
                )}

                <div className="grid gap-3 sm:grid-cols-2">
                  <div className="space-y-1.5">
                    <Label className="text-sm font-medium">Question Type</Label>
                    <Select value={type} onValueChange={(val) => setType(val as QuestionType)}>
                      <SelectTrigger>
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
                    <Label htmlFor="points" className="text-sm font-medium">
                      Points
                    </Label>
                    <Input
                      id="points"
                      type="number"
                      min="1"
                      max="100"
                      value={points}
                      onChange={(e) => setPoints(e.target.value)}
                    />
                  </div>
                </div>

                <div className="space-y-1.5">
                  <Label htmlFor="question-text" className="text-sm font-medium">
                    Question Prompt <span className="text-destructive">*</span>
                  </Label>
                  <Textarea
                    id="question-text"
                    rows={3}
                    placeholder="e.g. Which cellular structure is responsible for adenosine triphosphate (ATP) synthesis?"
                    value={text}
                    onChange={(e) => setText(e.target.value)}
                    aria-invalid={!!formErrors.text}
                  />
                  {formErrors.text && <p className="text-xs text-destructive">{formErrors.text}</p>}
                </div>

                {type === "MCQ" && (
                  <div className="space-y-3 border-t pt-3">
                    <div className="flex items-center justify-between">
                      <Label className="text-sm font-medium">Answer Options</Label>
                      {options.length < 4 && (
                        <Button
                          type="button"
                          variant="outline"
                          size="sm"
                          className="text-xs h-7"
                          onClick={() => {
                            const nextKey =
                              ["A", "B", "C", "D"][options.length] ||
                              `Option ${options.length + 1}`;
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
                      <p className="text-xs text-destructive">{formErrors.options}</p>
                    )}

                    <div className="space-y-2">
                      {options.map((opt, idx) => (
                        <div key={opt.id} className="flex items-center gap-2">
                          <Button
                            type="button"
                            variant={opt.isCorrect ? "default" : "outline"}
                            size="sm"
                            className="size-8 p-0 font-bold shrink-0"
                            title={opt.isCorrect ? "Marked as correct" : "Click to mark as correct"}
                            onClick={() =>
                              setOptions(options.map((o) => ({ ...o, isCorrect: o.id === opt.id })))
                            }
                          >
                            {opt.optionKey}
                          </Button>
                          <Input
                            placeholder={`Option ${opt.optionKey} text...`}
                            value={opt.optionText}
                            onChange={(e) => {
                              const val = e.target.value;
                              setOptions(
                                options.map((o) =>
                                  o.id === opt.id ? { ...o, optionText: val } : o
                                )
                              );
                            }}
                          />
                          {options.length > 2 && (
                            <Button
                              type="button"
                              variant="ghost"
                              size="icon"
                              className="size-8 text-muted-foreground hover:text-destructive shrink-0"
                              onClick={() => setOptions(options.filter((o) => o.id !== opt.id))}
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
                <Button variant="outline" type="button" onClick={() => setOpen(false)}>
                  Cancel
                </Button>
                <Button type="button" onClick={handleAddQuestion} disabled={saving}>
                  {saving && <Loader2 className="mr-2 size-4 animate-spin" />}
                  {saving ? "Saving..." : "Save Question"}
                </Button>
              </DialogFooter>
            </DialogContent>
          </Dialog>
        </div>

        {/* Question Cards List */}
        <div className="flex flex-col gap-2 max-h-[600px] overflow-y-auto pr-1">
          {items.map((q) => (
            <button
              key={q.id}
              onClick={() => setSelected(q.id)}
              className={`flex items-start justify-between gap-2 rounded-lg border p-3 text-left transition-all ${
                selected === q.id
                  ? "border-primary bg-primary/5 shadow-sm"
                  : "border-border hover:bg-muted/50"
              }`}
            >
              <div className="space-y-1 overflow-hidden">
                <div className="flex items-center gap-2">
                  <span className="text-xs font-bold text-muted-foreground">
                    Q{q.questionNumber}
                  </span>
                  <Badge variant="outline" className="text-[10px] py-0 px-1 font-medium">
                    {q.type}
                  </Badge>
                  <span className="text-[10px] text-muted-foreground">{q.points} pts</span>
                </div>
                <p className="text-xs text-foreground truncate font-medium">{q.text}</p>
              </div>
            </button>
          ))}
        </div>
      </aside>

      {/* Main Question Preview Area */}
      <main>
        {selectedQuestion ? (
          <Card className="border border-border">
            <CardHeader className="flex flex-row items-start justify-between border-b pb-4">
              <div className="space-y-1">
                <div className="flex items-center gap-2">
                  <Badge variant="secondary">Question {selectedQuestion.questionNumber}</Badge>
                  <Badge variant="outline">{selectedQuestion.type}</Badge>
                  <span className="text-xs text-muted-foreground font-medium">
                    {selectedQuestion.points} points
                  </span>
                </div>
                <CardTitle className="text-lg font-bold mt-2">{selectedQuestion.text}</CardTitle>
              </div>

              <Button
                variant="ghost"
                size="icon"
                className="text-muted-foreground hover:text-destructive"
                aria-label={`Delete question ${selectedQuestion.questionNumber}`}
                onClick={() => removeQuestion(selectedQuestion.id)}
              >
                <Trash2 className="size-4" />
              </Button>
            </CardHeader>

            <CardContent className="pt-6 space-y-4">
              {selectedQuestion.type === "MCQ" && (
                <div className="space-y-2">
                  <p className="text-xs font-semibold uppercase tracking-wider text-muted-foreground mb-3">
                    Configured Multiple Choice Options:
                  </p>
                  <div className="grid gap-2">
                    {selectedQuestion.options.map((opt) => (
                      <div
                        key={opt.id}
                        className={`flex items-center justify-between rounded-lg border p-3 text-sm ${
                          opt.isCorrect
                            ? "border-emerald-500/40 bg-emerald-500/10 text-emerald-950 dark:text-emerald-200"
                            : "border-border bg-card"
                        }`}
                      >
                        <div className="flex items-center gap-3">
                          <span className="font-bold text-xs rounded-md bg-muted px-2 py-1">
                            {opt.optionKey}
                          </span>
                          <span>{opt.optionText}</span>
                        </div>
                        {opt.isCorrect && (
                          <Badge
                            variant="outline"
                            className="text-xs text-emerald-600 border-emerald-300"
                          >
                            Correct
                          </Badge>
                        )}
                      </div>
                    ))}
                  </div>
                </div>
              )}

              {selectedQuestion.type === "ESSAY" && (
                <div className="rounded-lg border border-dashed p-6 text-center text-sm text-muted-foreground">
                  <Type className="mx-auto size-8 text-muted-foreground/50 mb-2" />
                  Candidate will respond with a free-form written essay. Speech-to-text dictation
                  and word count indicators will be provided in the test room.
                </div>
              )}

              {selectedQuestion.type === "VOICE" && (
                <div className="rounded-lg border border-dashed p-6 text-center text-sm text-muted-foreground">
                  <Mic className="mx-auto size-8 text-muted-foreground/50 mb-2" />
                  Voice-only accommodation mode. The candidate speaks their response, and it will be
                  transcribed in real time.
                </div>
              )}
            </CardContent>
          </Card>
        ) : (
          <Card className="flex flex-col items-center justify-center p-12 text-center border-dashed">
            <FilePlus2 className="size-12 text-muted-foreground/50 mb-3" />
            <h3 className="text-lg font-medium">No Questions Added Yet</h3>
            <p className="text-sm text-muted-foreground mt-1 max-w-sm">
              Click &quot;Add Question&quot; to define questions and scoring for this exam.
            </p>
          </Card>
        )}
      </main>
    </div>
  );
}
export default QuestionManager;
