"use client";
import { useQueryClient } from "@tanstack/react-query";
import { useRouter } from "next/navigation";
import { useState, type FormEvent } from "react";
import { api, unwrap, type Schemas } from "@/lib/api/client";
import { Button } from "@/components/ui/Button";
import { Field } from "@/components/ui/Field";
import { useActivateDevice, useLogout, useManagerLogin } from "@/lib/pos/mutations";
import { posKeys } from "@/lib/pos/queries";
import { errorMessage } from "@/lib/pos/util";
import { Logo } from "./PosHeader";

/**
 * Screen 7 – device activation (W-01). A branch manager (or the owner) signs in with email + password,
 * picks the branch and names the tablet. Afterwards the manager session is closed so the tablet
 * is left on the PIN screen, showing the branch.
 */
export function ActivateScreen() {
  const router = useRouter();
  const qc = useQueryClient();
  const login = useManagerLogin();
  const activate = useActivateDevice();
  const logout = useLogout();
  const [email, setEmail] = useState("");
  const [password, setPassword] = useState("");
  const [branches, setBranches] = useState<Schemas["Branch"][] | null>(null);
  const [branchId, setBranchId] = useState<number | "">("");
  const [name, setName] = useState("iPad quầy 1");
  const [error, setError] = useState("");

  const signIn = async (e: FormEvent) => {
    e.preventDefault();
    setError("");
    try {
      const me = await login.mutateAsync({ email: email.trim(), password });
      if (me.user.role !== "branch_manager" && me.user.role !== "owner") {
        setError("Chỉ quản lý chi nhánh hoặc chủ chuỗi được kích hoạt máy.");
        return;
      }
      const list = await api.GET("/branches").then(unwrap);
      const active = list.filter((b) => b.isActive);
      setBranches(active);
      setBranchId(me.user.branchId ?? active[0]?.id ?? "");
    } catch (err) {
      setError(errorMessage(err));
    }
  };

  const submit = async (e: FormEvent) => {
    e.preventDefault();
    if (branchId === "" || !name.trim()) return;
    setError("");
    try {
      await activate.mutateAsync({ email: email.trim(), password, branchId, name: name.trim() });
      // Close the manager session: staff log in with their PIN on this tablet.
      await logout.mutateAsync();
      await qc.invalidateQueries({ queryKey: posKeys.device });
      router.replace("/pos/dang-nhap");
    } catch (err) {
      setError(errorMessage(err));
    }
  };

  return (
    <div className="flex h-dvh">
      <aside className="flex w-[500px] shrink-0 flex-col justify-between bg-navy p-10 text-white">
        <div>
          <Logo size={56} />
          <p className="mt-4 text-xl font-semibold">Kích hoạt máy bán hàng</p>
          <p className="mt-2 text-white/70">Máy tính bảng này chưa được gắn với chi nhánh nào. Quản lý chi nhánh đăng nhập để kích hoạt.</p>
        </div>
        <ol className="space-y-2 text-white/80">
          <li>1. Quản lý đăng nhập bằng email và mật khẩu</li>
          <li>2. Chọn chi nhánh và đặt tên máy</li>
          <li>3. Nhân viên đăng nhập bằng PIN</li>
        </ol>
      </aside>
      <section className="flex flex-1 items-center justify-center bg-white p-10">
        {!branches ? (
          <form onSubmit={signIn} className="flex w-[420px] flex-col gap-4" aria-label="Quản lý đăng nhập">
            <h1 className="text-3xl font-bold">Quản lý đăng nhập</h1>
            <Field label="Email" name="email" type="email" autoComplete="username" value={email} onChange={(e) => setEmail(e.target.value)} required />
            <Field
              label="Mật khẩu"
              name="password"
              type="password"
              autoComplete="current-password"
              value={password}
              onChange={(e) => setPassword(e.target.value)}
              required
            />
            {error && (
              <p role="alert" className="font-semibold text-bad">
                {error}
              </p>
            )}
            <Button type="submit" variant="primary" size="xl" disabled={login.isPending || !email || !password}>
              Tiếp tục
            </Button>
          </form>
        ) : (
          <form onSubmit={submit} className="flex w-[420px] flex-col gap-4" aria-label="Kích hoạt máy">
            <h1 className="text-3xl font-bold">Gắn máy vào chi nhánh</h1>
            <label className="flex flex-col gap-1.5 text-sm font-semibold text-muted">
              Chi nhánh
              <select
                name="branch"
                value={branchId}
                onChange={(e) => setBranchId(Number(e.target.value))}
                className="h-12 rounded-xl border border-line-strong bg-white px-3 text-base font-normal text-ink"
              >
                {branches.map((b) => (
                  <option key={b.id} value={b.id}>
                    {b.code} · {b.name}
                  </option>
                ))}
              </select>
            </label>
            <Field label="Tên máy" name="deviceName" value={name} onChange={(e) => setName(e.target.value)} required />
            {error && (
              <p role="alert" className="font-semibold text-bad">
                {error}
              </p>
            )}
            <Button type="submit" variant="primary" size="xl" disabled={activate.isPending || branchId === "" || !name.trim()}>
              Kích hoạt máy
            </Button>
          </form>
        )}
      </section>
    </div>
  );
}
