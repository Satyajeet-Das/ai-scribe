"use client";

import { useEffect } from "react";
import { useRouter } from "next/navigation";
import { useAuthStore } from "@/store/auth-store";
import { getRolePortal } from "@/lib/constants";

/**
 * Renders nothing. Sends already-signed-in users straight to their portal.
 * Lives in its own client component so the landing page itself can stay a Server Component.
 */
export function AuthRedirect() {
  const router = useRouter();
  const user = useAuthStore((s) => s.user);
  const isAuthenticated = useAuthStore((s) => s.isAuthenticated);
  const isHydrated = useAuthStore((s) => s.isHydrated);

  useEffect(() => {
    if (isHydrated && isAuthenticated && user) {
      router.replace(getRolePortal(user.role));
    }
  }, [isHydrated, isAuthenticated, user, router]);

  return null;
}