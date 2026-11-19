"use client";

import { useQuery } from "@tanstack/react-query";
import Link from "next/link";
import { useSearchParams } from "next/navigation";
import { Fragment, useState } from "react";
import { Chip } from "@/components/ui/Chip";
import { Select } from "@/components/ui/Select";
import { api, unwrap, type Schemas } from "@/lib/api/client";
import { errorMessage, retryServerErrors, useBranchCodes, useScope } from "@/lib/admin/hooks";
import { invoiceStatus, orderStatus, paymentMethodLabel } from "@/lib/admin/labels";
import { longDate } from "@/lib/admin/reports";
import { scopeToBranchId } from "@/lib/admin/scope";
import { formatTime, formatVND, todayVN } from "@/lib/format";
import { ScopeSelect } from "./ScopeSelect";
import { Cell, DateInput, ErrorBox, Filters, GridRow, GridTable, Loading, PageHeader, Pager } from "./parts";

const PAGE_SIZE = 50;
const statusOptions = [{ value: "", label: "Tất cả" }, ...Object.entries(orderStatus).map(([value, s]) => ({ value, label: s.label }))];

function payMethods(o: Schemas["Order"]): string {
  const methods = [...new Set(o.payments.filter((p) => p.status === "confirmed").map((p) => p.method))];
  return methods.length ? methods.map(paymentMethodLabel).join(" + ") : "–";
}

/** Screen 23: Đơn hàng. */
export function OrdersPage() {
  const params = useSearchParams();
  // ?shiftId=12 comes from the dashboard alert "shift_cash_diff" (/admin/ca/{id})
  const [shiftId, setShiftId] = useState<number | undefined>(() => {
    const n = Number(params?.get("shiftId"));
    return Number.isInteger(n) && n > 0 ? n : undefined;
  });
  const [scope, setScope] = useScope();
  const [date, setDate] = useState(() => todayVN());
  const [status, setStatus] = useState<Schemas["OrderStatus"] | "">("");
  const [search, setSearch] = useState("");
  const [page, setPage] = useState(1);
  const [openId, setOpenId] = useState<number | null>(null);
  const code = useBranchCodes();
  const branchId = scopeToBranchId(scope);

  // a shift filter replaces the date filter (a shift can span midnight)
  const query = shiftId
    ? { branchId, shiftId, status: status || undefined, q: search.trim() || undefined, page, pageSize: PAGE_SIZE }
    : { branchId, date, status: status || undefined, q: search.trim() || undefined, page, pageSize: PAGE_SIZE };
  const q = useQuery({
    queryKey: ["orders", query],
    queryFn: async () => unwrap(await api.GET("/orders", { params: { query } })),
    retry: retryServerErrors,
  });

  const reset = <T,>(set: (v: T) => void) => (v: T) => {
    set(v);
    setPage(1);
  };

  return (
    <>
      <PageHeader title="Đơn hàng" subtitle={shiftId ? `Đơn của ca #${shiftId}` : `Ngày ${longDate(date)}`} />
      <Filters>
        <ScopeSelect value={scope} onChange={reset(setScope)} />
        {shiftId ? (
          <div className="flex h-11 items-center gap-2 rounded-[10px] border border-line-strong bg-subtle px-3 text-sm font-semibold">
            Ca #{shiftId}
            <button type="button" onClick={() => reset(setShiftId)(undefined)} className="text-navy underline" aria-label="Bỏ lọc theo ca">
              Bỏ lọc
            </button>
          </div>
        ) : (
          <DateInput id="orders-date" label="Ngày" value={date} onChange={reset(setDate)} />
        )}
        <Select id="orders-status" label="Trạng thái" value={status} options={statusOptions} onChange={(e) => reset(setStatus)(e.target.value as Schemas["OrderStatus"] | "")} />
        <label htmlFor="orders-q" className="flex min-w-48 flex-1 flex-col gap-1.5 text-sm font-semibold text-muted">
          Tìm
          <input
            id="orders-q"
            type="search"
            placeholder="Mã đơn, bàn, số tiền"
            value={search}
            onChange={(e) => reset(setSearch)(e.target.value)}
            className="h-11 rounded-[10px] border border-line-strong bg-white px-3 text-[15px] font-medium text-ink"
          />
        </label>
      </Filters>
      {q.isPending && <Loading />}
      {q.isError && <ErrorBox message={errorMessage(q.error)} onRetry={() => q.refetch()} />}
      {q.data && (
        <>
          <GridTable
            label="Danh sách đơn hàng"
            cols="190px 60px 60px 90px 60px 120px 150px 130px 1fr"
            head={["Mã đơn", "CN", "Giờ", "Bàn", "Món", "Tổng", "Thanh toán", "Trạng thái", "Hoá đơn"]}
            minWidth={1000}
            empty={q.data.items.length === 0}
          >
            {q.data.items.map((o) => (
              <Fragment key={o.id}>
                <GridRow onClick={() => setOpenId(openId === o.id ? null : o.id)}>
                  <Cell>
                    <button type="button" aria-expanded={openId === o.id} className="font-semibold text-navy underline-offset-2 hover:underline">
                      {o.code}
                    </button>
                  </Cell>
                  <Cell>{code(o.branchId)}</Cell>
                  <Cell className="tabular-nums">{formatTime(o.paidAt ?? o.createdAt)}</Cell>
                  <Cell>{o.tableName ?? "Mang đi"}</Cell>
                  <Cell className="tabular-nums">{o.items.reduce((s, i) => s + i.qty, 0)}</Cell>
                  <Cell className="font-semibold tabular-nums">{formatVND(o.total)}</Cell>
                  <Cell>{payMethods(o)}</Cell>
                  <Cell>
                    <Chip tone={orderStatus[o.status].tone}>{orderStatus[o.status].label}</Chip>
                  </Cell>
                  <Cell>
                    {o.invoice ? (
                      <span className="flex flex-wrap items-center gap-1.5">
                        <Link href={`/tra-cuu/${encodeURIComponent(o.invoice.lookupCode)}`} className="text-navy underline" onClick={(e) => e.stopPropagation()}>
                          {o.invoice.number}
                        </Link>
                        <Chip tone={invoiceStatus[o.invoice.status].tone}>{invoiceStatus[o.invoice.status].label}</Chip>
                      </span>
                    ) : (
                      "–"
                    )}
                  </Cell>
                </GridRow>
                {openId === o.id && <OrderDetail order={o} />}
              </Fragment>
            ))}
          </GridTable>
          <Pager page={page} pageSize={PAGE_SIZE} total={q.data.total} onPage={setPage} />
        </>
      )}
    </>
  );
}

