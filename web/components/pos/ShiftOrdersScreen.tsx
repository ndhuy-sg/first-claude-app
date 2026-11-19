"use client";
import Link from "next/link";
import { useDeferredValue, useState } from "react";
import { Chip } from "@/components/ui/Chip";
import type { Schemas } from "@/lib/api/client";
import { formatTime, formatVND } from "@/lib/format";
import { useOnline } from "@/lib/pos/connectivity";
import { itemDetail, orderTitle, paymentMethodsLabel } from "@/lib/pos/money";
import { useIdempotencyKey, useRefundOrder } from "@/lib/pos/mutations";
import { useOrders } from "@/lib/pos/queries";
import { errorCode, errorMessage, isManagerRole } from "@/lib/pos/util";
import { Dialog } from "./Dialog";
import { EmailInvoiceForm, INVOICE_STATUS_NOTE, InvoiceCard } from "./DoneScreen";
import { useManagers } from "./OrderScreen";
import { PinPad } from "./PinPad";
import { PosLoading, usePosSession } from "./PosFrame";

type Order = Schemas["Order"];
type Status = Schemas["OrderStatus"];

const FILTERS: [Status | "all", string][] = [
  ["all", "Tất cả"],
  ["open", "Đang mở"],
  ["paid", "Đã thanh toán"],
  ["cancelled", "Đã huỷ"],
  ["refunded", "Đã hoàn tiền"],
];

export const ORDER_STATUS: Record<Status, { label: string; tone: "ok" | "warn" | "bad" | "info" | "navy" }> = {
  open: { label: "Đang mở", tone: "warn" },
  paid: { label: "Đã thanh toán", tone: "ok" },
  cancelled: { label: "Đã huỷ", tone: "info" },
  merged: { label: "Đã gộp", tone: "info" },
  refunded: { label: "Đã hoàn tiền", tone: "bad" },
};

const INVOICE_LABEL: Record<Schemas["InvoiceStatus"], string> = {
  not_sent: "Chưa gửi cơ quan thuế",
  sending: "Đang gửi",
  sent: "Đã gửi cơ quan thuế",
  failed: "Gửi lỗi, đang thử lại",
};

const REFUND_REASONS = ["Khách trả món", "Thu nhầm tiền", "Món lỗi", "Khác"];

function RefundDialog({ order, onClose }: { order: Order; onClose: () => void }) {
  const { me } = usePosSession();
  const managers = useManagers();
  const refund = useRefundOrder(order.id);
  const idem = useIdempotencyKey();
  const [reason, setReason] = useState(REFUND_REASONS[0]);
  const [pickedManager, setManagerId] = useState<number | null>(null);
  const managerId = pickedManager ?? managers[0]?.id ?? null;
  const [error, setError] = useState("");
  const selfApprove = isManagerRole(me.user.role);

  const send = (extra: { managerId?: number; managerPin?: string }) => {
    setError("");
    const body = { reason, ...extra };
    return refund.mutateAsync({ ...body, key: idem.get(JSON.stringify(body)) }).then(onClose, (e) => {
      setError(errorCode(e) === "INVALID_MANAGER_PIN" ? "PIN quản lý không đúng" : errorMessage(e));
      throw e;
    });
  };

  return (
    <Dialog title={`Hoàn tiền ${order.code}`} onClose={onClose} width={selfApprove ? 480 : 820}>
      <div className="flex gap-6">
        <div className="flex flex-1 flex-col gap-4">
          <p>
            Hoàn <strong>{formatVND(order.paidAmount)}</strong> cho khách. Hệ thống tự lập hoá đơn huỷ và gửi cơ quan thuế.
          </p>
          <label className="flex flex-col gap-1.5 font-semibold">
            Lý do hoàn tiền
            <select value={reason} onChange={(e) => setReason(e.target.value)} className="h-12 rounded-xl border border-line-strong bg-white px-3 font-normal">
              {REFUND_REASONS.map((r) => (
                <option key={r}>{r}</option>
              ))}
            </select>
          </label>
          {selfApprove && (
            <>
              {error && (
                <p role="alert" className="font-semibold text-bad">
                  {error}
                </p>
              )}
              <button
                type="button"
                disabled={refund.isPending}
                onClick={() => void send({}).catch(() => {})}
                className="h-14 rounded-xl border border-bad bg-white font-bold text-bad"
              >
                Xác nhận hoàn tiền
              </button>
            </>
          )}
        </div>
        {!selfApprove && (
          <section aria-label="Cần quản lý duyệt" className="flex w-[360px] flex-col gap-3 rounded-2xl border border-gold-line bg-gold-soft p-5">
            <h3 className="text-lg font-bold">Cần quản lý duyệt</h3>
            <div className="flex flex-wrap gap-2">
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
              busy={refund.isPending || managerId === null}
              onChange={() => setError("")}
              onSubmit={(pin) => send({ managerId: managerId!, managerPin: pin })}
            />
          </section>
        )}
      </div>
    </Dialog>
  );
}

