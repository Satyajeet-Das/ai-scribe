---
name: ui-ux-quality
description: >-
  Use this skill when building, reviewing, or polishing UI components and
  pages. Covers accessibility (a11y), responsive design, animation, dark
  mode, loading/empty/error states, and visual consistency. Apply whenever
  creating user-facing interfaces.
---

# UI/UX Quality

## When to Activate

- Building any user-facing component or page
- Reviewing UI for accessibility, responsiveness, or polish
- Implementing loading, error, or empty states
- Adding animations, transitions, or interactive feedback

---

## Core Rules

### Accessibility (a11y)

1. **Every interactive element must be keyboard-operable.** Tab order, Enter/Space
   activation, Escape to dismiss.
2. **Every form input has a `<Label>` with `htmlFor`.** No floating labels without
   an accessible name.
3. **Use semantic HTML first.** `<button>` not `<div onClick>`. `<nav>`, `<main>`,
   `<aside>`, `<section>` with headings.
4. **ARIA only when HTML semantics are insufficient.**
   - `aria-label` for icon-only buttons
   - `aria-invalid` + `aria-describedby` for form errors
   - `role="alert"` or `role="status"` for dynamic messages
5. **Images: `alt` text always.** Decorative images get `aria-hidden="true"`.
6. **Colour contrast:** 4.5:1 for normal text, 3:1 for large text (WCAG AA).
7. **Focus indicators:** Never remove `outline` without a visible replacement.

### Responsive Design

1. **Mobile-first.** Base styles for mobile, `sm:` / `md:` / `lg:` for wider.
2. **Breakpoint discipline:**
   - `sm` (640px): Tablets
   - `md` (768px): Small laptops
   - `lg` (1024px): Desktops
   - `xl` (1280px): Wide screens
3. **Touch targets:** Minimum 44×44px for buttons and interactive elements.
4. **No horizontal scroll.** Use `max-w-full`, `overflow-hidden`, `truncate`.
5. **Test at 320px, 768px, 1024px, 1440px.** Every page must work at these.

### Component States

Every data-driven component must handle **all five states:**

| State    | UI Treatment                                              |
| -------- | --------------------------------------------------------- |
| Loading  | Skeleton loader matching the content shape                |
| Empty    | Illustration + message + CTA ("Create your first exam")   |
| Error    | Alert banner + retry button + error details (dev mode)    |
| Success  | Brief toast or inline confirmation, auto-dismiss          |
| Data     | The normal rendered content                               |

### Visual Consistency

1. **Use design tokens** from `lib/constants.ts` — never hard-code spacing,
   colours, or font sizes.
2. **Consistent border radius:** Use CSS custom properties (`--radius`).
3. **Consistent spacing scale:** 4px increments (1, 1.5, 2, 3, 4, 6, 8).
4. **Typography hierarchy:** One `<h1>` per page, descending `<h2>` → `<h3>`.

---

## Production Patterns

### Skeleton Loaders

```tsx
function ExamCardSkeleton() {
  return (
    <div className="animate-pulse rounded-xl border border-border p-4 space-y-3">
      <div className="h-4 w-3/4 rounded bg-muted" />
      <div className="h-3 w-1/2 rounded bg-muted" />
      <div className="h-3 w-full rounded bg-muted" />
    </div>
  );
}
```

### Empty States

```tsx
function EmptyExams() {
  return (
    <div className="flex flex-col items-center justify-center py-16 text-center">
      <BookOpen className="size-12 text-muted-foreground/40 mb-4" />
      <h3 className="text-lg font-semibold">No exams yet</h3>
      <p className="text-sm text-muted-foreground mt-1 mb-4 max-w-xs">
        Create your first assessment to get started.
      </p>
      <Button>Create Exam</Button>
    </div>
  );
}
```

### Error Boundaries

```tsx
// Must be "use client"
function ErrorFallback({ error, reset }: { error: Error; reset: () => void }) {
  return (
    <div role="alert" className="rounded-xl border border-destructive/20 bg-destructive/5 p-6">
      <h2 className="text-lg font-semibold text-destructive">Something went wrong</h2>
      <p className="text-sm text-muted-foreground mt-1">{error.message}</p>
      <Button onClick={reset} variant="outline" className="mt-4">Try again</Button>
    </div>
  );
}
```

### Animations

1. **Enter/exit:** `animate-in fade-in slide-in-from-top-2 duration-200`
2. **Hover:** `transition-colors` or `transition-transform duration-150`
3. **Loading spinner:** `animate-spin` on a border ring, not a GIF
4. **Never animate layout properties** (`width`, `height`) — use `transform` and
   `opacity` only for 60fps

---

## Common Mistakes

| Mistake                                | Fix                                              |
| -------------------------------------- | ------------------------------------------------ |
| Icon-only button without `aria-label`  | Add `aria-label="Close"` or visually-hidden text |
| Form error not linked to input         | `aria-describedby="email-error"` on `<Input>`    |
| Skeleton doesn't match content shape   | Mirror the real UI's dimensions                  |
| Hard-coded `px` values                 | Use Tailwind spacing scale or design tokens      |
| Missing dark mode support              | Use `text-foreground`, `bg-background`, not hard colours |
| No empty state                         | Always handle zero-item case with helpful CTA    |
| `cursor-pointer` on non-interactive    | Only on `<button>`, `<a>`, `<label>` elements    |

---

## Production Checklist

- [ ] Every form field has `<Label>`, `aria-invalid`, `aria-describedby` for errors
- [ ] All icon-only buttons have `aria-label`
- [ ] Loading skeletons exist for every async-loaded section
- [ ] Empty states exist for every list/table that could be empty
- [ ] Error states show user-friendly message + retry action
- [ ] Page renders correctly at 320px, 768px, 1440px
- [ ] Dark mode tested — no hard-coded colours
- [ ] Heading hierarchy is correct (`h1` → `h2` → `h3`)
- [ ] All animations use `transform`/`opacity` only
- [ ] Colour contrast meets WCAG AA (4.5:1)