function OrderDetail({ order }: { order: Schemas["Order"] }) {
  return (
    <div role="row" className="border-t border-line bg-surface px-4 py-3 text-sm">
      <div role="cell" className="grid gap-4 md:grid-cols-[2fr_1fr]">
        <ul className="flex flex-col gap-1">
          {order.items.map((i) => (
            <li key={i.id} className="flex justify-between gap-3">
              <span>
                {i.qty} × {i.name}
                {i.sizeName && ` (${i.sizeName})`}
                {i.modifiers.length > 0 && <span className="text-muted"> · {i.modifiers.map((m) => m.name).join(", ")}</span>}
                {i.note && <span className="text-muted"> · {i.note}</span>}
              </span>
              <span className="tabular-nums">{formatVND(i.lineTotal)}</span>
            </li>
          ))}
        </ul>
        <dl className="grid grid-cols-[auto_1fr] gap-x-3 gap-y-1">
          <dt className="text-muted">Tạm tính</dt>
          <dd className="text-right tabular-nums">{formatVND(order.subtotal)}</dd>
          {order.discount && (
            <>
              <dt className="text-muted">Giảm giá</dt>
              <dd className="text-right tabular-nums">
                −{formatVND(order.discount.amount)}
                <span className="block text-[12px] text-muted">
                  {order.discount.reason}
                  {order.discount.approvedBy && ` · duyệt: ${order.discount.approvedBy.name}`}
                </span>
              </dd>
            </>
          )}
          <dt className="font-semibold">Tổng</dt>
          <dd className="text-right font-bold tabular-nums">{formatVND(order.total)}</dd>
          <dt className="text-muted">Thu ngân</dt>
          <dd className="text-right">{order.createdBy.name}</dd>
          <dt className="text-muted">Người mua</dt>
          <dd className="text-right">{order.buyer.name || "Khách lẻ"}</dd>
        </dl>
      </div>
    </div>
  );
}