function OrderDetail({ order }: { order: Order }) {
  const online = useOnline();
  const [dialog, setDialog] = useState<null | "qr" | "email" | "refund">(null);
  const st = ORDER_STATUS[order.status];
  return (
    <div className="flex flex-col gap-4">
      <div className="flex items-start justify-between">
        <div>
          <h2 className="text-2xl font-bold">
            {orderTitle(order)} · {order.code}
          </h2>
          <p className="text-muted">
            Tạo lúc {formatTime(order.createdAt)} bởi {order.createdBy.name}
            {order.paidAt ? ` · thanh toán lúc ${formatTime(order.paidAt)}` : ""}
          </p>
        </div>
        <Chip tone={st.tone}>{st.label}</Chip>
      </div>
      <ul className="flex flex-col divide-y divide-line rounded-xl border border-line bg-white">
        {order.items.map((i) => (
          <li key={i.id} className="flex justify-between px-4 py-2">
            <span>
              {i.qty} × {i.name}
              <span className="block text-xs text-muted">{itemDetail(i)}</span>
            </span>
            <span className="font-semibold">{formatVND(i.lineTotal)}</span>
          </li>
        ))}
        {order.discount && (
          <li className="flex justify-between px-4 py-2 text-muted">
            <span>
              Giảm giá ({order.discount.reason}
              {order.discount.approvedBy ? `, duyệt: ${order.discount.approvedBy.name}` : ""})
            </span>
            <span>{formatVND(-order.discount.amount)}</span>
          </li>
        )}
        <li className="flex justify-between px-4 py-2 text-lg font-bold">
          <span>Tổng cộng</span>
          <span>{formatVND(order.total)}</span>
        </li>
      </ul>
      {order.payments.length > 0 && (
        <div>
          <h3 className="mb-2 font-bold">Thanh toán · {paymentMethodsLabel(order.payments)}</h3>
          <ul className="flex flex-col gap-1 text-sm">
            {order.payments.map((p) => (
              <li key={p.id} className="flex justify-between">
                <span>
                  {p.method === "cash" ? "Tiền mặt" : "Chuyển khoản QR"} ·{" "}
                  {p.status === "confirmed" ? "đã nhận" : p.status === "pending" ? "đang chờ" : "đã huỷ"}
                  {p.cashReceived ? ` · khách đưa ${formatVND(p.cashReceived)}, thối ${formatVND(p.changeAmount ?? 0)}` : ""}
                </span>
                <span className="font-semibold">{formatVND(p.amount)}</span>
              </li>
            ))}
          </ul>
        </div>
      )}
      {order.invoice && (
        <p className="text-sm">
          Hoá đơn {order.invoice.symbol} số {order.invoice.number} · <strong>{INVOICE_LABEL[order.invoice.status]}</strong>
          <span className="block text-xs text-muted">{INVOICE_STATUS_NOTE[order.invoice.status]}</span>
        </p>
      )}
      <div className="flex flex-wrap gap-2">
        {order.status === "open" && (
          <Link href={`/pos/don/${order.id}`} className="inline-flex h-14 items-center rounded-xl bg-gold px-5 font-bold text-navy">
            Mở đơn
          </Link>
        )}
        {order.invoice && (
          <>
            <button type="button" onClick={() => setDialog("qr")} className="h-14 rounded-xl border border-line-strong bg-white px-5 font-semibold">
              Hiện QR hoá đơn
            </button>
            <button type="button" onClick={() => setDialog("email")} className="h-14 rounded-xl border border-line-strong bg-white px-5 font-semibold">
              Gửi email
            </button>
          </>
        )}
        {order.status === "paid" && (
          <button
            type="button"
            disabled={!online}
            onClick={() => setDialog("refund")}
            className="h-14 rounded-xl border border-bad/40 bg-white px-5 font-semibold text-bad disabled:text-muted"
          >
            Hoàn tiền (cần quản lý)
          </button>
        )}
      </div>
      {dialog === "qr" && order.invoice && (
        <Dialog title="Quét để nhận hoá đơn điện tử" onClose={() => setDialog(null)} width={480}>
          <InvoiceCard invoice={order.invoice} buyer={order.buyer} />
        </Dialog>
      )}
      {dialog === "email" && order.invoice && (
        <Dialog title="Gửi hoá đơn qua email" onClose={() => setDialog(null)} width={560}>
          <EmailInvoiceForm invoiceId={order.invoice.id} initialEmail={order.buyer.email} />
        </Dialog>
      )}
      {dialog === "refund" && <RefundDialog order={order} onClose={() => setDialog(null)} />}
    </div>
  );
}

