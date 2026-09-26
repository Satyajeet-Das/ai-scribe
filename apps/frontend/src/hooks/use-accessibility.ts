"use client";

import { useState, useEffect } from "react";

export function useAccessibility() {
  const [highContrast, setHighContrast] = useState(false);
  const [largeText, setLargeText] = useState(false);

  useEffect(() => {
    if (highContrast) {
      document.documentElement.classList.add("high-contrast");
    } else {
      document.documentElement.classList.remove("high-contrast");
    }
  }, [highContrast]);

  return {
    highContrast,
    setHighContrast,
    largeText,
    setLargeText,
  };
}
