"use client";
import Link from "next/link";
import { useRouter } from "next/navigation";
import { useEffect, useState } from "react";
import type { Schemas } from "@/lib/api/client";
import { formatDate, formatTime, formatVND } from "@/lib/format";
import { useOnline } from "@/lib/pos/connectivity";
import { countedTotal, DENOMINATIONS, SHIFT_NOTE_THRESHOLD, shiftName } from "@/lib/pos/money";
import { useCloseShift, useIdempotencyKey } from "@/lib/pos/mutations";
import { useShiftSummary } from "@/lib/pos/queries";
import { errorCode, errorMessage } from "@/lib/pos/util";
import { PosLoading, usePosSession } from "./PosFrame";

type Summary = Schemas["ShiftSummary"];

function Kpi({ label, value, testId }: { label: string; value: string; testId?: string }) {
  return (
    <div className="rounded-2xl border border-line bg-white p-4">
      <p className="text-sm text-muted">{label}</p>
      <p className="mt-1 text-2xl font-bold" data-testid={testId}>
        {value}
      </p>
    </div>
  );
}

function Check({ ok, children }: { ok: boolean; children: React.ReactNode }) {
  return (
    <li className="flex items-center gap-3">
      <span aria-hidden className={`flex h-6 w-6 items-center justify-center rounded-full text-sm font-bold ${ok ? "bg-ok-soft text-ok" : "bg-bad-soft text-bad"}`}>
        {ok ? "✓" : "!"}
      </span>
      <span>
        <span className="sr-only">{ok ? "Đạt: " : "Chưa đạt: "}</span>
        {children}
      </span>
    </li>
  );
}

function diffClass(diff: number) {
  return diff < 0 ? "text-bad" : diff > 0 ? "text-[#5c4400]" : "text-ok";
}

function signed(n: number) {
  return n > 0 ? `+${formatVND(n)}` : formatVND(n);
}

