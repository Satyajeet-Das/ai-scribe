"use client";

import { useEffect } from "react";

export default function Error({
  error,
  reset,
}: {
  error: Error & { digest?: string };
  reset: () => void;
}) {
  useEffect(() => {
    console.error(error);
  }, [error]);

  return (
    <div className="flex min-h-screen flex-col items-center justify-center p-4 text-center">
      <h2 className="text-2xl font-bold">Something went wrong!</h2>
      <button
        onClick={() => reset()}
        className="mt-4 rounded bg-sky-500 px-4 py-2 text-white hover:bg-sky-400"
      >
        Try again
      </button>
    </div>
  );
}
