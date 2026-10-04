import { Skeleton } from "@/components/ui/skeleton";
import { ExamCardsSkeleton, StatsSkeleton } from "@/components/ui/exam-skeleton";
import { DESIGN_TOKENS } from "@/lib/constants";

export default function Loading() {
  return (
    <div
      role="status"
      aria-label="Loading page content"
      aria-live="polite"
      className={`${DESIGN_TOKENS.layout.container} py-8 space-y-8 animate-pulse`}
    >
      <div className="space-y-3">
        <Skeleton className="h-8 w-48 rounded-lg" />
        <Skeleton className="h-4 w-80 max-w-full rounded" />
      </div>
      <StatsSkeleton />
      <div className="space-y-4 pt-4">
        <div className="flex items-center justify-between">
          <Skeleton className="h-10 w-72 max-w-full rounded-md" />
          <Skeleton className="h-10 w-32 rounded-md" />
        </div>
        <ExamCardsSkeleton count={4} />
      </div>
      <span className="sr-only">Loading page content...</span>
    </div>
  );
}
