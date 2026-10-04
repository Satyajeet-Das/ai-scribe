"use client";

import { useState } from "react";
import { UserPlus, X, Loader2, AlertCircle, Users } from "lucide-react";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardHeader, CardTitle, CardDescription } from "@/components/ui/card";
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

const fallbackAssignments: Assignment[] = [
  {
    id: "assignment-1",
    examId: "exam-1",
    studentId: "student-0038",
    studentRollNo: "23CS001",
    studentName: "Rahul Sharma",
    assignedAt: "2025-02-20",
    status: "ACTIVE",
  },
  {
    id: "assignment-2",
    examId: "exam-1",
    studentId: "student-0042",
    studentRollNo: "23CS014",
    studentName: "Priya Singh",
    assignedAt: "2025-02-20",
    status: "ACTIVE",
  },
];

export function AssignmentManager({
  examId = "exam-1",
  assignments = fallbackAssignments,
}: {
  examId?: string;
  assignments?: Assignment[];
}) {
  const [items, setItems] = useState<Assignment[]>(
    assignments.length > 0 ? assignments : fallbackAssignments
  );
  const [open, setOpen] = useState(false);
  const [selectedStudent, setSelectedStudent] = useState<StudentSearchResult | null>(null);
  const [saving, setSaving] = useState(false);
  const [error, setError] = useState<string | null>(null);

  const active = items.filter((item) => item.status === "ACTIVE");
  const activeStudentIds = active.map((item) => item.studentId);

  const handleAssign = async () => {
    setError(null);
    if (!selectedStudent) {
      setError("Please search and select a candidate by roll number or name");
      return;
    }

    if (activeStudentIds.includes(selectedStudent.id)) {
      setError(`Student with roll number ${selectedStudent.rollNo || selectedStudent.roll_no} is already assigned to this exam`);
      return;
    }

    setSaving(true);
    try {
      // Backend assignment API call transmits the selected student's internal UUID
      const created = await assignmentsApi.createAssignment(examId, selectedStudent.id);
      const enriched: Assignment = {
        ...created,
        studentRollNo: selectedStudent.rollNo || selectedStudent.roll_no,
        studentName: selectedStudent.name,
      };
      setItems((current) => [enriched, ...current]);
      setSelectedStudent(null);
      setOpen(false);
    } catch (err: unknown) {
      // If backend returns duplicate or invalid, display backend authoritative error
      const msg = err instanceof Error ? err.message : "Failed to assign student";
      setError(msg);
    } finally {
      setSaving(false);
    }
  };

  return (
    <Card className="border border-border">
      <CardHeader className="flex flex-col sm:flex-row sm:items-center sm:justify-between gap-4 border-b pb-4">
        <div>
          <CardTitle className="text-lg font-bold">Assigned Candidates</CardTitle>
          <CardDescription className="text-xs text-muted-foreground mt-0.5">
            {active.length} active students currently authorized to take this assessment.
          </CardDescription>
        </div>

        <Dialog
          open={open}
          onOpenChange={(isOpen) => {
            setOpen(isOpen);
            if (!isOpen) {
              setSelectedStudent(null);
              setError(null);
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
          <DialogContent className="sm:max-w-md">
            <DialogHeader>
              <DialogTitle>Assign Candidate to Exam</DialogTitle>
              <DialogDescription>
                Search candidates by roll number, name, or registered email.
              </DialogDescription>
            </DialogHeader>

            <div className="space-y-4 py-2">
              {error && (
                <div
                  role="alert"
                  className="flex items-center gap-2 rounded-lg bg-destructive/10 p-2.5 text-xs text-destructive"
                >
                  <AlertCircle className="size-4 shrink-0" />
                  <span>{error}</span>
                </div>
              )}

              <div className="space-y-2">
                <Label htmlFor="student-selector" className="text-sm font-medium">
                  Search Candidate <span className="text-destructive">*</span>
                </Label>
                <StudentAutocompleteSelector
                  selectedStudent={selectedStudent}
                  onSelect={(student) => {
                    setSelectedStudent(student);
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
                  setError(null);
                }}
              >
                Cancel
              </Button>
              <Button
                type="button"
                onClick={handleAssign}
                disabled={saving || !selectedStudent || activeStudentIds.includes(selectedStudent.id)}
              >
                {saving && <Loader2 className="mr-2 size-4 animate-spin" />}
                {saving ? "Assigning..." : "Confirm Assignment"}
              </Button>
            </DialogFooter>
          </DialogContent>
        </Dialog>
      </CardHeader>

      <CardContent className="pt-6">
        {items.length === 0 ? (
          <div className="text-center py-8 text-sm text-muted-foreground border border-dashed rounded-lg">
            <Users className="mx-auto size-8 text-muted-foreground/50 mb-2" />
            No candidates have been assigned to this exam yet.
          </div>
        ) : (
          <div className="grid gap-3 sm:grid-cols-2">
            {items.map((assignment) => {
              const displayRoll = assignment.studentRollNo;
              const displayName = assignment.studentName;

              return (
                <div
                  key={assignment.id}
                  className="flex items-center justify-between gap-3 rounded-lg border border-border p-3 bg-card"
                >
                  <div>
                    <div className="flex items-center gap-2">
                      {displayRoll ? (
                        <span className="font-mono text-xs font-bold text-foreground">
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
                          <span className="text-xs font-medium text-foreground truncate max-w-[140px]">
                            {displayName}
                          </span>
                        </>
                      )}
                    </div>
                    <p className="text-[11px] text-muted-foreground mt-0.5" suppressHydrationWarning>
                      Assigned {assignment.assignedAt ? assignment.assignedAt.slice(0, 10) : "N/A"}
                    </p>
                  </div>
                  <div className="flex items-center gap-2">
                    <Badge
                      variant={assignment.status === "ACTIVE" ? "default" : "outline"}
                      className="text-xs"
                    >
                      {assignment.status}
                    </Badge>
                    {assignment.status === "ACTIVE" && (
                      <Button
                        size="icon"
                        variant="ghost"
                        className="size-7 text-muted-foreground hover:text-destructive"
                        aria-label={`Revoke assignment for ${displayRoll || assignment.studentId}`}
                        onClick={() =>
                          setItems((current) =>
                            current.map((item) =>
                              item.id === assignment.id ? { ...item, status: "REVOKED" } : item
                            )
                          )
                        }
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
      </CardContent>
    </Card>
  );
}

export default AssignmentManager;
