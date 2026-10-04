---
name: go-production
description: >-
  Use this skill when writing, reviewing, or structuring Go backend code for
  production. Covers project layout, error handling, graceful shutdown,
  configuration, and Go idioms for a clean-architecture modular monolith.
---

# Go Production

## When to Activate

- Creating new Go packages, handlers, or services
- Reviewing Go code for production readiness
- Configuring the application (env vars, config struct)
- Implementing graceful shutdown or signal handling

---

## Project Layout

```
internal/
├── config/           # Configuration loading (env, YAML)
├── platform/         # Infrastructure adapters
│   ├── database/     # pgx pool, migrations
│   ├── http/         # Echo server, middleware, router
│   └── cache/        # Redis client
├── auth/             # Auth domain (handler, service, repository)
│   ├── handler.go
│   ├── service.go
│   ├── repository.go
│   └── model.go
├── exam/             # Exam domain
├── session/          # Session domain
└── shared/           # Cross-cutting: errors, validation, context
```

### Rules

1. **`internal/` for everything.** No exported packages outside `cmd/`.
2. **Domain packages are self-contained.** Each has its own handler, service,
   repository, and model. No cross-domain repository access.
3. **Dependencies flow inward.** Handler → Service → Repository. Never reverse.
4. **`platform/` is infrastructure.** Database, HTTP, cache — domain packages
   depend on abstractions, not on `platform/` directly.

---

## Error Handling

### Rules

1. **Wrap errors with context.** `fmt.Errorf("exam.GetByID(%s): %w", id, err)`
2. **Define domain error types.** Not raw strings.
3. **Map domain errors to HTTP at the handler layer only.**
4. **Never swallow errors.** If you can't handle it, return it.
5. **`errors.Is` / `errors.As` for checking.** Never compare error strings.

```go
// shared/errors.go
var (
    ErrNotFound       = errors.New("not found")
    ErrConflict       = errors.New("conflict")
    ErrUnauthorized   = errors.New("unauthorized")
    ErrForbidden      = errors.New("forbidden")
    ErrValidation     = errors.New("validation error")
)

// handler layer
func mapDomainError(err error) (int, string) {
    switch {
    case errors.Is(err, shared.ErrNotFound):
        return http.StatusNotFound, "Resource not found"
    case errors.Is(err, shared.ErrConflict):
        return http.StatusConflict, "Resource already exists"
    case errors.Is(err, shared.ErrUnauthorized):
        return http.StatusUnauthorized, "Authentication required"
    case errors.Is(err, shared.ErrForbidden):
        return http.StatusForbidden, "Insufficient permissions"
    case errors.Is(err, shared.ErrValidation):
        return http.StatusBadRequest, err.Error()
    default:
        return http.StatusInternalServerError, "Internal server error"
    }
}
```

---

## Configuration

```go
type Config struct {
    Server   ServerConfig
    Database DatabaseConfig
    Auth     AuthConfig
    Redis    RedisConfig
}

type ServerConfig struct {
    Port         int           `env:"PORT" envDefault:"8080"`
    ReadTimeout  time.Duration `env:"READ_TIMEOUT" envDefault:"10s"`
    WriteTimeout time.Duration `env:"WRITE_TIMEOUT" envDefault:"30s"`
    IdleTimeout  time.Duration `env:"IDLE_TIMEOUT" envDefault:"120s"`
}
```

**Rules:**
1. All config from environment variables — never hard-coded.
2. Validate at startup — fail fast if required config is missing.
3. Use `envDefault` for development convenience, but require explicit values in
   production.

---

## Graceful Shutdown

```go
func main() {
    // ... setup

    go func() {
        if err := server.Start(cfg.Server.Port); err != nil && !errors.Is(err, http.ErrServerClosed) {
            log.Fatal("server error", "err", err)
        }
    }()

    quit := make(chan os.Signal, 1)
    signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
    <-quit

    ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
    defer cancel()

    // Drain in reverse init order
    server.Shutdown(ctx)
    dbPool.Close()
    redisClient.Close()
}
```

---

## Go Idioms

1. **Accept interfaces, return structs.** Define interfaces at the call site.
2. **Context flows through.** Every handler, service, and repository method
   takes `ctx context.Context` as the first parameter.
3. **Short variable names in small scopes.** `s` for service in a 5-line
   function; `examService` in a constructor.
4. **Functional options for configuration.** Not builders, not large structs.
5. **Table-driven tests.** Always.

---

## Common Mistakes

| Mistake                                | Fix                                              |
| -------------------------------------- | ------------------------------------------------ |
| Panicking in production code           | Return errors; reserve `panic` for truly unrecoverable init failures |
| Logging and returning an error         | Do one or the other — logging + returning leads to duplicate logs |
| Unbounded goroutines                   | Use `errgroup` or a worker pool                  |
| Global mutable state                   | Inject dependencies through constructors          |
| `init()` functions                     | Explicit initialisation in `main()` or constructors |
| `interface{}` / `any` in domain models | Use concrete types or generics                    |

---

## Production Checklist

- [ ] All config from env vars with validation at startup
- [ ] Graceful shutdown drains connections and flushes logs
- [ ] Errors wrapped with context at every return site
- [ ] Domain errors mapped to HTTP status codes in handler only
- [ ] No `panic` outside of `main` initialisation
- [ ] Context propagated through every layer
- [ ] Dependencies injected, no global state
- [ ] Structured logging with `slog` or `zerolog`
- [ ] Build with `-ldflags "-s -w"` for production
- [ ] Linted with `golangci-lint` — zero warnings
