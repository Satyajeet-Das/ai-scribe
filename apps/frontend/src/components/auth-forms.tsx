"use client";

import { useState } from "react";
import { Eye, EyeOff, Loader2, AlertCircle, Clock } from "lucide-react";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Tabs, TabsList, TabsTrigger } from "@/components/ui/tabs";
import { LoginSchema, RegisterSchema } from "@/lib/validations";

export type AuthRole = "TEACHER" | "STUDENT";
type AuthMode = "login" | "register";

export type AuthSubmitData =
  | { mode: "login"; email: string; password: string }
  | {
      mode: "register";
      email: string;
      password: string;
      firstName: string;
      lastName: string;
      role: AuthRole;
    };

interface AuthFormsProps {
  initialMode?: AuthMode;
  onSubmit: (data: AuthSubmitData) => Promise<void>;
  onModeChange?: (mode: AuthMode) => void;
  isLoading?: boolean;
  errorMessage?: string | null;
  sessionExpired?: boolean;
}

const ROLE_OPTIONS: { value: AuthRole; label: string }[] = [
  { value: "STUDENT", label: "Candidate" },
  { value: "TEACHER", label: "Teacher" },
];

function FieldError({ id, message }: { id: string; message?: string }) {
  if (!message) return null;
  return (
    <p id={id} className="text-xs text-destructive" role="alert">
      {message}
    </p>
  );
}

