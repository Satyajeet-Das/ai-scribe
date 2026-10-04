---
name: nextjs-production
description: >-
  Use this skill when building, configuring, or reviewing Next.js applications
  for production readiness. Covers App Router patterns, server/client component
  boundaries, rendering strategies, performance optimisation, and deployment
  configuration.
---

# Next.js Production

## When to Activate

- Creating or modifying Next.js pages, layouts, or route handlers
- Configuring `next.config.ts`, middleware, or environment variables
- Reviewing a Next.js app for production readiness
- Debugging hydration, rendering, or build issues

---

## Core Rules

### Server vs Client Components

1. **Default to Server Components.** Only add `"use client"` when you need:
   - Browser APIs (`window`, `localStorage`, `IntersectionObserver`)
   - React hooks (`useState`, `useEffect`, `useRef`, custom hooks)
   - Event handlers (`onClick`, `onChange`, `onSubmit`)
   - Third-party client-only libraries
2. **Push `"use client"` to the leaf.** Never put it on layouts or pages unless
   truly necessary — extract the interactive piece into a small client component.
3. **Never import server-only code into a `"use client"` file.** This includes
   database clients, secret env vars, or Node-only modules.

### Rendering Strategy

| Route type          | Strategy                | When                                      |
| ------------------- | ----------------------- | ----------------------------------------- |
| Marketing / docs    | Static (SSG)            | Content rarely changes                    |
| Dashboard / feeds   | Dynamic (SSR) or CSR    | Data changes per request or per user      |
| API proxy           | Route Handler           | Thin proxy to Go backend                  |
| Auth-gated pages    | Middleware + SSR        | Must validate session before render       |

### File Conventions

```
app/
├── (public)/           # Route group – no auth required
│   └── page.tsx        # Landing
├── (dashboard)/        # Route group – auth required
│   ├── layout.tsx      # Shared sidebar/header (server component)
│   └── exams/
│       ├── page.tsx    # List page
│       └── [id]/
│           └── page.tsx
├── (auth)/
│   └── login/
│       └── page.tsx
├── layout.tsx          # Root layout (server component, <html>, <body>)
├── not-found.tsx
├── error.tsx           # Must be "use client"
└── loading.tsx
```

---

## Production Patterns

### Environment Variables

```ts
// lib/env.ts — validated at build/boot time
const env = {
  API_URL: process.env.NEXT_PUBLIC_API_URL!,
  // Server-only (no NEXT_PUBLIC_ prefix):
  SESSION_SECRET: process.env.SESSION_SECRET!,
} as const;

// Fail fast
for (const [key, val] of Object.entries(env)) {
  if (!val) throw new Error(`Missing env var: ${key}`);
}
export default env;
```

### Metadata & SEO

Every page must export or generate metadata:

```ts
export const metadata: Metadata = {
  title: "Exams | AI Scribe",
  description: "Manage and publish exams",
};
```

For dynamic pages use `generateMetadata`:

```ts
export async function generateMetadata({ params }: Props): Promise<Metadata> {
  const exam = await getExam(params.id);
  return { title: `${exam.title} | AI Scribe` };
}
```

### Image Optimisation

- Always use `next/image` with explicit `width`/`height` or `fill`.
- Serve images from `/public` or a CDN; never inline base64 for images > 4 KB.

### Route Handlers (API Routes)

```ts
// app/api/proxy/[...path]/route.ts
import { NextRequest, NextResponse } from "next/server";

export async function GET(req: NextRequest) {
  const backendRes = await fetch(`${process.env.BACKEND_URL}${req.nextUrl.pathname}`, {
    headers: { Authorization: req.headers.get("Authorization") ?? "" },
  });
  return NextResponse.json(await backendRes.json(), { status: backendRes.status });
}
```

---

## Common Mistakes

| Mistake                                    | Fix                                                        |
| ------------------------------------------ | ---------------------------------------------------------- |
| `"use client"` on layout or page           | Extract interactive parts into leaf client components       |
| Fetching data client-side when SSR suffices | Use server components or route handlers                     |
| Missing `loading.tsx`                       | Always provide a loading UI per route segment               |
| Hard-coded API URLs                        | Use `NEXT_PUBLIC_*` env vars via a validated config module  |
| No `error.tsx` boundary                    | Every route group needs an error boundary                   |
| Forgetting `Suspense` around `useSearchParams` | Wrap in `<Suspense>` to avoid full-page deopts          |

---

## Production Checklist

- [ ] Every page has `metadata` or `generateMetadata`
- [ ] `error.tsx` and `not-found.tsx` exist at root and per route group
- [ ] `loading.tsx` or `<Suspense>` skeletons for every async boundary
- [ ] No secrets in `NEXT_PUBLIC_*` variables
- [ ] `next.config.ts` has `output: "standalone"` for Docker deployments
- [ ] Middleware handles auth redirects and CSP headers
- [ ] Images use `next/image`; no unoptimised `<img>` tags
- [ ] Bundle analysed with `@next/bundle-analyzer` — no page > 200 KB JS
- [ ] `suppressHydrationWarning` on `<html>` and `<body>` only (for extensions)
- [ ] All dynamic routes have proper `generateStaticParams` or are opt-out
