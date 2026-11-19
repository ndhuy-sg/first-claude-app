"use client";

import { useQueryClient } from "@tanstack/react-query";
import { useRouter } from "next/navigation";
import { Suspense, useEffect, type ReactNode } from "react";
import { Button } from "@/components/ui/Button";
import { api, ApiRequestError } from "@/lib/api/client";
import { errorMessage, MeContext, useMeQuery } from "@/lib/admin/hooks";
import { roleLabels } from "@/lib/admin/labels";
import { isAdminRole, type Role } from "@/lib/admin/permissions";
import { ErrorBox, Loading } from "./parts";
import { NavList, SidebarNav } from "./Sidebar";

export const LOGIN_PATH = "/admin/dang-nhap";

/**
 * Admin layout + guard: GET /auth/me. Unauthenticated → login page.
 * Cashiers/baristas (POS-only roles) get a "no access" message.
 */
export function AdminShell({ children }: { children: ReactNode }) {
  const router = useRouter();
  const qc = useQueryClient();
  const me = useMeQuery();
  const unauthenticated = me.error instanceof ApiRequestError && me.error.status === 401;

  useEffect(() => {
    if (unauthenticated) router.replace(LOGIN_PATH);
  }, [unauthenticated, router]);

  async function logout() {
    await api.POST("/auth/logout").catch(() => undefined);
    qc.clear();
    router.replace(LOGIN_PATH);
  }

  if (me.isPending || unauthenticated) {
    return (
      <div className="flex min-h-screen items-center justify-center">
        <Loading label={unauthenticated ? "Đang chuyển tới trang đăng nhập…" : "Đang kiểm tra đăng nhập…"} />
      </div>
    );
  }
  if (me.isError) {
    return (
      <div className="mx-auto max-w-lg p-6">
        <ErrorBox message={errorMessage(me.error)} onRetry={() => me.refetch()} />
      </div>
    );
  }

  const user = me.data.user;
  if (!isAdminRole(user.role)) {
    return (
      <div className="mx-auto mt-24 max-w-md rounded-2xl border border-line bg-white p-6 text-center">
        <h1 className="text-lg font-bold">Không có quyền vào trang quản trị</h1>
        <p className="mt-2 text-sm text-muted">Tài khoản {roleLabels[user.role].toLowerCase()} chỉ dùng trên máy POS.</p>
        <Button variant="primary" className="mt-4" onClick={logout}>
          Đăng xuất
        </Button>
      </div>
    );
  }

  return (
    <MeContext.Provider value={me.data}>
      <div className="flex min-h-screen flex-wrap">
        <aside className="w-full bg-navy px-3 py-5 text-white md:sticky md:top-0 md:h-screen md:w-60 md:shrink-0 md:overflow-y-auto">
          <div className="mb-5 flex items-baseline gap-2 px-3">
            <span className="font-display text-2xl font-bold text-gold">Mộc</span>
            <span className="text-sm text-white/70">Quản trị</span>
          </div>
          <Suspense fallback={<NavList role={user.role as Role} pathname="" />}>
            <SidebarNav role={user.role} />
          </Suspense>
          <div className="mt-6 border-t border-white/15 px-3 pt-4 text-sm">
            <div className="font-semibold">{user.name}</div>
            <div className="text-white/60">{roleLabels[user.role]}</div>
            <button type="button" onClick={logout} className="mt-2 h-10 font-semibold text-gold underline-offset-2 hover:underline">
              Đăng xuất
            </button>
          </div>
        </aside>
        <main className="min-w-0 flex-1 basis-[640px] bg-surface p-4 md:p-8">{children}</main>
      </div>
    </MeContext.Provider>
  );
}