/** Screen 6 – shift close: KPIs, checklist, cash count by denomination, difference and note (W-10, S-02..S-04). */
export function ShiftCloseScreen() {
  const router = useRouter();
  const { shift, lock } = usePosSession();
  const [closedId, setClosedId] = useState<number | null>(null);
  const shiftId = shift?.id ?? closedId ?? undefined;
  const summary = useShiftSummary(shiftId);
  const close = useCloseShift(shiftId ?? 0);
  const idem = useIdempotencyKey();
  const online = useOnline();
  const [counts, setCounts] = useState<Record<number, number>>({});
  const [note, setNote] = useState("");
  const [openOrders, setOpenOrders] = useState<{ id: number; code: string }[]>([]);

  useEffect(() => {
    if (!shift && closedId === null) router.replace("/pos/mo-ca");
  }, [shift, closedId, router]);

  const s: Summary | undefined = summary.data;
  if (!s) return <PosLoading />;

  const counted = countedTotal(counts);
  const diff = counted - s.expectedCash;
  const noteRequired = Math.abs(diff) > SHIFT_NOTE_THRESHOLD;
  const closed = s.shift.status === "closed";
  const blocked = s.openOrders > 0 || s.pendingQrPayments > 0;

  const submit = () => {
    setOpenOrders([]);
    const body = { countedCash: counted, note: note.trim() || undefined };
    close.mutate(
      { ...body, key: idem.get(JSON.stringify(body)) },
      {
        onSuccess: (r) => setClosedId(r.shift.id),
        onError: (e) => {
          if (errorCode(e) === "OPEN_ORDERS") {
            const list = (e as { body?: { details?: { orders?: { id: number; code: string }[] } } }).body?.details?.orders;
            setOpenOrders(Array.isArray(list) ? list : []);
            void summary.refetch();
          }
        },
      },
    );
  };

  const shownDiff = closed ? (s.cashDiff ?? 0) : diff;
  const shownCounted = closed ? (s.countedCash ?? 0) : counted;

  return (
    <div className="flex min-w-0 flex-1">
      <section className="flex min-w-0 flex-1 flex-col gap-5 overflow-y-auto p-6">
        <div>
          <h1 className="text-2xl font-bold">
            {closed ? "Đã chốt ca" : "Chốt ca"} · {shiftName(s.shift.openedAt)}
          </h1>
          <p className="text-muted">
            Mở lúc {formatTime(s.shift.openedAt)} {formatDate(s.shift.openedAt)} bởi {s.shift.openedBy.name}
            {s.shift.closedAt ? ` · chốt lúc ${formatTime(s.shift.closedAt)}` : ""}
          </p>
        </div>
        <div className="grid grid-cols-3 gap-3">
          <Kpi label="Doanh thu" value={formatVND(s.revenue)} testId="kpi-revenue" />
          <Kpi label="Số đơn" value={String(s.orderCount)} />
          <Kpi label="Tiền mặt" value={formatVND(s.cashSales)} />
          <Kpi label="Chuyển khoản QR" value={formatVND(s.qrSales)} />
          <Kpi label="Tiền đầu ca" value={formatVND(s.shift.openingCash)} />
          <Kpi label="Hoàn tiền mặt" value={formatVND(s.cashRefunds)} />
        </div>

        <div className="rounded-2xl border border-line bg-white p-5">
          <h2 className="mb-3 font-bold">Kiểm tra trước khi chốt</h2>
          <ul className="flex flex-col gap-2">
            <Check ok={s.openOrders === 0}>
              {s.openOrders === 0 ? "Không còn đơn chưa thanh toán" : `Còn ${s.openOrders} đơn chưa thanh toán`}
              {s.openOrders > 0 && (
                <Link href="/pos/don-trong-ca" className="ml-2 font-semibold underline">
                  Xem đơn
                </Link>
              )}
            </Check>
            <Check ok={s.pendingQrPayments === 0}>
              {s.pendingQrPayments === 0 ? "Đã xác nhận mọi khoản chuyển khoản QR" : `Còn ${s.pendingQrPayments} khoản QR chưa xác nhận`}
            </Check>
            <Check ok={s.invoicesNotSent === 0}>
              {s.invoicesNotSent === 0
                ? "Mọi hoá đơn đã gửi cơ quan thuế"
                : `${s.invoicesNotSent} hoá đơn chưa gửi cơ quan thuế (hệ thống tự gửi, không chặn chốt ca)`}
            </Check>
          </ul>
          {openOrders.length > 0 && (
            <ul className="mt-3 text-sm text-bad">
              {openOrders.map((o) => (
                <li key={o.id}>
                  <Link href={`/pos/don/${o.id}`} className="underline">
                    {o.code}
                  </Link>
                </li>
              ))}
            </ul>
          )}
        </div>

        <div className="grid grid-cols-3 gap-3 rounded-2xl border border-line bg-white p-5">
          <div>
            <p className="text-sm text-muted">Tiền mặt theo hệ thống</p>
            <p className="text-2xl font-bold" data-testid="expected-cash">
              {formatVND(s.expectedCash)}
            </p>
            <p className="text-xs text-muted">= đầu ca + thu tiền mặt − hoàn tiền</p>
          </div>
          <div>
            <p className="text-sm text-muted">Tiền đếm được</p>
            <p className="text-2xl font-bold" data-testid="counted-cash">
              {formatVND(shownCounted)}
            </p>
          </div>
          <div>
            <p className="text-sm text-muted">Chênh lệch</p>
            <p className={`text-2xl font-bold ${diffClass(shownDiff)}`} data-testid="cash-diff">
              {signed(shownDiff)}
            </p>
            <p className="text-xs text-muted">{shownDiff < 0 ? "Thiếu tiền" : shownDiff > 0 ? "Thừa tiền" : "Khớp"}</p>
          </div>
        </div>

        {!closed && (
          <label className="flex flex-col gap-1.5 font-semibold">
            Ghi chú {noteRequired ? "(bắt buộc vì lệch trên 50.000đ)" : "(không bắt buộc)"}
            <input
              value={note}
              onChange={(e) => setNote(e.target.value)}
              aria-invalid={noteRequired && !note.trim() ? true : undefined}
              className="h-12 rounded-xl border border-line-strong px-3.5 font-normal aria-invalid:border-bad"
              placeholder="Lý do chênh lệch"
            />
          </label>
        )}
        {close.error && (
          <p role="alert" className="font-semibold text-bad">
            {errorMessage(close.error)}
          </p>
        )}
      </section>

      <aside className="flex w-[380px] shrink-0 flex-col gap-3 border-l border-line bg-white p-5" aria-label="Đếm tiền">
        <h2 className="font-bold">Đếm tiền theo mệnh giá</h2>
        <ul className="flex flex-col gap-2 overflow-y-auto">
          {DENOMINATIONS.map((d) => {
            const n = counts[d] ?? 0;
            const setN = (v: number) => setCounts((c) => ({ ...c, [d]: Math.max(0, v) }));
            return (
              <li key={d} className="flex items-center justify-between gap-2">
                <span className="w-24 font-semibold">{formatVND(d)}</span>
                <button
                  type="button"
                  aria-label={`Bớt một tờ ${formatVND(d)}`}
                  disabled={closed || n === 0}
                  onClick={() => setN(n - 1)}
                  className="h-11 w-11 rounded-lg border border-line-strong text-xl disabled:text-muted"
                >
                  −
                </button>
                <span className="w-8 text-center font-bold" data-testid={`count-${d}`}>
                  {n}
                </span>
                <button
                  type="button"
                  aria-label={`Thêm một tờ ${formatVND(d)}`}
                  disabled={closed}
                  onClick={() => setN(n + 1)}
                  className="h-11 w-11 rounded-lg border border-line-strong text-xl disabled:text-muted"
                >
                  +
                </button>
                <span className="w-28 text-right text-sm text-muted">{formatVND(d * n)}</span>
              </li>
            );
          })}
        </ul>
        <p className="mt-auto flex items-baseline justify-between border-t border-line pt-3">
          <span className="font-semibold">Tổng đếm</span>
          <span className="text-2xl font-bold" data-testid="counted-total">
            {formatVND(shownCounted)}
          </span>
        </p>
        {closed ? (
          <button type="button" onClick={lock} className="h-16 rounded-[14px] bg-gold text-xl font-bold text-navy">
            Đăng xuất
          </button>
        ) : (
          <button
            type="button"
            onClick={submit}
            disabled={!online || close.isPending || blocked || (noteRequired && !note.trim())}
            className="h-16 rounded-[14px] bg-gold text-xl font-bold text-navy disabled:cursor-not-allowed disabled:bg-subtle disabled:text-muted"
          >
            Chốt ca
          </button>
        )}
      </aside>
    </div>
  );
}
