"use client";
import { useRouter } from "next/navigation";
import { useCallback, useEffect, useState } from "react";
import { ApiRequestError } from "@/lib/api/client";
import { isNetworkError } from "./connectivity";
import { useLogout } from "./mutations";

/** Current time, refreshed every `ms` (for "12 phút" on table cards and the login clock). */
export function useNow(ms = 30_000): number {
  const [now, setNow] = useState(() => Date.now());
  useEffect(() => {
    const t = setInterval(() => setNow(Date.now()), ms);
    return () => clearInterval(t);
  }, [ms]);
  return now;
}

/** Vietnamese message for an error from a mutation or query. */
export function errorMessage(err: unknown): string {
  if (!err) return "";
  if (isNetworkError(err)) return "Mất kết nối tới máy chủ. Thao tác chưa được lưu, hãy thử lại khi có mạng.";
  if (err instanceof ApiRequestError) return err.body.message || "Có lỗi xảy ra, vui lòng thử lại";
  return "Có lỗi xảy ra, vui lòng thử lại";
}

export function errorCode(err: unknown): string | undefined {
  return err instanceof ApiRequestError ? err.code : undefined;
}

/** End the user session and go back to the PIN screen. The shift stays open on the device. */
export function useLockScreen() {
  const router = useRouter();
  const logout = useLogout();
  const { mutate } = logout;
  return useCallback(() => {
    mutate(undefined, { onSettled: () => router.replace("/pos/dang-nhap") });
  }, [mutate, router]);
}

export const ROLE_LABEL: Record<string, string> = {
  owner: "Chủ chuỗi",
  warehouse_manager: "Quản lý kho",
  branch_manager: "Quản lý",
  cashier: "Thu ngân",
  barista: "Pha chế",
};

export function isManagerRole(role: string | undefined): boolean {
  return role === "branch_manager" || role === "owner";
}
