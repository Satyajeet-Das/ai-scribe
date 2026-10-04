"use client";

import { useEffect, useRef } from "react";
import { useAuthStore } from "@/store/auth-store";

interface AuthProviderProps {
  children: React.ReactNode;
}

export function AuthProvider({ children }: AuthProviderProps) {
  const restoreSession = useAuthStore((s) => s.restoreSession);
  const isHydrated = useAuthStore((s) => s.isHydrated);
  const initialized = useRef(false);

  useEffect(() => {
    if (!initialized.current && isHydrated) {
      initialized.current = true;
      restoreSession().catch(() => {
        // Silently handled within restoreSession
      });
    }
  }, [isHydrated, restoreSession]);

  return <>{children}</>;
}