export function AuthForms({
  initialMode = "login",
  onSubmit,
  onModeChange,
  isLoading = false,
  errorMessage = null,
  sessionExpired = false,
}: AuthFormsProps) {
  const [mode, setMode] = useState<AuthMode>(initialMode);
  const [role, setRole] = useState<AuthRole>("STUDENT");

  const [email, setEmail] = useState("");
  const [password, setPassword] = useState("");
  const [firstName, setFirstName] = useState("");
  const [lastName, setLastName] = useState("");
  const [showPassword, setShowPassword] = useState(false);
  const [errors, setErrors] = useState<Record<string, string>>({});

  const isRegister = mode === "register";

  const clearError = (field: string) =>
    setErrors((prev) => {
      if (!prev[field]) return prev;
      const next = { ...prev };
      delete next[field];
      return next;
    });

  const validate = (): boolean => {
    const result = isRegister
      ? RegisterSchema.safeParse({
          email: email.trim(),
          password,
          firstName: firstName.trim(),
          lastName: lastName.trim(),
          role,
        })
      : LoginSchema.safeParse({ email: email.trim(), password });

    if (result.success) {
      setErrors({});
      return true;
    }

    const fieldErrors: Record<string, string> = {};
    for (const issue of result.error.issues) {
      const key = issue.path[0];
      if (typeof key === "string" && !fieldErrors[key]) fieldErrors[key] = issue.message;
    }
    setErrors(fieldErrors);
    return false;
  };

  const handleSubmit = async (e: React.FormEvent) => {
    e.preventDefault();
    if (isLoading || !validate()) return;

    if (isRegister) {
      await onSubmit({
        mode: "register",
        email: email.trim(),
        password,
        firstName: firstName.trim(),
        lastName: lastName.trim(),
        role,
      });
    } else {
      await onSubmit({ mode: "login", email: email.trim(), password });
    }
  };

  const handleModeChange = (value: string) => {
    const next = value as AuthMode;
    setMode(next);
    setErrors({});
    setShowPassword(false);
    onModeChange?.(next);
  };

  const getPasswordStrength = () => {
    if (!password) return null;
    const score = [password.length >= 8, /[A-Za-z]/.test(password), /[0-9]/.test(password)].filter(
      Boolean
    ).length;
    if (score === 3)
      return { label: "Strong password", color: "text-emerald-600 dark:text-emerald-400" };
    if (score === 2)
      return { label: "Moderate password", color: "text-amber-600 dark:text-amber-400" };
    return { label: "Use 8+ characters with letters and numbers", color: "text-destructive" };
  };

  const strength = isRegister ? getPasswordStrength() : null;

  return (
    <Card className="w-full border border-border bg-card shadow-lg">
      <CardHeader className="space-y-1.5 pb-2 text-center">
        <CardTitle className="text-2xl font-bold tracking-tight">
          {isRegister ? "Create your account" : "Welcome back"}
        </CardTitle>
        <CardDescription className="text-sm">
          {isRegister
            ? "Join as a teacher or candidate to get started"
            : "Sign in to access your exams and sessions"}
        </CardDescription>
      </CardHeader>

      <CardContent className="space-y-5">
        {/* Tabs are used purely as a switcher; the form lives outside the Tabs root */}
        <Tabs value={mode} onValueChange={handleModeChange} className="w-full">
          <TabsList className="grid w-full grid-cols-2">
            <TabsTrigger value="login">Sign In</TabsTrigger>
            <TabsTrigger value="register">Register</TabsTrigger>
          </TabsList>
        </Tabs>

        {sessionExpired && !errorMessage && (
          <div
            role="status"
            aria-live="polite"
            className="flex items-start gap-2.5 rounded-lg border border-amber-500/30 bg-amber-500/10 p-3 text-sm text-amber-700 dark:text-amber-300"
          >
            <Clock className="mt-0.5 size-4 shrink-0" aria-hidden="true" />
            <span>Your session has expired. Please sign in again to continue.</span>
          </div>
        )}

        {errorMessage && (
          <div
            role="alert"
            aria-live="assertive"
            className="flex items-start gap-2.5 rounded-lg border border-destructive/20 bg-destructive/10 p-3 text-sm text-destructive"
          >
            <AlertCircle className="mt-0.5 size-4 shrink-0" aria-hidden="true" />
            <span>{errorMessage}</span>
          </div>
        )}

        <form onSubmit={handleSubmit} className="space-y-4" noValidate>
          {isRegister && (
            <>
              <fieldset className="space-y-2">
                <legend className="text-sm font-medium">I am a</legend>
                <div className="grid grid-cols-2 gap-2">
                  {ROLE_OPTIONS.map((option) => (
                    <Button
                      key={option.value}
                      type="button"
                      variant={role === option.value ? "default" : "outline"}
                      className="h-10 w-full"
                      aria-pressed={role === option.value}
                      onClick={() => setRole(option.value)}
                    >
                      {option.label}
                    </Button>
                  ))}
                </div>
              </fieldset>

              <div className="grid grid-cols-1 gap-4 sm:grid-cols-2">
                <div className="space-y-1.5">
                  <Label htmlFor="firstName">First name</Label>
                  <Input
                    id="firstName"
                    placeholder="Jane"
                    value={firstName}
                    onChange={(e) => {
                      setFirstName(e.target.value);
                      clearError("firstName");
                    }}
                    autoComplete="given-name"
                    aria-invalid={!!errors.firstName}
                    aria-describedby={errors.firstName ? "firstName-error" : undefined}
                  />
                  <FieldError id="firstName-error" message={errors.firstName} />
                </div>
                <div className="space-y-1.5">
                  <Label htmlFor="lastName">Last name</Label>
                  <Input
                    id="lastName"
                    placeholder="Doe"
                    value={lastName}
                    onChange={(e) => {
                      setLastName(e.target.value);
                      clearError("lastName");
                    }}
                    autoComplete="family-name"
                    aria-invalid={!!errors.lastName}
                    aria-describedby={errors.lastName ? "lastName-error" : undefined}
                  />
                  <FieldError id="lastName-error" message={errors.lastName} />
                </div>
              </div>
            </>
          )}

          <div className="space-y-1.5">
            <Label htmlFor="email">Email</Label>
            <Input
              id="email"
              type="email"
              placeholder="name@example.com"
              value={email}
              onChange={(e) => {
                setEmail(e.target.value);
                clearError("email");
              }}
              autoComplete="email"
              aria-invalid={!!errors.email}
              aria-describedby={errors.email ? "email-error" : undefined}
            />
            <FieldError id="email-error" message={errors.email} />
          </div>

          <div className="space-y-1.5">
            <Label htmlFor="password">Password</Label>
            <div className="relative">
              <Input
                id="password"
                type={showPassword ? "text" : "password"}
                placeholder="••••••••"
                value={password}
                onChange={(e) => {
                  setPassword(e.target.value);
                  clearError("password");
                }}
                autoComplete={isRegister ? "new-password" : "current-password"}
                className="pr-10"
                aria-invalid={!!errors.password}
                aria-describedby={
                  errors.password ? "password-error" : strength ? "password-strength" : undefined
                }
              />
              <Button
                type="button"
                variant="ghost"
                size="icon"
                className="absolute right-1 top-1/2 size-8 -translate-y-1/2 text-muted-foreground hover:text-foreground"
                aria-label={showPassword ? "Hide password" : "Show password"}
                aria-pressed={showPassword}
                onClick={() => setShowPassword((v) => !v)}
              >
                {showPassword ? (
                  <EyeOff className="size-4" aria-hidden="true" />
                ) : (
                  <Eye className="size-4" aria-hidden="true" />
                )}
              </Button>
            </div>
            <FieldError id="password-error" message={errors.password} />
            {!errors.password && strength && (
              <p id="password-strength" className={`text-xs font-medium ${strength.color}`}>
                {strength.label}
              </p>
            )}
          </div>

          <Button type="submit" className="h-10 w-full font-semibold" disabled={isLoading}>
            {isLoading ? (
              <>
                <Loader2 className="mr-2 size-4 animate-spin" aria-hidden="true" />
                Please wait…
              </>
            ) : isRegister ? (
              "Create account"
            ) : (
              "Sign in"
            )}
          </Button>
        </form>
      </CardContent>
    </Card>
  );
}

export default AuthForms;