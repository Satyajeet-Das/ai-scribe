import { describe, it, expect, vi, beforeEach } from "vitest";
import { render, screen } from "@testing-library/react";
import UnauthorizedPage from "../unauthorized/page";
import ForbiddenPage from "../forbidden/page";
import { useAuthStore } from "@/store/auth-store";

const mockPush = vi.fn();

vi.mock("next/navigation", () => ({
  useRouter: () => ({
    push: mockPush,
    replace: vi.fn(),
  }),
  useSearchParams: () => new URLSearchParams({ returnUrl: "/exams" }),
}));

describe("Auth Error Pages", () => {
  beforeEach(() => {
    vi.clearAllMocks();
  });

  describe("UnauthorizedPage (401)", () => {
    it("renders authentication required message and sign in link preserving returnUrl", () => {
      render(<UnauthorizedPage />);

      expect(screen.getByText("Authentication Required")).toBeInTheDocument();
      expect(
        screen.getByText("You must be signed in to access this page or assessment resource.")
      ).toBeInTheDocument();

      const signInLink = screen.getByRole("link", { name: /sign in to continue/i });
      expect(signInLink).toBeInTheDocument();
      expect(signInLink).toHaveAttribute("href", "/login?returnUrl=%2Fexams");
    });
  });

  describe("ForbiddenPage (403)", () => {
    it("renders access restricted message and portal navigation link", () => {
      useAuthStore.setState({
        isHydrated: true,
        isAuthenticated: true,
        user: {
          id: "student-1",
          email: "student@test.com",
          firstName: "John",
          lastName: "Doe",
          role: "STUDENT",
        },
      });

      render(<ForbiddenPage />);

      expect(screen.getByText("Access Restricted")).toBeInTheDocument();
      expect(
        screen.getByText("Your account does not have authorization to view this section.")
      ).toBeInTheDocument();
      expect(screen.getByText("STUDENT")).toBeInTheDocument();

      const portalLink = screen.getByRole("link", { name: /go to your candidate portal/i });
      expect(portalLink).toBeInTheDocument();
      expect(portalLink).toHaveAttribute("href", "/sessions");
    });
  });
});
