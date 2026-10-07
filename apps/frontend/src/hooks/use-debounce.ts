import { useEffect, useState } from "react";

export function useDebounce<T>(
  value: T,
  delay: number = 300,
  options?: { immediateIfEmpty?: boolean }
): T {
  const [debouncedValue, setDebouncedValue] = useState<T>(value);

  useEffect(() => {
    if (
      options?.immediateIfEmpty &&
      (value === "" || (typeof value === "string" && value.trim() === ""))
    ) {
      setDebouncedValue(value);
      return;
    }

    const timer = setTimeout(() => {
      setDebouncedValue(value);
    }, delay);

    return () => {
      clearTimeout(timer);
    };
  }, [value, delay, options?.immediateIfEmpty]);

  return debouncedValue;
}
