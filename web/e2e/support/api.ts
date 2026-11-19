/**
 * Small typed wrapper around Playwright's APIRequestContext for e2e setup, cleanup and checks.
 * Calls go through Next.js (/api/* rewrite), with an Origin header on every request because the
 * API rejects writes from foreign origins (A-15).
 *
 * Use `new Api(context.request)` to share cookies with a browser context (e.g. PIN-login the POS
 * page without clicking), or `await Api.create(baseURL)` for an independent session (e.g. owner).
 */
import { request as pwRequest, expect, type APIRequestContext, type APIResponse } from "@playwright/test";
import type { components } from "../../lib/api/schema";
import { API_PREFIX } from "./routes";
import { ACCOUNTS, DEVICE_TOKENS, WEB_ORIGIN, vnToday } from "./seed";

export type S = components["schemas"];

type Query = Record<string, string | number | boolean | undefined>;

export class ApiError extends Error {
  constructor(
    public status: number,
    public body: unknown,
    what: string,
  ) {
    super(`${what} → HTTP ${status}: ${JSON.stringify(body)}`);
  }
}

export class Api {
  constructor(public readonly req: APIRequestContext) {}

  /** A fresh, isolated cookie jar (not shared with any page). */
  static async create(baseURL: string, deviceToken?: string): Promise<Api> {
    const req = await pwRequest.newContext({
      baseURL,
      extraHTTPHeaders: { Origin: WEB_ORIGIN },
      storageState: deviceToken
        ? { cookies: [deviceCookieFor(baseURL, deviceToken)], origins: [] }
        : undefined,
    });
    return new Api(req);
  }

  async dispose() {
    await this.req.dispose();
  }

  // ---- raw calls -------------------------------------------------------------------------
  raw(method: "GET" | "POST" | "PUT" | "PATCH" | "DELETE", path: string, opts: { body?: unknown; query?: Query; headers?: Record<string, string> } = {}): Promise<APIResponse> {
    const params: Record<string, string> = {};
    for (const [k, v] of Object.entries(opts.query ?? {})) if (v !== undefined) params[k] = String(v);
    return this.req.fetch(API_PREFIX + path, {
      method,
      params,
      data: opts.body,
      headers: { Origin: WEB_ORIGIN, ...(opts.headers ?? {}) },
    });
  }

  async call<T>(method: "GET" | "POST" | "PUT" | "PATCH" | "DELETE", path: string, opts: { body?: unknown; query?: Query; headers?: Record<string, string> } = {}): Promise<T> {
    const res = await this.raw(method, path, opts);
    const text = await res.text();
    const body = text ? safeJson(text) : undefined;
    if (!res.ok()) throw new ApiError(res.status(), body, `${method} ${path}`);
    return body as T;
  }

  get<T>(path: string, query?: Query) {
    return this.call<T>("GET", path, { query });
  }
  post<T>(path: string, body?: unknown, headers?: Record<string, string>) {
    return this.call<T>("POST", path, { body: body ?? {}, headers });
  }
  put<T>(path: string, body?: unknown) {
    return this.call<T>("PUT", path, { body: body ?? {} });
  }

  // ---- auth ------------------------------------------------------------------------------
  login(email: string, password: string) {
    return this.post<S["Me"]>("/auth/login", { email, password });
  }
  loginOwner() {
    return this.login(ACCOUNTS.owner.email, ACCOUNTS.owner.password);
  }
  /** PIN login on the device whose cookie this context carries; finds the user id by display name. */
  async pinLogin(staffName: string, pin: string) {
    const dev = await this.get<S["DeviceInfo"]>("/device");
    const who = dev.staff.find((s) => s.name === staffName);
    if (!who) throw new Error(`staff "${staffName}" not offered on device ${dev.device.name}: ${dev.staff.map((s) => s.name).join(", ")}`);
    return this.post<S["Me"]>("/auth/pin", { userId: who.id, pin });
  }
  me() {
    return this.get<S["Me"]>("/auth/me");
  }

