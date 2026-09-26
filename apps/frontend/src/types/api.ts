export interface HealthResponse {
  status: "healthy" | "unhealthy";
  timestamp: string;
  environment: string;
  checks: {
    database: {
      status: string;
      response_time?: string;
      error?: string;
    };
    redis?: {
      status: string;
      response_time?: string;
      error?: string;
    };
  };
}

export interface PaginatedResult<T> {
  data: T[];
  page: number;
  limit: number;
  total: number;
  totalPages: number;
}
