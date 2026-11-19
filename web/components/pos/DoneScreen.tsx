"use client";
import { useRouter } from "next/navigation";
import { QRCodeSVG } from "qrcode.react";
import { useEffect, useState } from "react";
import { Field } from "@/components/ui/Field";
import type { Schemas } from "@/lib/api/client";
import { formatVND } from "@/lib/format";
import { isValidEmail, orderTitle, paymentMethodsLabel } from "@/lib/pos/money";
import { useEmailInvoice } from "@/lib/pos/mutations";
import { useOrder } from "@/lib/pos/queries";
import { errorMessage } from "@/lib/pos/util";
import { PosLoading, usePosSession } from "./PosFrame";

export const INVOICE_STATUS_NOTE: Record<Schemas["InvoiceStatus"], string> = {
  not_sent: "Hoá đơn đã được cấp số và lưu, khách tra cứu được ngay. Chưa gửi tới cơ quan thuế.",
  sending: "Đang gửi dữ liệu hoá đơn tới cơ quan thuế…",
  sent: "Đã gửi dữ liệu hoá đơn tới cơ quan thuế.",
  failed: "Gửi tới cơ quan thuế chưa thành công, hệ thống sẽ tự gửi lại. Khách vẫn tra cứu được.",
};

/** QR + numbers of an e-invoice (used on screen 5 and in the order detail of screen 11). */
export function InvoiceCard({ invoice, buyer }: { invoice: Schemas["InvoiceSummary"]; buyer: Schemas["Buyer"] }) {
  return (
    <div className="flex flex-col items-center gap-4">
      <div className="rounded-xl border border-line bg-white p-3">
        <QRCodeSVG value={invoice.lookupUrl} size={220} level="M" title="Mã QR tra cứu hoá đơn điện tử" data-testid="invoice-qr" />
      </div>
      <dl className="grid w-full grid-cols-2 gap-x-4 gap-y-1 text-sm">
        <dt className="text-muted">Ký hiệu</dt>
        <dd className="font-semibold">{invoice.symbol}</dd>
        <dt className="text-muted">Số hoá đơn</dt>
        <dd className="font-semibold" data-testid="invoice-number">
          {invoice.number}
        </dd>
        <dt className="text-muted">Người mua</dt>
        <dd className="font-semibold">{buyer.name || "Khách lẻ"}</dd>
        <dt className="text-muted">Mã tra cứu</dt>
        <dd className="font-mono font-semibold">{invoice.lookupCode}</dd>
      </dl>
      <p className="text-center text-xs text-muted">{INVOICE_STATUS_NOTE[invoice.status]}</p>
    </div>
  );
}

export function EmailInvoiceForm({ invoiceId, initialEmail, onDone }: { invoiceId: number; initialEmail?: string; onDone?: () => void }) {
  const send = useEmailInvoice();
  const [email, setEmail] = useState(initialEmail ?? "");
  const [touched, setTouched] = useState(false);
  const invalid = touched && !isValidEmail(email);
  return (
    <form
      className="flex items-end gap-2"
      onSubmit={(e) => {
        e.preventDefault();
        setTouched(true);
        if (isValidEmail(email)) send.mutate({ invoiceId, email: email.trim() }, { onSuccess: onDone });
      }}
    >
      <Field className="flex-1" label="Email khách" name="invoiceEmail" type="email" value={email} onChange={(e) => setEmail(e.target.value)} error={invalid ? "Email không hợp lệ" : undefined} />
      <button type="submit" disabled={send.isPending} className="h-12 rounded-xl bg-navy px-5 font-semibold text-white">
        Gửi
      </button>
      {send.isSuccess && (
        <span role="status" className="text-sm font-semibold text-ok">
          Đã xếp lịch gửi
        </span>
      )}
      {send.error && (
        <span role="alert" className="text-sm font-semibold text-bad">
          {errorMessage(send.error)}
        </span>
      )}
    </form>
  );
}

/** Screen 5 – payment done: amount, change, and the e-invoice QR for the customer to scan (W-07, W-08). */
export function DoneScreen({ orderId }: { orderId: number }) {
  const router = useRouter();
  const { me } = usePosSession();
  const order = useOrder(orderId);
  const [emailOpen, setEmailOpen] = useState(false);
  const o = order.data;
  const { refetch } = order;
  const waitingInvoice = !!o && !o.invoice;
  useEffect(() => {
    if (!waitingInvoice) return;
    const t = setInterval(() => void refetch(), 2000);
    return () => clearInterval(t);
  }, [waitingInvoice, refetch]);
  if (!o) return <PosLoading />;

  const cash = o.payments.filter((p) => p.method === "cash" && p.status === "confirmed");
  const cashReceived = cash.reduce((s, p) => s + (p.cashReceived ?? p.amount), 0);
  const change = cash.reduce((s, p) => s + (p.changeAmount ?? 0), 0);

  return (
    <div className="flex flex-1 gap-8 p-8">
      <section className="flex flex-1 flex-col gap-6">
        <div className="flex items-center gap-4">
          <span aria-hidden className="flex h-16 w-16 items-center justify-center rounded-full bg-ok text-3xl font-bold text-white">
            ✓
          </span>
          <div>
            <h1 className="text-3xl font-bold">Đã thanh toán {formatVND(o.total)}</h1>
            <p className="text-muted">
              {orderTitle(o)} · {paymentMethodsLabel(o.payments)} · Thu ngân {me.user.name}
            </p>
          </div>
        </div>
        {cash.length > 0 && (
          <div className="grid grid-cols-2 gap-4">
            <div className="rounded-2xl border border-line bg-white p-5">
              <p className="text-muted">Khách đưa</p>
              <p className="text-3xl font-bold" data-testid="done-given">
                {formatVND(cashReceived)}
              </p>
            </div>
            <div className="rounded-2xl border border-line bg-ok-soft p-5">
              <p className="text-[#2f4f3e]">Tiền thối</p>
              <p className="text-3xl font-bold" data-testid="done-change">
                {formatVND(change)}
              </p>
            </div>
          </div>
        )}
        {o.status === "refunded" && <p className="font-semibold text-bad">Đơn này đã được hoàn tiền.</p>}
        <div className="mt-auto flex flex-col gap-3">
          {emailOpen && o.invoice && <EmailInvoiceForm invoiceId={o.invoice.id} initialEmail={o.buyer.email} onDone={() => setEmailOpen(false)} />}
          <div className="grid grid-cols-2 gap-3">
            <button type="button" onClick={() => router.replace("/pos/ban")} className="h-16 rounded-[14px] bg-gold text-xl font-bold text-navy">
              Đơn mới
            </button>
            <button
              type="button"
              disabled={!o.invoice}
              onClick={() => setEmailOpen(true)}
              className="h-16 rounded-[14px] border border-line-strong bg-white text-lg font-semibold disabled:text-muted"
            >
              Gửi hoá đơn qua email
            </button>
          </div>
        </div>
      </section>
      <aside className="w-[440px] shrink-0 rounded-2xl border border-line bg-white p-6" aria-label="Hoá đơn điện tử">
        <h2 className="mb-4 text-center text-xl font-bold">Quét để nhận hoá đơn điện tử</h2>
        {o.invoice ? (
          <InvoiceCard invoice={o.invoice} buyer={o.buyer} />
        ) : (
          <p role="status" className="text-center text-muted">
            Đang phát hành hoá đơn…
          </p>
        )}
      </aside>
    </div>
  );
}
