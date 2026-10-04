"use client";

import React, { useEffect, useSyncExternalStore } from "react";
import { useRouter, usePathname } from "next/navigation";
import { useAuthStore } from "@/store/auth-store";
import { getRolePortal } from "@/lib/constants";
import { ShieldAlert, LogIn, ArrowLeft } from "lucide-react";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardHeader, CardTitle, CardDescription } from "@/components/ui/card";
import { Skeleton } from "@/components/ui/skeleton";
import type { Role } from "@/types/auth";

interface ProtectedRouteProps {
  children: React.ReactNode;
  allowedRoles?: Role[];
}

// React 19 / 18 idiomatic client-side detection without setState in useEffect
function useIsClient(): boolean {
  return useSyncExternalStore(
    () => () => {},
    () => true,
    () => false
  );
}

export function ProtectedRoute({ children, allowedRoles }: ProtectedRouteProps) {
  const router = useRouter();
  const pathname = usePathname();
  const { user, isAuthenticated, isHydrated, status } = useAuthStore();
  const isClient = useIsClient();

  const isCheckingAuth = !isClient || !isHydrated || status === "restoring";

  useEffect(() => {
    if (!isCheckingAuth && !isAuthenticated) {
      router.replace(`/login?returnUrl=${encodeURIComponent(pathname)}`);
    }
  }, [isCheckingAuth, isAuthenticated, router, pathname]);

  // Loading skeleton while rehydrating or restoring auth state
  if (isCheckingAuth) {
    return (
      <div className="mx-auto max-w-7xl px-4 py-8 sm:px-6 lg:px-8 space-y-6">
        <div className="space-y-2">
          <Skeleton className="h-8 w-64" />
          <Skeleton className="h-4 w-96" />
        </div>
        <div className="grid gap-4 md:grid-cols-3">
          <Skeleton className="h-28 rounded-xl" />
          <Skeleton className="h-28 rounded-xl" />
          <Skeleton className="h-28 rounded-xl" />
        </div>
        <Skeleton className="h-96 rounded-xl" />
      </div>
    );
  }

  // Not authenticated fallback
  if (!isAuthenticated) {
    return (
      <div className="flex min-h-[60vh] items-center justify-center p-4">
        <Card className="max-w-md w-full text-center shadow-md">
          <CardHeader>
            <div className="mx-auto flex size-12 items-center justify-center rounded-full bg-primary/10 text-primary mb-2">
              <LogIn className="size-6" aria-hidden="true" />
            </div>
            <CardTitle>Authentication Required</CardTitle>
            <CardDescription>Please log in to access this page.</CardDescription>
          </CardHeader>
          <CardContent>
            <Button
              className="w-full font-semibold"
              onClick={() => router.replace(`/login?returnUrl=${encodeURIComponent(pathname)}`)}
            >
              Go to Login
            </Button>
          </CardContent>
        </Card>
      </div>
    );
  }

  // Role check: if user's role is not permitted
  if (allowedRoles && user && !allowedRoles.includes(user.role)) {
    const defaultRoute = getRolePortal(user.role);
    const portalName = user.role === "TEACHER" || user.role === "ADMIN" ? "Educator" : "Candidate";

    return (
      <div className="flex min-h-[60vh] items-center justify-center p-4">
        <Card className="max-w-md w-full text-center border-destructive/20 shadow-md">
          <CardHeader>
            <div className="mx-auto flex size-12 items-center justify-center rounded-full bg-destructive/10 text-destructive mb-2">
              <ShieldAlert className="size-6" aria-hidden="true" />
            </div>
            <CardTitle className="text-xl">Access Restricted</CardTitle>
            <CardDescription>
              Your account role (<strong>{user.role}</strong>) does not have permission to view this
              section.
            </CardDescription>
          </CardHeader>
          <CardContent className="space-y-3">
            <Button className="w-full font-semibold" onClick={() => router.replace(defaultRoute)}>
              <ArrowLeft className="mr-2 size-4" aria-hidden="true" />
              Go to your portal ({portalName})
            </Button>
          </CardContent>
        </Card>
      </div>
    );
  }

  return <>{children}</>;
}
export default ProtectedRoute;
