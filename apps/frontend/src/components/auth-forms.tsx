"use client";

import { useState } from "react";
import { Eye, EyeOff, LoaderCircle } from "lucide-react";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Tabs, TabsContent, TabsList, TabsTrigger } from "@/components/ui/tabs";
export type AuthRole = "TEACHER" | "STUDENT";
export function AuthForms({
  onSubmit,
}: {
  onSubmit?: (data: {
    mode: "login" | "register";
    email: string;
    password: string;
    role: AuthRole;
    firstName?: string;
  }) => void;
}) {
  const [mode, setMode] = useState<"login" | "register">("login");
  const [role, setRole] = useState<AuthRole>("STUDENT");
  const [email, setEmail] = useState("");
  const [password, setPassword] = useState("");
  const [firstName, setFirstName] = useState("");
  const [showPassword, setShowPassword] = useState(false);
  const [error, setError] = useState("");
  const [loading, setLoading] = useState(false);
  const strength =
    password.length >= 12
      ? "Strong password"
      : password.length >= 8
        ? "Good password"
        : password
          ? "Use 8+ characters"
          : "";
  const submit = async (event: React.FormEvent) => {
    event.preventDefault();
    setError("");
    if (!email.includes("@")) return setError("Enter a valid email address.");
    if (password.length < 8) return setError("Password must be at least 8 characters.");
    if (mode === "register" && !firstName.trim()) return setError("Enter your first name.");
    setLoading(true);
    await new Promise((resolve) => setTimeout(resolve, 700));
    onSubmit?.({ mode, email, password, role, firstName: firstName || undefined });
    setLoading(false);
  };
  return (
    <Card className="mx-auto w-full max-w-md shadow-lg">
      <CardHeader>
        <CardTitle className="text-2xl">Welcome to AI Exam Scribe</CardTitle>
        <CardDescription>Accessible assessment tools for every learner.</CardDescription>
      </CardHeader>
      <CardContent>
        <Tabs value={mode} onValueChange={(value) => setMode(value as typeof mode)}>
          <TabsList className="grid w-full grid-cols-2">
            <TabsTrigger value="login">Log in</TabsTrigger>
            <TabsTrigger value="register">Register</TabsTrigger>
          </TabsList>
          <form onSubmit={submit} className="mt-6 flex flex-col gap-5" noValidate>
            <div className="flex flex-col gap-2">
              <Label>Role</Label>
              <div className="grid grid-cols-2 gap-2" role="group" aria-label="Choose role">
                <Button
                  type="button"
                  variant={role === "STUDENT" ? "default" : "outline"}
                  onClick={() => setRole("STUDENT")}
                >
                  Student
                </Button>
                <Button
                  type="button"
                  variant={role === "TEACHER" ? "default" : "outline"}
                  onClick={() => setRole("TEACHER")}
                >
                  Teacher
                </Button>
              </div>
            </div>
            {mode === "register" && (
              <div className="flex flex-col gap-2">
                <Label htmlFor="first-name">First name</Label>
                <Input
                  id="first-name"
                  value={firstName}
                  onChange={(event) => setFirstName(event.target.value)}
                  autoComplete="given-name"
                />
              </div>
            )}
            <div className="flex flex-col gap-2">
              <Label htmlFor="auth-email">Email</Label>
              <Input
                id="auth-email"
                type="email"
                value={email}
                onChange={(event) => setEmail(event.target.value)}
                autoComplete="email"
                aria-invalid={Boolean(error && !email.includes("@"))}
              />
            </div>
            <div className="flex flex-col gap-2">
              <Label htmlFor="auth-password">Password</Label>
              <div className="relative">
                <Input
                  id="auth-password"
                  className="pr-11"
                  type={showPassword ? "text" : "password"}
                  value={password}
                  onChange={(event) => setPassword(event.target.value)}
                  autoComplete={mode === "login" ? "current-password" : "new-password"}
                  aria-invalid={Boolean(error && password.length < 8)}
                />
                <Button
                  type="button"
                  variant="ghost"
                  size="icon"
                  className="absolute right-1 top-1"
                  aria-label={showPassword ? "Hide password" : "Show password"}
                  onClick={() => setShowPassword(!showPassword)}
                >
                  {showPassword ? <EyeOff /> : <Eye />}
                </Button>
              </div>
              {mode === "register" && (
                <p className="text-xs text-muted-foreground" aria-live="polite">
                  {strength}
                </p>
              )}
            </div>
            {error && (
              <p role="alert" className="text-sm text-destructive">
                {error}
              </p>
            )}
            <Button type="submit" disabled={loading}>
              {loading && <LoaderCircle className="animate-spin" data-icon="inline-start" />}
              {loading ? "Please wait" : mode === "login" ? "Log in" : "Create account"}
            </Button>
          </form>
        </Tabs>
      </CardContent>
    </Card>
  );
}
export default AuthForms;
