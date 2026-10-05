import type { Metadata } from "next";
import Link from "next/link";
import {
  Accessibility,
  Check,
  ClipboardList,
  Contrast,
  Focus,
  Headphones,
  Keyboard,
  KeyRound,
  ListChecks,
  Mic,
  ShieldCheck,
  UserCog,
  Users,
} from "lucide-react";
import { Badge } from "@/components/ui/badge";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";
import { AuthRedirect } from "@/components/auth/auth-redirect";
import { APP_CONFIG } from "@/lib/constants";

export const metadata: Metadata = {
  title: "AI Exam Scribe: accessible exams for visually impaired students",
  description:
    "AI Exam Scribe lets visually impaired students take exams independently with a keyboard- and screen-reader-friendly exam room, while teachers keep full control of exam integrity.",
  openGraph: {
    title: "AI Exam Scribe: accessible exams for visually impaired students",
    description:
      "An accessible examination platform where students take exams independently and teachers stay in control.",
    type: "website",
  },
};

/* Link-styled buttons. Plain classes keep this file a Server Component with valid markup
   (no <button> inside <a>). Swap for buttonVariants() if your button.tsx exports it server-safely. */
const btnBase =
  "inline-flex items-center justify-center rounded-md text-sm font-semibold transition-colors focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring focus-visible:ring-offset-2 focus-visible:ring-offset-background";
const btnPrimary = `${btnBase} h-11 px-6 bg-primary text-primary-foreground shadow-sm hover:bg-primary/90`;
const btnOutline = `${btnBase} h-11 px-6 border border-border bg-background hover:bg-muted`;

const steps = [
  {
    title: "Teacher creates an exam",
    text: "Set the subject and duration, add questions, then publish when it is ready.",
  },
  {
    title: "Teacher assigns students",
    text: "Assign the exam to students by roll number. Only assigned students can open it.",
  },
  {
    title: "Student starts the exam",
    text: "The student signs in, finds their assigned exam, and begins with a running countdown.",
  },
  {
    title: "Accessible, controlled completion",
    text: "Answers autosave as the student works, and the backend verifies the final submission.",
  },
];

const capabilities = [
  {
    icon: Accessibility,
    title: "Accessible examination",
    text: "A keyboard-operable exam room with adjustable contrast and text size.",
  },
  {
    icon: ClipboardList,
    title: "Exam management",
    text: "Create exams with a subject and duration, and move them from draft to published to archived.",
  },
  {
    icon: ListChecks,
    title: "Question management",
    text: "Add and manage the questions that belong to each exam.",
  },
  {
    icon: Users,
    title: "Student assignment",
    text: "Assign exams to the right students using their roll numbers.",
  },
  {
    icon: KeyRound,
    title: "Secure authentication",
    text: "Short-lived access tokens, with refresh tokens kept in HttpOnly cookies.",
  },
  {
    icon: UserCog,
    title: "Role-based workflows",
    text: "Teachers and students each get their own portal, enforced on the server.",
  },
  {
    icon: ShieldCheck,
    title: "Exam integrity",
    text: "The backend controls exam state and verifies submissions, so the browser cannot skip steps.",
  },
];

const accessibilityFeatures = [
  {
    icon: Keyboard,
    title: "Keyboard operable",
    text: "Every control in the exam room can be reached and used without a mouse.",
  },
  {
    icon: Headphones,
    title: "Screen reader friendly",
    text: "Semantic structure and labelled controls let assistive technology announce the exam clearly.",
  },
  {
    icon: Focus,
    title: "Clear focus states",
    text: "A visible focus indicator always shows the student where they are.",
  },
  {
    icon: Contrast,
    title: "High contrast and large text",
    text: "Students can switch to high-contrast and larger-text modes while they work.",
  },
];

