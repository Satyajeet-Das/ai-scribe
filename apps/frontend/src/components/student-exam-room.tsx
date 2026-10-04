"use client";

import { useEffect, useMemo, useState } from "react";
import {
  Check,
  ChevronLeft,
  ChevronRight,
  Clock3,
  Eye,
  Highlighter,
  Menu,
  Save,
  Send,
  X,
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
  AlertDialogTrigger,
} from "@/components/ui/alert-dialog";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { RadioGroup, RadioGroupItem } from "@/components/ui/radio-group";
import { Switch } from "@/components/ui/switch";
import { Textarea } from "@/components/ui/textarea";
import type { Question, QuestionOption } from "@/types/exam-types";
const sampleQuestions: Question[] = [
  {
    id: "q1",
    examId: "exam",
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
    id: "q2",
    examId: "exam",
    questionNumber: 2,
    text: "Explain how plants use sunlight to make food.",
    type: "ESSAY",
    points: 5,
    options: [],
  },
  {
    id: "q3",
    examId: "exam",
    questionNumber: 3,
    text: "Describe one adaptation that helps an animal survive.",
    type: "VOICE",
    points: 3,
    options: [],
  },
];
export function StudentExamRoom({
  questions = sampleQuestions,
  initialSeconds = 42 * 60,
  onSubmit,
}: {
  questions?: Question[];
  initialSeconds?: number;
  onSubmit?: (answers: Record<string, string>) => void;
}) {
  const [index, setIndex] = useState(0);
  const [seconds, setSeconds] = useState(initialSeconds);
  const [largeText, setLargeText] = useState(false);
  const [contrast, setContrast] = useState(false);
  const [answers, setAnswers] = useState<Record<string, string>>({});
  const [saving, setSaving] = useState(false);
  const question = questions[index];
  useEffect(() => {
    const timer = window.setInterval(() => setSeconds((value) => Math.max(0, value - 1)), 1000);
    return () => window.clearInterval(timer);
  }, []);
  useEffect(() => {
    if (saving) {
      const timer = window.setTimeout(() => setSaving(false), 700);
      return () => window.clearTimeout(timer);
    }
  }, [saving]);
  const time = useMemo(
    () =>
      `${String(Math.floor(seconds / 60)).padStart(2, "0")}:${String(seconds % 60).padStart(2, "0")}`,
    [seconds]
  );
  const setAnswer = (value: string) => {
    setSaving(true);
    setAnswers((current) => ({ ...current, [question.id]: value }));
  };
  return (
    <div
      className={`${largeText ? "text-lg" : ""} ${contrast ? "bg-foreground text-background" : "bg-background text-foreground"} min-h-screen`}
    >
      <header className="sticky top-0 z-10 border-b bg-background/95 backdrop-blur">
        <div className="mx-auto flex max-w-5xl flex-wrap items-center justify-between gap-4 px-4 py-4">
          <div>
            <p className="text-sm text-muted-foreground">Biology · Midterm assessment</p>
            <h1 className="text-xl font-semibold">Foundations of Biology</h1>
          </div>
          <div className="flex items-center gap-3">
            <Badge variant={seconds < 300 ? "destructive" : "secondary"}>
              <Clock3 data-icon="inline-start" />
              {time} remaining
            </Badge>
            <AlertDialog>
              <AlertDialogTrigger render={<Button />}>
                <Send data-icon="inline-start" />
                Submit exam
              </AlertDialogTrigger>
              <AlertDialogContent>
                <AlertDialogHeader>
                  <AlertDialogTitle>Submit your exam?</AlertDialogTitle>
                  <AlertDialogDescription>
                    You have answered {Object.keys(answers).length} of {questions.length} questions.
                    You cannot edit answers after submitting.
                  </AlertDialogDescription>
                </AlertDialogHeader>
                <AlertDialogFooter>
                  <AlertDialogCancel>Keep working</AlertDialogCancel>
                  <AlertDialogAction onClick={() => onSubmit?.(answers)}>
                    Submit answers
                  </AlertDialogAction>
                </AlertDialogFooter>
              </AlertDialogContent>
            </AlertDialog>
          </div>
        </div>
      </header>
      <main className="mx-auto grid max-w-5xl gap-6 px-4 py-6 md:grid-cols-[220px_1fr]">
        <aside className="flex flex-col gap-5">
          <Card className="bg-background">
            <CardHeader>
              <CardTitle className="text-base">Accessibility</CardTitle>
            </CardHeader>
            <CardContent className="flex flex-col gap-4">
              <ToggleRow
                label="Larger text"
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
          <nav
            aria-label="Question navigator"
            className="flex flex-wrap gap-2 md:grid md:grid-cols-4"
          >
            {questions.map((item, itemIndex) => (
              <button
                key={item.id}
                aria-label={`Question ${item.questionNumber}${answers[item.id] ? ", answered" : ""}`}
                aria-current={index === itemIndex ? "step" : undefined}
                onClick={() => setIndex(itemIndex)}
                className={`grid size-10 place-items-center rounded-full border text-sm font-semibold focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring ${index === itemIndex ? "border-primary bg-primary text-primary-foreground" : answers[item.id] ? "border-primary/50 bg-primary/10" : "bg-background"}`}
              >
                {itemIndex + 1}
              </button>
            ))}
          </nav>
        </aside>
        <Card className="min-h-[480px] bg-background">
          <CardHeader className="flex flex-row items-center justify-between gap-4">
            <div>
              <p className="text-sm text-muted-foreground">
                Question {question.questionNumber} of {questions.length} · {question.points} points
              </p>
              <CardTitle className="mt-3 max-w-3xl text-2xl leading-relaxed md:text-3xl">
                {question.text}
              </CardTitle>
            </div>
            <span
              className="flex items-center gap-2 text-sm text-muted-foreground"
              role="status"
              aria-live="polite"
            >
              {saving ? (
                <>
                  <Save data-icon="inline-start" />
                  Saving
                </>
              ) : (
                <>
                  <Check data-icon="inline-start" />
                  Saved
                </>
              )}
            </span>
          </CardHeader>
          <CardContent className="flex flex-col gap-8">
            <div className="min-h-48">
              {question.type === "MCQ" ? (
                <RadioGroup
                  value={answers[question.id] ?? ""}
                  onValueChange={setAnswer}
                  aria-label="Answer choices"
                  className="flex flex-col gap-3"
                >
                  {question.options.map((option: QuestionOption) => (
                    <label
                      key={option.id}
                      htmlFor={`answer-${option.id}`}
                      className="flex cursor-pointer items-center gap-4 rounded-xl border p-4 transition-colors hover:bg-muted"
                    >
                      <RadioGroupItem value={option.id} id={`answer-${option.id}`} />
                      <span className="grid size-8 place-items-center rounded-full bg-muted font-semibold">
                        {option.optionKey}
                      </span>
                      <span>{option.optionText}</span>
                    </label>
                  ))}
                </RadioGroup>
              ) : (
                <div className="flex flex-col gap-3">
                  <label htmlFor="written-answer" className="font-medium">
                    {question.type === "VOICE" ? "Written or voice response" : "Your response"}
                  </label>
                  <Textarea
                    id="written-answer"
                    value={answers[question.id] ?? ""}
                    onChange={(event) => setAnswer(event.target.value)}
                    placeholder="Type your response here. Voice input can be connected through the platform API."
                    className="min-h-44 text-base leading-7"
                  />
                  <p className="text-sm text-muted-foreground">
                    Your response is saved automatically as you type.
                  </p>
                </div>
              )}
            </div>
            <div className="flex items-center justify-between border-t pt-5">
              <Button
                variant="outline"
                onClick={() => setIndex(Math.max(0, index - 1))}
                disabled={index === 0}
              >
                <ChevronLeft data-icon="inline-start" />
                Previous
              </Button>
              <Button
                onClick={() => setIndex(Math.min(questions.length - 1, index + 1))}
                disabled={index === questions.length - 1}
              >
                Next
                <ChevronRight data-icon="inline-end" />
              </Button>
            </div>
          </CardContent>
        </Card>
      </main>
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
    <label className="flex items-center justify-between gap-3 text-sm">
      <span className="flex items-center gap-2">
        <Icon aria-hidden="true" />
        {label}
      </span>
      <Switch checked={checked} onCheckedChange={onCheckedChange} aria-label={label} />
    </label>
  );
}
export default StudentExamRoom;
