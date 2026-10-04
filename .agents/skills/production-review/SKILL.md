---
name: production-review
description: >-
  Use this skill when performing a production readiness review of the full
  stack — Go backend, Next.js frontend, and PostgreSQL database. This is the
  master checklist that composes checks from all other skills into a single
  pre-deployment verification pass.
---

# Production Review

## When to Activate

- Before deploying to staging or production
- After completing a major feature or sprint
- When the user asks "is this production-ready?"
- During a pre-launch audit

---

## Review Process

Work through each section in order. Flag items as ✅ pass, ⚠️ warning, or ❌
fail. Any ❌ is a deployment blocker.

---

## 1. Build & Compile

- [ ] `npm run build` succeeds with zero errors
- [ ] `go build ./...` succeeds with zero errors
- [ ] `go vet ./...` reports no issues
- [ ] `golangci-lint run` reports no errors
- [ ] TypeScript strict mode (`"strict": true`) enabled
- [ ] No `// @ts-ignore` or `// @ts-expect-error` without justification

## 2. Tests

- [ ] `go test -race ./...` passes
- [ ] `npm test` passes (if test suite exists)
- [ ] No skipped tests without documented reason
- [ ] Integration tests pass against real PostgreSQL
- [ ] Error paths are tested, not just happy paths

## 3. Security

- [ ] No secrets in source code or version control
- [ ] `.env` and key files in `.gitignore`
- [ ] Passwords hashed with bcrypt (cost ≥ 10)
- [ ] SQL queries use parameterised placeholders only
- [ ] CORS restricted to specific frontend origin(s)
- [ ] Rate limiting on auth and write endpoints
- [ ] Security headers set (X-Content-Type-Options, X-Frame-Options)
- [ ] Input validated at handler boundary (max lengths, type checks)
- [ ] No `dangerouslySetInnerHTML` without sanitisation
- [ ] JWT secret from env var; algorithm explicitly validated
- [ ] Refresh tokens stored in DB; revocable
- [ ] Login errors: "Invalid credentials" (no user enumeration)

## 4. Authentication & Authorization

- [ ] Auth provider implements `AuthProvider` interface
- [ ] Access token TTL ≤ 15 minutes
- [ ] RBAC middleware on all protected routes
- [ ] Resource ownership checked in service layer
- [ ] Logout revokes refresh token
- [ ] `returnUrl` validated against open redirect

## 5. API Quality

- [ ] All endpoints follow `/api/v1/resource` convention
- [ ] Every list endpoint is paginated
- [ ] Error response shape is consistent
- [ ] Status codes match REST conventions
- [ ] OpenAPI spec exists and matches implementation
- [ ] No internal details leaked in error messages

## 6. Database

- [ ] Connection pool has explicit `MaxConns` and `MaxConnLifetime`
- [ ] Pool pinged at startup; closed on graceful shutdown
- [ ] All queries use parameterised placeholders
- [ ] Migrations are sequential, never modified after deploy
- [ ] Foreign keys have indexes
- [ ] Every table has `id`, `created_at`, `updated_at`
- [ ] No `SELECT *` in production code

## 7. Frontend

- [ ] Every page has `metadata` or `generateMetadata`
- [ ] `error.tsx` and `not-found.tsx` exist
- [ ] Loading skeletons for every async section
- [ ] Empty states for every list/table
- [ ] Forms validated with Zod
- [ ] Auth state gated on `isHydrated`
- [ ] No `any` types in TypeScript
- [ ] Responsive at 320px, 768px, 1440px
- [ ] Accessible: labels, `aria-*`, keyboard navigation

## 8. Observability

- [ ] Structured JSON logging in production
- [ ] Request IDs generated and propagated
- [ ] Liveness endpoint at `/health/live`
- [ ] Readiness endpoint at `/health/ready`
- [ ] Auth events logged
- [ ] No tokens or PII in logs
- [ ] Error boundaries in frontend with reporting

## 9. Performance

- [ ] Database queries analysed with `EXPLAIN ANALYZE`
- [ ] No N+1 queries in list endpoints
- [ ] Frontend bundle < 200 KB JS per page
- [ ] Images use `next/image` with optimisation
- [ ] Connection pooling configured and bounded
- [ ] HTTP timeouts set (read, write, idle)

## 10. Deployment & Operations

- [ ] Graceful shutdown implemented (drain connections, flush logs)
- [ ] Config from environment variables (validated at startup)
- [ ] Docker build uses multi-stage (`builder` → `scratch`/`alpine`)
- [ ] `next.config.ts` has `output: "standalone"` (if Docker)
- [ ] Health checks configured in container orchestrator
- [ ] Database migrations run via CI/CD pipeline (not at app startup in prod)
- [ ] Rollback plan documented

---

## Severity Classification

| Severity | Meaning                                          | Action        |
| -------- | ------------------------------------------------ | ------------- |
| ❌ FAIL  | Security vulnerability, data loss risk, or crash | Block deploy  |
| ⚠️ WARN  | Suboptimal but functional; tech debt             | Track in backlog |
| ✅ PASS  | Meets production standard                        | Ship it       |

---

## Post-Review Actions

1. Fix all ❌ items before deployment.
2. Create tickets for ⚠️ items with priority labels.
3. Document any accepted risks with explicit sign-off.
4. Re-run the checklist after fixes — don't assume.
