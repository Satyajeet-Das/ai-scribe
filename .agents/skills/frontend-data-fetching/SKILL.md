---
name: frontend-data-fetching
description: >-
  Use this skill when implementing data fetching, caching, pagination, search,
  filtering, or real-time updates in the Next.js frontend. Covers SWR/React
  Query patterns, debouncing, optimistic updates, and error retry strategies.
---

# Frontend Data Fetching

## When to Activate

- Fetching data from the Go backend API
- Implementing paginated lists, search, or filters
- Adding optimistic updates or cache invalidation
- Debugging stale data, race conditions, or waterfall requests

---

## Core Rules

1. **All fetches go through `services/api.ts`.** Never call `fetch()` directly
   in components.
2. **Server components fetch on the server.** Client components use hooks
   (SWR / React Query / `useEffect`).
3. **Always handle: loading, error, empty, data.** No partial state coverage.
4. **Debounce user input** (search, filters) at 300ms minimum.
5. **Paginate everything.** Never fetch unbounded lists. Default: 20 items.

---

## Patterns

### API Fetch Wrapper

```ts
// services/api.ts
async function apiFetch<T>(path: string, init?: RequestInit): Promise<T> {
  const res = await fetch(`${API_BASE_URL}${path}`, {
    ...init,
    headers: {
      "Content-Type": "application/json",
      Authorization: `Bearer ${getStoredToken()}`,
      ...init?.headers,
    },
    credentials: "include",
  });

  if (!res.ok) {
    const body = await res.json().catch(() => ({}));
    throw new ApiError(body.message || res.statusText, res.status, body.code);
  }
  return res.json();
}
```

### Debounced Search

```ts
// hooks/use-debounce.ts
import { useState, useEffect } from "react";

export function useDebounce<T>(value: T, delayMs = 300): T {
  const [debounced, setDebounced] = useState(value);
  useEffect(() => {
    const id = setTimeout(() => setDebounced(value), delayMs);
    return () => clearTimeout(id);
  }, [value, delayMs]);
  return debounced;
}
```

Usage:

```tsx
const [search, setSearch] = useState("");
const debouncedSearch = useDebounce(search, 300);

useEffect(() => {
  fetchExams({ search: debouncedSearch, page: 1 });
}, [debouncedSearch]);
```

### Pagination

```tsx
interface PaginationParams {
  limit: number;
  offset: number;
}

interface PaginatedResponse<T> {
  items: T[];
  total: number;
  limit: number;
  offset: number;
}

function usePagination(initialLimit = 20) {
  const [page, setPage] = useState(1);
  const limit = initialLimit;
  const offset = (page - 1) * limit;
  return { page, setPage, limit, offset };
}
```

### Token Refresh on 401

```ts
async function apiFetchWithRetry<T>(path: string, init?: RequestInit): Promise<T> {
  try {
    return await apiFetch<T>(path, init);
  } catch (err) {
    if (err instanceof ApiError && err.status === 401) {
      const newToken = await authApi.refresh();
      if (newToken?.accessToken) {
        setStoredToken(newToken.accessToken);
        return apiFetch<T>(path, init); // retry once
      }
    }
    throw err;
  }
}
```

### Optimistic Updates

```ts
const handleRevoke = async (id: string) => {
  // Optimistic: remove from list immediately
  const previous = items;
  setItems((curr) => curr.filter((i) => i.id !== id));

  try {
    await assignmentsApi.revokeAssignment(id);
  } catch {
    // Rollback on failure
    setItems(previous);
    toast.error("Failed to revoke assignment");
  }
};
```

---

## Common Mistakes

| Mistake                                 | Fix                                              |
| --------------------------------------- | ------------------------------------------------ |
| Fetching in `useEffect` without cleanup | Return cleanup function or use AbortController   |
| No debounce on search input             | `useDebounce(search, 300)`                       |
| Fetching all items without pagination   | Always pass `limit` and `offset`                 |
| Ignoring race conditions                | Use AbortController or check request freshness   |
| Catching errors silently                | Always surface errors to UI with `ApiError`      |
| Hard-coding page size                   | Define in constants, allow user override          |
| No loading state on refetch             | Show subtle indicator, not full skeleton          |

---

## Production Checklist

- [ ] Every data fetch uses the typed `apiFetch` wrapper
- [ ] 401 responses trigger token refresh + single retry
- [ ] Search inputs are debounced (≥ 300ms)
- [ ] All lists are paginated with `limit`/`offset`
- [ ] Loading, error, and empty states handled for every fetch
- [ ] AbortController used for cancelable requests
- [ ] No console logs in production — use structured error reporting
- [ ] Optimistic updates have rollback on failure
