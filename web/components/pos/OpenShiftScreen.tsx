"use client";
import { useRouter } from "next/navigation";
import { useEffect, useState } from "react";
import { Button } from "@/components/ui/Button";
import { formatTime, formatVND } from "@/lib/format";
import { keypadAppend, shiftName } from "@/lib/pos/money";
import { useIdempotencyKey, useOpenShift } from "@/lib/pos/mutations";
import { errorCode, errorMessage, useNow } from "@/lib/pos/util";
import { AmountKeypad } from "./Keypad";
import { usePosSession } from "./PosFrame";

function Tile({ label, value }: { label: string; value: string }) {
  return (
    <div className="rounded-2xl border border-line bg-white p-5">
      <p className="text-sm text-muted">{label}</p>
      <p className="mt-1 text-xl font-bold" suppressHydrationWarning>
        {value}
      </p>
    </div>
  );
}

/** Screen 8 – open shift with the opening cash (W-10). Rendered inside <PosFrame requireShift={false}>. */
export function OpenShiftScreen() {
  const router = useRouter();
  const { me, shift } = usePosSession();
  const open = useOpenShift();
  const idem = useIdempotencyKey();
  const [cash, setCash] = useState("");
  const now = useNow();
  const amount = Number(cash || "0");

  useEffect(() => {
    if (shift) router.replace("/pos/ban");
  }, [shift, router]);

  const submit = () =>
    open.mutate(
      { openingCash: amount, key: idem.get(String(amount)) },
      {
        onSuccess: () => router.replace("/pos/ban"),
        onError: (e) => {
          if (errorCode(e) === "SHIFT_ALREADY_OPEN") router.replace("/pos/ban");
        },
      },
    );

  return (
    <div className="flex flex-1 gap-8 p-8">
      <section className="flex flex-1 flex-col gap-6">
        <h1 className="text-3xl font-bold">Mở ca</h1>
        <div className="grid grid-cols-3 gap-4">
          <Tile label="Ca" value={shiftName(new Date(now).toISOString())} />
          <Tile label="Máy" value={`${me.device?.branchCode ?? ""} · ${me.device?.name ?? ""}`} />
          <Tile label="Bắt đầu" value={formatTime(new Date(now))} />
        </div>
        <div className="rounded-2xl border border-line bg-white p-6">
          <p className="text-muted">Tiền đầu ca (tiền mặt trong két)</p>
          <p className="mt-2 text-5xl font-bold tabular-nums" data-testid="opening-cash">
            {formatVND(amount)}
          </p>
          <p className="mt-3 text-sm text-muted">Đếm tiền lẻ trong két trước khi bán. Số này dùng để đối chiếu khi chốt ca.</p>
        </div>
        {open.error && (
          <p role="alert" className="font-semibold text-bad">
            {errorMessage(open.error)}
          </p>
        )}
        <Button variant="primary" size="xl" className="mt-auto" onClick={submit} disabled={open.isPending}>
          Mở ca và bắt đầu bán
        </Button>
      </section>
      <section className="w-[400px]">
        <AmountKeypad onKey={(k) => setCash((c) => keypadAppend(c, k))} keyHeight={76} />
      </section>
    </div>
  );
}