  // ---- lookups -------------------------------------------------------------------------
  async branchId(code: string): Promise<number> {
    const branches = await this.get<S["Branch"][]>("/branches");
    const b = branches.find((x) => x.code === code);
    if (!b) throw new Error(`branch ${code} not found`);
    return b.id;
  }
  async userId(name: string, branchId?: number): Promise<number> {
    const users = await this.get<S["User"][]>("/users", { branchId });
    const u = users.find((x) => x.name === name);
    if (!u) throw new Error(`user ${name} not found`);
    return u.id;
  }
  async product(name: string): Promise<S["Product"]> {
    const all = await this.get<S["Product"][]>("/products");
    const p = all.find((x) => x.name === name);
    if (!p) throw new Error(`product ${name} not found`);
    return p;
  }
  async menuProduct(name: string): Promise<S["MenuProduct"]> {
    const menu = await this.get<S["Menu"]>("/menu");
    for (const c of menu.categories) for (const p of c.products) if (p.name === name) return p;
    throw new Error(`menu product ${name} not found`);
  }
  async tableId(name: string): Promise<number> {
    const tables = await this.get<S["Table"][]>("/tables");
    const t = tables.find((x) => x.name === name);
    if (!t) throw new Error(`table ${name} not found`);
    return t.id;
  }

  // ---- shifts and orders (on the POS device of this context) -----------------------------
  currentShift() {
    return this.get<S["Shift"] | null>("/shifts/current");
  }
  async ensureOpenShift(openingCash = 500_000): Promise<S["Shift"]> {
    return (await this.currentShift()) ?? (await this.post<S["Shift"]>("/shifts/open", { openingCash }));
  }
  /** Cancels this device's open orders of today and closes the open shift, counting exactly the expected cash. */
  async closeShiftIfOpen(): Promise<void> {
    const shift = await this.currentShift();
    if (!shift) return;
    const open = await this.get<S["OrderList"]>("/orders", { status: "open", shiftId: shift.id, pageSize: 200 });
    for (const o of open.items) await this.post(`/orders/${o.id}/cancel`, { reason: "Dọn dữ liệu sau test e2e" });
    const sum = await this.get<S["ShiftSummary"]>(`/shifts/${shift.id}/summary`);
    await this.post(`/shifts/${shift.id}/close`, { countedCash: sum.expectedCash, note: "e2e" });
  }
  createOrder(body: { type: S["OrderType"]; tableId?: number }) {
    return this.post<S["Order"]>("/orders", body);
  }
  addItem(orderId: number, body: S["AddItemRequest"]) {
    return this.post<S["Order"]>(`/orders/${orderId}/items`, body);
  }
  payCash(order: S["Order"], cashReceived = order.total) {
    return this.post<S["PaymentResult"]>(`/orders/${order.id}/payments`, { method: "cash", amount: order.total - order.paidAmount, cashReceived }, { "Idempotency-Key": crypto.randomUUID() });
  }
  order(id: number) {
    return this.get<S["Order"]>(`/orders/${id}`);
  }

  // ---- reports ---------------------------------------------------------------------------
  /** Paid revenue of one branch (or the whole chain when branchId is undefined) for a VN business date. */
  async revenue(branchId: number | undefined, date = vnToday()): Promise<number> {
    const r = await this.get<S["SalesReport"]>("/reports/sales", { branchId, from: date, to: date, groupBy: "day" });
    return r.totals.revenue;
  }
}

export function deviceCookieFor(baseURL: string, token: string = DEVICE_TOKENS.CN1) {
  const u = new URL(baseURL);
  return { name: "moc_device", value: token, domain: u.hostname, path: "/", expires: -1, httpOnly: true, secure: false, sameSite: "Lax" as const };
}

function safeJson(text: string): unknown {
  try {
    return JSON.parse(text);
  } catch {
    return text;
  }
}

/** Polls fn until it returns a truthy value (expect.poll with a readable message). */
export async function eventually<T>(fn: () => Promise<T>, message: string, timeout = 30_000): Promise<void> {
  await expect.poll(fn, { message, timeout, intervals: [250, 500, 1000, 2000] }).toBeTruthy();
}
