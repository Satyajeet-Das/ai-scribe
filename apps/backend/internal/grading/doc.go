// Package grading lets a teacher mark a student's paper: marks and feedback for
// each answer, a total for the paper, and a final, locked evaluation.
//
// It is the write-side companion of the read-only review package, which stays
// read-only on purpose. Marking is done by hand. Multiple-choice answers get a
// suggested mark from the stored correct option, which the teacher may override;
// nothing here uses an AI model to mark.
//
// Marks are integers of half-marks (7.5 marks is 15), so every value is a
// multiple of 0.5 and totals never drift.
package grading
