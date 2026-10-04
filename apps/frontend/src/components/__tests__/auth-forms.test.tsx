import { describe, it, expect, vi } from "vitest";
import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { AuthForms } from "../auth-forms";

describe("AuthForms", () => {
  it("renders login form by default", () => {
    render(<AuthForms onSubmit={vi.fn()} />);

    expect(screen.getByText("Welcome back")).toBeInTheDocument();
    expect(screen.getByLabelText(/^email$/i)).toBeInTheDocument();
    expect(screen.getByLabelText(/^password$/i)).toBeInTheDocument();
    expect(screen.getByRole("button", { name: /^sign in$/i })).toBeInTheDocument();
  });

  it("switches to registration mode and displays Candidate/Teacher role selectors", async () => {
    const user = userEvent.setup();
    render(<AuthForms initialMode="login" onSubmit={vi.fn()} />);

    // Click register tab to switch mode
    const registerTab = screen.getByRole("tab", { name: /register/i });
    await user.click(registerTab);

    expect(screen.getByText("Create your account")).toBeInTheDocument();
    expect(screen.getByRole("button", { name: /^candidate$/i })).toBeInTheDocument();
    expect(screen.getByRole("button", { name: /^teacher$/i })).toBeInTheDocument();
    expect(screen.getByLabelText(/first name/i)).toBeInTheDocument();
    expect(screen.getByLabelText(/last name/i)).toBeInTheDocument();
    expect(screen.getByRole("button", { name: /^create account$/i })).toBeInTheDocument();
  });

  it("submits valid credentials when login button is clicked", async () => {
    const user = userEvent.setup();
    const handleSubmit = vi.fn();

    render(<AuthForms initialMode="login" onSubmit={handleSubmit} />);

    await user.type(screen.getByLabelText(/^email$/i), "user@example.com");
    await user.type(screen.getByLabelText(/^password$/i), "ValidPassword123");
    await user.click(screen.getByRole("button", { name: /^sign in$/i }));

    expect(handleSubmit).toHaveBeenCalledWith({
      mode: "login",
      email: "user@example.com",
      password: "ValidPassword123",
    });
  });

  it("shows error message banner when provided", () => {
    render(
      <AuthForms
        initialMode="login"
        onSubmit={vi.fn()}
        errorMessage="Invalid username or password"
      />
    );

    expect(screen.getByRole("alert")).toBeInTheDocument();
    expect(screen.getByText("Invalid username or password")).toBeInTheDocument();
  });
});
