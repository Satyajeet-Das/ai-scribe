"use client";

import { useState } from "react";
import { Check, ChevronDown, FilePlus2, Mic, Plus, Trash2, Type } from "lucide-react";
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

export type QuestionType = "MCQ" | "ESSAY" | "VOICE";
export type QuestionOption = {
  id: string;
  optionKey: string;
  optionText: string;
  displayOrder: number;
  isCorrect: boolean;
};
export type Question = {
  id: string;
  examId: string;
  questionNumber: number;
  text: string;
  type: QuestionType;
  points: number;
  options: QuestionOption[];
};
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
  const [items, setItems] = useState(questions);
  const [open, setOpen] = useState(false);
  const [selected, setSelected] = useState<Question["id"]>(questions[0]?.id ?? "");
  const [type, setType] = useState<QuestionType>("MCQ");
  const [text, setText] = useState("");
  const [points, setPoints] = useState("2");
  const [options, setOptions] = useState<QuestionOption[]>(
    ["A", "B"].map((key, index) => ({
      id: crypto.randomUUID(),
      optionKey: key,
      optionText: "",
      displayOrder: index + 1,
      isCorrect: index === 0,
    }))
  );
  const addQuestion = () => {
    if (!text.trim()) return;
    const question: Question = {
      id: crypto.randomUUID(),
      examId,
      questionNumber: items.length + 1,
      text: text.trim(),
      type,
      points: Math.max(1, Number(points) || 1),
      options: type === "MCQ" ? options : [],
    };
    setItems((current) => [...current, question]);
    setSelected(question.id);
    setText("");
    setOpen(false);
  };
  const removeQuestion = (id: string) =>
    setItems((current) =>
      current
        .filter((item) => item.id !== id)
        .map((item, index) => ({ ...item, questionNumber: index + 1 }))
    );
  return (
    <section className="mx-auto grid max-w-6xl gap-6 p-6 md:grid-cols-[280px_1fr] md:p-10">
      <aside className="flex flex-col gap-4">
        <div>
          <p className="text-sm text-muted-foreground">Exam authoring</p>
          <h1 className="text-2xl font-semibold">Question studio</h1>
        </div>
        <Dialog open={open} onOpenChange={setOpen}>
          <DialogTrigger render={<Button />}>
            <Plus data-icon="inline-start" />
            Add question
          </DialogTrigger>
          <DialogContent className="max-h-[90vh] overflow-y-auto">
            <DialogHeader>
              <DialogTitle>Add a question</DialogTitle>
              <DialogDescription>
                Use clear language and keep accessible response paths in mind.
              </DialogDescription>
            </DialogHeader>
            <div className="flex flex-col gap-5">
              <div className="grid gap-4 sm:grid-cols-2">
                <div className="flex flex-col gap-2">
                  <Label>Question type</Label>
                  <Select value={type} onValueChange={(value) => setType(value as QuestionType)}>
                    <SelectTrigger>
                      <SelectValue />
                    </SelectTrigger>
                    <SelectContent>
                      <SelectItem value="MCQ">Multiple choice</SelectItem>
                      <SelectItem value="ESSAY">Essay response</SelectItem>
                      <SelectItem value="VOICE">Voice response</SelectItem>
                    </SelectContent>
                  </Select>
                </div>
                <div className="flex flex-col gap-2">
                  <Label htmlFor="points">Points</Label>
                  <Input
                    id="points"
                    type="number"
                    min="1"
                    value={points}
                    onChange={(event) => setPoints(event.target.value)}
                  />
                </div>
              </div>
              <div className="flex flex-col gap-2">
                <Label htmlFor="question-text">Prompt</Label>
                <Textarea
                  id="question-text"
                  value={text}
                  onChange={(event) => setText(event.target.value)}
                  placeholder="Write the question prompt"
                />
              </div>
              {type === "MCQ" && (
                <div className="flex flex-col gap-3">
                  <div className="flex items-center justify-between">
                    <Label>Options</Label>
                    <Button
                      type="button"
                      variant="outline"
                      size="sm"
                      onClick={() =>
                        options.length < 4 &&
                        setOptions([
                          ...options,
                          {
                            id: crypto.randomUUID(),
                            optionKey: String.fromCharCode(65 + options.length),
                            optionText: "",
                            displayOrder: options.length + 1,
                            isCorrect: false,
                          },
                        ])
                      }
                      disabled={options.length >= 4}
                    >
                      <Plus data-icon="inline-start" />
                      Add option
                    </Button>
                  </div>
                  <RadioGroup
                    value={options.find((option) => option.isCorrect)?.id}
                    onValueChange={(id) =>
                      setOptions(
                        options.map((option) => ({ ...option, isCorrect: option.id === id }))
                      )
                    }
                  >
                    {options.map((option, index) => (
                      <div className="flex items-center gap-2" key={option.id}>
                        <RadioGroupItem
                          value={option.id}
                          id={`option-${option.id}`}
                          aria-label={`Mark option ${option.optionKey} correct`}
                        />
                        <Label htmlFor={`option-${option.id}`} className="w-6">
                          {option.optionKey}
                        </Label>
                        <Input
                          aria-label={`Option ${option.optionKey} text`}
                          value={option.optionText}
                          onChange={(event) =>
                            setOptions(
                              options.map((item) =>
                                item.id === option.id
                                  ? { ...item, optionText: event.target.value }
                                  : item
                              )
                            )
                          }
                        />
                        <Button
                          type="button"
                          variant="ghost"
                          size="icon"
                          aria-label={`Delete option ${option.optionKey}`}
                          onClick={() =>
                            options.length > 2 &&
                            setOptions(options.filter((item) => item.id !== option.id))
                          }
                          disabled={options.length <= 2}
                        >
                          <Trash2 />
                        </Button>
                      </div>
                    ))}
                  </RadioGroup>
                </div>
              )}
            </div>
            <DialogFooter>
              <Button variant="outline" onClick={() => setOpen(false)}>
                Cancel
              </Button>
              <Button onClick={addQuestion} disabled={!text.trim()}>
                Save question
              </Button>
            </DialogFooter>
          </DialogContent>
        </Dialog>
        <div className="flex flex-col gap-2" role="list" aria-label="Questions">
          {items.map((question) => (
            <button
              key={question.id}
              role="listitem"
              onClick={() => setSelected(question.id)}
              className={`flex items-center gap-3 rounded-lg border p-3 text-left transition-colors ${selected === question.id ? "border-primary bg-primary/5" : "hover:bg-muted"}`}
            >
              <span className="grid size-8 place-items-center rounded-full bg-muted text-sm font-semibold">
                {question.questionNumber}
              </span>
              <span className="min-w-0 flex-1 truncate text-sm">{question.text}</span>
              <ChevronDown aria-hidden="true" className="rotate-[-90deg] text-muted-foreground" />
            </button>
          ))}
        </div>
      </aside>
      <main>
        {items
          .filter((item) => item.id === selected)
          .map((question) => (
            <Card key={question.id}>
              <CardHeader>
                <div className="flex items-start justify-between gap-4">
                  <div>
                    <CardDescription>
                      Question {question.questionNumber} · {question.points} points
                    </CardDescription>
                    <CardTitle className="mt-2 max-w-2xl text-2xl leading-relaxed">
                      {question.text}
                    </CardTitle>
                  </div>
                  <Badge variant="secondary">
                    {question.type === "MCQ" ? (
                      "Multiple choice"
                    ) : question.type === "VOICE" ? (
                      <>
                        <Mic data-icon="inline-start" />
                        Voice
                      </>
                    ) : (
                      <>
                        <Type data-icon="inline-start" />
                        Essay
                      </>
                    )}
                  </Badge>
                </div>
              </CardHeader>
              <CardContent className="flex flex-col gap-4">
                {question.options.map((option) => (
                  <div
                    className={`flex items-center gap-3 rounded-lg border p-4 ${option.isCorrect ? "border-primary bg-primary/5" : ""}`}
                    key={option.id}
                  >
                    <span className="font-semibold">{option.optionKey}</span>
                    <span className="flex-1">{option.optionText || "Untitled option"}</span>
                    {option.isCorrect && (
                      <Check aria-label="Correct option" className="text-primary" />
                    )}
                  </div>
                ))}
                <Button
                  variant="destructive"
                  className="self-start"
                  onClick={() => removeQuestion(question.id)}
                >
                  <Trash2 data-icon="inline-start" />
                  Delete question
                </Button>
              </CardContent>
            </Card>
          ))}
      </main>
    </section>
  );
}
export default QuestionManager;
