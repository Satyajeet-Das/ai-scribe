import "@testing-library/jest-dom/vitest";
import { cleanup } from "@testing-library/react";
import { afterEach } from "vitest";

const createLocalStorageMock = () => {
  let store: Record<string, string> = {};
  return {
    getItem: (key: string) => store[key] ?? null,
    setItem: (key: string, value: string) => {
      store[key] = String(value);
    },
    removeItem: (key: string) => {
      delete store[key];
    },
    clear: () => {
      store = {};
    },
    get length() {
      return Object.keys(store).length;
    },
    key: (index: number) => Object.keys(store)[index] ?? null,
  };
};

const mockStorage = createLocalStorageMock();

Object.defineProperty(window, "localStorage", {
  value: mockStorage,
  writable: true,
});

Object.defineProperty(globalThis, "localStorage", {
  value: mockStorage,
  writable: true,
});

afterEach(() => {
  cleanup();
  mockStorage.clear();
});
