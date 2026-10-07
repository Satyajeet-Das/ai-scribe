"use client";

import { useState, useMemo } from "react";
import {
  UserPlus,
  X,
  Loader2,
  AlertCircle,
  Users,
  Search,
  Filter,
  CheckCircle2,
  ChevronLeft,
  ChevronRight,
  Mail,
  Calendar,
} from "lucide-react";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardHeader, CardTitle, CardDescription } from "@/components/ui/card";
import { Input } from "@/components/ui/input";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
  DialogTrigger,
} from "@/components/ui/dialog";
import { Label } from "@/components/ui/label";
import { assignmentsApi } from "@/services/api";
import { StudentAutocompleteSelector } from "@/components/student-autocomplete-selector";
import type { Assignment } from "@/types/exam-types";
import type { StudentSearchResult } from "@/types/auth";

const PAGE_SIZE = 6;

export function AssignmentManager({
  examId = "exam-1",
  assignments = [],
  onAssignmentChanged,
}: {
  examId?: string;
  assignments?: Assignment[];
  onAssignmentChanged?: () => void;
}) {
  const [items, setItems] = useState<Assignment[]>(assignments ?? []);
  const [open, setOpen] = useState(false);
  const [studentToRevoke, setStudentToRevoke] = useState<Assignment | null>(null);

  // Assignment selection state (supports both single and multi-select)
  const [isBulkMode, setIsBulkMode] = useState(false);
  const [selectedStudent, setSelectedStudent] = useState<StudentSearchResult | null>(null);
  const [selectedStudents, setSelectedStudents] = useState<StudentSearchResult[]>([]);
  const [saving, setSaving] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [bulkFeedback, setBulkFeedback] = useState<{
    assignedCount: number;
    failed: Array<{ identifier: string; reason: string }>;
  } | null>(null);

  // Search & filter state
  const [searchFilter, setSearchFilter] = useState("");
  const [statusFilter, setStatusFilter] = useState<"ALL" | "ACTIVE" | "REVOKED">("ALL");
  const [currentPage, setCurrentPage] = useState(1);

  // Sync state when assignments prop reference changes
  const [prevAssignments, setPrevAssignments] = useState(assignments);
  if (assignments !== prevAssignments) {
    setPrevAssignments(assignments);
    setItems(assignments ?? []);
  }

  const isActive = (status: string) => status === "ACTIVE" || status === "ASSIGNED";
  const activeCount = useMemo(() => items.filter((item) => isActive(item.status)).length, [items]);
  const activeStudentIds = useMemo(
    () => items.filter((item) => isActive(item.status)).map((item) => item.studentId),
    [items]
  );

  // Filtered & Paginated items
  const filteredItems = useMemo(() => {
    return items.filter((item) => {
      // Status filter
      if (statusFilter === "ACTIVE" && !isActive(item.status)) return false;
      if (statusFilter === "REVOKED" && isActive(item.status)) return false;

      // Text search
      if (searchFilter.trim() !== "") {
        const query = searchFilter.toLowerCase().trim();
        const roll = (item.studentRollNo || "").toLowerCase();
        const name = (item.studentName || "").toLowerCase();
        const email = (item.studentEmail || "").toLowerCase();
        const id = item.studentId.toLowerCase();
        if (!roll.includes(query) && !name.includes(query) && !email.includes(query) && !id.includes(query)) {
          return false;
        }
      }

      return true;
    });
  }, [items, statusFilter, searchFilter]);

  const totalPages = Math.max(1, Math.ceil(filteredItems.length / PAGE_SIZE));
  const paginatedItems = useMemo(() => {
    const start = (currentPage - 1) * PAGE_SIZE;
    return filteredItems.slice(start, start + PAGE_SIZE);
  }, [filteredItems, currentPage]);

  const handleAssign = async () => {
    setError(null);
    setBulkFeedback(null);

    // Multi-select / Bulk Assign flow
    if (isBulkMode || selectedStudents.length > 1) {
      if (selectedStudents.length === 0) {
        setError("Please search and select at least one candidate");
        return;
      }

      setSaving(true);
      try {
        const studentIds = selectedStudents.map((s) => s.id);
        const res = await assignmentsApi.bulkAssign(examId, studentIds);

        if (res.assigned && res.assigned.length > 0) {
          // Merge newly created assignments
          const newAssignedMap = new Map(selectedStudents.map((s) => [s.id, s]));
          const enriched: Assignment[] = res.assigned.map((a) => {
            const stu = newAssignedMap.get(a.studentId);
            return {
              ...a,
              studentRollNo: stu?.rollNo || stu?.roll_no || a.studentRollNo,
              studentName: stu?.name || a.studentName,
              studentEmail: stu?.email || a.studentEmail,
            };
          });

          setItems((current) => [...enriched, ...current]);
        }

        if (res.totalFailed > 0) {
          setBulkFeedback({
            assignedCount: res.totalAssigned,
            failed: res.failed,
          });
        } else {
          setOpen(false);
          setSelectedStudents([]);
          setSelectedStudent(null);
        }

        onAssignmentChanged?.();
      } catch (err: unknown) {
        const msg = err instanceof Error ? err.message : "Failed to complete bulk assignment";
        setError(msg);
      } finally {
        setSaving(false);
      }
      return;
    }

    // Single Assign flow
    const studentToAssign = selectedStudent || (selectedStudents.length === 1 ? selectedStudents[0] : null);
    if (!studentToAssign) {
      setError("Please search and select a candidate by roll number or name");
      return;
    }

    if (activeStudentIds.includes(studentToAssign.id)) {
      setError(
        `Student with roll number ${studentToAssign.rollNo || studentToAssign.roll_no} is already assigned to this exam`
      );
      return;
    }

    setSaving(true);
    try {
      const created = await assignmentsApi.createAssignment(examId, studentToAssign.id);
      const enriched: Assignment = {
        ...created,
        studentRollNo: studentToAssign.rollNo || studentToAssign.roll_no,
        studentName: studentToAssign.name,
        studentEmail: studentToAssign.email,
      };
      setItems((current) => [enriched, ...current]);
      setSelectedStudent(null);
      setSelectedStudents([]);
      setOpen(false);
      onAssignmentChanged?.();
    } catch (err: unknown) {
      const msg = err instanceof Error ? err.message : "Failed to assign student";
      setError(msg);
    } finally {
      setSaving(false);
    }
  };

  const handleConfirmRevoke = async () => {
    if (!studentToRevoke) return;
    setSaving(true);
    setError(null);
    try {
      await assignmentsApi.revokeAssignment(studentToRevoke.id);
      setItems((current) =>
        current.map((item) =>
          item.id === studentToRevoke.id ? { ...item, status: "REVOKED" } : item
        )
      );
      setStudentToRevoke(null);
      onAssignmentChanged?.();
    } catch (err: unknown) {
      const msg = err instanceof Error ? err.message : "Failed to remove student assignment";
      setError(msg);
    } finally {
      setSaving(false);
    }
  };

  return (
    <Card className="border border-border">
      <CardHeader className="flex flex-col sm:flex-row sm:items-center sm:justify-between gap-4 border-b pb-4">
        <div>
          <div className="flex items-center gap-2.5">
            <CardTitle className="text-lg font-bold">Assigned Candidates</CardTitle>
            <Badge variant="secondary" className="font-mono text-xs">
              {activeCount} Assigned
            </Badge>
          </div>
          <CardDescription className="text-xs text-muted-foreground mt-1">
            {activeCount} candidate{activeCount === 1 ? "" : "s"} currently authorized to take this assessment.
          </CardDescription>
        </div>

        {/* Revocation Confirmation Dialog */}
        <Dialog
          open={!!studentToRevoke}
          onOpenChange={(isOpen) => {
            if (!isOpen) setStudentToRevoke(null);
          }}
        >
          <DialogContent className="sm:max-w-md">
            <DialogHeader>
              <DialogTitle>Remove Student Assignment</DialogTitle>
              <DialogDescription>
                Are you sure you want to unassign{" "}
                <span className="font-semibold text-foreground">
                  {studentToRevoke?.studentName || studentToRevoke?.studentRollNo || "this candidate"}
                </span>{" "}
                {studentToRevoke?.studentRollNo && `(${studentToRevoke.studentRollNo})`} from this exam?
                They will immediately lose access to start or continue this assessment.
              </DialogDescription>
            </DialogHeader>
            <DialogFooter className="gap-2 sm:gap-0">
              <Button
                variant="outline"
                type="button"
                onClick={() => setStudentToRevoke(null)}
                disabled={saving}
              >
                Cancel
              </Button>
              <Button
                variant="destructive"
                type="button"
                onClick={handleConfirmRevoke}
                disabled={saving}
              >
                {saving && <Loader2 className="mr-2 size-4 animate-spin" />}
                {saving ? "Removing..." : "Confirm Removal"}
              </Button>
            </DialogFooter>
          </DialogContent>
        </Dialog>

        {/* Assign Candidate Dialog (Supports Single & Multiple Bulk Selection) */}
        <Dialog
          open={open}
          onOpenChange={(isOpen) => {
            setOpen(isOpen);
            if (!isOpen) {
              setSelectedStudent(null);
              setSelectedStudents([]);
              setError(null);
              setBulkFeedback(null);
            }
          }}
        >
          <DialogTrigger
            render={
              <Button size="sm" className="font-semibold">
                <UserPlus className="mr-1.5 size-4" />
                Assign Candidate
              </Button>
            }
          />
          <DialogContent className="sm:max-w-lg">
            <DialogHeader>
              <DialogTitle>Assign Candidate to Exam</DialogTitle>
              <DialogDescription>
                Search candidates by roll number, name, or registered email.
              </DialogDescription>
            </DialogHeader>

            <div className="space-y-4 py-2">
              {/* Mode Toggle: Single vs Multi-Student Batch */}
              <div className="flex items-center justify-between border-b pb-2">
                <span className="text-xs font-semibold text-muted-foreground uppercase tracking-wider">
                  Assignment Mode
                </span>
                <div className="flex rounded-md border p-0.5 bg-muted/40 text-xs">
                  <button
                    type="button"
                    onClick={() => {
                      setIsBulkMode(false);
                      setSelectedStudents([]);
                      setError(null);
                    }}
                    className={`px-2.5 py-1 rounded transition-colors ${
                      !isBulkMode
                        ? "bg-background text-foreground font-semibold shadow-xs"
                        : "text-muted-foreground hover:text-foreground"
                    }`}
                  >
                    Single Candidate
                  </button>
                  <button
                    type="button"
                    onClick={() => {
                      setIsBulkMode(true);
                      setSelectedStudent(null);
                      setError(null);
                    }}
                    className={`px-2.5 py-1 rounded transition-colors ${
                      isBulkMode
                        ? "bg-background text-foreground font-semibold shadow-xs"
                        : "text-muted-foreground hover:text-foreground"
                    }`}
                  >
                    Batch Multi-Select
                  </button>
                </div>
              </div>

              {error && (
                <div
                  role="alert"
                  className="flex items-center gap-2 rounded-lg bg-destructive/10 p-2.5 text-xs text-destructive"
                >
                  <AlertCircle className="size-4 shrink-0" />
                  <span>{error}</span>
                </div>
              )}

              {bulkFeedback && (
                <div
                  role="alert"
                  className="rounded-lg border border-amber-500/30 bg-amber-50 dark:bg-amber-950/20 p-3 text-xs space-y-1.5 text-amber-900 dark:text-amber-200"
                >
                  <div className="flex items-center gap-2 font-semibold">
                    <CheckCircle2 className="size-4 text-emerald-600" />
                    <span>Successfully assigned {bulkFeedback.assignedCount} candidate(s).</span>
                  </div>
                  {bulkFeedback.failed.length > 0 && (
                    <div className="mt-1 space-y-1">
                      <p className="font-medium text-destructive">
                        {bulkFeedback.failed.length} candidate(s) could not be assigned:
                      </p>
                      <ul className="list-disc pl-4 space-y-0.5 text-[11px] text-muted-foreground">
                        {bulkFeedback.failed.map((f, i) => (
                          <li key={i}>
                            <strong className="text-foreground">{f.identifier}</strong>: {f.reason}
                          </li>
                        ))}
                      </ul>
                    </div>
                  )}
                </div>
              )}

              <div className="space-y-2">
                <Label htmlFor="student-selector" className="text-sm font-medium">
                  {isBulkMode ? "Search & Add Candidates" : "Search Candidate"}{" "}
                  <span className="text-destructive">*</span>
                </Label>
                <StudentAutocompleteSelector
                  multiSelect={isBulkMode}
                  selectedStudent={selectedStudent}
                  selectedStudents={selectedStudents}
                  onSelect={(student) => {
                    setSelectedStudent(student);
                    setError(null);
                  }}
                  onSelectMultiple={(students) => {
                    setSelectedStudents(students);
                    setError(null);
                  }}
                  existingStudentIds={activeStudentIds}
                  disabled={saving}
                />
              </div>
            </div>

            <DialogFooter className="gap-2 sm:gap-0">
              <Button
                variant="outline"
                type="button"
                onClick={() => {
                  setOpen(false);
                  setSelectedStudent(null);
                  setSelectedStudents([]);
                  setError(null);
                  setBulkFeedback(null);
                }}
              >
                Cancel
              </Button>
              <Button
                type="button"
                onClick={handleAssign}
                disabled={
                  saving ||
                  (!isBulkMode &&
                    (!selectedStudent || activeStudentIds.includes(selectedStudent.id))) ||
                  (isBulkMode && selectedStudents.length === 0)
                }
              >
                {saving && <Loader2 className="mr-2 size-4 animate-spin" />}
                {saving
                  ? "Assigning..."
                  : isBulkMode
                  ? `Assign ${selectedStudents.length} Candidate${selectedStudents.length === 1 ? "" : "s"}`
                  : "Confirm Assignment"}
              </Button>
            </DialogFooter>
          </DialogContent>
        </Dialog>
      </CardHeader>

      <CardContent className="pt-4 space-y-4">
        {/* Search & Filter Toolbar */}
        {items.length > 0 && (
          <div className="flex flex-col sm:flex-row items-stretch sm:items-center justify-between gap-3 bg-muted/20 p-2.5 rounded-lg border">
            <div className="relative flex-1">
              <Search className="absolute left-2.5 top-1/2 size-3.5 -translate-y-1/2 text-muted-foreground pointer-events-none" />
              <Input
                placeholder="Filter assigned candidates by roll number or name..."
                value={searchFilter}
                onChange={(e) => {
                  setSearchFilter(e.target.value);
                  setCurrentPage(1);
                }}
                className="pl-8 h-8 text-xs bg-background"
              />
            </div>

            <div className="flex items-center gap-1.5 self-end sm:self-auto">
              <Filter className="size-3.5 text-muted-foreground" />
              <div className="flex rounded-md border bg-background p-0.5 text-xs">
                <button
                  type="button"
                  onClick={() => {
                    setStatusFilter("ALL");
                    setCurrentPage(1);
                  }}
                  className={`px-2 py-0.5 rounded text-[11px] font-medium transition-colors ${
                    statusFilter === "ALL" ? "bg-primary text-primary-foreground font-semibold" : "text-muted-foreground hover:text-foreground"
                  }`}
                >
                  All ({items.length})
                </button>
                <button
                  type="button"
                  onClick={() => {
                    setStatusFilter("ACTIVE");
                    setCurrentPage(1);
                  }}
                  className={`px-2 py-0.5 rounded text-[11px] font-medium transition-colors ${
                    statusFilter === "ACTIVE" ? "bg-primary text-primary-foreground font-semibold" : "text-muted-foreground hover:text-foreground"
                  }`}
                >
                  Active ({activeCount})
                </button>
                <button
                  type="button"
                  onClick={() => {
                    setStatusFilter("REVOKED");
                    setCurrentPage(1);
                  }}
                  className={`px-2 py-0.5 rounded text-[11px] font-medium transition-colors ${
                    statusFilter === "REVOKED" ? "bg-primary text-primary-foreground font-semibold" : "text-muted-foreground hover:text-foreground"
                  }`}
                >
                  Revoked ({items.length - activeCount})
                </button>
              </div>
            </div>
          </div>
        )}

        {/* Student Cards Grid */}
        {items.length === 0 ? (
          <div className="flex flex-col items-center justify-center py-10 px-4 text-center text-sm text-muted-foreground border border-dashed rounded-lg">
            <Users className="mx-auto size-10 text-muted-foreground/40 mb-3" />
            <p className="font-semibold text-foreground text-base">No Candidates Assigned</p>
            <p className="text-xs text-muted-foreground mt-1 max-w-sm">
              No students are currently allocated to this assessment. Use their roll number or email to grant access.
            </p>
            <Button
              size="sm"
              variant="outline"
              className="mt-4 gap-1.5 font-medium"
              onClick={() => setOpen(true)}
            >
              <UserPlus className="size-4" />
              Assign Candidate
            </Button>
          </div>
        ) : filteredItems.length === 0 ? (
          <div className="p-8 text-center text-xs text-muted-foreground border rounded-lg">
            <p className="font-semibold text-foreground text-sm">No Matching Candidates</p>
            <p className="mt-1">
              No assigned candidates matched &ldquo;{searchFilter}&rdquo; with status &ldquo;{statusFilter}&rdquo;.
            </p>
            <Button
              size="sm"
              variant="ghost"
              onClick={() => {
                setSearchFilter("");
                setStatusFilter("ALL");
              }}
              className="mt-3 text-xs"
            >
              Reset Filters
            </Button>
          </div>
        ) : (
          <div className="grid gap-3 sm:grid-cols-2">
            {paginatedItems.map((assignment) => {
              const displayRoll = assignment.studentRollNo;
              const displayName = assignment.studentName;
              const displayEmail = assignment.studentEmail;

              return (
                <div
                  key={assignment.id}
                  className="flex items-center justify-between gap-3 rounded-lg border border-border p-3.5 bg-card hover:border-primary/30 transition-colors shadow-2xs"
                >
                  <div className="min-w-0 flex-1">
                    <div className="flex items-center gap-2">
                      {displayRoll ? (
                        <span className="font-mono text-xs font-bold px-1.5 py-0.5 rounded bg-muted text-foreground">
                          {displayRoll}
                        </span>
                      ) : (
                        <span className="font-mono text-xs font-semibold text-foreground">
                          {assignment.studentId}
                        </span>
                      )}
                      {displayName && (
                        <>
                          <span className="text-muted-foreground text-xs">—</span>
                          <span className="text-xs font-medium text-foreground truncate max-w-[150px]">
                            {displayName}
                          </span>
                        </>
                      )}
                    </div>

                    {displayEmail && (
                      <p className="text-[11px] text-muted-foreground mt-1 flex items-center gap-1 truncate">
                        <Mail className="size-3 shrink-0" />
                        <span className="truncate">{displayEmail}</span>
                      </p>
                    )}

                    <p className="text-[11px] text-muted-foreground mt-1 flex items-center gap-1" suppressHydrationWarning>
                      <Calendar className="size-3 shrink-0" />
                      Assigned {assignment.assignedAt ? assignment.assignedAt.slice(0, 10) : "N/A"}
                    </p>
                  </div>

                  <div className="flex items-center gap-2 shrink-0">
                    <Badge
                      variant={isActive(assignment.status) ? "default" : "outline"}
                      className={`text-xs ${
                        isActive(assignment.status)
                          ? "bg-emerald-600 hover:bg-emerald-600 text-white"
                          : "text-muted-foreground border-muted"
                      }`}
                    >
                      {assignment.status}
                    </Badge>
                    {isActive(assignment.status) && (
                      <Button
                        size="icon"
                        variant="ghost"
                        className="size-7 text-muted-foreground hover:text-destructive hover:bg-destructive/10"
                        aria-label={`Revoke assignment for ${displayRoll || assignment.studentId}`}
                        onClick={() => setStudentToRevoke(assignment)}
                      >
                        <X className="size-3.5" />
                      </Button>
                    )}
                  </div>
                </div>
              );
            })}
          </div>
        )}

        {/* Pagination Controls */}
        {filteredItems.length > PAGE_SIZE && (
          <div className="flex items-center justify-between border-t pt-3 text-xs text-muted-foreground">
            <div>
              Showing {(currentPage - 1) * PAGE_SIZE + 1} to{" "}
              {Math.min(currentPage * PAGE_SIZE, filteredItems.length)} of {filteredItems.length}
            </div>
            <div className="flex items-center gap-1.5">
              <Button
                variant="outline"
                size="sm"
                className="h-7 px-2 text-xs"
                onClick={() => setCurrentPage((p) => Math.max(1, p - 1))}
                disabled={currentPage === 1}
              >
                <ChevronLeft className="size-3.5 mr-1" />
                Previous
              </Button>
              <span className="px-2 font-medium">
                {currentPage} / {totalPages}
              </span>
              <Button
                variant="outline"
                size="sm"
                className="h-7 px-2 text-xs"
                onClick={() => setCurrentPage((p) => Math.min(totalPages, p + 1))}
                disabled={currentPage === totalPages}
              >
                Next
                <ChevronRight className="size-3.5 ml-1" />
              </Button>
            </div>
          </div>
        )}
      </CardContent>
    </Card>
  );
}

export default AssignmentManager;