const audiences = [
  {
    title: "Teachers",
    points: [
      "Create and publish exams",
      "Manage questions for each exam",
      "Assign students by roll number",
    ],
  },
  {
    title: "Students",
    points: [
      "Sign in and find assigned exams",
      "Work in an accessible exam room",
      "Rely on autosave and a clear countdown",
    ],
  },
  {
    title: "Administrators",
    points: [
      "Role-based access across the platform",
      "Authorization enforced by the backend",
      "A consistent, auditable exam workflow",
    ],
  },
];

const trustPoints = [
  {
    title: "Backend-controlled workflow",
    text: "A state machine in the Go backend decides what a student can do and when. The interface only reflects it.",
  },
  {
    title: "Secure sessions",
    text: "Short-lived access tokens and rotating refresh tokens protect every request.",
  },
  {
    title: "Server-side authorization",
    text: "Role checks happen on the backend, so hiding a button is never the only protection.",
  },
  {
    title: "Reliable storage",
    text: "Exam data lives in PostgreSQL, with Redis alongside for fast, short-lived state.",
  },
];

function ExamRoomPreview() {
  const options = [
    { label: "Momentum only", selected: false },
    { label: "Kinetic energy only", selected: false },
    { label: "Both momentum and kinetic energy", selected: true },
    { label: "Neither", selected: false },
  ];

  return (
    <div
      aria-hidden="true"
      className="mx-auto w-full max-w-md rounded-xl border border-border bg-card shadow-xl motion-safe:animate-in motion-safe:fade-in motion-safe:duration-700"
    >
      <div className="flex items-center justify-between border-b border-border px-5 py-4">
        <div>
          <p className="text-xs text-muted-foreground">Physics midterm</p>
          <p className="text-sm font-semibold">Question 3 of 12</p>
        </div>
        <p className="rounded-md bg-muted px-3 py-1.5 font-mono text-sm font-semibold tabular-nums">
          42:18
        </p>
      </div>

      <div className="space-y-4 px-5 py-5">
        <p className="text-base font-medium leading-snug">
          Which quantity is conserved in an elastic collision?
        </p>
        <ul className="space-y-2">
          {options.map((o, i) => (
            <li
              key={o.label}
              className={`flex items-center gap-3 rounded-lg border px-3.5 py-3 text-sm ${
                o.selected
                  ? "border-primary bg-primary/5 ring-2 ring-primary/30"
                  : "border-border"
              }`}
            >
              <span
                className={`flex size-6 shrink-0 items-center justify-center rounded-full border text-xs font-semibold ${
                  o.selected
                    ? "border-primary bg-primary text-primary-foreground"
                    : "border-border text-muted-foreground"
                }`}
              >
                {String.fromCharCode(65 + i)}
              </span>
              {o.label}
            </li>
          ))}
        </ul>
      </div>

      <div className="flex items-center justify-between gap-3 border-t border-border bg-muted/40 px-5 py-3 text-xs text-muted-foreground">
        <span className="inline-flex items-center gap-1.5">
          <Check className="size-3.5 text-emerald-600 dark:text-emerald-400" />
          All changes saved
        </span>
        <span className="flex gap-1.5">
          <span className="rounded border border-border bg-background px-2 py-0.5">
            High contrast
          </span>
          <span className="rounded border border-border bg-background px-2 py-0.5">
            Large text
          </span>
        </span>
      </div>
    </div>
  );
}

