import { describe, it, expect } from "vitest";
import {
  LoginSchema,
  RegisterSchema,
  CreateExamSchema,
  CreateQuestionSchema,
  AssignCandidateSchema,
} from "../validations";

describe("LoginSchema", () => {
  it("accepts valid email and password", () => {
    const res = LoginSchema.safeParse({
      email: "teacher@school.edu",
      password: "SecretPassword123",
    });
    expect(res.success).toBe(true);
  });

  it("rejects invalid email formats", () => {
    const res = LoginSchema.safeParse({
      email: "not-an-email",
      password: "SecretPassword123",
    });
    expect(res.success).toBe(false);
    if (!res.success) {
      expect(res.error.issues[0].message).toMatch(/valid email/i);
    }
  });

  it("rejects empty password", () => {
    const res = LoginSchema.safeParse({
      email: "valid@email.com",
      password: "",
    });
    expect(res.success).toBe(false);
  });
});

describe("RegisterSchema", () => {
  const validPayload = {
    email: "candidate@school.edu",
    password: "Password123",
    firstName: "Ada",
    lastName: "Lovelace",
    role: "STUDENT" as const,
  };

  it("accepts valid registration data for student/candidate", () => {
    const res = RegisterSchema.safeParse(validPayload);
    expect(res.success).toBe(true);
  });

  it("accepts valid registration data for teacher", () => {
    const res = RegisterSchema.safeParse({
      ...validPayload,
      role: "TEACHER",
    });
    expect(res.success).toBe(true);
  });

  it("rejects passwords under 8 characters", () => {
    const res = RegisterSchema.safeParse({
      ...validPayload,
      password: "Pass1",
    });
    expect(res.success).toBe(false);
    if (!res.success) {
      expect(res.error.issues[0].message).toMatch(/at least 8 characters/i);
    }
  });

  it("rejects passwords without numbers", () => {
    const res = RegisterSchema.safeParse({
      ...validPayload,
      password: "PasswordWithoutNumber",
    });
    expect(res.success).toBe(false);
    if (!res.success) {
      expect(res.error.issues[0].message).toMatch(/at least one number/i);
    }
  });

  it("rejects invalid role", () => {
    const res = RegisterSchema.safeParse({
      ...validPayload,
      role: "SUPERUSER",
    });
    expect(res.success).toBe(false);
  });
});

describe("CreateExamSchema", () => {
  it("accepts valid exam details", () => {
    const res = CreateExamSchema.safeParse({
      title: "Biology 101 Midterm",
      subject: "Biology",
      description: "Accessible examination on cell mitosis",
      durationMins: 60,
    });
    expect(res.success).toBe(true);
  });

  it("rejects durations under 5 minutes", () => {
    const res = CreateExamSchema.safeParse({
      title: "Quick Quiz",
      subject: "Math",
      durationMins: 3,
    });
    expect(res.success).toBe(false);
  });

  it("rejects durations over 360 minutes", () => {
    const res = CreateExamSchema.safeParse({
      title: "Marathon Exam",
      subject: "Math",
      durationMins: 500,
    });
    expect(res.success).toBe(false);
  });
});

describe("CreateQuestionSchema", () => {
  it("accepts valid MCQ with 2+ options and a correct option", () => {
    const res = CreateQuestionSchema.safeParse({
      text: "What is the powerhouse of the cell?",
      type: "MCQ",
      points: 5,
      options: [
        { optionKey: "A", optionText: "Mitochondria", displayOrder: 1, isCorrect: true },
        { optionKey: "B", optionText: "Nucleus", displayOrder: 2, isCorrect: false },
      ],
    });
    expect(res.success).toBe(true);
  });

  it("rejects MCQ with fewer than 2 options", () => {
    const res = CreateQuestionSchema.safeParse({
      text: "What is the powerhouse of the cell?",
      type: "MCQ",
      points: 5,
      options: [
        { optionKey: "A", optionText: "Mitochondria", displayOrder: 1, isCorrect: true },
      ],
    });
    expect(res.success).toBe(false);
    if (!res.success) {
      expect(res.error.issues[0].message).toMatch(/at least 2 options/i);
    }
  });

  it("rejects MCQ where no option is marked correct", () => {
    const res = CreateQuestionSchema.safeParse({
      text: "What is the powerhouse of the cell?",
      type: "MCQ",
      points: 5,
      options: [
        { optionKey: "A", optionText: "Mitochondria", displayOrder: 1, isCorrect: false },
        { optionKey: "B", optionText: "Nucleus", displayOrder: 2, isCorrect: false },
      ],
    });
    expect(res.success).toBe(false);
    if (!res.success) {
      expect(res.error.issues[0].message).toMatch(/marked as correct/i);
    }
  });

  it("accepts ESSAY question without options", () => {
    const res = CreateQuestionSchema.safeParse({
      text: "Describe the process of photosynthesis in detail.",
      type: "ESSAY",
      points: 10,
    });
    expect(res.success).toBe(true);
  });

  it("accepts VOICE question without options", () => {
    const res = CreateQuestionSchema.safeParse({
      text: "Pronounce and explain the term 'neuroplasticity'.",
      type: "VOICE",
      points: 10,
    });
    expect(res.success).toBe(true);
  });
});

describe("AssignCandidateSchema", () => {
  it("accepts valid candidate id", () => {
    const res = AssignCandidateSchema.safeParse({ studentId: "cand_12345" });
    expect(res.success).toBe(true);
  });

  it("rejects short candidate id", () => {
    const res = AssignCandidateSchema.safeParse({ studentId: "ab" });
    expect(res.success).toBe(false);
  });
});
