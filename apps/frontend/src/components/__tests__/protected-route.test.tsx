import { describe, it, expect, vi, beforeEach } from "vitest";
import { render, screen } from "@testing-library/react";
import { ProtectedRoute } from "../auth/protected-route";
import { useAuthStore } from "@/store/auth-store";

const mockPush = vi.fn();
const mockReplace = vi.fn();

vi.mock("next/navigation", () => ({
  useRouter: () => ({
    push: mockPush,
    replace: mockReplace,
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
      status: "idle",
      isLoading: false,
      sessionExpired: false,
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

  it("shows skeleton while auth state is restoring session", () => {
    useAuthStore.setState({ isHydrated: true, status: "restoring" });

    render(
      <ProtectedRoute>
        <div>Secret Content</div>
      </ProtectedRoute>
    );

    expect(screen.queryByText("Secret Content")).not.toBeInTheDocument();
  });

  it("prompts for authentication and redirects when user is not logged in", () => {
    useAuthStore.setState({ isHydrated: true, isAuthenticated: false, status: "unauthenticated" });

    render(
      <ProtectedRoute>
        <div>Secret Content</div>
      </ProtectedRoute>
    );

    expect(screen.getByText("Authentication Required")).toBeInTheDocument();
    expect(screen.queryByText("Secret Content")).not.toBeInTheDocument();
    expect(mockReplace).toHaveBeenCalledWith("/login?returnUrl=%2Fexams");
  });

  it("blocks user with unauthorized role and shows link to their portal", () => {
    useAuthStore.setState({
      isHydrated: true,
      isAuthenticated: true,
      status: "authenticated",
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
      status: "authenticated",
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
