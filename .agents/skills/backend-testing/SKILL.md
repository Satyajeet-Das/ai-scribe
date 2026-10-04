---
name: backend-testing
description: >-
  Use this skill when writing or reviewing tests for the Go backend. Covers
  table-driven tests, test fixtures, integration tests with testcontainers,
  HTTP handler tests, repository tests, and test database management.
---

# Backend Testing (Go)

## When to Activate

- Writing unit tests for services or utilities
- Writing integration tests for repositories or handlers
- Setting up test infrastructure (testcontainers, fixtures)
- Reviewing test quality or coverage

---

## Testing Philosophy

1. **Table-driven tests.** Always. No exceptions.
2. **Test behaviour through public APIs.** Not internal implementation.
3. **Integration tests for repositories.** Unit tests for services.
4. **Test the happy path AND the failure path.** Missing error-path tests is a
   critical gap.
5. **No mocking the database.** Use testcontainers for a real PostgreSQL.

---

## Patterns

### Table-Driven Unit Test

```go
func TestHashPassword(t *testing.T) {
    tests := []struct {
        name     string
        password string
        wantErr  bool
    }{
        {name: "valid password", password: "SecurePass1!", wantErr: false},
        {name: "empty password", password: "", wantErr: false}, // bcrypt handles this
    }

    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            hash, err := HashPassword(tt.password)
            if tt.wantErr {
                require.Error(t, err)
                return
            }
            require.NoError(t, err)
            assert.NotEmpty(t, hash)
            assert.True(t, CheckPassword(tt.password, hash))
        })
    }
}
```

### Service Unit Test (with Mocks)

```go
func TestExamService_Create(t *testing.T) {
    mockRepo := &MockExamRepository{}
    svc := NewExamService(mockRepo)

    t.Run("creates exam successfully", func(t *testing.T) {
        mockRepo.CreateFunc = func(ctx context.Context, exam *Exam) error {
            exam.ID = uuid.New()
            return nil
        }

        exam, err := svc.Create(context.Background(), CreateExamRequest{
            Title:   "Biology 101",
            Subject: "Biology",
        })

        require.NoError(t, err)
        assert.Equal(t, "Biology 101", exam.Title)
    })

    t.Run("returns error on duplicate title", func(t *testing.T) {
        mockRepo.CreateFunc = func(ctx context.Context, exam *Exam) error {
            return shared.ErrConflict
        }

        _, err := svc.Create(context.Background(), CreateExamRequest{
            Title: "Duplicate",
        })

        require.Error(t, err)
        assert.True(t, errors.Is(err, shared.ErrConflict))
    })
}
```

### Repository Integration Test (testcontainers)

```go
func TestExamRepository_Integration(t *testing.T) {
    if testing.Short() {
        t.Skip("skipping integration test in short mode")
    }

    ctx := context.Background()
    pool := setupTestDB(t) // starts PostgreSQL container, runs migrations
    repo := NewExamRepository(pool)

    t.Run("create and get", func(t *testing.T) {
        exam := &Exam{Title: "Test Exam", Subject: "Math", Status: "DRAFT"}
        err := repo.Create(ctx, exam)
        require.NoError(t, err)
        assert.NotEqual(t, uuid.Nil, exam.ID)

        got, err := repo.GetByID(ctx, exam.ID)
        require.NoError(t, err)
        assert.Equal(t, exam.Title, got.Title)
    })

    t.Run("get nonexistent returns ErrNotFound", func(t *testing.T) {
        _, err := repo.GetByID(ctx, uuid.New())
        require.Error(t, err)
        assert.True(t, errors.Is(err, shared.ErrNotFound))
    })
}
```

### HTTP Handler Test

```go
func TestExamHandler_GetExams(t *testing.T) {
    e := echo.New()
    mockSvc := &MockExamService{
        ListFunc: func(ctx context.Context, p ListParams) ([]Exam, int, error) {
            return []Exam{{Title: "Exam 1"}}, 1, nil
        },
    }
    handler := NewExamHandler(mockSvc)

    req := httptest.NewRequest(http.MethodGet, "/api/v1/exams?limit=20", nil)
    rec := httptest.NewRecorder()
    c := e.NewContext(req, rec)

    err := handler.GetExams(c)
    require.NoError(t, err)
    assert.Equal(t, http.StatusOK, rec.Code)

    var body map[string]interface{}
    require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))
    assert.Equal(t, float64(1), body["total"])
}
```

---

## Test Database Setup

```go
func setupTestDB(t *testing.T) *pgxpool.Pool {
    t.Helper()

    ctx := context.Background()
    container, err := testcontainers.GenericContainer(ctx, testcontainers.GenericContainerRequest{
        ContainerRequest: testcontainers.ContainerRequest{
            Image:        "postgres:16-alpine",
            ExposedPorts: []string{"5432/tcp"},
            Env: map[string]string{
                "POSTGRES_DB":       "test_db",
                "POSTGRES_USER":     "test",
                "POSTGRES_PASSWORD": "test",
            },
            WaitingFor: wait.ForListeningPort("5432/tcp"),
        },
        Started: true,
    })
    require.NoError(t, err)
    t.Cleanup(func() { container.Terminate(ctx) })

    host, _ := container.Host(ctx)
    port, _ := container.MappedPort(ctx, "5432")

    dsn := fmt.Sprintf("postgres://test:test@%s:%s/test_db?sslmode=disable", host, port.Port())
    pool, err := pgxpool.New(ctx, dsn)
    require.NoError(t, err)
    t.Cleanup(func() { pool.Close() })

    runMigrations(t, pool)
    return pool
}
```

---

## Test Conventions

```
internal/
├── exam/
│   ├── handler.go
│   ├── handler_test.go      # Handler tests
│   ├── service.go
│   ├── service_test.go      # Unit tests
│   ├── repository.go
│   └── repository_test.go   # Integration tests
```

### Naming

- `Test<Function>_<scenario>` — e.g., `TestExamService_Create_DuplicateTitle`
- Test functions in `_test.go` files next to the code they test.

---

## Common Mistakes

| Mistake                               | Fix                                              |
| ------------------------------------- | ------------------------------------------------ |
| Mocking the database in repo tests    | Use testcontainers with real PostgreSQL           |
| No error-path tests                   | Test every `error` return with specific assertions |
| Not using `t.Helper()`               | Add to all test helper functions                  |
| Global test state                     | Each test gets its own setup; use `t.Cleanup()`   |
| Skipping `-race` in CI               | `go test -race ./...` in every CI run             |
| Not testing HTTP status codes          | Assert both status code and response body         |

---

## Production Checklist

- [ ] All tests are table-driven with clear names
- [ ] Repository tests use testcontainers (real PostgreSQL)
- [ ] Service tests mock only the repository interface
- [ ] Handler tests verify status codes and response shapes
- [ ] Error paths tested for every function
- [ ] `t.Helper()` on all helper functions
- [ ] `t.Cleanup()` for resource teardown
- [ ] `-race` flag in CI
- [ ] `-short` flag skips integration tests locally
- [ ] Coverage: ≥ 70% for service layer, ≥ 50% for handlers
