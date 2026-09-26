"use client";

import { useState } from "react";
import { useRouter } from "next/navigation";
import Link from "next/link";
import { AuthForms, type AuthRole } from "@/components/auth-forms";
import { authApi, ApiError } from "@/services/api";

export default function LoginPage() {
  const router = useRouter();
  const [feedback, setFeedback] = useState<{ message: string; type: "success" | "error" } | null>(
    null
  );

  const handleSubmit = async (data: {
    mode: "login" | "register";
    email: string;
    password: string;
    role: AuthRole;
    firstName?: string;
  }) => {
    setFeedback(null);
    try {
      if (data.mode === "register") {
        await authApi.register({
          email: data.email,
          password: data.password,
          firstName: data.firstName || "User",
          lastName: "Candidate",
          role: data.role,
        });
        setFeedback({
          message: "Account registered successfully! Logging you in...",
          type: "success",
        });
      }

      // Perform login to retrieve JWT and set refresh cookie
      const loginRes = await authApi.login({
        email: data.email,
        password: data.password,
      });

      setFeedback({
        message: "Welcome back! Redirecting to your workspace...",
        type: "success",
      });

      const role = loginRes.user.role || data.role;
      setTimeout(() => {
        if (role === "TEACHER" || role === "ADMIN") {
          router.push("/exams");
        } else {
          router.push("/sessions");
        }
      }, 500);
    } catch (err) {
      const msg =
        err instanceof ApiError ? err.message : "Authentication failed. Please check credentials.";
      setFeedback({ message: msg, type: "error" });
    }
  };

  return (
    <div className="flex min-h-screen flex-col items-center justify-center p-4 bg-background">
      <div className="w-full max-w-md mb-6 text-center">
        <Link href="/" className="inline-flex items-center gap-2 mb-2">
          <div className="size-8 rounded-lg bg-primary text-primary-foreground flex items-center justify-center font-bold text-sm">
            AS
          </div>
          <span className="font-bold text-xl tracking-tight">AI Exam Scribe</span>
        </Link>
      </div>

      {feedback && (
        <div
          role="alert"
          className={`mb-4 w-full max-w-md rounded-lg p-3 text-sm font-medium ${
            feedback.type === "success"
              ? "bg-emerald-100 text-emerald-800 dark:bg-emerald-950 dark:text-emerald-200"
              : "bg-destructive/10 text-destructive border border-destructive/20"
          }`}
        >
          {feedback.message}
        </div>
      )}

      <AuthForms onSubmit={handleSubmit} />
    </div>
  );
}
