"use client";

import { useCallback, useState } from "react";
import Link from "next/link";
import { ArrowLeft, ChevronLeft, ChevronRight, RefreshCw, Search, Users } from "lucide-react";
import { Button } from "@/components/ui/button";
import { Card } from "@/components/ui/card";
import { Input } from "@/components/ui/input";
import { Skeleton } from "@/components/ui/skeleton";
import { ErrorPanel } from "@/components/response-review/error-panel";
import {
  ReviewStatusBadge,
  formatDateTime,
  statusLabel,
} from "@/components/response-review/status-badge";
import { useDebounce } from "@/hooks/use-debounce";
import { useRemoteResource } from "@/hooks/use-remote-resource";
import { responseReviewApi } from "@/services/response-review-api";
import { DESIGN_TOKENS } from "@/lib/constants";
import type { ReviewStatus, ReviewStudentRow, ReviewSummary } from "@/types/response-review";

const PAGE_SIZE = 20;
const SEARCH_DEBOUNCE_MS = 300;

type StatusFilter = ReviewStatus | "ALL";

const FILTERS: { value: StatusFilter; label: string; count: (s: ReviewSummary) => number }[] = [
  { value: "ALL", label: "All", count: (s) => s.assigned },
  { value: "NOT_STARTED", label: "Not started", count: (s) => s.notStarted },
  { value: "IN_PROGRESS", label: "In progress", count: (s) => s.inProgress },
  { value: "SUBMITTED", label: "Submitted", count: (s) => s.submitted },
  { value: "EXPIRED", label: "Time expired", count: (s) => s.expired },
];

export function ResponseOverview({ examId }: { examId: string }) {
  const [searchText, setSearchText] = useState("");
  const [status, setStatus] = useState<StatusFilter>("ALL");
  const search = useDebounce(searchText.trim(), SEARCH_DEBOUNCE_MS, { immediateIfEmpty: true });

  // A page number belongs to the filters it was chosen under; changing the
  // filters starts again from the first page without needing an effect.
  const filterKey = `${examId}|${search}|${status}`;
  const [pageState, setPageState] = useState({ filterKey, offset: 0 });
  const offset = pageState.filterKey === filterKey ? pageState.offset : 0;
  const setOffset = (next: number) => setPageState({ filterKey, offset: next });

  const key = `${filterKey}|${offset}`;
  const load = useCallback(
    (signal: AbortSignal) =>
      responseReviewApi.listStudents(
        examId,
        {
          search,
          status: status === "ALL" ? undefined : status,
          limit: PAGE_SIZE,
          offset,
        },
        signal
      ),
    [examId, search, status, offset]
  );
  const { data, error, loading, refreshing, reload } = useRemoteResource(key, load, examId);

  const filtered = search !== "" || status !== "ALL";
  const total = data?.total ?? 0;
  const first = total === 0 ? 0 : offset + 1;
  const last = Math.min(offset + PAGE_SIZE, total);

  return (
    <div className="flex flex-col gap-6">
      <div>
        <Link
          href={`/exams/${examId}`}
          className="mb-4 inline-flex items-center gap-1.5 text-sm text-muted-foreground transition-colors hover:text-foreground"
        >
          <ArrowLeft className="size-4" aria-hidden="true" />
          Back to exam
        </Link>
        <div className="flex flex-wrap items-start justify-between gap-3">
          <div className="min-w-0">
            <h1 className={DESIGN_TOKENS.typography.h1}>Student responses</h1>
            {data ? (
              <p className="mt-1 text-sm text-muted-foreground">
                <span className="font-semibold text-foreground">{data.exam.title}</span>
                {" · "}
                {data.exam.subject}
                {" · "}
                {data.exam.totalQuestions} questions
                {" · "}
                {data.exam.durationMins} minutes
              </p>
            ) : (
              <Skeleton className="mt-2 h-4 w-72" />
            )}
          </div>
          <Button
            variant="outline"
            size="sm"
            onClick={reload}
            disabled={loading}
            className="gap-1.5"
          >
            <RefreshCw
              className={`size-3.5 ${refreshing ? "motion-safe:animate-spin" : ""}`}
              aria-hidden="true"
            />
            Refresh
          </Button>
        </div>
      </div>

      {data && <SummaryCards summary={data.summary} />}

      <Card className="gap-0 p-0">
        <div className="flex flex-col gap-3 border-b p-4">
          <div className="relative max-w-md">
            <Search
              className="pointer-events-none absolute left-3 top-1/2 size-4 -translate-y-1/2 text-muted-foreground"
              aria-hidden="true"
            />
            <Input
              type="search"
              value={searchText}
              onChange={(e) => setSearchText(e.target.value)}
              placeholder="Search by name or roll number"
              aria-label="Search students by name or roll number"
              className="pl-9"
              maxLength={100}
            />
          </div>
          <div role="group" aria-label="Filter by status" className="flex flex-wrap gap-2">
            {FILTERS.map((f) => {
              const active = status === f.value;
              return (
                <Button
                  key={f.value}
                  size="sm"
                  variant={active ? "default" : "outline"}
                  aria-pressed={active}
                  onClick={() => setStatus(f.value)}
                >
                  {f.label}
                  {data && (
                    <span className="ml-1 tabular-nums opacity-80">({f.count(data.summary)})</span>
                  )}
                </Button>
              );
            })}
          </div>
        </div>

        {/* One short announcement when results change; never one per keystroke. */}
        <p className="sr-only" role="status" aria-live="polite">
          {data && !loading
            ? total === 0
              ? "No students found."
              : `Showing ${first} to ${last} of ${total} students.`
            : ""}
        </p>

        <div aria-busy={loading}>
          {error ? (
            <div className="p-4">
              <ErrorPanel error={error} onRetry={reload} />
            </div>
          ) : !data ? (
            <RosterSkeleton />
          ) : data.data.length === 0 ? (
            <EmptyState filtered={filtered} />
          ) : (
            <ul className={`divide-y ${loading ? "opacity-60" : ""}`} aria-label="Students">
              {data.data.map((row) => (
                <StudentItem
                  key={row.studentId}
                  row={row}
                  examId={examId}
                  total={data.exam.totalQuestions}
                />
              ))}
            </ul>
          )}
        </div>

        {data && data.total > PAGE_SIZE && (
          <nav
            aria-label="Pagination"
            className="flex flex-wrap items-center justify-between gap-3 border-t p-4"
          >
            <p className="text-sm text-muted-foreground">
              {first}–{last} of {total}
            </p>
            <div className="flex gap-2">
              <Button
                size="sm"
                variant="outline"
                disabled={offset === 0 || loading}
                onClick={() => setOffset(Math.max(0, offset - PAGE_SIZE))}
                className="gap-1"
              >
                <ChevronLeft className="size-4" aria-hidden="true" />
                Previous
              </Button>
              <Button
                size="sm"
                variant="outline"
                disabled={offset + PAGE_SIZE >= total || loading}
                onClick={() => setOffset(offset + PAGE_SIZE)}
                className="gap-1"
              >
                Next
                <ChevronRight className="size-4" aria-hidden="true" />
              </Button>
            </div>
          </nav>
        )}
      </Card>
    </div>
  );
}

