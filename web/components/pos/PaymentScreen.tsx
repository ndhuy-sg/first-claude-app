"use client";
import { useRouter } from "next/navigation";
import { QRCodeSVG } from "qrcode.react";
import { useEffect, useRef, useState } from "react";
import type { Schemas } from "@/lib/api/client";
import { formatVND } from "@/lib/format";
import { useOnline } from "@/lib/pos/connectivity";
import { itemDetail, keypadAppend, orderTitle, vatFromGross } from "@/lib/pos/money";
import { useCancelPayment, useConfirmPayment, useCreatePayment, useIdempotencyKey, useSetBuyer } from "@/lib/pos/mutations";
import { useCashSuggestions, useOrder } from "@/lib/pos/queries";
import { errorMessage } from "@/lib/pos/util";
import { buyerBody, BuyerForm, draftFromBuyer, sameBuyer, validateBuyer, type BuyerDraft, type BuyerErrors } from "./BuyerForm";
import { AmountKeypad } from "./Keypad";
import { PosLoading, usePosSession } from "./PosFrame";

type Order = Schemas["Order"];
type Tab = "cash" | "qr" | "mixed";

const BANKS: Record<string, string> = {
  "970436": "Vietcombank",
  "970415": "VietinBank",
  "970418": "BIDV",
  "970405": "Agribank",
  "970407": "Techcombank",
  "970422": "MB Bank",
  "970423": "TPBank",
  "970432": "VPBank",
  "970416": "ACB",
};

function OrderSummary({ order, vatRateBp, due, children }: { order: Order; vatRateBp: number | undefined; due: number; children: React.ReactNode }) {
  const vat = vatRateBp !== undefined ? vatFromGross(order.total, vatRateBp) : null;
  return (
    <aside className="flex w-[420px] shrink-0 flex-col gap-4 overflow-y-auto border-r border-line bg-white p-6" aria-label="Tóm tắt đơn">
      <div>
        <h2 className="text-2xl font-bold">{orderTitle(order)}</h2>
        <p className="text-sm text-muted">{order.code}</p>
      </div>
      <ul className="flex flex-col gap-2">
        {order.items.map((i) => (
          <li key={i.id} className="flex justify-between gap-3">
            <span>
              {i.qty} × {i.name}
              {itemDetail(i) && <span className="block text-xs text-muted">{itemDetail(i)}</span>}
            </span>
            <span className="font-semibold">{formatVND(i.lineTotal)}</span>
          </li>
        ))}
      </ul>
      <dl className="flex flex-col gap-1 border-t border-line pt-3">
        <div className="flex justify-between text-muted">
          <dt>Tạm tính</dt>
          <dd>{formatVND(order.subtotal)}</dd>
        </div>
        <div className="flex justify-between text-muted">
          <dt>Giảm giá</dt>
          <dd>{formatVND(-(order.discount?.amount ?? 0))}</dd>
        </div>
        {order.paidAmount > 0 && (
          <div className="flex justify-between text-muted">
            <dt>Đã thu</dt>
            <dd>{formatVND(-order.paidAmount)}</dd>
          </div>
        )}
        <div className="flex items-baseline justify-between">
          <dt className="font-bold">Khách cần trả</dt>
          <dd className="text-[32px] font-bold" data-testid="amount-due">
            {formatVND(due)}
          </dd>
        </div>
        {vat && (
          <p className="text-sm text-muted">
            Giá đã gồm thuế GTGT {vatRateBp! / 100}% · Tiền thuế {formatVND(vat.vat)}
          </p>
        )}
      </dl>
      <div className="border-t border-line pt-4">{children}</div>
    </aside>
  );
}