export default function HomePage() {
  return (
    <div className="flex min-h-screen flex-col bg-background text-foreground">
      <AuthRedirect />

      <a
        href="#main"
        className="sr-only focus:not-sr-only focus:fixed focus:left-4 focus:top-4 focus:z-50 focus:rounded-md focus:bg-primary focus:px-4 focus:py-2 focus:text-sm focus:font-semibold focus:text-primary-foreground"
      >
        Skip to main content
      </a>

      {/* Header */}
      <header className="sticky top-0 z-40 w-full border-b border-border bg-background/90 backdrop-blur-md">
        <div className="mx-auto flex h-16 max-w-6xl items-center justify-between gap-6 px-6">
          <Link
            href="/"
            className="flex items-center gap-2.5 rounded-md focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring"
          >
            <span
              aria-hidden="true"
              className="flex size-9 items-center justify-center rounded-xl bg-primary text-sm font-bold text-primary-foreground shadow-sm"
            >
              AS
            </span>
            <span className="text-lg font-bold tracking-tight">{APP_CONFIG.name}</span>
          </Link>

          <nav aria-label="Primary" className="flex items-center gap-6">
            <ul className="hidden items-center gap-6 md:flex">
              {[
                ["How it works", "#how-it-works"],
                ["Capabilities", "#capabilities"],
                ["Accessibility", "#accessibility"],
              ].map(([label, href]) => (
                <li key={href}>
                  <a
                    href={href}
                    className="rounded text-sm font-medium text-muted-foreground transition-colors hover:text-foreground focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring"
                  >
                    {label}
                  </a>
                </li>
              ))}
            </ul>
            <Link href="/login" className={`${btnBase} h-9 bg-primary px-4 text-primary-foreground shadow-sm hover:bg-primary/90`}>
              Sign in
            </Link>
          </nav>
        </div>
      </header>

      <main id="main" className="flex-1">
        {/* Hero */}
        <section aria-labelledby="hero-title" className="px-6 py-16 md:py-24">
          <div className="mx-auto grid max-w-6xl items-center gap-12 lg:grid-cols-2 lg:gap-16">
            <div>
              <h1
                id="hero-title"
                className="text-balance text-4xl font-bold tracking-tight sm:text-5xl lg:text-6xl"
              >
                Exams that visually impaired students can take on their own
              </h1>
              <p className="mt-6 max-w-xl text-pretty text-lg leading-relaxed text-muted-foreground">
                Most exams assume the student can see the page, so many rely on a human reader or
                scribe. AI Exam Scribe gives students an accessible exam room and gives teachers a
                backend that keeps every exam controlled.
              </p>
              <div className="mt-8 flex flex-wrap gap-3">
                <Link href="/login?mode=register" className={btnPrimary}>
                  Get started
                </Link>
                <Link href="/login" className={btnOutline}>
                  Sign in
                </Link>
              </div>
              <p className="mt-6 text-sm text-muted-foreground">
                Voice interaction is planned for a future release.
              </p>
            </div>

            <ExamRoomPreview />
          </div>
        </section>

        {/* Problem and solution */}
        <section
          aria-labelledby="problem-title"
          className="border-t border-border bg-muted/30 px-6 py-20 md:py-24"
        >
          <div className="mx-auto max-w-6xl">
            <h2
              id="problem-title"
              className="max-w-2xl text-balance text-3xl font-bold tracking-tight"
            >
              Independence in the exam room is still the exception
            </h2>
            <div className="mt-10 grid gap-6 md:grid-cols-2">
              <div className="rounded-xl border border-border bg-background p-7">
                <h3 className="text-lg font-semibold">The problem with conventional exams</h3>
                <ul className="mt-4 space-y-3 text-muted-foreground">
                  <li>Printed papers and visual-only interfaces are hard or impossible to use.</li>
                  <li>Students depend on a reader or scribe to take part at all.</li>
                  <li>Scheduling a helper and trusting their accuracy adds stress to exam day.</li>
                </ul>
              </div>
              <div className="rounded-xl border border-primary/30 bg-primary/5 p-7">
                <h3 className="text-lg font-semibold">How AI Exam Scribe helps</h3>
                <ul className="mt-4 space-y-3 text-muted-foreground">
                  <li>
                    The exam room works with a keyboard and assistive technology, and lets students
                    adjust contrast and text size.
                  </li>
                  <li>Students sign in, open their assigned exam, and complete it themselves.</li>
                  <li>
                    The backend controls exam state and verifies submissions, so teachers can trust
                    the result.
                  </li>
                </ul>
              </div>
            </div>
          </div>
        </section>

        {/* How it works */}
        <section
          id="how-it-works"
          aria-labelledby="how-title"
          className="scroll-mt-16 px-6 py-20 md:py-24"
        >
          <div className="mx-auto max-w-6xl">
            <h2 id="how-title" className="text-3xl font-bold tracking-tight">
              How it works
            </h2>
            <ol className="mt-10 grid gap-8 sm:grid-cols-2 lg:grid-cols-4">
              {steps.map((step, i) => (
                <li key={step.title} className="border-t-2 border-primary pt-5">
                  <span className="text-sm font-semibold text-primary">Step {i + 1}</span>
                  <h3 className="mt-1 text-lg font-semibold">{step.title}</h3>
                  <p className="mt-2 text-sm leading-relaxed text-muted-foreground">{step.text}</p>
                </li>
              ))}
            </ol>
          </div>
        </section>

        {/* Capabilities */}
        <section
          id="capabilities"
          aria-labelledby="capabilities-title"
          className="scroll-mt-16 border-t border-border bg-muted/30 px-6 py-20 md:py-24"
        >
          <div className="mx-auto max-w-6xl">
            <h2 id="capabilities-title" className="text-3xl font-bold tracking-tight">
              What you can do today
            </h2>
            <ul className="mt-10 grid gap-x-10 gap-y-8 sm:grid-cols-2 lg:grid-cols-3">
              {capabilities.map(({ icon: Icon, title, text }) => (
                <li key={title} className="border-t border-border pt-5">
                  <Icon className="size-5 text-primary" aria-hidden="true" />
                  <h3 className="mt-3 font-semibold">{title}</h3>
                  <p className="mt-1.5 text-sm leading-relaxed text-muted-foreground">{text}</p>
                </li>
              ))}
            </ul>
          </div>
        </section>

        {/* Accessibility */}
        <section
          id="accessibility"
          aria-labelledby="accessibility-title"
          className="scroll-mt-16 px-6 py-20 md:py-24"
        >
          <div className="mx-auto grid max-w-6xl gap-12 lg:grid-cols-5">
            <div className="lg:col-span-2">
              <h2
                id="accessibility-title"
                className="text-balance text-3xl font-bold tracking-tight"
              >
                Accessibility is the product, not a setting
              </h2>
              <p className="mt-4 leading-relaxed text-muted-foreground">
                The exam room is designed around people who do not use a mouse or a screen the way
                most software expects.
              </p>
            </div>

            <div className="space-y-8 lg:col-span-3">
              <ul className="grid gap-6 sm:grid-cols-2">
                {accessibilityFeatures.map(({ icon: Icon, title, text }) => (
                  <li key={title} className="flex gap-4">
                    <span className="flex size-10 shrink-0 items-center justify-center rounded-lg bg-primary/10">
                      <Icon className="size-5 text-primary" aria-hidden="true" />
                    </span>
                    <div>
                      <h3 className="font-semibold">{title}</h3>
                      <p className="mt-1 text-sm leading-relaxed text-muted-foreground">{text}</p>
                    </div>
                  </li>
                ))}
              </ul>

              {/* Dashed border marks this as not yet available */}
              <div className="rounded-xl border border-dashed border-border p-6">
                <div className="flex flex-wrap items-center gap-3">
                  <Mic className="size-5 text-muted-foreground" aria-hidden="true" />
                  <h3 className="font-semibold">Voice-first exams</h3>
                  <Badge variant="outline">Planned</Badge>
                </div>
                <p className="mt-2 text-sm leading-relaxed text-muted-foreground">
                  Real-time voice interaction with speech-to-text, text-to-speech, and AI-assisted
                  accessibility is on the roadmap. It is not available yet.
                </p>
              </div>
            </div>
          </div>
        </section>

        {/* Audiences */}
        <section
          aria-labelledby="audiences-title"
          className="border-t border-border bg-muted/30 px-6 py-20 md:py-24"
        >
          <div className="mx-auto max-w-6xl">
            <h2 id="audiences-title" className="text-3xl font-bold tracking-tight">
              Built for everyone in the exam process
            </h2>
            <div className="mt-10 grid gap-6 md:grid-cols-3">
              {audiences.map((a) => (
                <Card key={a.title} className="border border-border bg-card">
                  <CardHeader>
                    <CardTitle className="text-xl">
                      <h3>{a.title}</h3>
                    </CardTitle>
                  </CardHeader>
                  <CardContent>
                    <ul className="space-y-2.5 text-sm text-muted-foreground">
                      {a.points.map((p) => (
                        <li key={p} className="flex items-start gap-2.5">
                          <Check
                            className="mt-0.5 size-4 shrink-0 text-emerald-600 dark:text-emerald-400"
                            aria-hidden="true"
                          />
                          {p}
                        </li>
                      ))}
                    </ul>
                  </CardContent>
                </Card>
              ))}
            </div>
          </div>
        </section>

        {/* Trust and architecture */}
        <section aria-labelledby="trust-title" className="px-6 py-20 md:py-24">
          <div className="mx-auto grid max-w-6xl gap-12 lg:grid-cols-5">
            <div className="lg:col-span-2">
              <h2 id="trust-title" className="text-balance text-3xl font-bold tracking-tight">
                Reliable and controlled by design
              </h2>
              <p className="mt-4 leading-relaxed text-muted-foreground">
                An exam is only useful if everyone can trust it. The rules live on the server, where
                they cannot be bypassed from the browser.
              </p>
              <ul className="mt-6 flex flex-wrap gap-2" aria-label="Technology">
                {["Go", "Next.js", "PostgreSQL", "Redis"].map((t) => (
                  <li key={t}>
                    <Badge variant="outline">{t}</Badge>
                  </li>
                ))}
              </ul>
            </div>

            <dl className="grid gap-x-10 gap-y-8 sm:grid-cols-2 lg:col-span-3">
              {trustPoints.map((t) => (
                <div key={t.title} className="border-t border-border pt-5">
                  <dt className="font-semibold">{t.title}</dt>
                  <dd className="mt-1.5 text-sm leading-relaxed text-muted-foreground">{t.text}</dd>
                </div>
              ))}
            </dl>
          </div>
        </section>

        {/* Final CTA */}
        <section aria-labelledby="cta-title" className="px-6 pb-20 md:pb-24">
          <div className="mx-auto max-w-6xl rounded-2xl bg-primary px-6 py-14 text-center text-primary-foreground sm:px-12">
            <h2 id="cta-title" className="text-balance text-3xl font-bold tracking-tight">
              Run your next exam in an accessible room
            </h2>
            <p className="mx-auto mt-3 max-w-xl text-primary-foreground/80">
              Sign in to manage exams as a teacher, or open your assigned exam as a student.
            </p>
            <div className="mt-8 flex flex-wrap justify-center gap-3">
              <Link
                href="/login?mode=register"
                className={`${btnBase} h-11 bg-background px-6 text-foreground hover:bg-background/90 focus-visible:ring-offset-primary`}
              >
                Create an account
              </Link>
              <Link
                href="/login"
                className={`${btnBase} h-11 border border-primary-foreground/40 px-6 text-primary-foreground hover:bg-primary-foreground/10 focus-visible:ring-offset-primary`}
              >
                Sign in
              </Link>
            </div>
          </div>
        </section>
      </main>

      {/* Footer */}
      <footer className="border-t border-border px-6 py-8">
        <div className="mx-auto flex max-w-6xl flex-col items-center justify-between gap-4 text-sm text-muted-foreground sm:flex-row">
          <p>
            © {new Date().getFullYear()} {APP_CONFIG.name}. Accessible examinations for everyone.
          </p>
          <nav aria-label="Footer">
            <ul className="flex gap-6">
              <li>
                <Link href="/login" className="rounded hover:text-foreground focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring">
                  Sign in
                </Link>
              </li>
              <li>
                <Link href="/login?mode=register" className="rounded hover:text-foreground focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring">
                  Register
                </Link>
              </li>
            </ul>
          </nav>
        </div>
      </footer>
    </div>
  );
}