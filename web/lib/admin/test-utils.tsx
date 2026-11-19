// Test helpers for admin and lookup pages: a fake API behind `fetch`.
//
// IMPORTANT: import this module BEFORE any module that imports "@/lib/api/client".
// openapi-fetch captures `fetch` and `Request` when the client is created, and Node's
// Request rejects the relative base URL "/api/v1", so both are patched here at load time.
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { render } from "@testing-library/react";
import type { ReactElement } from "react";

const ORIGIN = "http://localhost";
const NativeRequest = globalThis.Request;

class RelativeRequest extends NativeRequest {
  constructor(input: RequestInfo | URL, init?: RequestInit) {
    super(typeof input === "string" && input.startsWith("/") ? ORIGIN + input : input, init);
  }
}
globalThis.Request = RelativeRequest as typeof Request;

export type MockReply = { status?: number; body?: unknown } | ((req: CapturedRequest) => { status?: number; body?: unknown });

export type CapturedRequest = {
  method: string;
  /** path without /api/v1 prefix, e.g. "/reports/sales" */
  path: string;
  query: Record<string, string>;
  body: unknown;
};

let routes = new Map<string, MockReply>();
export const calls: CapturedRequest[] = [];

/**
 * Install fake routes. Keys are "METHOD /path" with the path after /api/v1 (no query string),
 * e.g. { "GET /auth/me": { body: me } }. Unknown routes answer 404 NOT_FOUND.
 */
export function mockApi(map: Record<string, MockReply>): void {
  routes = new Map(Object.entries(map));
  calls.length = 0;
}

/** Add or replace one route without clearing the others. */
export function setRoute(key: string, reply: MockReply): void {
  routes.set(key, reply);
}

export function callsTo(method: string, path: string): CapturedRequest[] {
  return calls.filter((c) => c.method === method && c.path === path);
}

globalThis.fetch = (async (input: RequestInfo | URL, init?: RequestInit) => {
  const req = input instanceof NativeRequest ? input : new RelativeRequest(input, init);
  const url = new URL(req.url);
  const path = url.pathname.replace(/^\/api\/v1/, "");
  const text = req.method === "GET" || req.method === "HEAD" ? "" : await req.text();
  const captured: CapturedRequest = {
    method: req.method,
    path,
    query: Object.fromEntries(url.searchParams.entries()),
    body: text ? JSON.parse(text) : undefined,
  };
  calls.push(captured);
  const route = routes.get(`${req.method} ${path}`);
  if (!route) {
    return json(404, { error: { code: "NOT_FOUND", message: "Không tìm thấy" } });
  }
  const reply = typeof route === "function" ? route(captured) : route;
  const status = reply.status ?? 200;
  if (status === 204 || status === 202) return new Response(null, { status });
  return json(status, reply.body ?? null);
}) as typeof fetch;

function json(status: number, body: unknown): Response {
  return new Response(JSON.stringify(body), { status, headers: { "Content-Type": "application/json" } });
}

export function apiError(status: number, code: string, message: string, details?: Record<string, unknown>) {
  return { status, body: { error: { code, message, details } } };
}

export function renderWithClient(ui: ReactElement) {
  const client = new QueryClient({ defaultOptions: { queries: { retry: false, staleTime: Infinity }, mutations: { retry: 0 } } });
  return { client, ...render(<QueryClientProvider client={client}>{ui}</QueryClientProvider>) };
}
