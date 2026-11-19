"use client";

import { useMutation, useQueryClient } from "@tanstack/react-query";
import { useRouter } from "next/navigation";
import { useState, type FormEvent } from "react";
import { Button } from "@/components/ui/Button";
import { Field } from "@/components/ui/Field";
import { api, ApiRequestError, unwrap } from "@/lib/api/client";
import { qk } from "@/lib/admin/hooks";
import { homeFor } from "@/lib/admin/nav";
import { isAdminRole } from "@/lib/admin/permissions";
import { formatTime } from "@/lib/format";

/** Text shown under the login form for an API error (A-02: generic message + remaining attempts, lock). */
export function loginErrorText(err: unknown): string {
  if (err instanceof ApiRequestError) {
    const d = err.body.details ?? {};
    if (err.status === 423 || err.code === "ACCOUNT_LOCKED") {
      const until = typeof d.lockedUntil === "string" ? ` đến ${formatTime(d.lockedUntil)}` : "";
      return `Tài khoản tạm khoá${until} do nhập sai mật khẩu nhiều lần. Vui lòng thử lại sau.`;
    }
    if (err.status === 401) {
      const left = typeof d.remainingAttempts === "number" ? d.remainingAttempts : null;
      const base = "Email hoặc mật khẩu không đúng.";
      if (left === null) return base;
      if (left <= 0) return `${base} Tài khoản sẽ bị khoá 15 phút.`;
      return `${base} Còn ${left} lần thử trước khi tài khoản bị khoá 15 phút.`;
    }
    return err.body.message;
  }
  return "Không kết nối được máy chủ, vui lòng thử lại";
}

export function LoginForm() {
  const router = useRouter();
  const qc = useQueryClient();
  const [email, setEmail] = useState("");
  const [password, setPassword] = useState("");
  const [roleError, setRoleError] = useState<string | null>(null);

  const login = useMutation({
    mutationFn: async () => unwrap(await api.POST("/auth/login", { body: { email: email.trim(), password } })),
    onSuccess: async (me) => {
      if (!isAdminRole(me.user.role)) {
        await api.POST("/auth/logout").catch(() => undefined);
        setRoleError("Tài khoản này chỉ dùng trên máy POS, không vào được trang quản trị.");
        return;
      }
      qc.setQueryData(qk.me, me);
      router.replace(homeFor(me.user.role));
    },
  });

  function submit(e: FormEvent) {
    e.preventDefault();
    setRoleError(null);
    login.mutate();
  }

  const error = roleError ?? (login.isError ? loginErrorText(login.error) : null);

  return (
    <div className="flex min-h-screen items-center justify-center bg-navy p-4">
      <form onSubmit={submit} className="w-full max-w-sm rounded-2xl bg-white p-7 shadow-xl" aria-labelledby="login-title">
        <div className="mb-6 text-center">
          <div className="font-display text-4xl font-bold text-gold">Mộc</div>
          <h1 id="login-title" className="mt-1 text-lg font-bold text-ink">
            Đăng nhập quản trị
          </h1>
        </div>
        <div className="flex flex-col gap-4">
          <Field label="Email" name="email" type="email" autoComplete="username" required value={email} onChange={(e) => setEmail(e.target.value)} />
          <Field label="Mật khẩu" name="password" type="password" autoComplete="current-password" required value={password} onChange={(e) => setPassword(e.target.value)} />
          {error && (
            <p role="alert" className="rounded-xl bg-bad-soft px-3.5 py-2.5 text-sm font-medium text-[#7a2a1e]">
              {error}
            </p>
          )}
          <Button type="submit" variant="primary" size="lg" disabled={login.isPending}>
            {login.isPending ? "Đang đăng nhập…" : "Đăng nhập"}
          </Button>
        </div>
        <p className="mt-5 text-center text-[13px] text-muted">Nhân viên bán hàng đăng nhập bằng PIN trên máy POS.</p>
      </form>
    </div>
  );
}
