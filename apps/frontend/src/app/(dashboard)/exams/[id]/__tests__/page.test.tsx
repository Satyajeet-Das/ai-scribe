import { Suspense } from "react";
import { describe, it, expect, vi, beforeEach } from "vitest";
import { render, screen } from "@testing-library/react";
import { useAuthStore } from "@/store/auth-store";
import type { Exam } from "@/types/exam-types";

vi.mock("next/navigation", () => ({
  useRouter: () => ({ push: vi.fn(), replace: vi.fn(), prefetch: vi.fn() }),
  usePathname: () => "/exams/exam-1",
}));

vi.mock("@/hooks/use-exam-documents", () => ({
  useExamDocuments: () => ({ documents: [], loading: false, error: undefined, reload: () => {} }),
}));

vi.mock("@/services/api", async (orig) => ({
  ...(await orig<typeof import("@/services/api")>()),
  examsApi: {
    getExam: vi.fn(), updateExam: vi.fn(), publishExam: vi.fn(), unpublishExam: vi.fn(), archiveExam: vi.fn(), deleteExam: vi.fn(),
  },
  questionsApi: { getQuestions: vi.fn() },
  assignmentsApi: { getExamAssignments: vi.fn() },
}));

import { examsApi, questionsApi, assignmentsApi } from "@/services/api";
import ExamDetailPage from "../page";

function exam(status: Exam["status"]): Exam {
  return {
    id: "exam-1", title: "Physics Midterm", subject: "Physics", description: "Forces and motion", durationMins: 60, status,
    createdBy: "t1", createdAt: "2026-01-01T00:00:00Z", updatedAt: "2026-01-01T00:00:00Z",
  };
}

// React reads a promise that already carries its result without suspending.
const resolvedParams = Object.assign(Promise.resolve({ id: "exam-1" }), { status: "fulfilled", value: { id: "exam-1" } });

async function show(status: Exam["status"]) {
  vi.mocked(examsApi.getExam).mockResolvedValue(exam(status));
  vi.mocked(questionsApi.getQuestions).mockResolvedValue([]);
  vi.mocked(assignmentsApi.getExamAssignments).mockResolvedValue([]);
  render(
    <Suspense fallback={<p>suspended</p>}>
      <ExamDetailPage params={resolvedParams} />
    </Suspense>
  );
  return screen.findByRole("heading", { level: 1, name: "Physics Midterm" });
}

describe("exam detail page with PDF import", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    useAuthStore.setState({
      user: { id: "t1", email: "t@school.edu", role: "TEACHER", firstName: "Terry", lastName: "Teacher" },
      isAuthenticated: true,
      isHydrated: true,
      status: "authenticated",
    });
  });

  it("offers PDF import on a draft exam, ahead of the question studio, without disturbing the rest", async () => {
    await show("DRAFT");
    const importer = screen.getByRole("region", { name: /import questions from a pdf/i });
    expect(screen.getByLabelText(/exam paper \(pdf\)/i)).toBeEnabled();
    // The existing page is all still there.
    expect(screen.getByRole("button", { name: /^publish$/i })).toBeInTheDocument();
    expect(screen.getByRole("button", { name: /edit exam/i })).toBeInTheDocument();
    expect(screen.getByText(/forces and motion/i)).toBeInTheDocument();
    // And the import comes before the question studio in reading order.
    const studio = screen.getByText(/question studio/i);
    expect(importer.compareDocumentPosition(studio) & Node.DOCUMENT_POSITION_FOLLOWING).toBeTruthy();
  });

  it("does not offer uploading on a published exam, and says why", async () => {
    await show("PUBLISHED");
    expect(screen.getByLabelText(/exam paper \(pdf\)/i)).toBeDisabled();
    expect(screen.getByText(/only be uploaded while the exam is a draft/i)).toBeInTheDocument();
    expect(screen.getByRole("button", { name: /revert to draft/i })).toBeInTheDocument();
  });
});
