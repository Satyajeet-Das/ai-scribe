import Link from "next/link";
import { AlertCircle, RotateCcw } from "lucide-react";
import { Button } from "@/components/ui/button";
import type { ResourceError } from "@/hooks/use-remote-resource";

/**
 * Failure state. Retrying only makes sense for transient failures; an expired
 * session needs a new sign-in, and permission or not-found errors will not
 * change on retry.
 */
export function ErrorPanel({ error, onRetry }: { error: ResourceError; onRetry: () => void }) {
  const retryable = error.kind === "network" || error.kind === "server";
  return (
    <div
      role="alert"
      className="flex flex-col items-start gap-3 rounded-xl border border-destructive/30 bg-destructive/10 p-5 text-sm sm:flex-row sm:items-center"
    >
      <AlertCircle className="size-5 shrink-0 text-destructive" aria-hidden="true" />
      <p className="flex-1 font-medium text-foreground">{error.message}</p>
      {retryable && (
        <Button size="sm" variant="outline" onClick={onRetry} className="gap-1.5">
          <RotateCcw className="size-3.5" aria-hidden="true" />
          Try again
        </Button>
      )}
      {error.kind === "auth" && (
        <Link
          href="/login"
          className="inline-flex h-8 items-center rounded-lg border px-3 text-sm font-medium hover:bg-muted"
        >
          Sign in
        </Link>
      )}
    </div>
  );
}
