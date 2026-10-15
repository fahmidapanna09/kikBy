import type { ApiError } from "@/types/models";

export class ServiceError extends Error implements ApiError {
  code: string;
  status?: number;
  fields?: Record<string, string>;
  constructor(code: string, message: string, opts: { status?: number; fields?: Record<string, string> } = {}) {
    super(message);
    this.name = "ServiceError";
    this.code = code;
    this.status = opts.status;
    this.fields = opts.fields;
  }
}

export const isServiceError = (e: unknown): e is ServiceError => e instanceof ServiceError;
export const errorMessage = (e: unknown, fallback = "Something went wrong. Please try again."): string =>
  isServiceError(e) ? e.message : e instanceof Error && e.message ? fallback : fallback;
