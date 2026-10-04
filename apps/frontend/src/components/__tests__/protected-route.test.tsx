import { describe, it, expect, vi, beforeEach } from "vitest";
import { render, screen } from "@testing-library/react";
import { ProtectedRoute } from "../auth/protected-route";
import { useAuthStore } from "@/store/auth-store";

const mockPush = vi.fn();
vi.mock("next/navigation", () => ({
  useRouter: () => ({
    push: mockPush,
    replace: vi.fn(),
  }),
  usePathname: () => "/exams",
}));

describe("ProtectedRoute", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    useAuthStore.setState({
      user: null,
      accessToken: null,
      isAuthenticated: false,
      isHydrated: false,
      isLoading: false,
      error: null,
    });
  });

  it("shows skeleton while auth state is hydrating", () => {
    useAuthStore.setState({ isHydrated: false });

    render(
      <ProtectedRoute>
        <div>Secret Content</div>
      </ProtectedRoute>
    );

    expect(screen.queryByText("Secret Content")).not.toBeInTheDocument();
  });

  it("prompts for authentication when user is not logged in", () => {
    useAuthStore.setState({ isHydrated: true, isAuthenticated: false });

    render(
      <ProtectedRoute>
        <div>Secret Content</div>
      </ProtectedRoute>
    );

    expect(screen.getByText("Authentication Required")).toBeInTheDocument();
    expect(screen.queryByText("Secret Content")).not.toBeInTheDocument();
  });

  it("blocks user with unauthorized role and shows link to their portal", () => {
    useAuthStore.setState({
      isHydrated: true,
      isAuthenticated: true,
      user: {
        id: "student-1",
        email: "cand@test.com",
        firstName: "Sam",
        lastName: "Smith",
        role: "STUDENT",
      },
    });

    render(
      <ProtectedRoute allowedRoles={["TEACHER", "ADMIN"]}>
        <div>Teacher Portal Content</div>
      </ProtectedRoute>
    );

    expect(screen.getByText("Access Restricted")).toBeInTheDocument();
    expect(screen.queryByText("Teacher Portal Content")).not.toBeInTheDocument();
    expect(screen.getByRole("button", { name: /go to your portal/i })).toBeInTheDocument();
  });

  it("renders protected content when user has authorized role", () => {
    useAuthStore.setState({
      isHydrated: true,
      isAuthenticated: true,
      user: {
        id: "teacher-1",
        email: "prof@test.com",
        firstName: "Ada",
        lastName: "Lovelace",
        role: "TEACHER",
      },
    });

    render(
      <ProtectedRoute allowedRoles={["TEACHER", "ADMIN"]}>
        <div>Teacher Portal Content</div>
      </ProtectedRoute>
    );

    expect(screen.getByText("Teacher Portal Content")).toBeInTheDocument();
  });
});
