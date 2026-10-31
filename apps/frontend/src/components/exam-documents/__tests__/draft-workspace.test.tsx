import { describe, it, expect, vi, beforeEach, afterEach } from "vitest";
import { render, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { ApiError } from "@/services/api";
import type { Exam } from "@/types/exam-types";
import type { DraftQuestion, ExamDocument } from "@/types/exam-document";

vi.mock("@/services/exam-documents-api", () => ({
  examDocumentsApi: {
    getDraft: vi.fn(), replaceQuestion: vi.fn(), addQuestion: vi.fn(),
    deleteQuestion: vi.fn(), reorder: vi.fn(), approve: vi.fn(), getFile: vi.fn(),
  },
}));

import { examDocumentsApi } from "@/services/exam-documents-api";
import { DraftWorkspace } from "../draft-workspace";
import { FakeDraftServer, makeDocument, makeQuestion } from "./fake-draft-server";

const api = vi.mocked(examDocumentsApi) as unknown as Record<string, ReturnType<typeof vi.fn>>;

const EXAM: Exam = {
  id: "e1", title: "Physics Midterm", subject: "Physics", description: "", durationMins: 60, status: "DRAFT",
  createdBy: "u1", createdAt: "2026-01-01T00:00:00Z", updatedAt: "2026-01-01T00:00:00Z", questions: 4,
};

const mcq = (over: Partial<DraftQuestion> = {}) =>
  makeQuestion({
    id: "q1", numberLabel: "1", type: "MCQ", text: "What is the SI unit of force?", points: 2,
    options: [
      { label: "A", text: "Newton", isCorrect: false },
      { label: "B", text: "Joule", isCorrect: false },
    ],
    ...over,
  });
const essay = (over: Partial<DraftQuestion> = {}) => makeQuestion({ id: "q2", numberLabel: "2", text: "Explain Newton's first law.", points: 5, ...over });
const unmarked = (over: Partial<DraftQuestion> = {}) => makeQuestion({ id: "q3", numberLabel: "3", text: "Define velocity.", points: null, ...over });

async function setup(
  questions: DraftQuestion[] = [mcq(), essay(), unmarked()],
  opts: { exam?: Partial<Exam>; document?: Partial<ExamDocument>; server?: (s: FakeDraftServer) => void } = {}
) {
  const server = new FakeDraftServer(questions, makeDocument(opts.document));
  opts.server?.(server);
  server.attach(api);
  const user = userEvent.setup();
  const utils = render(<DraftWorkspace examId="e1" documentId="d1" exam={{ ...EXAM, ...opts.exam }} />);
  await screen.findByRole("heading", { name: /^Questions \(/ });
  return { server, user, ...utils };
}

const card = (id: string) => within(screen.getByTestId(`question-${id}`));
const announcer = () => screen.getByTestId("workspace-announcer");

describe("DraftWorkspace", () => {
  beforeEach(() => {
    Object.values(api).forEach((f) => f.mockReset());
    sessionStorage.clear();
  });
  afterEach(() => vi.restoreAllMocks());

  describe("what the teacher sees", () => {
    it("labels the page's content as a draft, not part of the exam", async () => {
      await setup();
      const note = screen.getByRole("note");
      expect(note).toHaveTextContent(/draft for you to check, not part of the exam/i);
      expect(note).toHaveTextContent(/nothing is added to the exam until you approve/i);
    });

    it("lists the questions in order with number, type, marks and position", async () => {
      await setup();
      expect(card("q1").getByRole("heading", { level: 3 })).toHaveTextContent(/Question 1[\s\S]*Multiple choice[\s\S]*2 marks/);
      expect(card("q2").getByRole("heading", { level: 3 })).toHaveTextContent(/Question 2[\s\S]*Written answer[\s\S]*5 marks/);
      expect(card("q3").getByRole("heading", { level: 3 })).toHaveTextContent(/Marks not set/);
      expect(screen.getByText("Position 2 of 3 · page 1 of the paper")).toBeInTheDocument();
      expect(screen.getByRole("list", { name: "Draft questions" }).children).toHaveLength(3);
    });

    it("says in words which questions need attention, and why, and can show only those", async () => {
      const { user } = await setup();
      expect(screen.getByTestId("attention-summary")).toHaveTextContent("1 of 3 needs attention.");
      expect(card("q3").getByText(/needs attention/i)).toBeInTheDocument();
      expect(card("q3").getByText("Marks are not set.")).toBeInTheDocument();
      expect(within(screen.getByTestId("question-q1").parentElement!).queryAllByText(/^Needs attention/i)).toHaveLength(1);

      await user.click(screen.getByRole("checkbox", { name: /only questions that need attention/i }));
      expect(screen.queryByTestId("question-q1")).not.toBeInTheDocument();
      expect(screen.getByTestId("question-q3")).toBeInTheDocument();
    });

    it("shows the instructions found on the paper, and says they are not added to the exam", async () => {
      await setup([mcq()], { server: (s) => (s.documentInstructions = "Answer ALL questions.") });
      const section = screen.getByRole("region", { name: /instructions for the whole paper/i });
      expect(section).toHaveTextContent("Answer ALL questions.");
      expect(section).toHaveTextContent(/not added to the exam/i);
    });

    it("lists parts of the paper that were not read or could not be placed", async () => {
      await setup([mcq()], {
        document: { pages: [{ number: 2, kind: "IMAGE_ONLY", chars: 0, columns: 1, images: 0, imagesSkipped: 0, note: "This page is a picture and no vision model is configured, so it was not read." }] },
        server: (s) => (s.unassigned = [{ page: 3, text: "Formula sheet: v = u + at", reason: "Not a question" }]),
      });
      const section = screen.getByRole("region", { name: /parts of the paper that may be missing/i });
      expect(section).toHaveTextContent("Page 2: This page is a picture and no vision model is configured");
      expect(section).toHaveTextContent("Page 3: Not a question");
      expect(section).toHaveTextContent("Formula sheet: v = u + at");
    });

    it("has an honest empty state", async () => {
      await setup([]);
      expect(screen.getByText(/no questions in this draft/i)).toBeInTheDocument();
      expect(screen.getByRole("button", { name: /add a question/i })).toBeEnabled();
      expect(screen.getByRole("button", { name: /approve and add to exam/i })).toBeDisabled();
      expect(screen.getByText("There are no questions to add.")).toBeInTheDocument();
    });

    it("keeps unsupported content visible rather than dropping it: tables, diagrams and an equation note", async () => {
      const { user } = await setup([
        mcq({
          tables: [{ page: 1, markdown: "| a | b |\n|---|---|\n| 1 | 2 |" }],
          diagrams: [{ page: 1, region: "left", description: "A box on a slope.", descriptionSource: "MODEL", confirmed: false }],
        }),
      ]);
      await user.click(screen.getByRole("button", { name: /edit question 1/i }));
      expect(screen.getByLabelText(/table 1 \(one row per line/i)).toHaveValue("| a | b |\n|---|---|\n| 1 | 2 |");
      expect(screen.getByLabelText(/description of diagram 1/i)).toHaveValue("A box on a slope.");
      expect(screen.getByText(/written by the reader from what it could see/i)).toBeInTheDocument();
      expect(screen.getByText(/equations and symbols are kept as plain text/i)).toBeInTheDocument();
      expect(screen.getByText(/pictures are not copied into the exam/i)).toBeInTheDocument();
    });
  });

  describe("editing a question", () => {
    it("saves the complete question with the draft's version, and shows what the server saved", async () => {
      const { user, server } = await setup();
      await user.click(screen.getByRole("button", { name: /edit question 2/i }));
      const text = screen.getByLabelText("Question text");
      await user.clear(text);
      await user.type(text, "Explain Newton's second law.");
      await user.click(screen.getByRole("button", { name: /save changes/i }));

      await waitFor(() => expect(server.count("replace")).toBe(1));
      const sent = server.calls.find((c) => c.op === "replace")!.args as { expectedDraftVersion: number; question: { text: string; points: number; options: unknown[] } };
      expect(sent.expectedDraftVersion).toBe(1);
      expect(sent.question).toMatchObject({ text: "Explain Newton's second law.", points: 5, options: [] });
      // The editor stays open and shows what the server saved; closed, the preview shows it too.
      await waitFor(() => expect(announcer()).toHaveTextContent("Question 2 saved."));
      expect(screen.getByLabelText("Question text")).toHaveValue("Explain Newton's second law.");
      await user.click(screen.getByRole("button", { name: /close question 2/i }));
      expect(card("q2").getByTestId("question-preview")).toHaveTextContent("Explain Newton's second law.");
    });

    it("announces the save and puts the keyboard back on the question's heading", async () => {
      const { user } = await setup();
      await user.click(screen.getByRole("button", { name: /edit question 2/i }));
      await user.type(screen.getByLabelText("Question text"), " Give an example.");
      await user.click(screen.getByRole("button", { name: /save changes/i }));
      await waitFor(() => expect(announcer()).toHaveTextContent("Question 2 saved."));
      await waitFor(() => expect(card("q2").getByRole("heading", { level: 3 })).toHaveFocus());
    });

    it("only offers Save when something changed, and Discard puts everything back", async () => {
      const { user } = await setup();
      await user.click(screen.getByRole("button", { name: /edit question 2/i }));
      expect(screen.getByRole("button", { name: /save changes/i })).toBeDisabled();
      expect(screen.queryByRole("button", { name: /discard changes/i })).not.toBeInTheDocument();

      await user.type(screen.getByLabelText("Question text"), " extra");
      expect(screen.getByRole("button", { name: /save changes/i })).toBeEnabled();
      expect(screen.getByText(/you have unsaved changes/i)).toBeInTheDocument();
      await user.click(screen.getByRole("button", { name: /discard changes/i }));
      expect(screen.getByLabelText("Question text")).toHaveValue("Explain Newton's first law.");
      expect(screen.getByRole("button", { name: /save changes/i })).toBeDisabled();
    });

    it("edits multiple-choice options: change, add with the next label, remove, and one correct answer at most", async () => {
      const { user, server } = await setup([mcq()]);
      await user.click(screen.getByRole("button", { name: /edit question 1/i }));
      const optText = screen.getByLabelText("Option 2 text");
      await user.clear(optText);
      await user.type(optText, "Joules");
      await user.click(screen.getByRole("button", { name: /add option/i }));
      expect(screen.getByLabelText("Option 3 label")).toHaveValue("C");
      await user.type(screen.getByLabelText("Option 3 text"), "Watt");

      await user.click(screen.getByRole("checkbox", { name: /correct answer: option A/i }));
      await user.click(screen.getByRole("checkbox", { name: /correct answer: option C/i }));
      expect(screen.getByRole("checkbox", { name: /correct answer: option A/i })).not.toBeChecked();
      expect(screen.getByRole("checkbox", { name: /correct answer: option C/i })).toBeChecked();

      await user.click(screen.getByRole("button", { name: /remove option B/i }));
      await user.click(screen.getByRole("button", { name: /save changes/i }));
      await waitFor(() => expect(server.count("replace")).toBe(1));
      const sent = (server.calls[server.calls.length - 1].args as { question: { options: unknown[] } }).question.options;
      expect(sent).toEqual([
        { label: "A", text: "Newton", isCorrect: false },
        { label: "C", text: "Watt", isCorrect: true },
      ]);
    });

    it("never marks an answer correct on its own", async () => {
      const { user } = await setup([mcq()]);
      await user.click(screen.getByRole("button", { name: /edit question 1/i }));
      for (const box of screen.getAllByRole("checkbox", { name: /correct answer/i })) expect(box).not.toBeChecked();
      expect(screen.getByText(/the reader never chooses one/i)).toBeInTheDocument();
    });

    it("sends no options when a question is switched to a written answer", async () => {
      const { user, server } = await setup([mcq()]);
      await user.click(screen.getByRole("button", { name: /edit question 1/i }));
      await user.selectOptions(screen.getByLabelText("Question type"), "ESSAY");
      expect(screen.getByText(/options are kept here in case you switch back/i)).toBeInTheDocument();
      await user.click(screen.getByRole("button", { name: /save changes/i }));
      await waitFor(() => expect(server.count("replace")).toBe(1));
      expect((server.calls[server.calls.length - 1].args as { question: { type: string; options: unknown[] } }).question).toMatchObject({ type: "ESSAY", options: [] });
    });

    it("sets marks on a question that had none", async () => {
      const { user, server } = await setup();
      await user.click(screen.getByRole("button", { name: /edit question 3/i }));
      expect(screen.getByLabelText("Marks")).toHaveValue("");
      await user.type(screen.getByLabelText("Marks"), "4");
      await user.click(screen.getByRole("button", { name: /save changes/i }));
      await waitFor(() => expect(server.count("replace")).toBe(1));
      await waitFor(() => expect(card("q3").queryByText("Marks are not set.")).not.toBeInTheDocument());
      expect(screen.getByTestId("attention-summary")).toHaveTextContent("Every question is ready for approval.");
    });

    it("acknowledges a blocking warning, and that clears the question's attention", async () => {
      const warning = { code: "UNVERIFIED_TEXT", message: "The wording of the question text could not be found on the page.", blocking: true, acknowledged: false };
      const { user, server } = await setup([essay({ warnings: [warning] })]);
      expect(card("q2").getByText("A warning needs your check before approval.")).toBeInTheDocument();

      await user.click(screen.getByRole("button", { name: /edit question 2/i }));
      expect(screen.getByText(/needs your check:/i)).toBeInTheDocument();
      await user.click(screen.getByRole("checkbox", { name: /could not be found on the page/i }));
      await user.click(screen.getByRole("button", { name: /save changes/i }));
      await waitFor(() => expect(server.count("replace")).toBe(1));
      expect((server.calls[server.calls.length - 1].args as { question: { acknowledgedWarnings: string[] } }).question.acknowledgedWarnings).toEqual(["UNVERIFIED_TEXT"]);
      await waitFor(() => expect(card("q2").queryByText("A warning needs your check before approval.")).not.toBeInTheDocument());
    });

    it("shows non-blocking warnings and low confidence as notes, in words", async () => {
      const { user } = await setup([
        essay({ confidence: "LOW", warnings: [{ code: "PAGE_UNKNOWN", message: "The page this came from was not given.", blocking: false, acknowledged: false }] }),
      ]);
      await user.click(screen.getByRole("button", { name: /edit question 2/i }));
      const things = screen.getByRole("region", { name: "Things to check" });
      expect(things).toHaveTextContent("Note: The reader was not confident about this question");
      expect(things).toHaveTextContent("Note: The page this came from was not given.");
    });

    it("needs a diagram description to be confirmed, and editing the description withdraws the confirmation", async () => {
      const { user, server } = await setup([
        essay({ diagrams: [{ page: 1, region: "", description: "A circuit.", descriptionSource: "MODEL", confirmed: true }] }),
      ]);
      await user.click(screen.getByRole("button", { name: /edit question 2/i }));
      const confirm = screen.getByRole("checkbox", { name: /i have checked this description/i });
      expect(confirm).toBeChecked();
      await user.type(screen.getByLabelText(/description of diagram 1/i), " With a 4 ohm resistor.");
      expect(confirm).not.toBeChecked();
      await user.click(confirm);
      await user.click(screen.getByRole("button", { name: /save changes/i }));
      await waitFor(() => expect(server.count("replace")).toBe(1));
      expect((server.calls[server.calls.length - 1].args as { question: { diagrams: Array<{ description: string; confirmed: boolean }> } }).question.diagrams[0])
        .toMatchObject({ description: "A circuit. With a 4 ohm resistor.", confirmed: true });
    });
  });

  describe("validation and failure", () => {
    it("explains a problem at once, puts the cursor on it, and does not ask the server", async () => {
      const { user, server } = await setup();
      await user.click(screen.getByRole("button", { name: /edit question 2/i }));
      await user.clear(screen.getByLabelText("Question text"));
      await user.type(screen.getByLabelText("Marks"), "x");
      await user.click(screen.getByRole("button", { name: /save changes/i }));

      const text = screen.getByLabelText("Question text");
      expect(text).toHaveAttribute("aria-invalid", "true");
      expect(text).toHaveFocus();
      expect(document.getElementById(text.getAttribute("aria-describedby")!)).toHaveTextContent(/enter the question text/i);
      expect(screen.getByLabelText("Marks")).toHaveAttribute("aria-invalid", "true");
      expect(server.count("replace")).toBe(0);
    });

    it("shows the server's complaint beside the field and keeps everything typed", async () => {
      const { user, server } = await setup();
      server.failNext = new ApiError("bad", 400, "VALIDATION_ERROR", true, [{ field: "points", error: "must be between 0 and 1000" }]);
      await user.click(screen.getByRole("button", { name: /edit question 2/i }));
      await user.type(screen.getByLabelText("Question text"), " more");
      await user.click(screen.getByRole("button", { name: /save changes/i }));

      expect(await screen.findByText("must be between 0 and 1000")).toBeInTheDocument();
      expect(screen.getByLabelText("Marks")).toHaveAttribute("aria-invalid", "true");
      expect(screen.getByLabelText("Question text")).toHaveValue("Explain Newton's first law. more");
      expect(screen.getByRole("alert")).toHaveTextContent(/see the highlighted fields/i);
    });

    it("keeps the edit after a network failure and saves it on the next try", async () => {
      const { user, server } = await setup();
      server.failNext = new ApiError("down", 0);
      await user.click(screen.getByRole("button", { name: /edit question 2/i }));
      await user.type(screen.getByLabelText("Question text"), " more");
      await user.click(screen.getByRole("button", { name: /save changes/i }));

      const alert = await screen.findByRole("alert");
      expect(alert).toHaveTextContent(/could not reach the server/i);
      expect(alert).toHaveTextContent(/your changes are still here/i);
      expect(screen.getByLabelText("Question text")).toHaveValue("Explain Newton's first law. more");

      await user.click(screen.getByRole("button", { name: /try saving again/i }));
      await waitFor(() => expect(announcer()).toHaveTextContent("Question 2 saved."));
      expect(server.count("replace")).toBe(2);
      expect(server.questions[1].text).toBe("Explain Newton's first law. more");
    });

    it("sends a save only once however often the button is pressed", async () => {
      const { user, server } = await setup();
      await user.click(screen.getByRole("button", { name: /edit question 2/i }));
      await user.type(screen.getByLabelText("Question text"), " more");
      const save = screen.getByRole("button", { name: /save changes/i });
      await user.dblClick(save);
      save.focus();
      await user.keyboard("{Enter}{Enter}");
      await waitFor(() => expect(announcer()).toHaveTextContent("Question 2 saved."));
      expect(server.count("replace")).toBe(1);
    });

    it("will not close an editor that has unsaved changes, and says why", async () => {
      const { user } = await setup();
      await user.click(screen.getByRole("button", { name: /edit question 2/i }));
      await user.type(screen.getByLabelText("Question text"), " more");
      await user.click(screen.getByRole("button", { name: /close question 2/i }));
      expect(screen.getByLabelText("Question text")).toBeInTheDocument();
      expect(announcer()).toHaveTextContent(/unsaved changes\. Save or discard them before closing/i);
    });

    it("brings back an unsaved edit after the page was reloaded", async () => {
      const first = await setup();
      await first.user.click(screen.getByRole("button", { name: /edit question 2/i }));
      await first.user.type(screen.getByLabelText("Question text"), " (half typed");
      first.unmount();

      const again = userEvent.setup();
      render(<DraftWorkspace examId="e1" documentId="d1" exam={EXAM} />);
      await screen.findByRole("heading", { name: /^Questions \(/ });
      await again.click(screen.getByRole("button", { name: /edit question 2/i }));
      expect(screen.getByLabelText("Question text")).toHaveValue("Explain Newton's first law. (half typed");
      expect(screen.getByText(/earlier unsaved changes .* were restored/i)).toBeInTheDocument();
    });

    it("does not bring back an edit once it has been saved", async () => {
      const first = await setup();
      await first.user.click(screen.getByRole("button", { name: /edit question 2/i }));
      await first.user.type(screen.getByLabelText("Question text"), " saved");
      await first.user.click(screen.getByRole("button", { name: /save changes/i }));
      await waitFor(() => expect(announcer()).toHaveTextContent("saved."));
      expect(sessionStorage.length).toBe(0);
    });
  });

  describe("adding, deleting and reordering", () => {
    it("adds a question the reader missed, at a chosen position, and moves the keyboard to it", async () => {
      const { user, server } = await setup();
      await user.click(screen.getByRole("button", { name: /add a question/i }));
      const newQuestion = screen.getByRole("group", { name: "New question" });
      await user.selectOptions(within(newQuestion).getByLabelText(/put it at position/i), "1");
      await user.type(within(newQuestion).getByLabelText("Question text"), "A question the reader missed.");
      await user.type(within(newQuestion).getByLabelText("Marks"), "3");
      await user.click(within(newQuestion).getByRole("button", { name: "Add question" }));

      await waitFor(() => expect(server.count("add")).toBe(1));
      expect((server.calls.find((c) => c.op === "add")!.args as { position: number; expectedDraftVersion: number })).toMatchObject({ position: 1, expectedDraftVersion: 1 });
      await waitFor(() => expect(announcer()).toHaveTextContent("New question added at position 1."));
      expect(screen.queryByRole("group", { name: "New question" })).not.toBeInTheDocument();
      const first = screen.getByRole("list", { name: "Draft questions" }).children[0] as HTMLElement;
      expect(within(first).getByTestId("question-preview")).toHaveTextContent("A question the reader missed.");
      expect(within(first).getByText("Added by you")).toBeInTheDocument();
      await waitFor(() => expect(within(first).getByRole("heading", { level: 3 })).toHaveFocus());
    });

    it("validates a new question before sending it, and can be cancelled", async () => {
      const { user, server } = await setup();
      await user.click(screen.getByRole("button", { name: /add a question/i }));
      await user.click(within(screen.getByRole("group", { name: "New question" })).getByRole("button", { name: "Add question" }));
      expect(screen.getByLabelText("Question text")).toHaveAttribute("aria-invalid", "true");
      expect(server.count("add")).toBe(0);

      await user.click(within(screen.getByRole("group", { name: "New question" })).getByRole("button", { name: "Cancel" }));
      expect(screen.queryByRole("group", { name: "New question" })).not.toBeInTheDocument();
      expect(screen.getByRole("button", { name: /add a question/i })).toHaveFocus();
    });

    it("deletes only after confirmation, then focuses the next question", async () => {
      const { user, server } = await setup();
      await user.click(screen.getByRole("button", { name: /delete question 2/i }));
      const dialog = await screen.findByRole("dialog");
      expect(dialog).toHaveTextContent(/removed from this draft only/i);
      expect(server.count("delete")).toBe(0);

      await user.click(within(dialog).getByRole("button", { name: /delete question/i }));
      await waitFor(() => expect(server.count("delete")).toBe(1));
      await waitFor(() => expect(screen.queryByTestId("question-q2")).not.toBeInTheDocument());
      expect(announcer()).toHaveTextContent("Question 2 deleted from the draft. 2 questions left.");
      await waitFor(() => expect(card("q3").getByRole("heading", { level: 3 })).toHaveFocus());
    });

    it("backing out of a delete deletes nothing", async () => {
      const { user, server } = await setup();
      await user.click(screen.getByRole("button", { name: /delete question 2/i }));
      await user.click(within(await screen.findByRole("dialog")).getByRole("button", { name: /^cancel$/i }));
      await waitFor(() => expect(screen.queryByRole("dialog")).not.toBeInTheDocument());
      expect(server.count("delete")).toBe(0);
      expect(screen.getByTestId("question-q2")).toBeInTheDocument();
    });

    it("moves a question with buttons, announces where it went, and keeps the keyboard on the same button", async () => {
      const { user, server } = await setup();
      const down = screen.getByRole("button", { name: "Move question 1 down" });
      await user.click(down);
      await waitFor(() => expect(server.count("reorder")).toBe(1));
      expect((server.calls.find((c) => c.op === "reorder")!.args as { questionIds: string[] }).questionIds).toEqual(["q2", "q1", "q3"]);
      expect(announcer()).toHaveTextContent("Question 1 moved down to position 2 of 3.");
      const items = screen.getByRole("list", { name: "Draft questions" }).children;
      expect(within(items[0] as HTMLElement).getByRole("heading", { level: 3 })).toHaveTextContent("Question 2");
      await waitFor(() => expect(screen.getByRole("button", { name: "Move question 1 down" })).toHaveFocus());
    });

    it("works the same from the keyboard alone", async () => {
      const { user, server } = await setup();
      const up = screen.getByRole("button", { name: "Move question 3 up" });
      up.focus();
      await user.keyboard("{Enter}");
      await waitFor(() => expect(server.count("reorder")).toBe(1));
      expect(server.questions.map((q) => q.id)).toEqual(["q1", "q3", "q2"]);
    });

    it("cannot move the first question up or the last down", async () => {
      await setup();
      expect(screen.getByRole("button", { name: "Move question 1 up" })).toBeDisabled();
      expect(screen.getByRole("button", { name: "Move question 3 down" })).toBeDisabled();
    });

    it("moves focus to the heading when a question reaches the end of the list", async () => {
      const { user } = await setup([mcq(), essay()]);
      await user.click(screen.getByRole("button", { name: "Move question 1 down" }));
      await waitFor(() => expect(announcer()).toHaveTextContent("moved down to position 2 of 2"));
      await waitFor(() => expect(card("q1").getByRole("heading", { level: 3 })).toHaveFocus());
    });

    it("shows a failed move as an error and leaves the order alone", async () => {
      const { user, server } = await setup();
      server.failNext = new ApiError("down", 0);
      await user.click(screen.getByRole("button", { name: "Move question 1 down" }));
      expect(await screen.findByRole("alert")).toHaveTextContent(/could not reach the server/i);
      expect(server.questions.map((q) => q.id)).toEqual(["q1", "q2", "q3"]);
    });
  });

  describe("when the draft was changed somewhere else", () => {
    it("tells the teacher, refuses further changes, and recovers on reload without losing typing", async () => {
      const { user, server } = await setup();
      await user.click(screen.getByRole("button", { name: /edit question 2/i }));
      await user.type(screen.getByLabelText("Question text"), " mine");
      server.version++; // someone else saved in another tab
      server.questions[2].text = "Changed in the other tab.";

      await user.click(screen.getByRole("button", { name: /save changes/i }));
      const banner = await screen.findByText(/changed somewhere else \(perhaps in another tab\)/i);
      expect(banner.closest("[role=alert]")).toBeInTheDocument();
      expect(screen.getByRole("button", { name: /add a question/i })).toBeDisabled();
      expect(screen.getByRole("button", { name: "Move question 1 down" })).toBeDisabled();
      expect(screen.getByLabelText("Question text")).toHaveValue("Explain Newton's first law. mine");
      expect(screen.getByRole("button", { name: /approve and add to exam/i })).toBeDisabled();

      await user.click(screen.getByRole("button", { name: /reload the draft/i }));
      await waitFor(() => expect(screen.queryByText(/changed somewhere else \(perhaps/i)).not.toBeInTheDocument());
      expect(card("q3").getByTestId("question-preview")).toHaveTextContent("Changed in the other tab.");
      expect(screen.getByLabelText("Question text")).toHaveValue("Explain Newton's first law. mine");

      await user.click(screen.getByRole("button", { name: /save changes/i }));
      await waitFor(() => expect(server.questions[1].text).toBe("Explain Newton's first law. mine"));
    });
  });

  describe("approving", () => {
    const ready = () => [mcq(), essay(), unmarked({ points: 3 })];

    it("is blocked, with the reasons listed, while questions need attention or edits are unsaved", async () => {
      const { user } = await setup();
      const approve = screen.getByRole("button", { name: /approve and add to exam/i });
      expect(approve).toBeDisabled();
      const list = document.getElementById(approve.getAttribute("aria-describedby")!)!;
      expect(list).toHaveTextContent("1 question needs attention.");

      await user.click(screen.getByRole("button", { name: /edit question 3/i }));
      await user.type(screen.getByLabelText("Marks"), "2");
      expect(list).toHaveTextContent("Save or discard your unsaved changes.");
    });

    it("is blocked when the exam is no longer a draft, and says so", async () => {
      await setup(ready(), { exam: { status: "PUBLISHED" } });
      expect(screen.getByRole("button", { name: /approve and add to exam/i })).toBeDisabled();
      expect(screen.getByText(/the exam is published, so questions can't be added/i)).toBeInTheDocument();
    });

    it("explains what approval does before doing it, and does nothing if cancelled", async () => {
      const { user, server } = await setup(ready());
      await user.click(screen.getByRole("button", { name: /approve and add to exam/i }));
      const dialog = await screen.findByRole("dialog");
      expect(dialog).toHaveTextContent("Add 3 questions to the exam?");
      expect(dialog).toHaveTextContent("“Physics Midterm”");
      expect(dialog).toHaveTextContent("after its 4 existing questions");
      expect(dialog).toHaveTextContent(/nothing is published and no student is assigned/i);
      await user.click(within(dialog).getByRole("button", { name: /^cancel$/i }));
      await waitFor(() => expect(screen.queryByRole("dialog")).not.toBeInTheDocument());
      expect(server.count("approve")).toBe(0);
    });

    it("adds the questions once, however often confirm is pressed, then becomes a read-only record", async () => {
      const { user, server } = await setup(ready());
      await user.click(screen.getByRole("button", { name: /approve and add to exam/i }));
      const confirm = within(await screen.findByRole("dialog")).getByRole("button", { name: /add questions to exam/i });
      await user.dblClick(confirm);

      await waitFor(() => expect(server.count("approve")).toBe(1));
      expect((server.calls.find((c) => c.op === "approve")!.args as { expectedDraftVersion: number }).expectedDraftVersion).toBe(1);
      const success = await screen.findByRole("region", { name: /3 questions were added to the exam/i });
      expect(success).toHaveTextContent(/nothing\s+has been published or assigned/i);
      expect(within(success).getByRole("link", { name: /go to the exam/i })).toHaveAttribute("href", "/exams/e1");
      await waitFor(() => expect(within(success).getByRole("heading", { level: 2 })).toHaveFocus());
      expect(announcer()).toHaveTextContent("3 questions were added to the exam.");

      // Nothing can be edited any more.
      expect(screen.queryByRole("button", { name: /add a question/i })).not.toBeInTheDocument();
      expect(screen.queryByRole("button", { name: /approve and add to exam/i })).not.toBeInTheDocument();
      expect(screen.queryByRole("button", { name: /delete question/i })).not.toBeInTheDocument();
      expect(screen.getByRole("button", { name: "Move question 1 down" })).toBeDisabled();
      expect(screen.getByRole("button", { name: /view question 1/i })).toBeInTheDocument();
    });

    it("lists what the server found wrong, in words, when it refuses, and the draft stays editable", async () => {
      const { user, server } = await setup(ready());
      // A problem the page could not see: the server has the diagram unconfirmed.
      server.questions[0].diagrams = [{ page: 1, region: "", description: "A box.", confirmed: false }];
      await user.click(screen.getByRole("button", { name: /edit question 1/i }));
      await user.click(screen.getByRole("button", { name: /close question 1/i }));
      // The page's own copy says it is fine, so approval is offered; the server disagrees.
      await user.click(screen.getByRole("button", { name: /approve and add to exam/i }));
      await user.click(within(await screen.findByRole("dialog")).getByRole("button", { name: /add questions to exam/i }));

      const alert = await screen.findByRole("alert");
      expect(alert).toHaveTextContent(/isn't ready to approve yet/i);
      expect(alert).toHaveTextContent("Question 1, diagram 0 confirmed: Confirm the diagram.");
      expect(screen.getByRole("button", { name: /add a question/i })).toBeEnabled();
      expect(screen.queryByRole("region", { name: /were added to the exam/i })).not.toBeInTheDocument();
    });

    it("explains a refusal because the exam changed state", async () => {
      const { user, server } = await setup(ready());
      server.failNext = new ApiError("x", 409, "EXAM_NOT_DRAFT", true);
      await user.click(screen.getByRole("button", { name: /approve and add to exam/i }));
      await user.click(within(await screen.findByRole("dialog")).getByRole("button", { name: /add questions to exam/i }));
      expect(await screen.findByRole("alert")).toHaveTextContent(/no longer a draft/i);
    });
  });

  describe("a draft that was already approved", () => {
    it("is shown for reference only", async () => {
      await setup(
        [mcq(), essay()],
        { document: { status: "COMPLETED", approvedAt: "2026-03-02T10:00:00Z" } }
      );
      expect(screen.getByRole("note")).toHaveTextContent(/this draft was approved/i);
      expect(screen.queryByRole("button", { name: /add a question/i })).not.toBeInTheDocument();
      expect(screen.queryByRole("button", { name: /approve/i })).not.toBeInTheDocument();
      expect(screen.queryByRole("button", { name: /delete question/i })).not.toBeInTheDocument();
      expect(screen.queryByText(/needs attention/i)).not.toBeInTheDocument();
    });
  });

  describe("the original PDF", () => {
    function fakeTab() {
      const tab = { location: { href: "" }, opener: {} as unknown, close: vi.fn() };
      vi.spyOn(window, "open").mockReturnValue(tab as unknown as Window);
      return tab;
    }

    beforeEach(() => {
      vi.stubGlobal("URL", Object.assign(URL, { createObjectURL: vi.fn(() => "blob:original-pdf"), revokeObjectURL: vi.fn() }));
    });
    afterEach(() => vi.unstubAllGlobals());

    it("opens the source page in a new tab from the API, never from a storage address", async () => {
      api.getFile.mockResolvedValue(new Blob(["%PDF"], { type: "application/pdf" }));
      const tab = fakeTab();
      const { user } = await setup([mcq({ sourcePages: [2, 3] })]);
      await user.click(screen.getByRole("button", { name: /edit question 1/i }));
      expect(screen.getByText("Found on pages 2, 3 of the paper.")).toBeInTheDocument();

      await user.click(screen.getByRole("button", { name: "View page 3 in the original PDF" }));
      await waitFor(() => expect(tab.location.href).toBe("blob:original-pdf#page=3"));
      expect(tab.opener).toBeNull();
      expect(api.getFile).toHaveBeenCalledTimes(1);

      await user.click(screen.getByRole("button", { name: "View page 2 in the original PDF" }));
      await waitFor(() => expect(tab.location.href).toBe("blob:original-pdf#page=2"));
      expect(api.getFile).toHaveBeenCalledTimes(1); // fetched once, kept in memory
    });

    it("says so, and closes the empty tab, when the PDF cannot be fetched", async () => {
      api.getFile.mockRejectedValueOnce(new ApiError("gone", 404));
      const tab = fakeTab();
      const { user } = await setup([mcq()]);
      await user.click(screen.getByRole("button", { name: /edit question 1/i }));
      await user.click(screen.getByRole("button", { name: "View page 1 in the original PDF" }));
      await waitFor(() => expect(announcer()).toHaveTextContent(/original pdf could not be opened/i));
      expect(tab.close).toHaveBeenCalled();
    });

    it("explains a blocked pop-up", async () => {
      api.getFile.mockResolvedValue(new Blob(["%PDF"]));
      vi.spyOn(window, "open").mockReturnValue(null);
      const { user } = await setup([mcq()]);
      await user.click(screen.getByRole("button", { name: /edit question 1/i }));
      await user.click(screen.getByRole("button", { name: "View page 1 in the original PDF" }));
      await waitFor(() => expect(announcer()).toHaveTextContent(/allow pop-ups/i));
    });

    it("does not offer a source page for a question the teacher added", async () => {
      const { user } = await setup([essay({ origin: "TEACHER", sourcePages: [] })]);
      await user.click(screen.getByRole("button", { name: /edit question 2/i }));
      expect(screen.queryByRole("button", { name: /original pdf/i })).not.toBeInTheDocument();
    });
  });

  describe("loading and failing to load", () => {
    it("shows a failure with a way to try again", async () => {
      api.getDraft.mockRejectedValueOnce(new ApiError("down", 0));
      const server = new FakeDraftServer([mcq()]);
      api.getDraft.mockRejectedValueOnce(new ApiError("down", 0));
      server.attach(api);
      api.getDraft.mockReset();
      api.getDraft.mockRejectedValueOnce(new ApiError("down", 0)).mockImplementation(server.getDraft);
      const user = userEvent.setup();
      render(<DraftWorkspace examId="e1" documentId="d1" exam={EXAM} />);
      expect(await screen.findByRole("alert")).toHaveTextContent(/could not reach the server/i);
      await user.click(screen.getByRole("button", { name: /try again/i }));
      expect(await screen.findByRole("heading", { name: /^Questions \(1\)/ })).toBeInTheDocument();
    });
  });

  it("warns before the tab is closed with unsaved changes, and only then", async () => {
    const { user } = await setup();
    const before = new Event("beforeunload", { cancelable: true });
    window.dispatchEvent(before);
    expect(before.defaultPrevented).toBe(false);

    await user.click(screen.getByRole("button", { name: /edit question 2/i }));
    await user.type(screen.getByLabelText("Question text"), " more");
    const during = new Event("beforeunload", { cancelable: true });
    window.dispatchEvent(during);
    expect(during.defaultPrevented).toBe(true);
  });
});
