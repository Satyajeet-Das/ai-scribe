"use client";

import React, { useState } from "react";
import Link from "next/link";
import { useRouter } from "next/navigation";
import { useAuthStore } from "@/store/auth-store";
import { APP_CONFIG, DESIGN_TOKENS, getRolePortal } from "@/lib/constants";
import { LogIn, LogOut, Menu, X, User as UserIcon, BookOpen, Clock } from "lucide-react";
import { Button } from "@/components/ui/button";
import { Badge } from "@/components/ui/badge";

export function Header() {
  const router = useRouter();
  const { user, isAuthenticated, logout, isHydrated } = useAuthStore();
  const [mobileMenuOpen, setMobileMenuOpen] = useState(false);

  const portalRoute = isHydrated && isAuthenticated && user ? getRolePortal(user.role) : "/";

  const handleLogout = () => {
    logout();
    setMobileMenuOpen(false);
    router.push("/login");
  };

  return (
    <header className="sticky top-0 z-40 w-full border-b border-border bg-background/80 backdrop-blur-md">
      <div className={DESIGN_TOKENS.layout.container}>
        <div className="flex h-16 items-center justify-between">
          {/* Logo / Brand - takes logged in users directly to their portal */}
          <div className="flex items-center gap-6">
            <Link href={portalRoute} className="flex items-center gap-2.5">
              <div className="flex size-9 items-center justify-center rounded-xl bg-primary text-primary-foreground font-bold text-sm shadow-sm">
                AS
              </div>
              <div className="flex items-center gap-2">
                <span className="font-bold text-lg tracking-tight text-foreground">
                  {APP_CONFIG.name}
                </span>
                <span className="hidden sm:inline-block text-[10px] font-semibold uppercase tracking-wider rounded bg-primary/10 text-primary px-1.5 py-0.5">
                  v{APP_CONFIG.version}
                </span>
              </div>
            </Link>

            {/* Desktop Navigation */}
            <nav className="hidden md:flex items-center gap-5 text-sm font-medium">
              {isHydrated && isAuthenticated && (user?.role === "TEACHER" || user?.role === "ADMIN") && (
                <Link
                  href="/exams"
                  className="text-muted-foreground hover:text-foreground transition-colors flex items-center gap-1.5"
                >
                  <BookOpen className="size-4" />
                  Educator Portal
                </Link>
              )}
              {isHydrated && isAuthenticated && user?.role === "STUDENT" && (
                <Link
                  href="/sessions"
                  className="text-muted-foreground hover:text-foreground transition-colors flex items-center gap-1.5"
                >
                  <Clock className="size-4" />
                  Candidate Assessments
                </Link>
              )}
              {(!isHydrated || !isAuthenticated) && (
                <>
                  <Link href="/exams" className="text-muted-foreground hover:text-foreground transition-colors">
                    Educator Portal
                  </Link>
                  <Link href="/sessions" className="text-muted-foreground hover:text-foreground transition-colors">
                    Candidate Assessments
                  </Link>
                </>
              )}
            </nav>
          </div>

          {/* Desktop Auth Controls */}
          <div className="hidden md:flex items-center gap-3">
            {isHydrated && isAuthenticated && user ? (
              <div className="flex items-center gap-3">
                <div className="flex items-center gap-2 text-right">
                  <div className="flex flex-col">
                    <span className="text-xs font-medium text-foreground">
                      {user.firstName} {user.lastName}
                    </span>
                    <Badge variant="outline" className="text-[10px] py-0 px-1 w-fit self-end font-semibold">
                      {user.role}
                    </Badge>
                  </div>
                  <div className="flex size-8 items-center justify-center rounded-full bg-muted text-muted-foreground">
                    <UserIcon className="size-4" />
                  </div>
                </div>
                <Button
                  variant="ghost"
                  size="sm"
                  onClick={handleLogout}
                  className="text-muted-foreground hover:text-foreground"
                >
                  <LogOut className="size-4 mr-1.5" />
                  Sign Out
                </Button>
              </div>
            ) : (
              <div className="flex items-center gap-2">
                <Link
                  href="/login"
                  className="inline-flex h-7 items-center gap-1 rounded-[min(var(--radius-md),12px)] border border-border bg-background px-2.5 text-[0.8rem] font-medium hover:bg-muted transition-colors"
                >
                  Sign In
                </Link>
                <Link
                  href="/login?mode=register"
                  className="inline-flex h-7 items-center gap-1 rounded-[min(var(--radius-md),12px)] bg-primary text-primary-foreground px-2.5 text-[0.8rem] font-medium hover:bg-primary/80 transition-colors"
                >
                  Get Started
                </Link>
              </div>
            )}
          </div>

          {/* Mobile Hamburger */}
          <div className="flex md:hidden items-center gap-2">
            <Button
              variant="ghost"
              size="icon"
              aria-label="Toggle menu"
              onClick={() => setMobileMenuOpen(!mobileMenuOpen)}
            >
              {mobileMenuOpen ? <X className="size-5" /> : <Menu className="size-5" />}
            </Button>
          </div>
        </div>
      </div>

      {/* Mobile Drawer */}
      {mobileMenuOpen && (
        <div className="md:hidden border-b border-border bg-background px-4 py-4 space-y-3 animate-in slide-in-from-top-2 duration-200">
          <nav className="flex flex-col space-y-2">
            {isHydrated && isAuthenticated && (user?.role === "TEACHER" || user?.role === "ADMIN") && (
              <Link
                href="/exams"
                onClick={() => setMobileMenuOpen(false)}
                className="px-3 py-2 rounded-md text-sm font-medium hover:bg-muted text-foreground flex items-center gap-2"
              >
                <BookOpen className="size-4" />
                Educator Portal
              </Link>
            )}
            {isHydrated && isAuthenticated && user?.role === "STUDENT" && (
              <Link
                href="/sessions"
                onClick={() => setMobileMenuOpen(false)}
                className="px-3 py-2 rounded-md text-sm font-medium hover:bg-muted text-foreground flex items-center gap-2"
              >
                <Clock className="size-4" />
                Candidate Assessments
              </Link>
            )}
            {(!isHydrated || !isAuthenticated) && (
              <>
                <Link
                  href="/exams"
                  onClick={() => setMobileMenuOpen(false)}
                  className="px-3 py-2 rounded-md text-sm font-medium hover:bg-muted text-foreground flex items-center gap-2"
                >
                  <BookOpen className="size-4" />
                  Educator Portal
                </Link>
                <Link
                  href="/sessions"
                  onClick={() => setMobileMenuOpen(false)}
                  className="px-3 py-2 rounded-md text-sm font-medium hover:bg-muted text-foreground flex items-center gap-2"
                >
                  <Clock className="size-4" />
                  Candidate Assessments
                </Link>
              </>
            )}
          </nav>

          <div className="border-t border-border pt-3">
            {isHydrated && isAuthenticated && user ? (
              <div className="flex flex-col space-y-2">
                <div className="px-3 py-1 flex items-center justify-between">
                  <span className="text-sm font-semibold">{user.firstName} {user.lastName}</span>
                  <Badge variant="outline">{user.role}</Badge>
                </div>
                <Button variant="outline" size="sm" onClick={handleLogout} className="w-full justify-center">
                  <LogOut className="size-4 mr-2" />
                  Sign Out
                </Button>
              </div>
            ) : (
              <div className="grid grid-cols-2 gap-2">
                <Link
                  href="/login"
                  onClick={() => setMobileMenuOpen(false)}
                  className="inline-flex h-7 items-center justify-center rounded-[min(var(--radius-md),12px)] border border-border bg-background px-2.5 text-[0.8rem] font-medium hover:bg-muted transition-colors"
                >
                  Sign In
                </Link>
                <Link
                  href="/login?mode=register"
                  onClick={() => setMobileMenuOpen(false)}
                  className="inline-flex h-7 items-center justify-center rounded-[min(var(--radius-md),12px)] bg-primary text-primary-foreground px-2.5 text-[0.8rem] font-medium hover:bg-primary/80 transition-colors"
                >
                  Register
                </Link>
              </div>
            )}
          </div>
        </div>
      )}
    </header>
  );
}
export default Header;
