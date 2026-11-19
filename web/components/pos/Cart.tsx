"use client";
import type { Schemas } from "@/lib/api/client";
import { formatVND } from "@/lib/format";
import { itemCount, itemDetail, orderTitle } from "@/lib/pos/money";

type Order = Schemas["Order"];
type Item = Schemas["OrderItem"];

/** Cart panel of screen 3 (380px). */
export function Cart({
  order,
  online,
  busy,
  onQty,
  onRemove,
  onDiscount,
  onTableOps,
  onMoveTable,
  onCancel,
  onPay,
}: {
  order: Order;
  online: boolean;
  busy?: boolean;
  onQty: (item: Item, qty: number) => void;
  onRemove: (item: Item) => void;
  onDiscount: () => void;
  onTableOps: () => void;
  onMoveTable: () => void;
  onCancel: () => void;
  onPay: () => void;
}) {
  const count = itemCount(order);
  const empty = order.items.length === 0;
  return (
    <aside className="flex w-[380px] shrink-0 flex-col border-l border-line bg-white" aria-label="Giỏ hàng">
      <div className="flex items-start justify-between gap-3 border-b border-line p-4">
        <div className="min-w-0">
          <h2 className="text-2xl font-bold">{orderTitle(order)}</h2>
          <p className="truncate text-sm text-muted">
            {order.type === "dine_in" ? "Tại bàn" : "Mang đi"} · {order.code} · {count} món
          </p>
        </div>
        <div className="flex shrink-0 flex-col gap-1.5">
          {order.type === "dine_in" && (
            <button type="button" onClick={onMoveTable} disabled={!online} className="h-11 rounded-[10px] border border-line-strong px-3 text-sm font-semibold disabled:text-muted">
              Đổi bàn
            </button>
          )}
          {empty && (
            <button type="button" onClick={onCancel} disabled={!online} className="h-11 rounded-[10px] px-3 text-sm font-semibold text-bad hover:bg-bad-soft">
              Huỷ đơn
            </button>
          )}
        </div>
      </div>

      <ul className="flex min-h-0 flex-1 flex-col overflow-y-auto" aria-label="Món trong đơn">
        {empty && <li className="p-6 text-center text-muted">Chưa có món. Chạm vào món bên trái để thêm.</li>}
        {order.items.map((it) => (
          <li key={it.id} className="border-b border-line px-4 py-3" data-testid="cart-line">
            <div className="flex justify-between gap-3">
              <div className="min-w-0">
                <p className="font-semibold">{it.name}</p>
                {itemDetail(it) && <p className="text-sm text-muted">{itemDetail(it)}</p>}
                {it.note && <p className="text-sm italic text-muted">“{it.note}”</p>}
              </div>
              <div className="text-right">
                <p className="font-bold" data-testid="line-total">
                  {formatVND(it.lineTotal)}
                </p>
                <p className="text-xs text-muted">× {formatVND(it.unitPrice)}</p>
              </div>
            </div>
            <div className="mt-2 flex items-center gap-2">
              <button
                type="button"
                aria-label={it.qty === 1 ? `Xoá ${it.name}` : `Giảm ${it.name}`}
                onClick={() => (it.qty === 1 ? onRemove(it) : onQty(it, it.qty - 1))}
                disabled={busy || !online}
                className="h-10 w-11 rounded-lg border border-line-strong text-xl disabled:text-muted"
              >
                −
              </button>
              <span className="w-8 text-center font-bold">{it.qty}</span>
              <button
                type="button"
                aria-label={`Tăng ${it.name}`}
                onClick={() => onQty(it, it.qty + 1)}
                disabled={busy || !online}
                className="h-10 w-11 rounded-lg border border-line-strong text-xl disabled:text-muted"
              >
                +
              </button>
            </div>
          </li>
        ))}
      </ul>

      <div className="flex flex-col gap-2 border-t border-line p-4">
        <dl className="flex flex-col gap-1">
          <div className="flex justify-between text-muted">
            <dt>Tạm tính</dt>
            <dd>{formatVND(order.subtotal)}</dd>
          </div>
          <div className="flex justify-between text-muted">
            <dt>Giảm giá{order.discount?.type === "percent" ? ` (${order.discount.value}%)` : ""}</dt>
            <dd data-testid="cart-discount">{order.discount ? formatVND(-order.discount.amount) : formatVND(0)}</dd>
          </div>
          <div className="flex items-baseline justify-between">
            <dt className="font-bold">Tổng cộng</dt>
            <dd className="text-[28px] font-bold" data-testid="cart-total">
              {formatVND(order.total)}
            </dd>
          </div>
        </dl>
        <div className="grid grid-cols-2 gap-2">
          <button type="button" onClick={onDiscount} disabled={empty || !online} className="h-14 rounded-xl border border-line-strong font-semibold disabled:text-muted">
            Giảm giá
          </button>
          <button type="button" onClick={onTableOps} disabled={empty || !online} className="h-14 rounded-xl border border-line-strong font-semibold disabled:text-muted">
            Tách / Gộp
          </button>
        </div>
        <button
          type="button"
          onClick={onPay}
          disabled={empty || !online}
          className="h-[60px] rounded-[14px] bg-gold text-xl font-bold text-navy disabled:cursor-not-allowed disabled:bg-subtle disabled:text-muted"
        >
          {online ? `Thanh toán ${formatVND(order.total)}` : "Chờ kết nối để thanh toán"}
        </button>
      </div>
    </aside>
  );
}
