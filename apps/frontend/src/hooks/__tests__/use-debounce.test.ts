import { describe, it, expect, vi, beforeEach, afterEach } from "vitest";
import { renderHook, act } from "@testing-library/react";
import { useDebounce } from "../use-debounce";

describe("useDebounce", () => {
  beforeEach(() => {
    vi.useFakeTimers();
  });

  afterEach(() => {
    vi.useRealTimers();
  });

  it("returns initial value immediately", () => {
    const { result } = renderHook(() => useDebounce("hello", 300));
    expect(result.current).toBe("hello");
  });

  it("debounces value updates until timer elapses", () => {
    const { result, rerender } = renderHook(
      ({ value }) => useDebounce(value, 300),
      { initialProps: { value: "first" } }
    );

    rerender({ value: "second" });
    // Still old value immediately
    expect(result.current).toBe("first");

    act(() => {
      vi.advanceTimersByTime(200);
    });
    // Still old value at 200ms
    expect(result.current).toBe("first");

    act(() => {
      vi.advanceTimersByTime(100);
    });
    // Updated to second at 300ms
    expect(result.current).toBe("second");
  });

  it("resets debounce timer on rapid updates", () => {
    const { result, rerender } = renderHook(
      ({ value }) => useDebounce(value, 300),
      { initialProps: { value: "a" } }
    );

    act(() => {
      vi.advanceTimersByTime(150);
    });
    rerender({ value: "ab" });

    act(() => {
      vi.advanceTimersByTime(150);
    });
    rerender({ value: "abc" });

    expect(result.current).toBe("a");

    act(() => {
      vi.advanceTimersByTime(300);
    });
    expect(result.current).toBe("abc");
  });
});
