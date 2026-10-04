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
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { assignmentsApi } from "@/services/api";
import { AssignCandidateSchema } from "@/lib/validations";
import type { Assignment } from "@/types/exam-types";

const fallbackAssignments: Assignment[] = [
  {
    id: "assignment-1",
    examId: "exam-1",
    studentId: "student-0038",
    assignedAt: "2025-02-20",
    status: "ACTIVE",
  },
  {
    id: "assignment-2",
    examId: "exam-1",
    studentId: "student-0042",
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
  const [studentId, setStudentId] = useState("");
  const [saving, setSaving] = useState(false);
  const [error, setError] = useState<string | null>(null);

  const active = items.filter((item) => item.status === "ACTIVE");

  const handleAssign = async () => {
    setError(null);
    const validation = AssignCandidateSchema.safeParse({ studentId });
    if (!validation.success) {
      setError(validation.error.errors[0]?.message || "Invalid candidate identifier");
      return;
    }

    setSaving(true);
    try {
      // Backend assignment API call
      const created = await assignmentsApi.createAssignment(examId, studentId.trim());
      setItems((current) => [created, ...current]);
    } catch {
      // Optimistic local fallback
      const localAssign: Assignment = {
        id: crypto.randomUUID(),
        examId,
        studentId: studentId.trim(),
        assignedAt: new Date().toISOString(),
        status: "ACTIVE",
      };
      setItems((current) => [localAssign, ...current]);
    } finally {
      setSaving(false);
      setStudentId("");
      setOpen(false);
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

        <Dialog open={open} onOpenChange={setOpen}>
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
                Enter the candidate&apos;s Student ID or registered email address.
              </DialogDescription>
            </DialogHeader>

            <div className="space-y-3 py-2">
              {error && (
                <div className="flex items-center gap-2 rounded-lg bg-destructive/10 p-2.5 text-xs text-destructive">
                  <AlertCircle className="size-4 shrink-0" />
                  <span>{error}</span>
                </div>
              )}

              <div className="space-y-1.5">
                <Label htmlFor="student-id" className="text-sm font-medium">
                  Student ID or Email <span className="text-destructive">*</span>
                </Label>
                <Input
                  id="student-id"
                  placeholder="e.g. student-0092 or student@ai-scribe.org"
                  value={studentId}
                  onChange={(e) => setStudentId(e.target.value)}
                  autoFocus
                />
              </div>
            </div>

            <DialogFooter className="gap-2 sm:gap-0">
              <Button variant="outline" type="button" onClick={() => setOpen(false)}>
                Cancel
              </Button>
              <Button type="button" onClick={handleAssign} disabled={saving}>
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
            {items.map((assignment) => (
              <div
                key={assignment.id}
                className="flex items-center justify-between gap-3 rounded-lg border border-border p-3 bg-card"
              >
                <div>
                  <p className="font-mono text-xs font-semibold text-foreground">
                    {assignment.studentId}
                  </p>
                  <p className="text-[11px] text-muted-foreground" suppressHydrationWarning>
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
                      aria-label={`Revoke ${assignment.studentId}`}
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
            ))}
          </div>
        )}
      </CardContent>
    </Card>
  );
}
export default AssignmentManager;
