import { z } from "zod";

export const LoginSchema = z.object({
  email: z.string().trim().email("Please enter a valid email address"),
  password: z.string().min(1, "Password is required"),
});

export type LoginFormData = z.infer<typeof LoginSchema>;

export const RegisterSchema = z
  .object({
    email: z.string().trim().email("Please enter a valid email address"),
    password: z
      .string()
      .min(8, "Password must be at least 8 characters")
      .regex(/[A-Za-z]/, "Password must contain at least one letter")
      .regex(/[0-9]/, "Password must contain at least one number"),
    firstName: z.string().trim().min(2, "First name must be at least 2 characters"),
    lastName: z.string().trim().min(1, "Last name is required"),
    role: z.enum(["TEACHER", "STUDENT"], {
      required_error: "Please select a role",
    }),
    rollNo: z.string().trim().optional(),
  })
  .refine(
    (data) => {
      if (data.role === "STUDENT") {
        return Boolean(data.rollNo && data.rollNo.length > 0);
      }
      return true;
    },
    {
      message: "Roll number is required for students",
      path: ["rollNo"],
    }
  );

export type RegisterFormData = z.infer<typeof RegisterSchema>;

export const CreateExamSchema = z.object({
  title: z
    .string()
    .trim()
    .min(3, "Title must be at least 3 characters")
    .max(255, "Title cannot exceed 255 characters"),
  subject: z
    .string()
    .trim()
    .min(2, "Subject must be at least 2 characters")
    .max(100, "Subject cannot exceed 100 characters"),
  description: z
    .string()
    .trim()
    .max(2000, "Description cannot exceed 2000 characters")
    .optional()
    .default(""),
  durationMins: z
    .number({ invalid_type_error: "Duration must be a number" })
    .min(1, "Duration must be at least 1 minute")
    .max(600, "Duration cannot exceed 10 hours (600 minutes)"),
});

export type CreateExamFormData = z.infer<typeof CreateExamSchema>;

export const UpdateExamSchema = z.object({
  title: z
    .string()
    .trim()
    .min(3, "Title must be at least 3 characters")
    .max(255, "Title cannot exceed 255 characters")
    .optional(),
  subject: z
    .string()
    .trim()
    .min(2, "Subject must be at least 2 characters")
    .max(100, "Subject cannot exceed 100 characters")
    .optional(),
  description: z
    .string()
    .trim()
    .max(2000, "Description cannot exceed 2000 characters")
    .optional(),
  durationMins: z
    .number({ invalid_type_error: "Duration must be a number" })
    .min(1, "Duration must be at least 1 minute")
    .max(600, "Duration cannot exceed 10 hours (600 minutes)")
    .optional(),
});

export type UpdateExamFormData = z.infer<typeof UpdateExamSchema>;

export const OptionSchema = z.object({
  id: z.string().optional(),
  optionKey: z.string().min(1),
  optionText: z.string().trim().min(1, "Option text cannot be empty"),
  displayOrder: z.number(),
  isCorrect: z.boolean(),
});

export const CreateQuestionSchema = z
  .object({
    text: z.string().trim().min(5, "Question text must be at least 5 characters"),
    type: z.enum(["MCQ", "ESSAY", "VOICE"]),
    points: z.number().min(1, "Points must be at least 1").max(100, "Points cannot exceed 100"),
    options: z.array(OptionSchema).optional(),
  })
  .refine(
    (data) => {
      if (data.type === "MCQ") {
        return data.options && data.options.length >= 2;
      }
      return true;
    },
    {
      message: "Multiple choice questions must have at least 2 options",
      path: ["options"],
    }
  )
  .refine(
    (data) => {
      if (data.type === "MCQ" && data.options) {
        return data.options.some((opt) => opt.isCorrect);
      }
      return true;
    },
    {
      message: "At least one option must be marked as correct",
      path: ["options"],
    }
  );

export type CreateQuestionFormData = z.infer<typeof CreateQuestionSchema>;

export const AssignCandidateSchema = z.object({
  studentId: z.string().trim().min(3, "Candidate ID or Email is required"),
});

export type AssignCandidateFormData = z.infer<typeof AssignCandidateSchema>;
