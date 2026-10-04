---
name: observability
description: >-
  Use this skill when implementing or reviewing logging, metrics, tracing,
  health checks, or error reporting for the Go backend and Next.js frontend.
  Covers structured logging with slog, request tracing, health endpoints,
  and production monitoring patterns.
---

# Observability

## When to Activate

- Adding or reviewing logging in backend or frontend
- Implementing health check endpoints
- Setting up request tracing or correlation IDs
- Configuring error reporting or alerting
- Debugging production issues via logs

---

## Structured Logging (Go Backend)

### Use `log/slog`

```go
import "log/slog"

// Initialise once in main()
logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
    Level: slog.LevelInfo,
}))
slog.SetDefault(logger)
```

### Rules

1. **JSON format in production.** Text format only for local development.
2. **Log levels:**
   - `DEBUG` — detailed diagnostic info (off in production)
   - `INFO` — significant events (startup, shutdown, request served)
   - `WARN` — recoverable issues (retry succeeded, deprecated usage)
   - `ERROR` — failures requiring attention (DB down, auth failure)
3. **Structured fields, not string interpolation:**
   ```go
   // ✅ Good
   slog.Info("exam created", "exam_id", exam.ID, "user_id", userID)
   // ❌ Bad
   slog.Info(fmt.Sprintf("exam %s created by %s", exam.ID, userID))
   ```
4. **Include request_id in every log.**
5. **Never log:** passwords, tokens, PII, credit card numbers, raw SQL.

---

## Request ID / Correlation ID

```go
func RequestIDMiddleware(next echo.HandlerFunc) echo.HandlerFunc {
    return func(c echo.Context) error {
        reqID := c.Request().Header.Get("X-Request-ID")
        if reqID == "" {
            reqID = uuid.NewString()
        }
        c.Set("request_id", reqID)
        c.Response().Header().Set("X-Request-ID", reqID)

        // Add to context for downstream logging
        ctx := context.WithValue(c.Request().Context(), requestIDKey, reqID)
        c.SetRequest(c.Request().WithContext(ctx))

        return next(c)
    }
}
```

### Frontend

```ts
// Include X-Request-ID on every API call
headers: {
  "X-Request-ID": crypto.randomUUID(),
  ...init?.headers,
}
```

---

## Health Checks

### Liveness (is the process alive?)

```go
// GET /health/live
func LivenessHandler(c echo.Context) error {
    return c.JSON(http.StatusOK, map[string]string{"status": "ok"})
}
```

### Readiness (can it serve traffic?)

```go
// GET /health/ready
func ReadinessHandler(pool *pgxpool.Pool, redis *redis.Client) echo.HandlerFunc {
    return func(c echo.Context) error {
        ctx, cancel := context.WithTimeout(c.Request().Context(), 2*time.Second)
        defer cancel()

        checks := map[string]string{}

        if err := pool.Ping(ctx); err != nil {
            checks["postgres"] = "unhealthy: " + err.Error()
        } else {
            checks["postgres"] = "ok"
        }

        if err := redis.Ping(ctx).Err(); err != nil {
            checks["redis"] = "unhealthy: " + err.Error()
        } else {
            checks["redis"] = "ok"
        }

        for _, v := range checks {
            if v != "ok" {
                return c.JSON(http.StatusServiceUnavailable, checks)
            }
        }
        return c.JSON(http.StatusOK, checks)
    }
}
```

---

## Request Logging Middleware

```go
func RequestLogger(next echo.HandlerFunc) echo.HandlerFunc {
    return func(c echo.Context) error {
        start := time.Now()

        err := next(c)

        slog.Info("request",
            "method", c.Request().Method,
            "path", c.Path(),
            "status", c.Response().Status,
            "latency_ms", time.Since(start).Milliseconds(),
            "request_id", c.Get("request_id"),
            "ip", c.RealIP(),
            "user_agent", c.Request().UserAgent(),
        )

        return err
    }
}
```

---

## Frontend Error Reporting

```tsx
// app/error.tsx
"use client";

export default function GlobalError({ error, reset }: { error: Error; reset: () => void }) {
  useEffect(() => {
    // Report to your error tracking service
    console.error("Unhandled error:", error);
    // reportError(error); // Sentry, LogRocket, etc.
  }, [error]);

  return (
    <div role="alert">
      <h2>Something went wrong</h2>
      <button onClick={reset}>Try again</button>
    </div>
  );
}
```

---

## What to Log

| Event                    | Level  | Fields                                      |
| ------------------------ | ------ | ------------------------------------------- |
| Server started           | INFO   | port, version, environment                  |
| Request served           | INFO   | method, path, status, latency, request_id   |
| Auth: login success      | INFO   | user_id, ip                                 |
| Auth: login failed       | WARN   | email (hashed), ip, reason                  |
| Auth: token refreshed    | DEBUG  | user_id                                     |
| DB: query slow (>100ms)  | WARN   | query_name, duration_ms                     |
| DB: connection error     | ERROR  | error message, retry count                  |
| Unhandled error          | ERROR  | error, stack, request_id                    |
| Graceful shutdown        | INFO   | drain_duration                              |

---

## Common Mistakes

| Mistake                              | Fix                                            |
| ------------------------------------ | ---------------------------------------------- |
| `fmt.Println` for logging            | Use `slog` with structured fields              |
| Logging tokens or passwords          | Redact; log user_id instead                    |
| No request ID propagation            | Middleware sets X-Request-ID, pass through ctx  |
| Health check queries with no timeout | `context.WithTimeout(ctx, 2*time.Second)`      |
| Missing readiness check              | Check all dependencies (DB, cache, etc.)       |
| Log level too verbose in production  | INFO default; DEBUG only when needed           |

---

## Production Checklist

- [ ] Structured JSON logging with `slog` in production
- [ ] Request ID generated and propagated in headers + context
- [ ] Request logging middleware with method, path, status, latency
- [ ] Liveness endpoint at `/health/live`
- [ ] Readiness endpoint at `/health/ready` checking DB and Redis
- [ ] Auth events logged (login, logout, failed attempts)
- [ ] Slow queries logged (>100ms threshold)
- [ ] No tokens, passwords, or PII in logs
- [ ] Frontend error boundary with error reporting
- [ ] Log level configurable via env var