function SummaryCards({ summary }: { summary: ReviewSummary }) {
  const items = [
    { label: "Assigned", value: summary.assigned },
    { label: "Not started", value: summary.notStarted },
    { label: "In progress", value: summary.inProgress },
    { label: "Submitted", value: summary.submitted },
  ];
  return (
    <dl className="grid grid-cols-2 gap-3 md:grid-cols-4" aria-label="Exam summary">
      {items.map((item) => (
        <Card key={item.label} className="gap-1 p-4">
          <dt className="text-xs font-semibold uppercase tracking-wide text-muted-foreground">
            {item.label}
          </dt>
          <dd className="text-2xl font-bold tabular-nums">{item.value}</dd>
        </Card>
      ))}
    </dl>
  );
}

function StudentItem({
  row,
  examId,
  total,
}: {
  row: ReviewStudentRow;
  examId: string;
  total: number;
}) {
  const submitted = formatDateTime(row.submittedAt);
  const started = formatDateTime(row.startedAt);
  const hasSession = row.status !== "NOT_STARTED";
  return (
    <li className="flex flex-col gap-3 p-4 sm:flex-row sm:items-center sm:justify-between">
      <div className="min-w-0">
        <p className="truncate font-semibold">{row.name || "Unnamed student"}</p>
        <p className="text-sm text-muted-foreground">Roll no. {row.rollNo || "—"}</p>
      </div>
      <div className="flex flex-wrap items-center gap-x-4 gap-y-2 sm:justify-end">
        <ReviewStatusBadge status={row.status} />
        <div className="text-sm text-muted-foreground">
          {row.answeredCount !== undefined && (
            <p>
              {row.answeredCount} of {total} answered
            </p>
          )}
          {submitted ? (
            <p>Submitted {submitted}</p>
          ) : started ? (
            <p>Started {started}</p>
          ) : (
            <p>No attempt yet</p>
          )}
        </div>
        {hasSession ? (
          <Link
            href={`/exams/${examId}/responses/${row.studentId}`}
            aria-label={`Review responses for ${row.name} (${statusLabel(row.status)})`}
            className="inline-flex h-8 items-center rounded-lg bg-primary px-3 text-sm font-medium text-primary-foreground transition-colors hover:bg-primary/90 focus-visible:outline-none focus-visible:ring-[3px] focus-visible:ring-ring/50"
          >
            Review responses
          </Link>
        ) : (
          <span className="text-sm italic text-muted-foreground">Nothing to review</span>
        )}
      </div>
    </li>
  );
}

function RosterSkeleton() {
  return (
    <div className="divide-y" role="status" aria-label="Loading students">
      {[0, 1, 2, 3, 4].map((i) => (
        <div key={i} className="flex items-center justify-between gap-4 p-4">
          <div className="space-y-2">
            <Skeleton className="h-4 w-40" />
            <Skeleton className="h-3 w-24" />
          </div>
          <Skeleton className="h-6 w-24 rounded-full" />
        </div>
      ))}
    </div>
  );
}

function EmptyState({ filtered }: { filtered: boolean }) {
  return (
    <div className="flex flex-col items-center gap-2 px-6 py-12 text-center">
      <Users className="size-10 text-muted-foreground/50" aria-hidden="true" />
      <h2 className="text-lg font-semibold">
        {filtered ? "No students match your search" : "No students are assigned to this exam"}
      </h2>
      <p className="max-w-sm text-sm text-muted-foreground">
        {filtered
          ? "Try a different name or roll number, or clear the status filter."
          : "Assign students from the exam page and they will appear here."}
      </p>
    </div>
  );
}
