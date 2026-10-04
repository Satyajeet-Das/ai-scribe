---
name: go-concurrency
description: >-
  Use this skill when writing concurrent Go code — goroutines, channels,
  worker pools, context cancellation, or shared state. Covers safe
  concurrency patterns for web servers and background job processing.
---

# Go Concurrency

## When to Activate

- Launching goroutines for background work
- Using channels, mutexes, or sync primitives
- Implementing worker pools or fan-out/fan-in
- Debugging deadlocks, race conditions, or goroutine leaks
- Handling context cancellation and timeouts

---

## Core Rules

1. **Every goroutine must have a defined lifetime.** Know when and how it stops.
2. **Context is the cancellation signal.** Pass `ctx` to every goroutine; check
   `ctx.Done()`.
3. **Use `errgroup` for structured concurrency.** Not raw `go func()`.
4. **Never share mutable state without synchronisation.** Use channels or
   `sync.Mutex`.
5. **Run tests with `-race` flag.** Always.

---

## Patterns

### errgroup (Preferred for Request-Scoped Work)

```go
import "golang.org/x/sync/errgroup"

func (s *ExamService) EnrichExams(ctx context.Context, exams []Exam) error {
    g, ctx := errgroup.WithContext(ctx)

    for i := range exams {
        i := i // capture loop variable
        g.Go(func() error {
            count, err := s.questionRepo.CountByExamID(ctx, exams[i].ID)
            if err != nil {
                return fmt.Errorf("count questions for exam %s: %w", exams[i].ID, err)
            }
            exams[i].QuestionCount = count
            return nil
        })
    }

    return g.Wait()
}
```

### errgroup with Concurrency Limit

```go
g, ctx := errgroup.WithContext(ctx)
g.SetLimit(10) // max 10 concurrent goroutines

for _, id := range examIDs {
    id := id
    g.Go(func() error {
        return s.processExam(ctx, id)
    })
}
return g.Wait()
```

### Worker Pool

```go
func ProcessBatch(ctx context.Context, items []Item, workers int) error {
    ch := make(chan Item, len(items))
    for _, item := range items {
        ch <- item
    }
    close(ch)

    g, ctx := errgroup.WithContext(ctx)
    for i := 0; i < workers; i++ {
        g.Go(func() error {
            for item := range ch {
                select {
                case <-ctx.Done():
                    return ctx.Err()
                default:
                    if err := process(ctx, item); err != nil {
                        return err
                    }
                }
            }
            return nil
        })
    }
    return g.Wait()
}
```

### Mutex for Shared State

```go
type RateLimiter struct {
    mu       sync.Mutex
    counters map[string]int
}

func (r *RateLimiter) Increment(key string) int {
    r.mu.Lock()
    defer r.mu.Unlock()
    r.counters[key]++
    return r.counters[key]
}
```

**Use `sync.RWMutex` when reads vastly outnumber writes.**

### Timeout with Context

```go
ctx, cancel := context.WithTimeout(parentCtx, 5*time.Second)
defer cancel()

result, err := s.longRunningOperation(ctx)
if errors.Is(err, context.DeadlineExceeded) {
    return nil, fmt.Errorf("operation timed out after 5s")
}
```

---

## Anti-Patterns

| Anti-Pattern                           | Correct Pattern                                |
| -------------------------------------- | ---------------------------------------------- |
| `go func() { ... }()` with no lifecycle | Use `errgroup` or track with `sync.WaitGroup` |
| Unbuffered channel without reader      | Always ensure a goroutine will read             |
| `time.Sleep` for coordination          | Use channels, `sync.WaitGroup`, or `errgroup`  |
| Goroutine accessing shared slice       | Pass index, or use mutex/channel               |
| Ignoring `ctx.Done()` in loops         | Check `select { case <-ctx.Done(): }`          |
| Forgetting `defer cancel()` on context | Always defer immediately after `WithTimeout/Cancel` |

---

## Goroutine Leak Detection

Signs of a leak:
- Memory grows over time
- `runtime.NumGoroutine()` increases monotonically

Prevention:
1. Every goroutine checks `ctx.Done()`.
2. Every channel has a producer that eventually closes it.
3. Every `errgroup` is `.Wait()`-ed.

Test in CI:

```go
func TestNoGoroutineLeak(t *testing.T) {
    before := runtime.NumGoroutine()
    // ... run test
    time.Sleep(100 * time.Millisecond)
    after := runtime.NumGoroutine()
    if after > before+2 {
        t.Errorf("goroutine leak: before=%d after=%d", before, after)
    }
}
```

---

## Production Checklist

- [ ] All goroutines have a defined shutdown path via `ctx.Done()`
- [ ] `errgroup` used for structured concurrent work
- [ ] Concurrency limits set (`g.SetLimit(N)`) for fan-out operations
- [ ] Shared mutable state protected by `sync.Mutex` or channels
- [ ] `defer cancel()` on every derived context
- [ ] Tests run with `-race` flag in CI
- [ ] No `time.Sleep` for synchronisation
- [ ] Channel producers close channels when done
- [ ] Worker pools drain cleanly on shutdown
