"use client";

import { useEffect } from "react";
import Link from "next/link";
import { AlertTriangle, RefreshCw, Home } from "lucide-react";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";
import { APP_CONFIG } from "@/lib/constants";

export default function GlobalErrorPage({
  error,
  reset,
}: {
  error: Error & { digest?: string };
  reset: () => void;
}) {
  useEffect(() => {
    console.error("Unhandled client application error:", error);
  }, [error]);

  return (
    <div className="flex min-h-screen flex-col items-center justify-center bg-background px-4 py-12 sm:px-6 lg:px-8">
      <Card className="w-full max-w-lg border border-destructive/20 bg-card text-center shadow-lg">
        <CardHeader className="pb-4">
          <div className="mx-auto flex size-16 items-center justify-center rounded-2xl bg-destructive/10 text-destructive mb-3">
            <AlertTriangle className="size-8" />
          </div>
          <span className="text-xs font-bold uppercase tracking-wider text-destructive">
            Application Error
          </span>
          <CardTitle className="text-2xl sm:text-3xl font-extrabold tracking-tight mt-1">
            Something Went Wrong
          </CardTitle>
          <CardDescription className="text-sm sm:text-base mt-2">
            An unexpected error occurred while rendering this page.
          </CardDescription>
        </CardHeader>
        <CardContent className="space-y-4 pt-2">
          {error.message && (
            <div className="rounded-lg bg-muted/60 p-3 text-xs text-muted-foreground font-mono text-left break-all border border-border">
              {error.message}
            </div>
          )}

          <div className="flex flex-col sm:flex-row items-center justify-center gap-3 pt-2">
            <Button onClick={() => reset()} className="w-full sm:w-auto">
              <RefreshCw className="mr-2 size-4" />
              Try Again
            </Button>
            <Link
              href="/"
              className="inline-flex h-8 w-full sm:w-auto items-center justify-center gap-1.5 rounded-lg border border-border bg-background px-2.5 text-sm font-medium hover:bg-muted transition-colors"
            >
              <Home className="size-4" />
              Return Home
            </Link>
          </div>
        </CardContent>
      </Card>
      <p className="mt-8 text-xs text-muted-foreground">
        {APP_CONFIG.name} · System Resilience Layer
      </p>
    </div>
  );
}
