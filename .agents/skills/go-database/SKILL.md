---
name: go-database
description: >-
  Use this skill when writing, reviewing, or debugging PostgreSQL database
  code in Go. Covers pgx connection pooling, migrations, query patterns,
  transactions, SQL injection prevention, and performance optimisation.
---

# Go Database (PostgreSQL + pgx)

## When to Activate

- Writing SQL queries or repository methods
- Creating or modifying database migrations
- Setting up or configuring the pgx connection pool
- Debugging slow queries, connection leaks, or deadlocks
- Implementing transactions or batch operations

---

## Connection Pool

```go
func NewPool(ctx context.Context, cfg DatabaseConfig) (*pgxpool.Pool, error) {
    poolConfig, err := pgxpool.ParseConfig(cfg.URL)
    if err != nil {
        return nil, fmt.Errorf("parse database URL: %w", err)
    }

    poolConfig.MaxConns = int32(cfg.MaxConns)           // default: 25
    poolConfig.MinConns = int32(cfg.MinConns)           // default: 5
    poolConfig.MaxConnLifetime = 1 * time.Hour
    poolConfig.MaxConnIdleTime = 30 * time.Minute
    poolConfig.HealthCheckPeriod = 30 * time.Second

    pool, err := pgxpool.NewWithConfig(ctx, poolConfig)
    if err != nil {
        return nil, fmt.Errorf("create pool: %w", err)
    }

    if err := pool.Ping(ctx); err != nil {
        return nil, fmt.Errorf("ping database: %w", err)
    }
    return pool, nil
}
```

### Rules

1. **Always set `MaxConns`.** Default is unlimited → connection exhaustion.
2. **`MaxConnLifetime`** prevents stale connections behind load balancers.
3. **Ping at startup** to fail fast on misconfiguration.
4. **Close the pool on shutdown** in reverse init order.

---

## Migrations

### File Convention

```
migrations/
├── 001_create_users.sql
├── 002_create_exams.sql
├── 003_create_questions.sql
├── 004_create_assignments.sql
└── 005_create_sessions.sql
```

### Rules

1. **Numbered, sequential, never reordered.** `001_`, `002_`, etc.
2. **Each file is idempotent.** Use `IF NOT EXISTS`, `CREATE OR REPLACE`.
3. **Never modify a deployed migration.** Create a new one instead.
4. **Include both up and down** (or at minimum, up).
5. **Run migrations at startup** in development; via CI/CD pipeline in production.

### Schema Design Rules

1. **UUIDs for primary keys.** `gen_random_uuid()` in PostgreSQL 13+.
2. **`created_at` and `updated_at` on every table.** Use `DEFAULT NOW()`.
3. **Foreign keys with `ON DELETE` clauses** — explicit, not implicit.
4. **Indexes on foreign keys and frequently queried columns.**
5. **Enums as `VARCHAR` with CHECK constraints** — not PostgreSQL `ENUM` type
   (hard to modify).

```sql
CREATE TABLE IF NOT EXISTS exams (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    title       VARCHAR(255) NOT NULL,
    subject     VARCHAR(100) NOT NULL,
    status      VARCHAR(20) NOT NULL DEFAULT 'DRAFT'
                CHECK (status IN ('DRAFT', 'PUBLISHED', 'ARCHIVED')),
    created_by  UUID NOT NULL REFERENCES users(id),
    created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at  TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_exams_created_by ON exams(created_by);
CREATE INDEX IF NOT EXISTS idx_exams_status ON exams(status);
```

---

## Repository Pattern

```go
type ExamRepository struct {
    pool *pgxpool.Pool
}

func (r *ExamRepository) GetByID(ctx context.Context, id uuid.UUID) (*Exam, error) {
    var exam Exam
    err := r.pool.QueryRow(ctx,
        `SELECT id, title, subject, status, created_by, created_at, updated_at
         FROM exams WHERE id = $1`, id,
    ).Scan(
        &exam.ID, &exam.Title, &exam.Subject, &exam.Status,
        &exam.CreatedBy, &exam.CreatedAt, &exam.UpdatedAt,
    )
    if err != nil {
        if errors.Is(err, pgx.ErrNoRows) {
            return nil, fmt.Errorf("exam %s: %w", id, shared.ErrNotFound)
        }
        return nil, fmt.Errorf("exam.GetByID(%s): %w", id, err)
    }
    return &exam, nil
}
```

### Rules

1. **Always use parameterised queries** (`$1`, `$2`). Never string concatenation.
2. **Map `pgx.ErrNoRows` to domain `ErrNotFound`.**
3. **Wrap every error with context** (function name + params).
4. **Select only needed columns.** No `SELECT *`.
5. **Use `QueryRow` for single results, `Query` for lists.**

---

## Transactions

```go
func (r *ExamRepository) CreateWithQuestions(
    ctx context.Context, exam *Exam, questions []Question,
) error {
    tx, err := r.pool.Begin(ctx)
    if err != nil {
        return fmt.Errorf("begin tx: %w", err)
    }
    defer tx.Rollback(ctx) // no-op if already committed

    // Insert exam
    _, err = tx.Exec(ctx, `INSERT INTO exams (...) VALUES (...)`, ...)
    if err != nil {
        return fmt.Errorf("insert exam: %w", err)
    }

    // Insert questions
    for _, q := range questions {
        _, err = tx.Exec(ctx, `INSERT INTO questions (...) VALUES (...)`, ...)
        if err != nil {
            return fmt.Errorf("insert question %s: %w", q.ID, err)
        }
    }

    return tx.Commit(ctx)
}
```

### Rules

1. **`defer tx.Rollback(ctx)` immediately after `Begin`.** Safe even after commit.
2. **Keep transactions short.** No network calls or long computations inside.
3. **One transaction per business operation.** Not one per SQL statement.

---

## Performance

1. **Index foreign keys** — PostgreSQL doesn't auto-index them.
2. **`EXPLAIN ANALYZE` before deploying new queries.**
3. **Use `LIMIT` on all list queries.** Never unbounded.
4. **Batch inserts** with `pgx.Batch` for bulk operations.
5. **Connection pool monitoring** — log pool stats periodically.

---

## Common Mistakes

| Mistake                              | Fix                                              |
| ------------------------------------ | ------------------------------------------------ |
| String interpolation in SQL          | Use `$1`, `$2` parameterised queries             |
| No index on foreign keys             | `CREATE INDEX` on every FK column                |
| `SELECT *`                           | List specific columns                            |
| Modifying deployed migrations        | Create new migration files                       |
| Long-running transactions            | Keep txns under 100ms; no external calls inside  |
| Not checking `pgx.ErrNoRows`        | Map to domain `ErrNotFound`                      |
| Forgetting `defer tx.Rollback`       | Always defer immediately after `Begin`           |

---

## Production Checklist

- [ ] Connection pool has explicit `MaxConns`, `MaxConnLifetime`
- [ ] Pool is pinged at startup; closed on shutdown
- [ ] All queries use parameterised placeholders
- [ ] `pgx.ErrNoRows` mapped to `ErrNotFound`
- [ ] Every table has `id`, `created_at`, `updated_at`
- [ ] Foreign keys have indexes
- [ ] Migrations are numbered, sequential, and never modified after deploy
- [ ] Transactions have `defer tx.Rollback(ctx)`
- [ ] `EXPLAIN ANALYZE` run on complex queries
- [ ] No `SELECT *` in production code
