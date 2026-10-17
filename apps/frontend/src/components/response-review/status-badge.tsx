import { CheckCircle2, CircleDashed, Clock, TimerOff } from "lucide-react";
import { Badge } from "@/components/ui/badge";
import type { ReviewStatus } from "@/types/response-review";

const CONFIG: Record<
  ReviewStatus,
  { label: string; className: string; icon: typeof CheckCircle2 }
> = {
  NOT_STARTED: {
    label: "Not started",
    className: "border-border bg-muted text-foreground",
    icon: CircleDashed,
  },
  IN_PROGRESS: {
    label: "In progress",
    className:
      "border-amber-300 bg-amber-100 text-amber-900 dark:border-amber-800 dark:bg-amber-950 dark:text-amber-200",
    icon: Clock,
  },
  SUBMITTED: {
    label: "Submitted",
    className:
      "border-emerald-300 bg-emerald-100 text-emerald-900 dark:border-emerald-800 dark:bg-emerald-950 dark:text-emerald-200",
    icon: CheckCircle2,
  },
  EXPIRED: {
    label: "Time expired",
    className:
      "border-rose-300 bg-rose-100 text-rose-900 dark:border-rose-800 dark:bg-rose-950 dark:text-rose-200",
    icon: TimerOff,
  },
};

export function statusLabel(status: ReviewStatus): string {
  return CONFIG[status]?.label ?? status;
}

/** Status is always conveyed by an icon and text, never by color alone. */
export function ReviewStatusBadge({ status }: { status: ReviewStatus }) {
  const cfg = CONFIG[status];
  if (!cfg) return <Badge variant="outline">{status}</Badge>;
  const Icon = cfg.icon;
  return (
    <Badge variant="outline" className={`h-6 gap-1 px-2.5 ${cfg.className}`}>
      <Icon aria-hidden="true" />
      {cfg.label}
    </Badge>
  );
}

export function formatDateTime(value?: string): string | null {
  if (!value) return null;
  const d = new Date(value);
  if (Number.isNaN(d.getTime())) return null;
  return d.toLocaleString(undefined, { dateStyle: "medium", timeStyle: "short" });
}