function QrPanel({
  payment,
  order,
  bank,
  busy,
  onCancel,
  onConfirm,
}: {
  payment: Schemas["Payment"];
  order: Order;
  bank: Schemas["VietQRAccount"] | undefined;
  busy: boolean;
  onCancel: () => void;
  onConfirm: () => void;
}) {
  return (
    <div className="flex gap-6 rounded-2xl border border-line bg-white p-5" data-testid="qr-panel">
      <div className="rounded-xl border border-line p-3">
        <QRCodeSVG value={payment.qrPayload} size={240} level="M" title={`Mã VietQR ${formatVND(payment.amount)}`} data-testid="vietqr-code" />
      </div>
      <div className="flex flex-1 flex-col gap-2">
        <p className="text-sm text-muted">Số tiền</p>
        <p className="text-3xl font-bold" data-testid="qr-amount">
          {formatVND(payment.amount)}
        </p>
        {bank && (
          <dl className="text-sm">
            <div className="flex gap-2">
              <dt className="text-muted">Ngân hàng</dt>
              <dd className="font-semibold">{BANKS[bank.bankBin] ?? bank.bankBin}</dd>
            </div>
            <div className="flex gap-2">
              <dt className="text-muted">Số tài khoản</dt>
              <dd className="font-semibold">{bank.accountNo}</dd>
            </div>
            <div className="flex gap-2">
              <dt className="text-muted">Chủ tài khoản</dt>
              <dd className="font-semibold">{bank.accountName}</dd>
            </div>
            <div className="flex gap-2">
              <dt className="text-muted">Nội dung</dt>
              <dd className="font-semibold">{order.code}</dd>
            </div>
          </dl>
        )}
        <p className="mt-1 font-semibold text-[#5c4400]" role="status">
          Đang chờ khách chuyển khoản…
        </p>
        <div className="mt-auto grid grid-cols-2 gap-2">
          <button type="button" onClick={onCancel} disabled={busy} className="h-14 rounded-xl border border-line-strong font-semibold">
            Huỷ QR
          </button>
          <button type="button" onClick={onConfirm} disabled={busy} className="h-14 rounded-xl bg-navy font-bold text-white disabled:bg-subtle disabled:text-muted">
            Đã nhận tiền
          </button>
        </div>
      </div>
    </div>
  );
}

