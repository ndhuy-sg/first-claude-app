"use client";

import { useQuery } from "@tanstack/react-query";
import { createContext, useContext, useState } from "react";
import { api, ApiRequestError, unwrap } from "@/lib/api/client";
import type { Me } from "./permissions";
import { defaultScope, type Scope } from "./scope";

export const qk = {
  me: ["auth", "me"] as const,
  branches: ["branches"] as const,
  settings: ["settings"] as const,
  products: ["products"] as const,
};

/** Do not retry client errors (401/403/404): they will not fix themselves. */
export function retryServerErrors(count: number, err: unknown): boolean {
  if (err instanceof ApiRequestError && err.status < 500) return false;
  return count < 1;
}

export function useMeQuery() {
  return useQuery({
    queryKey: qk.me,
    queryFn: async () => unwrap(await api.GET("/auth/me")),
    retry: retryServerErrors,
    staleTime: 60_000,
  });
}

export function useBranches() {
  return useQuery({
    queryKey: qk.branches,
    queryFn: async () => unwrap(await api.GET("/branches")),
    retry: retryServerErrors,
    staleTime: 60_000,
  });
}

export function useSettings() {
  return useQuery({
    queryKey: qk.settings,
    queryFn: async () => unwrap(await api.GET("/settings")),
    retry: retryServerErrors,
  });
}

export const MeContext = createContext<Me | null>(null);

/** The signed-in admin user. Only valid below <AdminShell>, which renders children after /auth/me succeeds. */
export function useMe(): Me {
  const me = useContext(MeContext);
  if (!me) throw new Error("useMe() must be used inside <AdminShell>");
  return me;
}

/** Map branch id → code ("CN1") for table cells. */
export function useBranchCodes(): (id: number | null | undefined) => string {
  const { data } = useBranches();
  return (id) => {
    if (id === null || id === undefined) return "Toàn chuỗi";
    return data?.find((b) => b.id === id)?.code ?? `#${id}`;
  };
}

/** Branch scope state of a page, starting at the user's default (owner: Toàn chuỗi, ql: own branch). */
export function useScope(): [Scope, (s: Scope) => void] {
  const me = useMe();
  return useState<Scope>(() => defaultScope(me));
}

export function errorMessage(err: unknown): string {
  if (err instanceof ApiRequestError) return err.body.message;
  return "Không kết nối được máy chủ, vui lòng thử lại";
}
