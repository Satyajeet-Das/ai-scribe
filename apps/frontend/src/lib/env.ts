/**
 * Environment configuration for frontend application.
 * Validates and exposes runtime public environment variables.
 */
export const env = {
  API_URL: process.env.NEXT_PUBLIC_API_URL || "http://localhost:8080",
} as const;

export default env;
