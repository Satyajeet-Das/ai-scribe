---
name: frontend-architecture
description: >-
  Use this skill when designing or reviewing frontend project structure,
  component architecture, state management, form handling, and module
  boundaries in a Next.js + TypeScript application. Covers Zustand, Zod,
  constants, and the "dumb frontend" principle.
---

# Frontend Architecture

## When to Activate

- Structuring a new feature module (components, hooks, types, services)
- Deciding where state lives (server vs client vs global store)
- Designing form validation or API integration patterns
- Reviewing imports, barrel files, or circular dependency risks

---

## Core Principles

1. **The frontend is a thin client.** Auth decisions, business rules, and data
   integrity live in the Go backend. The frontend renders, collects input, and
   forwards to the API.
2. **Collocate by feature, not by layer.** Group related components, hooks,
   types, and services together — not in global `/components`, `/hooks`, etc.
3. **Types are the contract.** Every API response and request payload must have a
   TypeScript interface. Never use `any`.
4. **Single source of truth for constants.** Design tokens, API routes, storage
   keys, and config live in `lib/constants.ts`.

---

## Directory Convention

```
src/
├── app/                  # Next.js App Router pages
├── components/
│   ├── ui/               # Primitives (Button, Input, Card) — no business logic
│   └── layout/           # Shell components (Header, Footer, Sidebar)
├── features/             # Feature modules (optional, for large apps)
│   └── exams/
│       ├── components/
│       ├── hooks/
│       └── types.ts
├── hooks/                # Shared custom hooks
├── lib/
│   ├── constants.ts      # Design tokens, API routes, config
│   ├── utils.ts          # Pure utility functions
│   └── validations.ts    # Zod schemas
├── services/
│   └── api.ts            # HTTP client, typed API functions
├── store/
│   └── auth-store.ts     # Zustand stores
└── types/
    └── exam-types.ts     # Shared domain types
```

---

## State Management (Zustand)

### Rules

1. **One store per concern** — `auth-store`, `ui-store`, `exam-store`. Never one
   monolithic store.
2. **Persist only what survives a refresh** — user session, theme preference.
   Never persist derived or loading state.
3. **Hydration guard** — always track `isHydrated` when using `persist` to avoid
   SSR/CSR mismatch.

```ts
// Pattern: hydration-safe Zustand store
export const useAuthStore = create<AuthState>()(
  persist(
    (set, get) => ({
      user: null,
      isHydrated: false,
      setHydrated: (v: boolean) => set({ isHydrated: v }),
      // ... actions
    }),
    {
      name: "auth-storage",
      storage: createJSONStorage(() => localStorage),
      partialize: (s) => ({ user: s.user, accessToken: s.accessToken }),
      onRehydrateStorage: () => (state) => state?.setHydrated(true),
    }
  )
);
```

### Anti-Patterns

- ❌ Storing API response caches in Zustand (use React Query or SWR)
- ❌ Putting `isLoading` in persisted state
- ❌ Reading store inside server components

---

## Form Validation (Zod)

1. **One schema per form** in `lib/validations.ts`.
2. **Validate on submit, clear on field change.** Never validate on every
   keystroke — it's noisy.
3. **Reuse schemas** for both client validation and typing:

```ts
import { z } from "zod";

export const LoginSchema = z.object({
  email: z.string().email("Enter a valid email"),
  password: z.string().min(8, "Password must be at least 8 characters"),
});
export type LoginFormData = z.infer<typeof LoginSchema>;
```

---

## API Service Layer

### Rules

1. All HTTP calls go through `services/api.ts` — components never call `fetch`
   directly.
2. Every API function is typed: `(params: X) => Promise<Y>`.
3. Token management is handled inside the service layer, not in components.
4. Errors are thrown as typed `ApiError` instances with `status` and `code`.

```ts
export class ApiError extends Error {
  constructor(
    message: string,
    public status: number,
    public code?: string
  ) {
    super(message);
    this.name = "ApiError";
  }
}
```

---

## Import Rules

1. **Use path aliases** (`@/components/...`) — never relative paths deeper than
   one level (`../`).
2. **No circular imports.** Types flow: `types/ → services/ → store/ →
   components/`. Never backwards.
3. **No barrel re-exports** (`index.ts`) unless the module is a public API (like
   `ui/`).

---

## Production Checklist

- [ ] Every exported component has explicit TypeScript props
- [ ] No `any` types — use `unknown` and narrow
- [ ] Zustand stores have `isHydrated` guard for SSR safety
- [ ] Zod schemas exist for every user-facing form
- [ ] `ApiError` is caught and rendered with user-friendly messages
- [ ] Constants file has no magic strings — all API routes, storage keys, design
      tokens centralised
- [ ] No business logic in components — delegate to services/store
- [ ] Imports follow the one-way dependency graph
