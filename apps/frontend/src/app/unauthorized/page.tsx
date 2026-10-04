"use client";

import { Suspense } from "react";
import Link from "next/link";
import { useSearchParams } from "next/navigation";
import { LogIn, ArrowLeft } from "lucide-react";
import { buttonVariants } from "@/components/ui/button";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";
import { DESIGN_TOKENS } from "@/lib/constants";
import { cn } from "@/lib/utils";

function UnauthorizedContent() {
  const searchParams = useSearchParams();
  const rawReturnUrl = searchParams.get("returnUrl");
  const safeReturnUrl =
    rawReturnUrl &&
    rawReturnUrl.startsWith("/") &&
    !rawReturnUrl.startsWith("//") &&
    !rawReturnUrl.startsWith("/\\")
      ? rawReturnUrl
      : null;

  const loginHref = safeReturnUrl
    ? `/login?returnUrl=${encodeURIComponent(safeReturnUrl)}`
    : "/login";

  return (
    <main className="flex min-h-screen items-center justify-center bg-background px-4 py-12">
      <div className={DESIGN_TOKENS.layout.containerNarrow}>
        <Card className="w-full max-w-md mx-auto text-center border-border shadow-lg">
          <CardHeader className="space-y-2 pb-4">
            <div className="mx-auto flex size-14 items-center justify-center rounded-full bg-primary/10 text-primary">
              <LogIn className="size-7" aria-hidden="true" />
            </div>
            <CardTitle className="text-2xl font-bold tracking-tight">
              Authentication Required
            </CardTitle>
            <CardDescription className="text-sm">
              You must be signed in to access this page or assessment resource.
            </CardDescription>
          </CardHeader>
          <CardContent className="space-y-3 pt-2">
            <Link
              href={loginHref}
              className={cn(buttonVariants({ variant: "default", size: "lg" }), "w-full font-semibold")}
            >
              <LogIn className="mr-2 size-4" aria-hidden="true" />
              Sign In to Continue
            </Link>
            <Link
              href="/"
              className={cn(buttonVariants({ variant: "outline", size: "lg" }), "w-full")}
            >
              <ArrowLeft className="mr-2 size-4" aria-hidden="true" />
              Return to Homepage
            </Link>
          </CardContent>
        </Card>
      </div>
    </main>
  );
}

export default function UnauthorizedPage() {
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
      <UnauthorizedContent />
    </Suspense>
  );
}
