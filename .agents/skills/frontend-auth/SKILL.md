---
name: frontend-auth
description: >-
  Use this skill when implementing or reviewing authentication flows,
  protected routes, RBAC, session management, or token handling in the
  Next.js frontend. The Go backend owns all auth decisions — the frontend
  merely enforces UX guardrails.
---

# Frontend Auth

## When to Activate

- Implementing login, register, or logout flows
- Building protected route guards or RBAC wrappers
- Handling token storage, refresh, or expiration
- Reviewing auth-related security (XSS, open redirect, token leakage)

---

## Core Principle

> **The frontend never makes auth decisions.** It stores the token, sends it
> with requests, and redirects on 401. The Go backend validates, authorises,
> and decides.

---

## Token Management

### Storage Rules

| Storage           | Use for                  | Risk                  |
| ----------------- | ------------------------ | --------------------- |
| `httpOnly` cookie | Refresh token            | Safest — not JS-accessible |
| In-memory variable | Access token (runtime)  | Lost on tab close (acceptable) |
| `localStorage`    | Persisted session hint   | XSS-readable — store only non-sensitive user info |

### Pattern: Dual-Layer Token Storage

```ts
// In-memory for runtime security
let inMemoryToken: string | null = null;

export function getStoredToken(): string | null {
  if (inMemoryToken) return inMemoryToken;
  // Fallback: rehydrate from localStorage after page reload
  if (typeof window !== "undefined") {
    inMemoryToken = localStorage.getItem("access_token");
  }
  return inMemoryToken;
}

export function setStoredToken(token: string) {
  inMemoryToken = token;
  localStorage.setItem("access_token", token);
}

export function clearStoredToken() {
  inMemoryToken = null;
  localStorage.removeItem("access_token");
}
```

---

## Auth Store (Zustand)

```ts
interface AuthState {
  user: User | null;
  accessToken: string | null;
  isAuthenticated: boolean;
  isLoading: boolean;
  isHydrated: boolean; // CRITICAL: prevents SSR mismatch
  error: string | null;

  login: (creds: LoginCredentials) => Promise<User>;
  register: (payload: RegisterPayload) => Promise<User>;
  logout: () => void;
  refreshToken: () => Promise<string | null>;
  hasRole: (roles: string[]) => boolean;
}
```

### Hydration Safety

Always check `isHydrated` before rendering auth-dependent UI:

```tsx
const { isAuthenticated, isHydrated } = useAuthStore();

if (!isHydrated) return <Skeleton />;
if (!isAuthenticated) return <Redirect to="/login" />;
```

---

## Protected Routes

```tsx
"use client";

import { useAuthStore } from "@/store/auth-store";
import { useRouter } from "next/navigation";
import { useEffect } from "react";

interface ProtectedRouteProps {
  children: React.ReactNode;
  allowedRoles?: string[];
}

export function ProtectedRoute({ children, allowedRoles }: ProtectedRouteProps) {
  const router = useRouter();
  const { isAuthenticated, isHydrated, user } = useAuthStore();

  useEffect(() => {
    if (!isHydrated) return;
    if (!isAuthenticated) {
      router.replace(`/login?returnUrl=${encodeURIComponent(window.location.pathname)}`);
      return;
    }
    if (allowedRoles && user && !allowedRoles.includes(user.role)) {
      router.replace("/unauthorized");
    }
  }, [isHydrated, isAuthenticated, user, allowedRoles, router]);

  if (!isHydrated) return <LoadingSkeleton />;
  if (!isAuthenticated) return null;
  if (allowedRoles && user && !allowedRoles.includes(user.role)) return null;

  return <>{children}</>;
}
```

---

## Login Flow

1. User submits credentials → `authStore.login(creds)`
2. `services/api.ts` POSTs to `/auth/login`, receives `{ accessToken, user }`
3. Token stored in memory + localStorage
4. Zustand sets `user`, `isAuthenticated = true`
5. Router navigates to `returnUrl` or role-based default

## Logout Flow

1. `authStore.logout()` calls `authApi.logout()` (POST)
2. Clears in-memory token + localStorage
3. Resets Zustand state
4. Router navigates to `/login`

## Token Refresh Flow

1. API call returns 401
2. `apiFetch` catches, calls `authApi.refresh()` (uses httpOnly cookie)
3. New access token stored, original request retried once
4. If refresh fails → full logout + redirect

---

## Security Rules

1. **Validate `returnUrl`** — only allow same-site relative paths:
   ```ts
   function isSafeReturnUrl(url: string | null): boolean {
     if (!url) return false;
     return url.startsWith("/") && !url.startsWith("//") && !url.startsWith("/\\");
   }
   ```
2. **Never log tokens** — not even in development.
3. **Clear auth state on any 401** that can't be refreshed.
4. **Never trust client-side role checks for security** — they're UX only.
   Backend must enforce.
5. **CSRF:** Use `SameSite=Strict` cookies + `credentials: "include"`.

---

## Common Mistakes

| Mistake                                | Fix                                              |
| -------------------------------------- | ------------------------------------------------ |
| Storing access token only in cookie    | Use in-memory for speed, cookie for refresh only |
| Rendering auth UI before hydration     | Gate on `isHydrated` to prevent flash            |
| Open redirect via `returnUrl`          | Validate: must start with `/`, not `//`          |
| Logging out without clearing all state | Reset Zustand + clear localStorage + clear token |
| Role checks without backend enforcement| Client RBAC is UX only — backend must verify     |

---

## Production Checklist

- [ ] Access token stored in-memory with localStorage fallback
- [ ] Refresh token in `httpOnly` cookie (when backend supports it)
- [ ] `isHydrated` check before any auth-dependent rendering
- [ ] `ProtectedRoute` wrapper on all gated pages
- [ ] `returnUrl` validated against open redirect
- [ ] 401 triggers single refresh attempt, then logout
- [ ] Logout clears all auth artifacts (memory, localStorage, cookies)
- [ ] Role-based UI hiding is UX only — not a security boundary
- [ ] No tokens in console.log, error reports, or URL params
