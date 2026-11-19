"use client";
// TanStack Query hooks for POS reads. All calls go through the typed client (`api`) and `unwrap()`.
import { useQuery } from "@tanstack/react-query";
import { api, ApiRequestError, unwrap, type Schemas } from "@/lib/api/client";

export const posKeys = {
  all: ["pos"] as const,
  device: ["pos", "device"] as const,
  me: ["pos", "me"] as const,
  shift: ["pos", "shift"] as const,
  menu: ["pos", "menu"] as const,
  tables: ["pos", "tables"] as const,
  branches: ["pos", "branches"] as const,
  order: (id: number) => ["pos", "order", id] as const,
  orders: (params: Record<string, unknown>) => ["pos", "orders", params] as const,
  ordersAll: ["pos", "orders"] as const,
  shiftSummary: (id: number) => ["pos", "shift-summary", id] as const,
  cashSuggestions: (amount: number) => ["pos", "cash-suggestions", amount] as const,
};

export type Me = Schemas["Me"];
export type DeviceInfo = Schemas["DeviceInfo"];
export type Order = Schemas["Order"];
export type Shift = Schemas["Shift"];

/** Returns null instead of throwing for the given status codes (e.g. 401 = not logged in). */
async function orNull<T>(p: Promise<T>, statuses: number[] = [401]): Promise<T | null> {
  try {
    return await p;
  } catch (e) {
    if (e instanceof ApiRequestError && statuses.includes(e.status)) return null;
    throw e;
  }
}

const noRetryOn4xx = (count: number, err: unknown) => !(err instanceof ApiRequestError && err.status < 500) && count < 1;

/** The activated device and its staff. null = this tablet is not activated (401 DEVICE_REQUIRED). */
export function useDevice() {
  return useQuery({
    queryKey: posKeys.device,
    queryFn: () => orNull(api.GET("/device").then(unwrap)),
    retry: noRetryOn4xx,
    staleTime: 60_000,
  });
}

/** Current user session. null = not logged in (or session locked/expired). */
export function useMe() {
  return useQuery({
    queryKey: posKeys.me,
    queryFn: () => orNull(api.GET("/auth/me").then(unwrap)),
    retry: noRetryOn4xx,
    staleTime: 60_000,
  });
}

/** Open shift on this device, or null. */
export function useCurrentShift(enabled = true) {
  return useQuery({
    queryKey: posKeys.shift,
    queryFn: async () => (await api.GET("/shifts/current").then(unwrap)) ?? null,
    retry: noRetryOn4xx,
    enabled,
  });
}

export function useMenu() {
  return useQuery({ queryKey: posKeys.menu, queryFn: () => api.GET("/menu").then(unwrap), retry: noRetryOn4xx });
}

export function useTables() {
  return useQuery({ queryKey: posKeys.tables, queryFn: () => api.GET("/tables").then(unwrap), retry: noRetryOn4xx, refetchInterval: 30_000 });
}

/** The branch of the logged-in user (tax rate, VietQR account). */
export function useMyBranch() {
  return useQuery({
    queryKey: posKeys.branches,
    queryFn: () => orNull(api.GET("/branches").then(unwrap), [401, 403]),
    retry: noRetryOn4xx,
    staleTime: 5 * 60_000,
    select: (branches) => branches?.[0] ?? null,
  });
}

export function useOrder(id: number) {
  return useQuery({
    queryKey: posKeys.order(id),
    queryFn: () => api.GET("/orders/{id}", { params: { path: { id } } }).then(unwrap),
    retry: noRetryOn4xx,
    enabled: Number.isFinite(id) && id > 0,
  });
}

export type OrderFilter = { status?: Schemas["OrderStatus"]; shiftId?: number; q?: string };

export function useOrders(filter: OrderFilter, enabled = true) {
  return useQuery({
    queryKey: posKeys.orders(filter),
    queryFn: () =>
      api
        .GET("/orders", {
          params: { query: { status: filter.status, shiftId: filter.shiftId, q: filter.q || undefined, pageSize: 200 } },
        })
        .then(unwrap),
    retry: noRetryOn4xx,
    enabled,
  });
}

export function useShiftSummary(id: number | undefined) {
  return useQuery({
    queryKey: posKeys.shiftSummary(id ?? 0),
    queryFn: () => api.GET("/shifts/{id}/summary", { params: { path: { id: id! } } }).then(unwrap),
    retry: noRetryOn4xx,
    enabled: !!id,
  });
}

export function useCashSuggestions(amount: number) {
  return useQuery({
    queryKey: posKeys.cashSuggestions(amount),
    queryFn: () => api.GET("/payments/cash-suggestions", { params: { query: { amount } } }).then(unwrap),
    retry: noRetryOn4xx,
    enabled: amount > 0,
    staleTime: Infinity,
  });
}
