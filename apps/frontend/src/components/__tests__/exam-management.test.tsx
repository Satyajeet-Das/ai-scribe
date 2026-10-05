import { render, screen, fireEvent, waitFor } from "@testing-library/react";
import { describe, it, expect, vi, beforeEach } from "vitest";
import { ExamDashboard } from "../exam-dashboard";
import { CreateExamDialog } from "../create-exam-dialog";
import { EditExamDialog } from "../edit-exam-dialog";
import { useAuthStore } from "@/store/auth-store";
import type { Exam } from "@/types/exam-types";
import { ApiError } from "@/services/api";

const mockExams: Exam[] = [
  {
    id: "exam-draft-1",
    title: "Midterm Biology",
    subject: "Biology",
    description: "Cellular respiration and genetics",
    durationMins: 60,
    status: "DRAFT",
    createdBy: "teacher-1",
    createdAt: "2026-03-01T10:00:00Z",
    updatedAt: "2026-03-01T10:00:00Z",
  },
  {
    id: "exam-pub-2",
    title: "Final Chemistry",
    subject: "Chemistry",
    description: "Organic chemistry and thermodynamics",
    durationMins: 90,
    status: "PUBLISHED",
    createdBy: "teacher-1",
    createdAt: "2026-02-15T09:00:00Z",
    updatedAt: "2026-02-20T11:00:00Z",
    publishedAt: "2026-02-20T11:00:00Z",
  },
  {
    id: "exam-arch-3",
    title: "Old History Quiz",
    subject: "History",
    description: "World War II overview",
    durationMins: 45,
    status: "ARCHIVED",
    createdBy: "teacher-1",
    createdAt: "2026-01-10T08:00:00Z",
    updatedAt: "2026-01-25T12:00:00Z",
    publishedAt: "2026-01-15T09:00:00Z",
  },
];

