import { APP_CONFIG } from "./constants";

export class ApiError extends Error {
  constructor(
    public status: number,
    public code: string,
    message: string,
    public errors?: Array<{ field: string; error: string }>
  ) {
    super(message);
    this.name = "ApiError";
  }
}

export async function apiClient<T>(endpoint: string, options: RequestInit = {}): Promise<T> {
  const url = endpoint.startsWith("http") ? endpoint : `${APP_CONFIG.apiUrl}${endpoint}`;

  const headers = new Headers(options.headers);
  if (!headers.has("Content-Type")) {
    headers.set("Content-Type", "application/json");
  }

  const response = await fetch(url, {
    ...options,
    headers,
  });

  if (!response.ok) {
    let errorData: {
      code?: string;
      message?: string;
      errors?: Array<{ field: string; error: string }>;
    } = {};
    try {
      errorData = (await response.json()) as typeof errorData;
    } catch {
      // Body not JSON
    }

    throw new ApiError(
      response.status,
      errorData.code || "INTERNAL_ERROR",
      errorData.message || response.statusText,
      errorData.errors
    );
  }

  return response.json();
}
