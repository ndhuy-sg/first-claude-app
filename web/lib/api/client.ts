// Typed API client generated from api/openapi.yaml.
// Regenerate types after editing the spec:  npm run gen:api
import createClient from "openapi-fetch";
import type { paths, components } from "./schema";

export type Schemas = components["schemas"];

export const api = createClient<paths>({ baseUrl: "/api/v1", credentials: "include" });

/** Error body returned by the API: { error: { code, message, details } } */
export type ApiError = Schemas["ErrorBody"]["error"];

/** Throws the API error (so TanStack Query treats it as an error) or returns data. */
export function unwrap<T>(res: { data?: T; error?: unknown; response: Response }): T {
  if (res.error !== undefined || !res.response.ok) {
    const body = res.error as { error?: ApiError } | undefined;
    throw new ApiRequestError(res.response.status, body?.error ?? { code: "HTTP_" + res.response.status, message: "Có lỗi xảy ra, vui lòng thử lại" });
  }
  return res.data as T;
}

export class ApiRequestError extends Error {
  constructor(
    public status: number,
    public body: ApiError,
  ) {
    super(body.message);
  }
  get code() {
    return this.body.code;
  }
}

/**
 * One key per user action (e.g. one tap on "Hoàn tất thanh toán"). Reuse the same key
 * when retrying that action so the server replays the first result instead of charging twice.
 */
export function newIdempotencyKey(): string {
  return crypto.randomUUID();
}