describe("Exam Management Dashboard & Lifecycles", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    useAuthStore.setState({
      user: {
        id: "teacher-1",
        email: "teacher@school.edu",
        role: "TEACHER",
        firstName: "Professor",
        lastName: "Smith",
      },
      accessToken: "mock-token",
      isAuthenticated: true,
      isLoading: false,
    });
  });

  describe("ExamDashboard rendering & role awareness", () => {
    it("renders educator role badge for teacher", () => {
      render(<ExamDashboard exams={mockExams} />);
      expect(screen.getByText("Educator")).toBeInTheDocument();
      expect(screen.getByText("Midterm Biology")).toBeInTheDocument();
      expect(screen.getByText("Final Chemistry")).toBeInTheDocument();
      expect(screen.getByText("Old History Quiz")).toBeInTheDocument();
    });

    it("renders administrator role badge for admin", () => {
      useAuthStore.setState({
        user: {
          id: "admin-1",
          email: "admin@school.edu",
          role: "ADMIN",
          firstName: "Admin",
          lastName: "User",
        },
      });

      render(<ExamDashboard exams={mockExams} />);
      expect(screen.getByText("Administrator")).toBeInTheDocument();
    });

    it("renders skeleton loader when loading is true", () => {
      render(<ExamDashboard exams={[]} loading={true} />);
      expect(screen.getByTestId("exam-loading-skeleton")).toBeInTheDocument();
    });

    it("renders empty state when no exams match", () => {
      render(<ExamDashboard exams={[]} loading={false} />);
      expect(screen.getByText("No assessments found")).toBeInTheDocument();
    });
  });

  describe("Lifecycle actions on dashboard", () => {
    it("shows Edit, Publish, and Delete actions for DRAFT exams", () => {
      render(<ExamDashboard exams={[mockExams[0]]} />);
      expect(screen.getByRole("button", { name: /publish/i })).toBeInTheDocument();
      expect(screen.getByRole("button", { name: /edit/i })).toBeInTheDocument();
      expect(screen.getByRole("button", { name: /delete/i })).toBeInTheDocument();
    });

    it("shows Edit, Revert to Draft, and Archive actions for PUBLISHED exams", () => {
      render(<ExamDashboard exams={[mockExams[1]]} />);
      expect(screen.getByRole("button", { name: /revert to draft/i })).toBeInTheDocument();
      expect(screen.getByRole("button", { name: /archive/i })).toBeInTheDocument();
      expect(screen.getByRole("button", { name: /edit info/i })).toBeInTheDocument();
      expect(screen.queryByRole("button", { name: /delete/i })).not.toBeInTheDocument();
    });

    it("shows read-only badge and no modification actions for ARCHIVED exams", () => {
      render(<ExamDashboard exams={[mockExams[2]]} />);
      expect(screen.getByText("Archived (Read-Only)")).toBeInTheDocument();
      expect(screen.queryByRole("button", { name: /publish/i })).not.toBeInTheDocument();
      expect(screen.queryByRole("button", { name: /edit/i })).not.toBeInTheDocument();
      expect(screen.queryByRole("button", { name: /revert to draft/i })).not.toBeInTheDocument();
      expect(screen.queryByRole("button", { name: /delete/i })).not.toBeInTheDocument();
    });
  });

  describe("Confirmation Dialog for Actions", () => {
    it("opens publish confirmation and calls onPublishExam on confirm", async () => {
      const onPublish = vi.fn().mockResolvedValue({ ...mockExams[0], status: "PUBLISHED" });
      render(<ExamDashboard exams={[mockExams[0]]} onPublishExam={onPublish} />);

      // Click Publish on card
      fireEvent.click(screen.getByRole("button", { name: /publish/i }));

      // Confirmation modal appears
      expect(screen.getByRole("heading", { name: "Publish Assessment" })).toBeInTheDocument();
      expect(screen.getByText(/Are you sure you want to publish "Midterm Biology"/i)).toBeInTheDocument();

      // Click Confirm button inside dialog
      const confirmBtn = screen.getByRole("button", { name: /^Publish Assessment$/i });
      fireEvent.click(confirmBtn);

      await waitFor(() => {
        expect(onPublish).toHaveBeenCalledWith("exam-draft-1");
      });
    });

    it("opens unpublish confirmation and calls onUnpublishExam on confirm", async () => {
      const onUnpublish = vi.fn().mockResolvedValue({ ...mockExams[1], status: "DRAFT" });
      render(<ExamDashboard exams={[mockExams[1]]} onUnpublishExam={onUnpublish} />);

      // Click Revert to Draft on card
      fireEvent.click(screen.getByRole("button", { name: /revert to draft/i }));

      expect(screen.getByRole("heading", { name: "Revert Assessment to Draft" })).toBeInTheDocument();
      expect(screen.getByText(/Are you sure you want to revert "Final Chemistry"/i)).toBeInTheDocument();

      const confirmBtn = screen.getByRole("button", { name: /^Revert to Draft$/i });
      fireEvent.click(confirmBtn);

      await waitFor(() => {
        expect(onUnpublish).toHaveBeenCalledWith("exam-pub-2");
      });
    });

    it("opens archive confirmation and calls onArchiveExam on confirm", async () => {
      const onArchive = vi.fn().mockResolvedValue({ ...mockExams[1], status: "ARCHIVED" });
      render(<ExamDashboard exams={[mockExams[1]]} onArchiveExam={onArchive} />);

      // Click Archive on card
      fireEvent.click(screen.getByRole("button", { name: /archive/i }));

      expect(screen.getByRole("heading", { name: "Archive Assessment" })).toBeInTheDocument();
      expect(screen.getByText(/Archived assessments are permanently frozen/i)).toBeInTheDocument();

      const confirmBtn = screen.getByRole("button", { name: /^Archive Assessment$/i });
      fireEvent.click(confirmBtn);

      await waitFor(() => {
        expect(onArchive).toHaveBeenCalledWith("exam-pub-2");
      });
    });

    it("opens delete confirmation with destructive button and calls onDeleteExam on confirm", async () => {
      const onDelete = vi.fn().mockResolvedValue(undefined);
      render(<ExamDashboard exams={[mockExams[0]]} onDeleteExam={onDelete} />);

      // Click Delete on card
      fireEvent.click(screen.getByRole("button", { name: /delete/i }));

      expect(screen.getByRole("heading", { name: "Delete Assessment" })).toBeInTheDocument();
      expect(screen.getByText(/Are you sure you want to delete "Midterm Biology"/i)).toBeInTheDocument();

      const confirmBtn = screen.getByRole("button", { name: /^Delete Assessment$/i });
      fireEvent.click(confirmBtn);

      await waitFor(() => {
        expect(onDelete).toHaveBeenCalledWith("exam-draft-1");
      });
    });
  });

  describe("CreateExamDialog validation and submission", () => {
    it("validates form inputs and prevents submission with invalid fields", async () => {
      const onSubmit = vi.fn();
      render(<CreateExamDialog open={true} onOpenChange={vi.fn()} onSubmit={onSubmit} />);

      const submitBtn = screen.getByRole("button", { name: /create draft/i });
      fireEvent.click(submitBtn);

      await waitFor(() => {
        expect(screen.getByText(/Title must be at least 3 characters/i)).toBeInTheDocument();
        expect(screen.getByText(/Subject must be at least 2 characters/i)).toBeInTheDocument();
      });
      expect(onSubmit).not.toHaveBeenCalled();
    });

    it("submits valid exam payload and handles successful creation", async () => {
      const onSubmit = vi.fn().mockResolvedValue(mockExams[0]);
      const onOpenChange = vi.fn();
      render(<CreateExamDialog open={true} onOpenChange={onOpenChange} onSubmit={onSubmit} />);

      fireEvent.change(screen.getByLabelText(/exam title/i), {
        target: { value: "Physics Semester 1" },
      });
      fireEvent.change(screen.getByLabelText(/subject/i), {
        target: { value: "Physics" },
      });
      fireEvent.change(screen.getByLabelText(/duration/i), {
        target: { value: "75" },
      });
      fireEvent.change(screen.getByLabelText(/description/i), {
        target: { value: "Mechanics and thermodynamics" },
      });

      const submitBtn = screen.getByRole("button", { name: /create draft/i });
      fireEvent.click(submitBtn);

      await waitFor(() => {
        expect(onSubmit).toHaveBeenCalledWith({
          title: "Physics Semester 1",
          subject: "Physics",
          description: "Mechanics and thermodynamics",
          durationMins: 75,
        });
      });
    });

    it("displays backend ApiError message if creation fails", async () => {
      const onSubmit = vi.fn().mockRejectedValue(new ApiError("Duplicate exam title found", 400));
      render(<CreateExamDialog open={true} onOpenChange={vi.fn()} onSubmit={onSubmit} />);

      fireEvent.change(screen.getByLabelText(/exam title/i), {
        target: { value: "Physics Semester 1" },
      });
      fireEvent.change(screen.getByLabelText(/subject/i), {
        target: { value: "Physics" },
      });
      fireEvent.change(screen.getByLabelText(/duration/i), {
        target: { value: "75" },
      });

      const submitBtn = screen.getByRole("button", { name: /create draft/i });
      fireEvent.click(submitBtn);

      await waitFor(() => {
        expect(screen.getByText("Duplicate exam title found")).toBeInTheDocument();
      });
    });
  });

  describe("EditExamDialog lifecycle awareness", () => {
    it("allows editing all fields including duration for DRAFT exams", () => {
      render(
        <EditExamDialog
          exam={mockExams[0]}
          open={true}
          onOpenChange={vi.fn()}
          onSubmit={vi.fn()}
        />
      );

      const durationInput = screen.getByLabelText(/duration/i);
      expect(durationInput).not.toBeDisabled();
    });

    it("locks duration and displays structural freeze warning for PUBLISHED exams", () => {
      render(
        <EditExamDialog
          exam={mockExams[1]}
          open={true}
          onOpenChange={vi.fn()}
          onSubmit={vi.fn()}
        />
      );

      const durationInput = screen.getByLabelText(/duration/i);
      expect(durationInput).toBeDisabled();
      expect(
        screen.getByText(/Duration is locked to safeguard running and scheduled exam attempts/i)
      ).toBeInTheDocument();
    });

    it("disables all inputs and hides save button for ARCHIVED exams", () => {
      render(
        <EditExamDialog
          exam={mockExams[2]}
          open={true}
          onOpenChange={vi.fn()}
          onSubmit={vi.fn()}
        />
      );

      expect(
        screen.getByText(/This assessment is archived and permanently immutable/i)
      ).toBeInTheDocument();
      expect(screen.getByLabelText(/exam title/i)).toBeDisabled();
      expect(screen.getByLabelText(/subject/i)).toBeDisabled();
      expect(screen.getByLabelText(/duration/i)).toBeDisabled();
      expect(screen.getByLabelText(/description/i)).toBeDisabled();
      expect(screen.queryByRole("button", { name: /save changes/i })).not.toBeInTheDocument();
      expect(screen.getAllByRole("button", { name: /close/i })[0]).toBeInTheDocument();
    });
  });

  describe("Dashboard Search, Filter, and Pagination", () => {
    it("filters exams by status", async () => {
      render(<ExamDashboard exams={mockExams} />);

      // Verify all 3 initial exams are rendered
      expect(screen.getByText("Midterm Biology")).toBeInTheDocument();
      expect(screen.getByText("Final Chemistry")).toBeInTheDocument();
      expect(screen.getByText("Old History Quiz")).toBeInTheDocument();
    });

    it("paginates when exam list exceeds page size", () => {
      const manyExams: Exam[] = Array.from({ length: 15 }, (_, i) => ({
        id: `exam-${i}`,
        title: `Assessment Number ${i + 1}`,
        subject: "Mathematics",
        description: `Description for exam ${i + 1}`,
        durationMins: 45,
        status: "DRAFT" as const,
        createdBy: "teacher-1",
        createdAt: "2026-03-01T10:00:00Z",
        updatedAt: "2026-03-01T10:00:00Z",
      }));

      render(<ExamDashboard exams={manyExams} />);

      // Page size is 6, so page 1 shows items 1 through 6
      expect(screen.getByText("Assessment Number 1")).toBeInTheDocument();
      expect(screen.getByText("Assessment Number 6")).toBeInTheDocument();
      expect(screen.queryByText("Assessment Number 7")).not.toBeInTheDocument();

      // Click Next Page
      const nextBtn = screen.getByRole("button", { name: /next page/i });
      fireEvent.click(nextBtn);

      // Now page 2 items should appear
      expect(screen.getByText("Assessment Number 7")).toBeInTheDocument();
    });
  });

  describe("Primary Teacher Flow (E2E Integration)", () => {
    it("completes full cycle: Dashboard -> Create Exam -> Save Draft -> Publish", async () => {
      // 1. Authenticate Teacher
      useAuthStore.setState({
        user: {
          id: "teacher-1",
          email: "educator@institution.edu",
          role: "TEACHER",
          firstName: "Evelyn",
          lastName: "Reed",
        },
        accessToken: "mock-token",
        isAuthenticated: true,
        isLoading: false,
      });

      let currentExams: Exam[] = [];

      const handleCreate = vi.fn().mockImplementation(async (payload) => {
        const created: Exam = {
          id: "exam-new-999",
          title: payload.title,
          subject: payload.subject,
          description: payload.description,
          durationMins: payload.durationMins,
          status: "DRAFT",
          createdBy: "teacher-1",
          createdAt: new Date().toISOString(),
          updatedAt: new Date().toISOString(),
        };
        currentExams = [created, ...currentExams];
        return created;
      });

      const handlePublish = vi.fn().mockImplementation(async (id) => {
        const target = currentExams.find((e) => e.id === id);
        if (!target) throw new Error("Not found");
        const updated = {
          ...target,
          status: "PUBLISHED" as const,
          publishedAt: new Date().toISOString(),
        };
        currentExams = currentExams.map((e) => (e.id === id ? updated : e));
        return updated;
      });

      const { rerender } = render(
        <ExamDashboard
          exams={currentExams}
          onCreateExam={handleCreate}
          onPublishExam={handlePublish}
        />
      );

      // Initial empty state
      expect(screen.getByText("No assessments found")).toBeInTheDocument();

      // 2. Open Create Exam Dialog
      const createBtn = screen.getByRole("button", { name: /create new exam/i });
      fireEvent.click(createBtn);

      expect(screen.getByRole("heading", { name: "Create New Assessment" })).toBeInTheDocument();

      // 3. Fill in form fields
      fireEvent.change(screen.getByLabelText(/exam title/i), {
        target: { value: "Advanced Neurobiology Midterm" },
      });
      fireEvent.change(screen.getByLabelText(/subject/i), {
        target: { value: "Neuroscience" },
      });
      fireEvent.change(screen.getByLabelText(/duration/i), {
        target: { value: "90" },
      });
      fireEvent.change(screen.getByLabelText(/description/i), {
        target: { value: "Comprehensive synaptic transmission exam." },
      });

      // 4. Save Draft
      const submitBtn = screen.getByRole("button", { name: /create draft/i });
      fireEvent.click(submitBtn);

      await waitFor(() => {
        expect(handleCreate).toHaveBeenCalledWith({
          title: "Advanced Neurobiology Midterm",
          subject: "Neuroscience",
          durationMins: 90,
          description: "Comprehensive synaptic transmission exam.",
        });
      });

      // Re-render dashboard with newly drafted exam
      rerender(
        <ExamDashboard
          exams={currentExams}
          onCreateExam={handleCreate}
          onPublishExam={handlePublish}
        />
      );

      // Verify exam is drafted with DRAFT badge and action buttons
      expect(screen.getByText("Advanced Neurobiology Midterm")).toBeInTheDocument();
      expect(screen.getByText("DRAFT")).toBeInTheDocument();
      const publishBtn = screen.getByRole("button", { name: /publish/i });
      expect(publishBtn).toBeInTheDocument();

      // 5. Trigger Publish
      fireEvent.click(publishBtn);

      // Verify Confirmation Dialog
      expect(screen.getByRole("heading", { name: "Publish Assessment" })).toBeInTheDocument();
      expect(
        screen.getByText(/Are you sure you want to publish "Advanced Neurobiology Midterm"/i)
      ).toBeInTheDocument();

      // Confirm Publication
      const confirmPublishBtn = screen.getByRole("button", { name: /^Publish Assessment$/i });
      fireEvent.click(confirmPublishBtn);

      await waitFor(() => {
        expect(handlePublish).toHaveBeenCalledWith("exam-new-999");
      });

      // Re-render with published exam
      rerender(
        <ExamDashboard
          exams={currentExams}
          onCreateExam={handleCreate}
          onPublishExam={handlePublish}
        />
      );

      // Verify status is now PUBLISHED and appropriate actions shown
      expect(screen.getByText("PUBLISHED")).toBeInTheDocument();
      expect(screen.getByRole("button", { name: /revert to draft/i })).toBeInTheDocument();
      expect(screen.getByRole("button", { name: /archive/i })).toBeInTheDocument();
      expect(screen.queryByRole("button", { name: /^publish$/i })).not.toBeInTheDocument();
    });
  });
});
