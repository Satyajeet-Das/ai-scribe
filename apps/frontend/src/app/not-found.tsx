import Link from "next/link";
import { FileQuestion, Home, Compass } from "lucide-react";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";
import { APP_CONFIG } from "@/lib/constants";

export default function NotFound() {
  return (
    <div className="flex min-h-screen flex-col items-center justify-center bg-background px-4 py-12 sm:px-6 lg:px-8">
      <Card className="w-full max-w-lg border border-border bg-card text-center shadow-lg">
        <CardHeader className="pb-4">
          <div className="mx-auto flex size-16 items-center justify-center rounded-2xl bg-muted text-muted-foreground mb-3">
            <FileQuestion className="size-8 text-primary" />
          </div>
          <span className="text-xs font-bold uppercase tracking-wider text-primary">404 Error</span>
          <CardTitle className="text-2xl sm:text-3xl font-extrabold tracking-tight mt-1">
            Page Not Found
          </CardTitle>
          <CardDescription className="text-sm sm:text-base mt-2">
            The page you are looking for doesn&apos;t exist or may have been relocated.
          </CardDescription>
        </CardHeader>
        <CardContent className="space-y-4 pt-2">
          <div className="rounded-lg bg-muted/50 p-4 text-xs text-muted-foreground text-left space-y-1.5">
            <p className="font-semibold text-foreground">Common destinations:</p>
            <ul className="list-disc pl-4 space-y-1">
              <li>
                <strong>Educator Portal:</strong> Manage exams, assignments, and questions
              </li>
              <li>
                <strong>Candidate Portal:</strong> Access assigned assessments and active sessions
              </li>
            </ul>
          </div>

          <div className="flex flex-col sm:flex-row items-center justify-center gap-3 pt-2">
            <Link
              href="/"
              className="inline-flex h-8 w-full sm:w-auto items-center justify-center gap-1.5 rounded-lg border border-border bg-background px-2.5 text-sm font-medium hover:bg-muted transition-colors"
            >
              <Home className="size-4" />
              Return Home
            </Link>
            <Link
              href="/exams"
              className="inline-flex h-8 w-full sm:w-auto items-center justify-center gap-1.5 rounded-lg bg-primary text-primary-foreground px-2.5 text-sm font-medium hover:bg-primary/80 transition-colors"
            >
              <Compass className="size-4" />
              Educator Dashboard
            </Link>
          </div>
        </CardContent>
      </Card>
      <p className="mt-8 text-xs text-muted-foreground">
        {APP_CONFIG.name} · Accessible Examination System
      </p>
    </div>
  );
}
