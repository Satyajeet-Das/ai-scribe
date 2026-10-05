"use client";

import { useState, useEffect, Suspense } from "react";
import { useRouter, useSearchParams } from "next/navigation";
import Link from "next/link";
import { ArrowLeft } from "lucide-react";
import { AuthForms, type AuthSubmitData } from "@/components/auth-forms";
import { useAuthStore } from "@/store/auth-store";
import { APP_CONFIG, resolvePostAuthRedirect } from "@/lib/constants";

/** Only allow same-site relative paths to prevent open redirects. */
function getSafeReturnUrl(url: string | null): string | null {
  if (!url) return null;
  if (!url.startsWith("/") || url.startsWith("//") || url.startsWith("/\\")) return null;
  return url;
}

function getErrorMessage(err: unknown): string {
  const msg = err instanceof Error ? err.message : "";
  if (/failed to fetch|network|connection refused|timeout/i.test(msg)) {
    return "We couldn't reach the server. Check your connection and try again.";
  }
  if (/\b5\d{2}\b/.test(msg)) {
    return "Something went wrong on our end. Please try again in a moment.";
  }
  return msg || "Authentication failed. Please try again.";
}

function LoginContent() {
  const router = useRouter();
  const searchParams = useSearchParams();
  const returnUrl = getSafeReturnUrl(searchParams.get("returnUrl"));
  const initialMode = searchParams.get("mode") === "register" ? "register" : "login";
  const urlSessionExpired = searchParams.get("session_expired") === "true";

  const {
    user,
    isAuthenticated,
    isHydrated,
    login,
    register,
    isLoading,
    sessionExpired,
    clearSessionExpired,
  } = useAuthStore();

  const [errorMessage, setErrorMessage] = useState<string | null>(null);
  const [isRedirecting, setIsRedirecting] = useState(false);

  const isSessionExpiredNotice = urlSessionExpired || sessionExpired;

  // If the user is already authenticated, directly navigate to their designated portal
  useEffect(() => {
    if (isHydrated && isAuthenticated && user) {
      const destination = resolvePostAuthRedirect(user.role, returnUrl);
      router.replace(destination);
    }
  }, [isHydrated, isAuthenticated, user, returnUrl, router]);

  const handleSubmit = async (data: AuthSubmitData) => {
    setErrorMessage(null);
    clearSessionExpired();

    try {
      let authedUser = null;
      if (data.mode === "register") {
        authedUser = await register({
          email: data.email,
          password: data.password,
          firstName: data.firstName,
          lastName: data.lastName,
          role: data.role,
          rollNo: data.rollNo,
        });
      } else {
        authedUser = await login({ email: data.email, password: data.password });
      }

      // Keep the form disabled until navigation completes (prevents double submit)
      setIsRedirecting(true);

      // Determine respective portal for teacher or candidate
      const effectiveRole = authedUser?.role ?? useAuthStore.getState().user?.role;
      const destination = resolvePostAuthRedirect(effectiveRole, returnUrl);
      router.replace(destination);
    } catch (err: unknown) {
      setIsRedirecting(false);
      setErrorMessage(getErrorMessage(err));
    }
  };

  // Prevent flashing login form if user is already authenticated
  if (isHydrated && isAuthenticated && user) {
    return (
      <div className="flex min-h-screen items-center justify-center">
        <div
          role="status"
          aria-label="Redirecting to your portal"
          className="size-8 animate-spin rounded-full border-4 border-primary border-t-transparent"
        />
      </div>
    );
  }

  return (
    <main className="flex min-h-screen flex-col items-center justify-center bg-background px-4 py-8 sm:px-6">
      <div className="w-full max-w-md">
        {/* Top navigation */}
        <div className="mb-6 flex items-center justify-between">
          <Link
            href="/"
            className="inline-flex items-center gap-1.5 rounded-md px-2 py-1.5 text-sm font-medium text-muted-foreground transition-colors hover:text-foreground focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring"
          >
            <ArrowLeft className="size-4" aria-hidden="true" />
            Home
          </Link>
          <Link
            href="/"
            className="flex items-center gap-2 rounded-md focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring"
          >
            <span
              aria-hidden="true"
              className="flex size-8 items-center justify-center rounded-lg bg-primary text-xs font-bold text-primary-foreground"
            >
              AS
            </span>
            <span className="text-base font-bold tracking-tight">{APP_CONFIG.name}</span>
          </Link>
        </div>

        <AuthForms
          initialMode={initialMode}
          onSubmit={handleSubmit}
          onModeChange={() => {
            setErrorMessage(null);
            clearSessionExpired();
          }}
          isLoading={isLoading || isRedirecting}
          errorMessage={errorMessage}
          sessionExpired={isSessionExpiredNotice}
        />

        <p className="mt-8 text-center text-xs text-muted-foreground">
          Voice-first, keyboard-accessible exam administration.
        </p>
      </div>
    </main>
  );
}

export default function LoginPage() {
  return (
    <Suspense
      fallback={
        <div className="flex min-h-screen items-center justify-center">
          <div
            role="status"
            aria-label="Loading"
            className="size-8 animate-spin rounded-full border-4 border-primary border-t-transparent"
          />
        </div>
      }
    >
      <LoginContent />
    </Suspense>
  );
}