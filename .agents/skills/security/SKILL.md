---
name: security
description: >-
  Use this skill when reviewing or implementing security controls across the
  full stack. Covers OWASP Top 10 mitigations, input validation, CORS,
  CSP, rate limiting, secrets management, and secure defaults for the
  Go + Next.js + PostgreSQL stack.
---

# Security

## When to Activate

- Reviewing code for security vulnerabilities
- Implementing input validation, CORS, CSP, or rate limiting
- Managing secrets, API keys, or credentials
- Hardening the application for production deployment
- Any code that handles user input, authentication, or authorisation

---

## OWASP Top 10 Mitigations

### 1. Injection (SQL, NoSQL, OS)

- **Go:** Parameterised queries only (`$1`, `$2`). Never `fmt.Sprintf` for SQL.
- **Frontend:** Never construct URLs or queries from raw user input.
- **Validation:** Allowlist input characters and lengths.

### 2. Broken Authentication

- bcrypt for passwords (cost ≥ 10)
- Short-lived access tokens (15 min)
- Refresh tokens in httpOnly cookies
- Account lockout after N failed attempts
- No user enumeration in error messages

### 3. Sensitive Data Exposure

- HTTPS only in production (HSTS header)
- Never log passwords, tokens, or PII
- Encrypt sensitive fields at rest if required
- Redact sensitive fields in API responses

### 4. XXE / Insecure Deserialization

- Go's `encoding/json` is safe by default
- Never use `encoding/xml` with untrusted input without disabling DTDs

### 5. Broken Access Control

- RBAC middleware on every protected route
- Resource ownership checks in service layer
- Never trust client-side role assertions

### 6. Security Misconfiguration

- Remove default credentials and example configs
- Disable debug endpoints in production
- Set secure HTTP headers (see below)

### 7. XSS

- React/Next.js escapes by default — never use `dangerouslySetInnerHTML`
- CSP header to restrict script sources
- Sanitise any user content rendered as HTML

### 8. CSRF

- `SameSite=Strict` on cookies
- CORS restricted to frontend origin
- CSRF token for state-changing cookie-based requests

### 9. Using Components with Known Vulnerabilities

- `npm audit` and `go mod tidy` in CI
- Dependabot or Renovate for automated updates
- Pin major versions; review changelogs before upgrading

### 10. Insufficient Logging & Monitoring

- Log all auth events (login, logout, failed attempts, token refresh)
- Log all admin/privileged operations
- Never log sensitive data (passwords, tokens, PII)
- Structured logs with request IDs for tracing

---

## HTTP Security Headers

```go
func SecurityHeaders(next echo.HandlerFunc) echo.HandlerFunc {
    return func(c echo.Context) error {
        h := c.Response().Header()
        h.Set("X-Content-Type-Options", "nosniff")
        h.Set("X-Frame-Options", "DENY")
        h.Set("X-XSS-Protection", "0") // modern browsers use CSP instead
        h.Set("Referrer-Policy", "strict-origin-when-cross-origin")
        h.Set("Permissions-Policy", "camera=(), microphone=(), geolocation=()")
        // CSP set separately per deployment
        return next(c)
    }
}
```

---

## CORS Configuration

```go
func CORSConfig(frontendOrigin string) middleware.CORSConfig {
    return middleware.CORSConfig{
        AllowOrigins:     []string{frontendOrigin},
        AllowMethods:     []string{"GET", "POST", "PUT", "PATCH", "DELETE", "OPTIONS"},
        AllowHeaders:     []string{"Authorization", "Content-Type", "X-Request-ID"},
        AllowCredentials: true,
        MaxAge:           86400, // 24h preflight cache
    }
}
```

### Rules

1. **Never `AllowOrigins: ["*"]` with `AllowCredentials: true`.**
2. **Explicit origin allowlist** — one entry per environment.
3. **Preflight caching** — `MaxAge: 86400` to reduce OPTIONS requests.

---

## Rate Limiting

```go
// Per-IP rate limiting for auth endpoints
authGroup.Use(middleware.RateLimiter(
    middleware.NewRateLimiterMemoryStoreWithConfig(
        middleware.RateLimiterMemoryStoreConfig{
            Rate:      10,               // 10 requests
            Burst:     5,
            ExpiresIn: 1 * time.Minute,  // per minute
        },
    ),
))
```

| Endpoint Group | Rate         | Rationale                     |
| -------------- | ------------ | ----------------------------- |
| Auth (login)   | 10/min/IP    | Prevent brute force           |
| Auth (register)| 5/min/IP     | Prevent mass account creation |
| API (read)     | 100/min/user | Normal usage                  |
| API (write)    | 30/min/user  | Prevent abuse                 |

---

## Input Validation

### Go Backend

```go
type CreateExamRequest struct {
    Title       string `json:"title" validate:"required,min=3,max=255"`
    Subject     string `json:"subject" validate:"required,min=2,max=100"`
    DurationMins int   `json:"durationMins" validate:"required,min=5,max=480"`
}

// Validate at handler boundary
if err := c.Validate(&req); err != nil {
    return echo.NewHTTPError(http.StatusBadRequest, err.Error())
}
```

### Rules

1. **Validate at the boundary** — handler layer for HTTP, not deep in services.
2. **Allowlist, not blocklist.** Define what's valid, reject everything else.
3. **Max lengths on all string fields.** Prevent memory exhaustion.
4. **Sanitise before storage.** Trim whitespace, normalise unicode.

---

## Secrets Management

1. **`.env` files for local development only.** Never committed.
2. **Environment variables in production** — injected by deployment platform.
3. **`.env.example`** committed with placeholder values.
4. **`.gitignore`** must include: `.env`, `*.pem`, `*.key`, `*.secret`
5. **Rotate secrets periodically** — JWT signing key, database passwords.

---

## Common Mistakes

| Mistake                                 | Fix                                            |
| --------------------------------------- | ---------------------------------------------- |
| `CORS: AllowOrigins: ["*"]`            | Explicit origin per environment                |
| Logging access tokens                   | Never log tokens; log user IDs instead         |
| SQL string concatenation                | Parameterised queries only                     |
| `dangerouslySetInnerHTML`              | Never use without DOMPurify sanitisation       |
| Secrets in source code                  | Env vars; `.env` in `.gitignore`               |
| No rate limiting on auth endpoints      | 10/min/IP minimum                              |
| Missing `X-Content-Type-Options`       | Always set `nosniff`                           |

---

## Production Checklist

- [ ] All SQL uses parameterised queries
- [ ] Passwords hashed with bcrypt (cost ≥ 10)
- [ ] CORS allowlist set per environment
- [ ] Security headers set (X-Content-Type-Options, X-Frame-Options, etc.)
- [ ] Rate limiting on auth and write endpoints
- [ ] Input validation on every handler (max lengths, type checks)
- [ ] Secrets in env vars, not source code
- [ ] `.env` and key files in `.gitignore`
- [ ] `npm audit` and `go mod tidy` clean
- [ ] Auth events logged with request IDs
- [ ] No `dangerouslySetInnerHTML` without sanitisation
- [ ] No tokens or PII in logs or error messages
