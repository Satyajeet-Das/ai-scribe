"use client";

import { Suspense } from "react";
import Link from "next/link";
import { useRouter } from "next/navigation";
import { ShieldAlert, ArrowLeft, LogOut } from "lucide-react";
import { Button, buttonVariants } from "@/components/ui/button";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";
import { Badge } from "@/components/ui/badge";
import { useAuthStore } from "@/store/auth-store";
import { DESIGN_TOKENS, getRolePortal } from "@/lib/constants";
import { cn } from "@/lib/utils";

function ForbiddenContent() {
  const router = useRouter();
  const { user, logout, isHydrated } = useAuthStore();

  const portalRoute = isHydrated && user ? getRolePortal(user.role) : "/";
  const portalName = user?.role === "TEACHER" || user?.role === "ADMIN" ? "Educator Portal" : "Candidate Portal";

  const handleLogout = async () => {
    await logout();
    router.push("/login");
  };

  return (
    <main className="flex min-h-screen items-center justify-center bg-background px-4 py-12">
      <div className={DESIGN_TOKENS.layout.containerNarrow}>
        <Card className="w-full max-w-md mx-auto text-center border-destructive/20 shadow-lg">
          <CardHeader className="space-y-2 pb-4">
            <div className="mx-auto flex size-14 items-center justify-center rounded-full bg-destructive/10 text-destructive">
              <ShieldAlert className="size-7" aria-hidden="true" />
            </div>
            <CardTitle className="text-2xl font-bold tracking-tight">Access Restricted</CardTitle>
            <CardDescription className="text-sm">
              Your account does not have authorization to view this section.
            </CardDescription>
          </CardHeader>
          <CardContent className="space-y-3 pt-2">
            {isHydrated && user && (
              <div className="rounded-lg bg-muted/60 p-3 text-xs flex items-center justify-between">
                <span className="text-muted-foreground">Current Role:</span>
                <Badge variant="outline" className="font-semibold text-xs">
                  {user.role}
                </Badge>
              </div>
            )}

            <Link
              href={portalRoute}
              className={cn(buttonVariants({ variant: "default", size: "lg" }), "w-full font-semibold")}
            >
              <ArrowLeft className="mr-2 size-4" aria-hidden="true" />
              Go to your {portalName}
            </Link>

            <Button
              type="button"
              variant="outline"
              size="lg"
              onClick={handleLogout}
              className="w-full text-muted-foreground hover:text-foreground"
            >
              <LogOut className="mr-2 size-4" aria-hidden="true" />
              Sign Out & Switch Account
            </Button>
          </CardContent>
        </Card>
      </div>
    </main>
  );
}

export default function ForbiddenPage() {
  return (
    <Suspense
      fallback={
        <div className="flex min-h-screen items-center justify-center">
          <div
            role="status"
            aria-label="Loading"
            className="size-8 animate-spin rounded-full border-4 border-primary border-t-transparent"
          />
        </div>
      }
    >
      <ForbiddenContent />
    </Suspense>
  );
}
