"use client";

import { useState } from "react";
import { UserPlus, X } from "lucide-react";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
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
export type AssignmentStatus = "ACTIVE" | "REVOKED";
export type Assignment = {
  id: string;
  examId: string;
  studentId: string;
  assignedAt: string;
  status: AssignmentStatus;
};
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
  const [items, setItems] = useState(assignments);
  const [open, setOpen] = useState(false);
  const [studentId, setStudentId] = useState("");
  const active = items.filter((item) => item.status === "ACTIVE");
  const assign = () => {
    if (!studentId.trim()) return;
    setItems((current) => [
      {
        id: crypto.randomUUID(),
        examId,
        studentId: studentId.trim(),
        assignedAt: new Date().toISOString(),
        status: "ACTIVE",
      },
      ...current,
    ]);
    setStudentId("");
    setOpen(false);
  };
  return (
    <Card>
      <CardHeader className="flex flex-row items-center justify-between gap-4">
        <div>
          <CardTitle>Assigned candidates</CardTitle>
          <p className="mt-1 text-sm text-muted-foreground">{active.length} active assignments</p>
        </div>
        <Dialog open={open} onOpenChange={setOpen}>
          <DialogTrigger render={<Button />}>
            <UserPlus data-icon="inline-start" />
            Assign student
          </DialogTrigger>
          <DialogContent>
            <DialogHeader>
              <DialogTitle>Assign this exam</DialogTitle>
              <DialogDescription>
                Enter a student UUID to create an active assignment.
              </DialogDescription>
            </DialogHeader>
            <div className="flex flex-col gap-2">
              <Label htmlFor="student-id">Student UUID</Label>
              <Input
                id="student-id"
                value={studentId}
                onChange={(event) => setStudentId(event.target.value)}
                placeholder="student-0000"
              />
            </div>
            <DialogFooter>
              <Button variant="outline" onClick={() => setOpen(false)}>
                Cancel
              </Button>
              <Button onClick={assign} disabled={!studentId.trim()}>
                Create assignment
              </Button>
            </DialogFooter>
          </DialogContent>
        </Dialog>
      </CardHeader>
      <CardContent className="flex flex-col gap-2">
        {items.map((assignment) => (
          <div
            key={assignment.id}
            className="flex items-center justify-between gap-3 rounded-lg border p-3"
          >
            <div>
              <p className="font-mono text-sm">{assignment.studentId}</p>
              <p className="text-xs text-muted-foreground" suppressHydrationWarning>
                Assigned {assignment.assignedAt ? assignment.assignedAt.slice(0, 10) : "N/A"}
              </p>
            </div>
            <div className="flex items-center gap-2">
              <Badge variant={assignment.status === "ACTIVE" ? "secondary" : "outline"}>
                {assignment.status}
              </Badge>
              {assignment.status === "ACTIVE" && (
                <Button
                  size="icon"
                  variant="ghost"
                  aria-label={`Revoke ${assignment.studentId}`}
                  onClick={() =>
                    setItems((current) =>
                      current.map((item) =>
                        item.id === assignment.id ? { ...item, status: "REVOKED" } : item
                      )
                    )
                  }
                >
                  <X />
                </Button>
              )}
            </div>
          </div>
        ))}
      </CardContent>
    </Card>
  );
}
export default AssignmentManager;