/** Screen 4 – payment: cash with suggestions and change, VietQR, or both; retail or company invoice (W-07, W-08, W-09, W-14). */
export function PaymentScreen({ orderId }: { orderId: number }) {
  const router = useRouter();
  const { branch } = usePosSession();
  const order = useOrder(orderId);
  const online = useOnline();
  const createPay = useCreatePayment(orderId);
  const confirmPay = useConfirmPayment();
  const cancelPay = useCancelPayment();
  const setBuyer = useSetBuyer(orderId);
  const payKey = useIdempotencyKey();
  const qrKey = useIdempotencyKey();
  const confirmKey = useIdempotencyKey();

  const o = order.data;
  const due = o ? Math.max(0, o.total - o.paidAmount) : 0;
  const pendingQr = o?.payments.find((p) => p.method === "vietqr" && p.status === "pending");

  const [tab, setTab] = useState<Tab | null>(null);
  const activeTab: Tab = tab ?? (pendingQr ? "qr" : "cash");
  const [given, setGiven] = useState("");
  const [cashPart, setCashPart] = useState("");
  const [buyer, setBuyerDraft] = useState<BuyerDraft | null>(null);
  const [buyerErrors, setBuyerErrors] = useState<BuyerErrors>({});
  const [error, setError] = useState("");
  const suggestions = useCashSuggestions(activeTab === "cash" ? due : 0);
  const draft = buyer ?? draftFromBuyer(o?.buyer);

  useEffect(() => {
    if (o?.status === "paid") router.replace(`/pos/hoan-tat/${orderId}`);
    else if (o && o.status !== "open") router.replace("/pos/ban");
  }, [o, orderId, router]);

  // QR tab: create the VietQR payment for the amount due as soon as the tab is shown (once per amount).
  const qrRequested = useRef<number | null>(null);
  useEffect(() => {
    if (activeTab !== "qr" || !o || o.status !== "open" || pendingQr || due <= 0 || !online || createPay.isPending) return;
    if (qrRequested.current === due) return;
    qrRequested.current = due;
    createPay.mutate({ method: "vietqr", amount: due, key: qrKey.get(`qr:${due}`) }, { onError: (e) => setError(errorMessage(e)) });
  }, [activeTab, o, pendingQr, due, online, createPay, qrKey]);

  if (!o) return <PosLoading />;

  const busy = createPay.isPending || confirmPay.isPending || cancelPay.isPending || setBuyer.isPending;

  /** Saves the buyer before the payment that completes the order (the invoice is issued in that transaction). */
  const saveBuyer = async (): Promise<boolean> => {
    const errs = validateBuyer(draft);
    setBuyerErrors(errs);
    if (Object.keys(errs).length) {
      setError("Kiểm tra lại thông tin hoá đơn công ty.");
      return false;
    }
    const body = buyerBody(draft);
    if (!sameBuyer(body, o.buyer)) await setBuyer.mutateAsync(body);
    return true;
  };

  const run = async (fn: () => Promise<void>) => {
    setError("");
    try {
      await fn();
    } catch (e) {
      setError(errorMessage(e));
    }
  };

  const afterPayment = (r: Schemas["PaymentResult"]) => {
    if (r.order.status === "paid") router.replace(`/pos/hoan-tat/${orderId}`);
  };

  const givenAmount = Number(given || "0");
  const change = givenAmount - due;
  const payCash = () =>
    run(async () => {
      if (!(await saveBuyer())) return;
      const r = await createPay.mutateAsync({ method: "cash", amount: due, cashReceived: givenAmount, key: payKey.get(`cash:${due}:${givenAmount}`) });
      afterPayment(r);
    });

  const confirmQr = () =>
    run(async () => {
      if (!pendingQr || !(await saveBuyer())) return;
      const r = await confirmPay.mutateAsync({ paymentId: pendingQr.id, key: confirmKey.get(`confirm:${pendingQr.id}`) });
      afterPayment(r);
    });

  const cancelQr = () =>
    run(async () => {
      if (!pendingQr) return;
      await cancelPay.mutateAsync(pendingQr.id);
      qrRequested.current = null;
      qrKey.reset();
      setTab("cash");
    });

  const cashPartAmount = Math.min(Number(cashPart || "0"), due);
  const startMixed = () =>
    run(async () => {
      if (!(await saveBuyer())) return;
      if (cashPartAmount > 0) {
        const r = await createPay.mutateAsync({
          method: "cash",
          amount: cashPartAmount,
          cashReceived: cashPartAmount,
          key: payKey.get(`mixed-cash:${due}:${cashPartAmount}`),
        });
        if (r.order.status === "paid") return afterPayment(r);
      }
      const rest = due - cashPartAmount;
      await createPay.mutateAsync({ method: "vietqr", amount: rest, key: qrKey.get(`qr:${rest}`) });
      qrRequested.current = rest;
    });

  const switchTab = (t: Tab) => {
    if (t === activeTab) return;
    // Leaving the QR tab with a QR on screen cancels it (P-05). In the mixed flow the QR stays.
    if (activeTab === "qr" && pendingQr) {
      void run(async () => {
        await cancelPay.mutateAsync(pendingQr.id);
        qrRequested.current = null;
        qrKey.reset();
      });
    }
    setError("");
    setTab(t);
  };

  let action: { label: string; enabled: boolean };
  if (!online) action = { label: "Chờ kết nối để thanh toán", enabled: false };
  else if (activeTab === "cash") action = change >= 0 ? { label: "Hoàn tất thanh toán", enabled: true } : { label: `Còn thiếu ${formatVND(-change)}`, enabled: false };
  else if (pendingQr) action = { label: "Bấm “Đã nhận tiền” khi khách đã chuyển", enabled: false };
  else if (activeTab === "mixed")
    action = { label: `Thu ${formatVND(cashPartAmount)} tiền mặt · tạo QR ${formatVND(due - cashPartAmount)}`, enabled: due > 0 };
  else action = { label: "Đang tạo mã QR…", enabled: false };
  const onAction = () => {
    if (activeTab === "cash") void payCash();
    else if (activeTab === "mixed" && !pendingQr) void startMixed();
  };

  return (
    <div className="flex min-w-0 flex-1">
      <OrderSummary order={o} vatRateBp={branch?.vatRateBp} due={due}>
        <BuyerForm value={draft} onChange={setBuyerDraft} errors={buyerErrors} />
      </OrderSummary>

      <section className="flex min-w-0 flex-1 flex-col gap-4 p-6">
        <div role="tablist" aria-label="Cách thanh toán" className="grid grid-cols-3 gap-2">
          {(
            [
              ["cash", "Tiền mặt"],
              ["qr", "Chuyển khoản QR"],
              ["mixed", "Kết hợp"],
            ] as const
          ).map(([t, label]) => (
            <button
              key={t}
              type="button"
              role="tab"
              aria-selected={activeTab === t}
              onClick={() => switchTab(t)}
              disabled={busy}
              className={`h-14 rounded-xl text-lg font-semibold ${activeTab === t ? "bg-navy text-white" : "border border-line-strong bg-white"}`}
            >
              {label}
            </button>
          ))}
        </div>

        {activeTab === "cash" && (
          <div className="flex min-h-0 flex-1 gap-5">
            <div className="flex flex-1 flex-col gap-3">
              <p className="text-muted">Khách đưa</p>
              <p className="text-5xl font-bold tabular-nums" data-testid="cash-given">
                {formatVND(givenAmount)}
              </p>
              <div className="grid grid-cols-3 gap-2" aria-label="Gợi ý mệnh giá">
                {(suggestions.data ?? []).map((s) => (
                  <button
                    key={s}
                    type="button"
                    aria-pressed={givenAmount === s}
                    onClick={() => setGiven(String(s))}
                    className={`h-14 rounded-xl border text-lg font-bold ${givenAmount === s ? "border-navy bg-navy text-white" : "border-line-strong bg-white"}`}
                  >
                    {formatVND(s)}
                  </button>
                ))}
              </div>
              <div
                data-testid="change-box"
                className={`mt-auto rounded-2xl p-5 ${change >= 0 ? "bg-ok-soft text-[#2f4f3e]" : "bg-bad-soft text-[#7a2a1e]"}`}
                aria-live="polite"
              >
                <p className="font-semibold">{change >= 0 ? "Tiền thối" : "Còn thiếu"}</p>
                <p className="text-4xl font-bold">{formatVND(Math.abs(change))}</p>
              </div>
            </div>
            <div className="w-[340px]">
              <AmountKeypad onKey={(k) => setGiven((g) => keypadAppend(g, k))} />
            </div>
          </div>
        )}

        {activeTab === "qr" && (
          <div className="flex-1">
            {pendingQr ? (
              <QrPanel payment={pendingQr} order={o} bank={branch?.vietqr} busy={busy || !online} onCancel={cancelQr} onConfirm={confirmQr} />
            ) : (
              <p className="text-muted" role="status">
                Đang tạo mã QR…
              </p>
            )}
          </div>
        )}

        {activeTab === "mixed" && (
          <div className="flex min-h-0 flex-1 gap-5">
            {pendingQr ? (
              <div className="flex-1">
                <QrPanel payment={pendingQr} order={o} bank={branch?.vietqr} busy={busy || !online} onCancel={cancelQr} onConfirm={confirmQr} />
              </div>
            ) : (
              <>
                <div className="flex flex-1 flex-col gap-3">
                  <p className="text-muted">Phần trả bằng tiền mặt</p>
                  <p className="text-5xl font-bold tabular-nums" data-testid="cash-part">
                    {formatVND(cashPartAmount)}
                  </p>
                  <p className="text-lg">
                    Phần chuyển khoản QR: <strong data-testid="qr-part">{formatVND(due - cashPartAmount)}</strong>
                  </p>
                  <p className="text-sm text-muted">Thu tiền mặt trước, sau đó máy hiện mã QR cho phần còn lại.</p>
                </div>
                <div className="w-[340px]">
                  <AmountKeypad onKey={(k) => setCashPart((g) => keypadAppend(g, k))} />
                </div>
              </>
            )}
          </div>
        )}

        {error && (
          <p role="alert" className="font-semibold text-bad">
            {error}
          </p>
        )}
        <button
          type="button"
          onClick={onAction}
          disabled={!action.enabled || busy}
          className="h-16 shrink-0 rounded-[14px] bg-gold text-xl font-bold text-navy disabled:cursor-not-allowed disabled:bg-subtle disabled:text-muted"
        >
          {action.label}
        </button>
      </section>
    </div>
  );
}