/** Screen 11 – orders of the current shift: search, filter, detail, invoice QR / email, refund. */
export function ShiftOrdersScreen() {
  const { shift } = usePosSession();
  const [filter, setFilter] = useState<Status | "all">("all");
  const [q, setQ] = useState("");
  const query = useDeferredValue(q.trim());
  const orders = useOrders({ shiftId: shift?.id, status: filter === "all" ? undefined : filter, q: query }, !!shift);
  const [selectedId, setSelectedId] = useState<number | null>(null);
  const items = orders.data?.items ?? [];
  const selected = items.find((o) => o.id === selectedId) ?? items[0];

  return (
    <div className="flex min-w-0 flex-1">
      <section className="flex w-[460px] shrink-0 flex-col gap-3 border-r border-line bg-white p-4">
        <label className="sr-only" htmlFor="order-search">
          Tìm đơn
        </label>
        <input
          id="order-search"
          type="search"
          value={q}
          onChange={(e) => setQ(e.target.value)}
          placeholder="Tìm mã đơn, tên bàn, số tiền…"
          className="h-12 rounded-xl border border-line-strong px-4"
        />
        <div className="flex flex-wrap gap-2" aria-label="Lọc theo trạng thái">
          {FILTERS.map(([f, label]) => (
            <button
              key={f}
              type="button"
              aria-pressed={filter === f}
              onClick={() => setFilter(f)}
              className={`h-11 rounded-full px-4 text-sm font-semibold ${filter === f ? "bg-navy text-white" : "border border-line-strong bg-white"}`}
            >
              {label}
            </button>
          ))}
        </div>
        {orders.isPending ? (
          <PosLoading />
        ) : (
          <ul className="flex min-h-0 flex-1 flex-col gap-1 overflow-y-auto" aria-label="Đơn trong ca">
            {items.length === 0 && <li className="p-4 text-center text-muted">Không có đơn nào.</li>}
            {items.map((o) => (
              <li key={o.id}>
                <button
                  type="button"
                  aria-pressed={o.id === selected?.id}
                  onClick={() => setSelectedId(o.id)}
                  className={`flex min-h-16 w-full items-center justify-between gap-3 rounded-xl px-3 py-2 text-left ${
                    o.id === selected?.id ? "bg-subtle ring-2 ring-navy" : "hover:bg-subtle"
                  }`}
                >
                  <span>
                    <span className="block font-semibold">
                      {orderTitle(o)} · {o.code.split("-").pop()}
                    </span>
                    <span className="block text-xs text-muted">
                      {formatTime(o.createdAt)} · {ORDER_STATUS[o.status].label}
                    </span>
                  </span>
                  <span className="font-bold">{formatVND(o.total)}</span>
                </button>
              </li>
            ))}
          </ul>
        )}
        {orders.isError && (
          <p role="alert" className="text-bad">
            {errorMessage(orders.error)}
          </p>
        )}
      </section>
      <section className="min-w-0 flex-1 overflow-y-auto p-6" aria-label="Chi tiết đơn">
        {selected ? <OrderDetail key={selected.id} order={selected} /> : <p className="text-muted">Chọn một đơn để xem chi tiết.</p>}
      </section>
    </div>
  );
}
