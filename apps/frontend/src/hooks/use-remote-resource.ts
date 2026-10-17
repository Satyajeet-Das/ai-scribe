import { useCallback, useEffect, useState } from "react";
import { ApiError } from "@/services/api";

export type ResourceErrorKind = "auth" | "forbidden" | "notFound" | "network" | "server";

export interface ResourceError {
  kind: ResourceErrorKind;
  message: string;
}

export function describeError(err: unknown): ResourceError {
  if (err instanceof ApiError) {
    if (err.status === 401) {
      return { kind: "auth", message: "Your session has expired. Please sign in again." };
    }
    if (err.status === 403) {
      return { kind: "forbidden", message: "You do not have permission to view this." };
    }
    if (err.status === 404) {
      return { kind: "notFound", message: "This exam or student could not be found." };
    }
    if (err.status === 0) {
      return {
        kind: "network",
        message: "Could not reach the server. Check your connection and try again.",
      };
    }
  }
  return { kind: "server", message: "Something went wrong on our side. Please try again." };
}

interface Settled<T> {
  key: string;
  scope?: string;
  token: number;
  data?: T;
  error?: ResourceError;
}

export interface RemoteResource<T> {
  /**
   * Data for the current key. Data from a different key is returned only while
   * loading and only when both keys share the caller's `scope`.
   */
  data: T | undefined;
  error: ResourceError | undefined;
  /** True while a request for the current key is in flight. */
  loading: boolean;
  /** True when earlier data is still shown while the current key loads. */
  refreshing: boolean;
  reload: () => void;
}

/**
 * Loads one resource and cancels the request whenever the key changes, the
 * component unmounts or a reload starts. A response that arrives after it was
 * superseded is dropped, and data loaded for a different key is never returned.
 *
 * Pass `scope` (for example an exam id) to keep showing the previous result,
 * for the same scope only, while a new page or filter loads. Without it, data
 * is never carried across keys.
 *
 * `load` must be stable for a given `key` (wrap it in useCallback).
 */
export function useRemoteResource<T>(
  key: string,
  load: (signal: AbortSignal) => Promise<T>,
  scope?: string
): RemoteResource<T> {
  const [token, setToken] = useState(0);
  const [settled, setSettled] = useState<Settled<T> | null>(null);

  useEffect(() => {
    const controller = new AbortController();
    load(controller.signal).then(
      (data) => {
        if (controller.signal.aborted) return;
        setSettled({ key, scope, token, data });
      },
      (err: unknown) => {
        if (controller.signal.aborted) return;
        setSettled((prev) => ({
          key,
          scope,
          token,
          // Keep what was already shown for this key, but report the failure.
          data: prev && prev.key === key ? prev.data : undefined,
          error: describeError(err),
        }));
      }
    );
    return () => controller.abort();
  }, [key, scope, token, load]);

  const reload = useCallback(() => setToken((t) => t + 1), []);

  const sameKey = settled !== null && settled.key === key;
  const loading = !sameKey || settled.token !== token;
  const carried =
    !sameKey && scope !== undefined && settled !== null && settled.scope === scope
      ? settled.data
      : undefined;
  const data = sameKey ? settled.data : carried;
  return {
    data,
    error: loading ? undefined : settled?.error,
    loading,
    refreshing: loading && data !== undefined,
    reload,
  };
}
