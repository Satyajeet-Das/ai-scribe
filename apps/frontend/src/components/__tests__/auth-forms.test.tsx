import { describe, it, expect, vi } from "vitest";
import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { AuthForms } from "../auth-forms";

describe("AuthForms", () => {
  it("renders login form by default with required red stars on fields", () => {
    render(<AuthForms onSubmit={vi.fn()} />);

    expect(screen.getByText("Welcome back")).toBeInTheDocument();
    expect(screen.getByLabelText(/email/i, { selector: "input" })).toBeInTheDocument();
    expect(screen.getByLabelText(/^password/i, { selector: "input" })).toBeInTheDocument();
    expect(screen.getByRole("button", { name: /^sign in$/i })).toBeInTheDocument();

    // Verify required visual indicators (red stars)
    const emailLabel = screen.getByText(/^email/i).closest("label");
    expect(emailLabel).toHaveTextContent("*");
    const passwordLabel = screen.getByText(/^password/i).closest("label");
    expect(passwordLabel).toHaveTextContent("*");

    // Inputs have aria-required
    expect(screen.getByLabelText(/email/i, { selector: "input" })).toHaveAttribute("aria-required", "true");
    expect(screen.getByLabelText(/^password/i, { selector: "input" })).toHaveAttribute("aria-required", "true");
  });

  it("switches to registration mode and displays Candidate/Teacher role selectors with required indicators", async () => {
    const user = userEvent.setup();
    render(<AuthForms initialMode="login" onSubmit={vi.fn()} />);

    // Click register tab to switch mode
    const registerTab = screen.getByRole("tab", { name: /register/i });
    await user.click(registerTab);

    expect(screen.getByText("Create your account")).toBeInTheDocument();
    expect(screen.getByRole("button", { name: /^candidate$/i })).toBeInTheDocument();
    expect(screen.getByRole("button", { name: /^teacher$/i })).toBeInTheDocument();
    expect(screen.getByLabelText(/first name/i, { selector: "input" })).toBeInTheDocument();
    expect(screen.getByLabelText(/last name/i, { selector: "input" })).toBeInTheDocument();
    expect(screen.getByLabelText(/roll number/i, { selector: "input" })).toBeInTheDocument();
    expect(screen.getByLabelText(/email/i, { selector: "input" })).toBeInTheDocument();
    expect(screen.getByLabelText(/^password/i, { selector: "input" })).toBeInTheDocument();
    expect(screen.getByRole("button", { name: /^create account$/i })).toBeInTheDocument();

    // Verify required red stars on all register fields for candidate
    expect(screen.getByText(/i am a/i)).toHaveTextContent("*");
    expect(screen.getByText(/first name/i).closest("label")).toHaveTextContent("*");
    expect(screen.getByText(/last name/i).closest("label")).toHaveTextContent("*");
    expect(screen.getByText(/roll number/i).closest("label")).toHaveTextContent("*");
    expect(screen.getByText(/^email/i).closest("label")).toHaveTextContent("*");
    expect(screen.getByText(/^password/i).closest("label")).toHaveTextContent("*");
  });

  it("shows validation errors when submitting empty login form", async () => {
    const user = userEvent.setup();
    const handleSubmit = vi.fn();

    render(<AuthForms initialMode="login" onSubmit={handleSubmit} />);

    await user.click(screen.getByRole("button", { name: /^sign in$/i }));

    expect(handleSubmit).not.toHaveBeenCalled();
    expect(screen.getByText(/email is required/i)).toBeInTheDocument();
    expect(screen.getByText(/password is required/i)).toBeInTheDocument();
  });

  it("shows validation error on blur for invalid email", async () => {
    const user = userEvent.setup();
    render(<AuthForms initialMode="login" onSubmit={vi.fn()} />);

    const emailInput = screen.getByLabelText(/email/i, { selector: "input" });
    await user.type(emailInput, "not-an-email");
    await user.tab(); // Triggers blur

    expect(screen.getByText(/please enter a valid email address/i)).toBeInTheDocument();
  });

  it("submits valid credentials when login button is clicked", async () => {
    const user = userEvent.setup();
    const handleSubmit = vi.fn();

    render(<AuthForms initialMode="login" onSubmit={handleSubmit} />);

    await user.type(screen.getByLabelText(/email/i, { selector: "input" }), "user@example.com");
    await user.type(screen.getByLabelText(/^password/i, { selector: "input" }), "ValidPassword123");
    await user.click(screen.getByRole("button", { name: /^sign in$/i }));

    expect(handleSubmit).toHaveBeenCalledWith({
      mode: "login",
      email: "user@example.com",
      password: "ValidPassword123",
    });
  });

  it("validates student registration requiring roll number and submits with rollNo", async () => {
    const user = userEvent.setup();
    const handleSubmit = vi.fn();

    render(<AuthForms initialMode="register" onSubmit={handleSubmit} />);

    // Try submitting empty
    await user.click(screen.getByRole("button", { name: /^create account$/i }));
    expect(handleSubmit).not.toHaveBeenCalled();
    expect(screen.getByText(/first name is required/i)).toBeInTheDocument();
    expect(screen.getByText(/last name is required/i)).toBeInTheDocument();
    expect(screen.getByText(/roll number is required for students/i)).toBeInTheDocument();
    expect(screen.getByText(/email is required/i)).toBeInTheDocument();
    expect(screen.getByText(/password is required/i)).toBeInTheDocument();

    // Fill valid student details
    await user.type(screen.getByLabelText(/first name/i, { selector: "input" }), "Ada");
    await user.type(screen.getByLabelText(/last name/i, { selector: "input" }), "Lovelace");
    await user.type(screen.getByLabelText(/roll number/i, { selector: "input" }), "23cs001");
    await user.type(screen.getByLabelText(/email/i, { selector: "input" }), "ada@school.edu");
    await user.type(screen.getByLabelText(/^password/i, { selector: "input" }), "Password123");

    await user.click(screen.getByRole("button", { name: /^create account$/i }));

    expect(handleSubmit).toHaveBeenCalledWith({
      mode: "register",
      email: "ada@school.edu",
      password: "Password123",
      firstName: "Ada",
      lastName: "Lovelace",
      role: "STUDENT",
      rollNo: "23CS001",
    });
  });

  it("hides roll number when teacher role is selected", async () => {
    const user = userEvent.setup();
    const handleSubmit = vi.fn();

    render(<AuthForms initialMode="register" onSubmit={handleSubmit} />);

    // Select Teacher role
    await user.click(screen.getByRole("button", { name: /^teacher$/i }));

    // Roll number should not be visible for teacher
    expect(screen.queryByLabelText(/roll number/i, { selector: "input" })).not.toBeInTheDocument();

    // Fill teacher details
    await user.type(screen.getByLabelText(/first name/i, { selector: "input" }), "Grace");
    await user.type(screen.getByLabelText(/last name/i, { selector: "input" }), "Hopper");
    await user.type(screen.getByLabelText(/email/i, { selector: "input" }), "grace@school.edu");
    await user.type(screen.getByLabelText(/^password/i, { selector: "input" }), "Password123");

    await user.click(screen.getByRole("button", { name: /^create account$/i }));

    expect(handleSubmit).toHaveBeenCalledWith({
      mode: "register",
      email: "grace@school.edu",
      password: "Password123",
      firstName: "Grace",
      lastName: "Hopper",
      role: "TEACHER",
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

  it("shows session expired notification when sessionExpired is true", () => {
    render(
      <AuthForms
        initialMode="login"
        onSubmit={vi.fn()}
        sessionExpired={true}
      />
    );

    expect(
      screen.getByText("Your session has expired. Please sign in again to continue.")
    ).toBeInTheDocument();
  });
});
