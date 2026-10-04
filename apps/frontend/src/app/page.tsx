"use client";

import { useEffect } from "react";
import { useRouter } from "next/navigation";
import Link from "next/link";
import { useAuthStore } from "@/store/auth-store";
import { getRolePortal } from "@/lib/constants";
import {
  BookOpen,
  CheckCircle2,
  GraduationCap,
  Headphones,
  Shield,
  Sparkles,
} from "lucide-react";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";
import { Badge } from "@/components/ui/badge";

export default function HomePage() {
  const router = useRouter();
  const { user, isAuthenticated, isHydrated } = useAuthStore();

  // If user is logged in as teacher or candidate, directly redirect them to their respective portal
  useEffect(() => {
    if (isHydrated && isAuthenticated && user) {
      router.replace(getRolePortal(user.role));
    }
  }, [isHydrated, isAuthenticated, user, router]);

  // Prevent flash of landing page while redirecting authenticated user
  if (isHydrated && isAuthenticated && user) {
    return (
      <div className="flex min-h-screen items-center justify-center bg-background">
        <div
          role="status"
          aria-label="Redirecting to your portal"
          className="size-8 animate-spin rounded-full border-4 border-primary border-t-transparent"
        />
      </div>
    );
  }

  return (
    <div className="flex min-h-screen flex-col bg-background text-foreground">
      {/* Navigation Header */}
      <header className="sticky top-0 z-40 w-full border-b border-border bg-background/80 backdrop-blur-md">
        <div className="mx-auto flex h-16 max-w-6xl items-center justify-between px-6">
          <Link href="/" className="flex items-center gap-2.5">
            <div className="flex size-9 items-center justify-center rounded-xl bg-primary text-primary-foreground font-bold text-sm shadow-sm">
              AS
            </div>
            <div>
              <span className="font-bold text-lg tracking-tight">AI Exam Scribe</span>
              <span className="ml-2 text-[10px] font-semibold uppercase tracking-wider rounded bg-primary/10 text-primary px-1.5 py-0.5">
                Sprint 1
              </span>
            </div>
          </Link>

          <nav className="flex items-center gap-4">
            <Link
              href="/exams"
              className="text-sm font-medium text-muted-foreground hover:text-foreground transition-colors hidden sm:block"
            >
              Educator Portal
            </Link>
            <Link
              href="/sessions"
              className="text-sm font-medium text-muted-foreground hover:text-foreground transition-colors hidden sm:block"
            >
              Candidate Portal
            </Link>
            <Link href="/login">
              <Button size="sm">Sign In / Register</Button>
            </Link>
          </nav>
        </div>
      </header>

      {/* Hero Section */}
      <main className="flex-1">
        <section className="relative overflow-hidden py-20 px-6 md:py-32">
          <div className="mx-auto max-w-5xl text-center">
            <Badge
              variant="outline"
              className="mb-6 py-1 px-3 text-xs tracking-wider uppercase font-semibold border-primary/30 bg-primary/5"
            >
              <Sparkles className="size-3.5 mr-1.5 text-primary" />
              Accessible Examination Platform
            </Badge>

            <h1 className="text-balance text-4xl font-extrabold tracking-tight sm:text-6xl md:text-7xl">
              Accessible assessments for{" "}
              <span className="text-primary underline decoration-primary/30 decoration-wavy">
                every learner
              </span>
            </h1>

            <p className="mx-auto mt-6 max-w-2xl text-lg text-muted-foreground leading-relaxed sm:text-xl">
              An accessible, AI-powered examination engine built for visually impaired candidates
              and educators. Featuring real-time timing, voice-compatible response paths, and
              backend-authoritative security.
            </p>

            <div className="mt-10 flex flex-wrap items-center justify-center gap-4">
              <Link href="/exams">
                <Button size="lg" className="font-semibold shadow-md px-6">
                  <GraduationCap className="size-5 mr-2" />
                  Enter Educator Workspace
                </Button>
              </Link>
              <Link href="/sessions">
                <Button size="lg" variant="outline" className="font-semibold px-6">
                  <Headphones className="size-5 mr-2" />
                  Candidate Exam Room
                </Button>
              </Link>
            </div>
          </div>
        </section>

        {/* Feature Grid */}
        <section className="border-t border-border bg-muted/30 py-20 px-6">
          <div className="mx-auto max-w-6xl">
            <div className="mb-12 text-center">
              <h2 className="text-3xl font-bold tracking-tight">
                Core Architecture & Sprint 1 Capabilities
              </h2>
              <p className="mt-2 text-muted-foreground">
                Engineered with Go, Echo, PostgreSQL, and modern accessible Next.js components.
              </p>
            </div>

            <div className="grid gap-6 md:grid-cols-3">
              <Card className="border border-border bg-card">
                <CardHeader>
                  <div className="size-10 rounded-lg bg-primary/10 flex items-center justify-center mb-2">
                    <BookOpen className="size-5 text-primary" />
                  </div>
                  <CardTitle className="text-xl">Exam Authoring</CardTitle>
                  <CardDescription>
                    Educators can draft, publish, and archive exams with configurable duration,
                    point values, and subjects.
                  </CardDescription>
                </CardHeader>
                <CardContent>
                  <ul className="space-y-2 text-sm text-muted-foreground">
                    <li className="flex items-center gap-2">
                      <CheckCircle2 className="size-4 text-emerald-600 dark:text-emerald-400" />
                      Status lifecycle (Draft, Published, Archived)
                    </li>
                    <li className="flex items-center gap-2">
                      <CheckCircle2 className="size-4 text-emerald-600 dark:text-emerald-400" />
                      Dynamic question management
                    </li>
                    <li className="flex items-center gap-2">
                      <CheckCircle2 className="size-4 text-emerald-600 dark:text-emerald-400" />
                      Multi-choice, Essay, and Voice prompts
                    </li>
                  </ul>
                </CardContent>
              </Card>

              <Card className="border border-border bg-card">
                <CardHeader>
                  <div className="size-10 rounded-lg bg-primary/10 flex items-center justify-center mb-2">
                    <Headphones className="size-5 text-primary" />
                  </div>
                  <CardTitle className="text-xl">Accessible Candidate Room</CardTitle>
                  <CardDescription>
                    High contrast toggles, larger typography modes, real-time countdown, and
                    keyboard-first accessibility.
                  </CardDescription>
                </CardHeader>
                <CardContent>
                  <ul className="space-y-2 text-sm text-muted-foreground">
                    <li className="flex items-center gap-2">
                      <CheckCircle2 className="size-4 text-emerald-600 dark:text-emerald-400" />
                      High-contrast & large text toggles
                    </li>
                    <li className="flex items-center gap-2">
                      <CheckCircle2 className="size-4 text-emerald-600 dark:text-emerald-400" />
                      Debounced autosave indicators
                    </li>
                    <li className="flex items-center gap-2">
                      <CheckCircle2 className="size-4 text-emerald-600 dark:text-emerald-400" />
                      Audited submission verification
                    </li>
                  </ul>
                </CardContent>
              </Card>

              <Card className="border border-border bg-card">
                <CardHeader>
                  <div className="size-10 rounded-lg bg-primary/10 flex items-center justify-center mb-2">
                    <Shield className="size-5 text-primary" />
                  </div>
                  <CardTitle className="text-xl">Secure Architecture</CardTitle>
                  <CardDescription>
                    Pluggable JWT provider with refresh token rotation, replay attack detection, and
                    rate limiting.
                  </CardDescription>
                </CardHeader>
                <CardContent>
                  <ul className="space-y-2 text-sm text-muted-foreground">
                    <li className="flex items-center gap-2">
                      <CheckCircle2 className="size-4 text-emerald-600 dark:text-emerald-400" />
                      Short-lived access tokens (15m)
                    </li>
                    <li className="flex items-center gap-2">
                      <CheckCircle2 className="size-4 text-emerald-600 dark:text-emerald-400" />
                      HttpOnly cookie refresh tokens
                    </li>
                    <li className="flex items-center gap-2">
                      <CheckCircle2 className="size-4 text-emerald-600 dark:text-emerald-400" />
                      Backend-enforced role authorization
                    </li>
                  </ul>
                </CardContent>
              </Card>
            </div>
          </div>
        </section>
      </main>

      {/* Footer */}
      <footer className="border-t border-border py-8 px-6 text-center text-sm text-muted-foreground">
        <p>© 2026 AI Exam Scribe · Accessible Examination Platform. All rights reserved.</p>
      </footer>
    </div>
  );
}
