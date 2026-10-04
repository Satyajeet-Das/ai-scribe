---
name: frontend-testing
description: >-
  Use this skill when writing or reviewing frontend tests for the Next.js
  application. Covers unit tests (Vitest), component tests (React Testing
  Library), integration tests, and the testing philosophy for a thin-client
  architecture.
---

# Frontend Testing

## When to Activate

- Writing tests for components, hooks, or utility functions
- Setting up or configuring the test runner (Vitest)
- Reviewing test coverage or test quality
- Deciding what to test vs what to skip

---

## Testing Philosophy

> **Test behaviour, not implementation.** Assert what the user sees and
> does — not internal state, class names, or implementation details.

### What to Test

| Layer           | What to Test                                   | Tool                      |
| --------------- | ---------------------------------------------- | ------------------------- |
| Utilities       | Pure functions, formatters, validators         | Vitest                    |
| Zod Schemas     | Valid/invalid inputs, edge cases               | Vitest                    |
| Hooks           | State transitions, effect cleanup              | `renderHook` (RTL)        |
| Components      | Render output, user interactions, a11y         | React Testing Library     |
| API Service     | Request shaping, error handling, token logic   | Vitest + MSW              |
| Pages           | Route-level integration (optional)             | Playwright (e2e)          |

### What NOT to Test

- Styling or CSS classes
- Third-party library internals (shadcn, Radix)
- Implementation details (internal state values)
- Server-side rendering (test via e2e instead)

---

## Core Rules

1. **No `getByTestId` unless semantics fail.** Prefer `getByRole`,
   `getByLabelText`, `getByText`.
2. **`userEvent` over `fireEvent`.** `userEvent` simulates real user behaviour
   (focus, keyboard, etc.).
3. **Mock the API layer, not fetch.** Mock `services/api.ts` functions, or use
   MSW for integration tests.
4. **Every test must be independent.** No shared mutable state between tests.
5. **Arrange–Act–Assert** structure in every test.

---

## Patterns

### Component Test

```tsx
import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { ExamCard } from "./exam-card";

describe("ExamCard", () => {
  const exam = {
    id: "1",
    title: "Biology 101",
    status: "PUBLISHED" as const,
    durationMins: 60,
  };

  it("renders title and status", () => {
    render(<ExamCard exam={exam} />);
    expect(screen.getByText("Biology 101")).toBeInTheDocument();
    expect(screen.getByText("PUBLISHED")).toBeInTheDocument();
  });

  it("calls onEdit when edit button is clicked", async () => {
    const onEdit = vi.fn();
    render(<ExamCard exam={exam} onEdit={onEdit} />);
    await userEvent.click(screen.getByRole("button", { name: /edit/i }));
    expect(onEdit).toHaveBeenCalledWith("1");
  });
});
```

### Hook Test

```tsx
import { renderHook, act } from "@testing-library/react";
import { useDebounce } from "./use-debounce";

it("debounces value changes", async () => {
  vi.useFakeTimers();
  const { result, rerender } = renderHook(
    ({ value }) => useDebounce(value, 300),
    { initialProps: { value: "a" } }
  );

  rerender({ value: "ab" });
  expect(result.current).toBe("a"); // still old value

  act(() => vi.advanceTimersByTime(300));
  expect(result.current).toBe("ab"); // updated

  vi.useRealTimers();
});
```

### Zod Schema Test

```ts
import { LoginSchema } from "./validations";

describe("LoginSchema", () => {
  it("accepts valid credentials", () => {
    expect(LoginSchema.safeParse({
      email: "user@test.com",
      password: "Passw0rd!",
    }).success).toBe(true);
  });

  it("rejects empty email", () => {
    const result = LoginSchema.safeParse({ email: "", password: "pass" });
    expect(result.success).toBe(false);
  });
});
```

### Mocking the API Layer

```ts
import { vi } from "vitest";
import { examsApi } from "@/services/api";

vi.mock("@/services/api", () => ({
  examsApi: {
    getExams: vi.fn(),
    createExam: vi.fn(),
  },
}));

beforeEach(() => vi.clearAllMocks());
```

---

## Test File Conventions

```
src/
├── components/
│   ├── exam-card.tsx
│   └── __tests__/
│       └── exam-card.test.tsx
├── lib/
│   ├── validations.ts
│   └── __tests__/
│       └── validations.test.ts
└── hooks/
    ├── use-debounce.ts
    └── __tests__/
        └── use-debounce.test.ts
```

---

## Common Mistakes

| Mistake                              | Fix                                            |
| ------------------------------------ | ---------------------------------------------- |
| Testing implementation details       | Test user-visible behaviour instead             |
| Using `getByTestId` as default       | Use `getByRole`, `getByLabelText` first         |
| Not cleaning up after tests          | Use `afterEach(() => cleanup())` (RTL default)  |
| Mocking at the wrong layer           | Mock `services/api`, not global `fetch`          |
| Snapshot tests for UI                | Avoid — they break on any change and add noise   |
| No error-path tests                  | Always test the unhappy path (API failure, invalid input) |

---

## Production Checklist

- [ ] Vitest configured with `@testing-library/react` and `jsdom`
- [ ] All Zod schemas have valid + invalid input tests
- [ ] Shared hooks have dedicated tests
- [ ] Critical user flows (login, create exam) have component tests
- [ ] API mocks use `vi.mock` on the service layer, not fetch
- [ ] No snapshot tests — use explicit assertions
- [ ] Tests run in CI and block merge on failure
- [ ] Coverage target: ≥ 70% for `lib/`, `hooks/`, `store/`
