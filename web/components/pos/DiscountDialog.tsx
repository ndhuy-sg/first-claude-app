"use client";
import { useState } from "react";
import type { Schemas } from "@/lib/api/client";
import { formatVND } from "@/lib/format";
import { discountAmount, keypadAppend, needsApproval } from "@/lib/pos/money";
import { useIdempotencyKey, useSetDiscount } from "@/lib/pos/mutations";
import { errorCode, errorMessage } from "@/lib/pos/util";
import { Dialog } from "./Dialog";
import { AmountKeypad } from "./Keypad";
import { PinPad } from "./PinPad";

export const DISCOUNT_REASONS = ["Khách quen", "Khuyến mãi tại quầy", "Món lỗi / phục vụ chậm", "Nhân viên", "Khác"];
const PCT_CHIPS = [5, 10, 15, 20, 25];

/** Screen 9 – manual discount. Above `maxPct` a manager must approve with their PIN (W-06, O-09). */
export function DiscountDialog({
  order,
  maxPct,
  managers,
  onClose,
}: {
  order: Schemas["Order"];
  maxPct: number;
  managers: { id: number; name: string }[];
  onClose: () => void;
}) {
  const set = useSetDiscount(order.id);
  const idem = useIdempotencyKey();
  const [type, setType] = useState<"percent" | "amount">(order.discount?.type ?? "percent");
  const [pct, setPct] = useState<number>(order.discount?.type === "percent" ? order.discount.value : 0);
  const [amountStr, setAmountStr] = useState(order.discount?.type === "amount" ? String(order.discount.value) : "");
  const [reason, setReason] = useState(order.discount?.reason || DISCOUNT_REASONS[0]);
  const [pickedManager, setManagerId] = useState<number | null>(null);
  const managerId = pickedManager ?? managers[0]?.id ?? null;
  const [error, setError] = useState("");

  const value = type === "percent" ? pct : Number(amountStr || "0");
  const amount = Math.min(discountAmount(order.subtotal, type, value), order.subtotal);
  const tooLarge = type === "amount" && value > order.subtotal;
  const approval = needsApproval(order.subtotal, type, value, maxPct);

  const apply = (extra: { managerId?: number; managerPin?: string } = {}) => {
    setError("");
    const body = { type, value, reason, ...extra };
    return set.mutateAsync({ ...body, key: idem.get(JSON.stringify(body)) }).then(onClose, (e) => {
      setError(errorCode(e) === "INVALID_MANAGER_PIN" ? "PIN quản lý không đúng" : errorMessage(e));
      throw e;
    });
  };

  return (
    <Dialog title="Giảm giá" onClose={onClose} width={approval ? 960 : 600}>
      <div className="flex gap-6">
        <div className="flex flex-1 flex-col gap-4">
          <div role="tablist" aria-label="Kiểu giảm giá" className="inline-flex self-start rounded-xl border border-line-strong p-1">
            {(
              [
                ["percent", "Theo %"],
                ["amount", "Theo số tiền"],
              ] as const
            ).map(([t, label]) => (
              <button
                key={t}
                type="button"
                role="tab"
                aria-selected={type === t}
                onClick={() => setType(t)}
                className={`h-12 rounded-lg px-5 font-semibold ${type === t ? "bg-navy text-white" : ""}`}
              >
                {label}
              </button>
            ))}
          </div>

          {type === "percent" ? (
            <div className="flex flex-wrap gap-2" aria-label="Mức giảm">
              {PCT_CHIPS.map((p) => (
                <button
                  key={p}
                  type="button"
                  aria-pressed={pct === p}
                  onClick={() => setPct(p)}
                  className={`h-14 w-20 rounded-xl border text-lg font-bold ${pct === p ? "border-navy bg-navy text-white" : "border-line-strong bg-white"} ${
                    p > maxPct ? "ring-2 ring-gold/60" : ""
                  }`}
                >
                  {p}%
                </button>
              ))}
            </div>
          ) : (
            <div className="flex flex-col gap-3">
              <p className="text-3xl font-bold" data-testid="discount-amount-input">
                {formatVND(Number(amountStr || "0"))}
              </p>
              <AmountKeypad onKey={(k) => setAmountStr((s) => keypadAppend(s, k))} keyHeight={52} />
            </div>
          )}

          <label className="flex flex-col gap-1.5 font-semibold">
            Lý do
            <select value={reason} onChange={(e) => setReason(e.target.value)} className="h-12 rounded-xl border border-line-strong bg-white px-3 font-normal">
              {DISCOUNT_REASONS.map((r) => (
                <option key={r}>{r}</option>
              ))}
            </select>
          </label>

          <div className="rounded-xl bg-surface p-4" aria-live="polite">
            <p className="flex justify-between">
              <span>Tạm tính</span> <span>{formatVND(order.subtotal)}</span>
            </p>
            <p className="flex justify-between font-semibold">
              <span>Giảm</span> <span data-testid="discount-preview">{formatVND(-amount)}</span>
            </p>
            <p className="flex justify-between text-xl font-bold">
              <span>Còn lại</span> <span>{formatVND(order.subtotal - amount)}</span>
            </p>
            {tooLarge && (
              <p role="alert" className="mt-2 font-semibold text-bad">
                Số tiền giảm lớn hơn tổng đơn.
              </p>
            )}
          </div>

          {!approval && error && (
            <p role="alert" className="font-semibold text-bad">
              {error}
            </p>
          )}
          {!approval && (
            <div className="flex gap-2">
              {order.discount && (
                <button
                  type="button"
                  onClick={() => {
                    const body = { type, value: 0, reason: "Bỏ giảm giá" };
                    set.mutate({ ...body, key: idem.get(JSON.stringify(body)) }, { onSuccess: onClose, onError: (e) => setError(errorMessage(e)) });
                  }}
                  className="h-14 flex-1 rounded-xl border border-line-strong font-semibold"
                >
                  Bỏ giảm giá
                </button>
              )}
              <button
                type="button"
                disabled={value <= 0 || tooLarge || set.isPending}
                onClick={() => void apply().catch(() => {})}
                className="h-14 flex-[2] rounded-xl bg-gold text-lg font-bold text-navy disabled:bg-subtle disabled:text-muted"
              >
                Áp dụng giảm giá
              </button>
            </div>
          )}
        </div>

        {approval && (
          <section aria-label="Cần quản lý duyệt" className="flex w-[380px] flex-col gap-3 rounded-2xl border border-gold-line bg-gold-soft p-5">
            <h3 className="text-lg font-bold">Cần quản lý duyệt</h3>
            <p className="text-sm text-[#5c4400]">Giảm trên {maxPct}% phải có quản lý nhập PIN.</p>
            {managers.length === 0 ? (
              <p className="font-semibold text-bad">Chi nhánh chưa có quản lý có PIN.</p>
            ) : (
              <>
                <div className="flex flex-wrap gap-2" aria-label="Quản lý duyệt">
                  {managers.map((m) => (
                    <button
                      key={m.id}
                      type="button"
                      aria-pressed={m.id === managerId}
                      onClick={() => setManagerId(m.id)}
                      className={`h-11 rounded-full px-4 font-semibold ${m.id === managerId ? "bg-navy text-white" : "bg-white"}`}
                    >
                      {m.name}
                    </button>
                  ))}
                </div>
                <PinPad
                  submitLabel="Duyệt"
                  keyHeight={52}
                  error={error}
                  busy={set.isPending || tooLarge || managerId === null}
                  onChange={() => setError("")}
                  onSubmit={(pin) => apply({ managerId: managerId!, managerPin: pin })}
                />
              </>
            )}
          </section>
        )}
      </div>
    </Dialog>
  );
}
