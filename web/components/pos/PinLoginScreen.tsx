"use client";
import { useRouter } from "next/navigation";
import { useEffect, useState } from "react";
import { formatDate, formatTime } from "@/lib/format";
import { usePinLogin } from "@/lib/pos/mutations";
import { useDevice } from "@/lib/pos/queries";
import { errorCode, errorMessage, ROLE_LABEL, useNow } from "@/lib/pos/util";
import { PosLoading } from "./PosFrame";
import { initial, Logo } from "./PosHeader";
import { PinPad } from "./PinPad";

/** Screen 1 – PIN login (W-02). */
export function PinLoginScreen() {
  const router = useRouter();
  const device = useDevice();
  const login = usePinLogin();
  const [staffId, setStaffId] = useState<number | null>(null);
  const [error, setError] = useState("");
  const now = useNow(10_000);

  useEffect(() => {
    if (device.isSuccess && device.data === null) router.replace("/pos/kich-hoat");
  }, [device.isSuccess, device.data, router]);

  if (!device.data) return <PosLoading />;
  const { device: dev, staff } = device.data;
  const selected = staff.find((s) => s.id === staffId);

  const submit = (pin: string) =>
    login.mutateAsync({ userId: staffId!, pin }).then(
      () => router.replace("/pos"),
      (e) => {
        setError(errorCode(e) === "INVALID_PIN" || errorCode(e) === "UNAUTHENTICATED" ? "Mã PIN không đúng, vui lòng thử lại" : errorMessage(e));
        throw e;
      },
    );

  return (
    <div className="flex h-dvh">
      <aside className="flex w-[500px] shrink-0 flex-col gap-6 bg-navy p-10 text-white">
        <div>
          <Logo size={56} />
          <p className="mt-3 text-lg font-semibold" data-testid="device-branch">
            Chi nhánh {dev.branchCode} · {dev.name}
          </p>
        </div>
        <div>
          <p className="text-5xl font-bold tabular-nums" suppressHydrationWarning>{formatTime(new Date(now))}</p>
          <p className="mt-1 text-white/70" suppressHydrationWarning>{formatDate(new Date(now))}</p>
        </div>
        <div className="flex min-h-0 flex-1 flex-col gap-3">
          <h2 className="text-base font-semibold text-white/80">Chọn nhân viên</h2>
          <div className="grid grid-cols-2 gap-3 overflow-y-auto">
            {staff.map((s) => (
              <button
                key={s.id}
                type="button"
                aria-pressed={s.id === staffId}
                onClick={() => {
                  setStaffId(s.id);
                  setError("");
                }}
                className={`flex h-16 items-center gap-3 rounded-xl px-3 text-left ${
                  s.id === staffId ? "bg-gold text-navy" : "bg-navy-2 text-white hover:bg-navy-3"
                }`}
              >
                <span aria-hidden className={`flex h-10 w-10 items-center justify-center rounded-full font-bold ${s.id === staffId ? "bg-navy text-gold" : "bg-white/15"}`}>
                  {initial(s.name)}
                </span>
                <span>
                  <span className="block font-semibold">{s.name}</span>
                  <span className="block text-xs opacity-75">{ROLE_LABEL[s.role]}</span>
                </span>
              </button>
            ))}
          </div>
        </div>
        <p className="text-sm text-white/60">Tự khoá sau 15 phút không thao tác.</p>
      </aside>
      <section className="flex flex-1 items-center justify-center bg-white p-10">
        <div className="w-[380px]">
          <h1 className="mb-6 text-center text-3xl font-bold">{selected ? `Xin chào, ${selected.name}` : "Chọn nhân viên để vào ca"}</h1>
          <PinPad key={staffId ?? 0} onSubmit={submit} submitLabel="Vào ca" error={error} busy={!selected || login.isPending} onChange={() => setError("")} />
          <p className="mt-6 text-center text-sm text-muted">Quên PIN? Nhờ quản lý chi nhánh đặt lại.</p>
        </div>
      </section>
    </div>
  );
}
