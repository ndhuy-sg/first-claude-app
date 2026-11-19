// Test helpers for POS components: an in-memory replacement for the typed API client and a render wrapper.
// Usage in a test file:
//   vi.mock("@/lib/api/client", async (orig) => ({ ...(await orig<object>()), api: (await import("@/lib/pos/test-utils")).mockApi.api }));
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { render } from "@testing-library/react";
import type { ReactElement } from "react";
import { vi } from "vitest";

type Init = { params?: { path?: Record<string, unknown>; query?: Record<string, unknown> }; body?: unknown; headers?: Record<string, string> };
type Handler = (init: Init) => unknown;
type Method = "GET" | "POST" | "PUT" | "PATCH" | "DELETE";

/** Throw from a handler to answer with an API error. */
export class HttpError {
  constructor(
    public status: number,
    public code: string,
    public message = "Lỗi",
    public details?: Record<string, unknown>,
  ) {}
}

/** Throw from a handler to simulate a lost connection (fetch rejects). */
export const networkDown = () => new TypeError("Failed to fetch");

function createMockApi() {
  const routes = new Map<string, Handler>();
  const calls: { method: Method; path: string; init: Init }[] = [];
  const make = (method: Method) =>
    vi.fn(async (path: string, init: Init = {}) => {
      calls.push({ method, path, init });
      const h = routes.get(`${method} ${path}`);
      if (!h) {
        return { error: { error: { code: "NOT_MOCKED", message: `${method} ${path} not mocked` } }, response: new Response(null, { status: 500 }) };
      }
      try {
        const data = await h(init);
        return { data, response: new Response(null, { status: data === undefined ? 204 : 200 }) };
      } catch (e) {
        if (e instanceof HttpError) {
          return {
            error: { error: { code: e.code, message: e.message, details: e.details } },
            response: new Response(null, { status: e.status }),
          };
        }
        throw e;
      }
    });
  const api = { GET: make("GET"), POST: make("POST"), PUT: make("PUT"), PATCH: make("PATCH"), DELETE: make("DELETE") };
  return {
    api,
    calls,
    on(method: Method, path: string, handler: Handler | unknown) {
      routes.set(`${method} ${path}`, typeof handler === "function" ? (handler as Handler) : () => handler);
      return this;
    },
    reset() {
      routes.clear();
      calls.length = 0;
    },
    callsTo(method: Method, path: string) {
      return calls.filter((c) => c.method === method && c.path === path);
    },
  };
}

export const mockApi = createMockApi();

export const routerMock = {
  push: vi.fn(),
  replace: vi.fn(),
  back: vi.fn(),
  prefetch: vi.fn(),
  refresh: vi.fn(),
  forward: vi.fn(),
};

export function renderPos(ui: ReactElement) {
  const client = new QueryClient({ defaultOptions: { queries: { retry: false, staleTime: Infinity }, mutations: { retry: false } } });
  return { client, ...render(<QueryClientProvider client={client}>{ui}</QueryClientProvider>) };
}
