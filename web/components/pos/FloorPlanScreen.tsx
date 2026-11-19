"use client";
import Link from "next/link";
import { useRouter } from "next/navigation";
import { useState } from "react";
import type { Schemas } from "@/lib/api/client";
import { formatTime, formatVND } from "@/lib/format";
import { useOnline } from "@/lib/pos/connectivity";
import { minutesSince } from "@/lib/pos/money";
import { useCreateOrder } from "@/lib/pos/mutations";
import { useOrders, useTables } from "@/lib/pos/queries";
import { errorCode, errorMessage, useNow } from "@/lib/pos/util";
import { PosLoading } from "./PosFrame";

type Table = Schemas["Table"];

const STATUS: Record<Table["status"], { label: string; card: string }> = {
  free: { label: "Trống", card: "bg-white text-ink border-line-strong" },
  occupied: { label: "Có khách", card: "bg-navy text-white border-navy" },
  awaiting_payment: { label: "Chờ thanh toán", card: "bg-[#ffe8a3] text-navy border-gold-line" },
};

function TableCard({ table, now, onOpen, disabled }: { table: Table; now: number | null; onOpen: () => void; disabled: boolean }) {
  const s = STATUS[table.status];
  const o = table.openOrder;
  return (
    <button
      type="button"
      onClick={onOpen}
      disabled={disabled}
      aria-label={`${table.name} · ${s.label}${o ? ` · ${o.itemCount} món · ${formatVND(o.total)}` : ""}`}
      className={`flex h-[120px] flex-col justify-between rounded-2xl border-2 p-4 text-left transition active:scale-[0.98] disabled:opacity-60 ${s.card}`}
    >
      <span className="flex items-baseline justify-between">
        <span className="text-xl font-bold">{table.name}</span>
        {o && now && <span className="text-sm opacity-80">{minutesSince(o.createdAt, now)} phút</span>}
      </span>
      <span className="text-sm font-semibold">{o ? `${s.label} · ${o.itemCount} món` : s.label}</span>
      <span className="text-lg font-bold">{o ? formatVND(o.total) : " "}</span>
    </button>
  );
}

/** Screen 2 – floor plan with areas, live table status and takeaway orders. */
export function FloorPlanScreen() {
  const router = useRouter();
  const tables = useTables();
  const takeaways = useOrders({ status: "open" });
  const create = useCreateOrder();
  const online = useOnline();
  const now = useNow();
  const [areaId, setAreaId] = useState<number | null>(null);

  if (!tables.data) return <PosLoading />;
  const areas = tables.data;
  const area = areas.find((a) => a.id === areaId) ?? areas[0];
  const openTakeaways = (takeaways.data?.items ?? []).filter((o) => o.type === "takeaway");

  const openTable = (t: Table) => {
    if (t.openOrder) return router.push(`/pos/don/${t.openOrder.id}`);
    create.mutate(
      { type: "dine_in", tableId: t.id },
      {
        onSuccess: (o) => router.push(`/pos/don/${o.id}`),
        onError: (e) => {
          if (errorCode(e) === "TABLE_OCCUPIED") void tables.refetch();
        },
      },
    );
  };

  return (
    <div className="flex flex-1">
      <section className="flex min-w-0 flex-1 flex-col gap-4 p-5">
        <div className="flex items-center justify-between gap-4">
          <div role="tablist" aria-label="Khu vực" className="flex gap-2">
            {areas.map((a) => (
              <button
                key={a.id}
                type="button"
                role="tab"
                aria-selected={a.id === area?.id}
                onClick={() => setAreaId(a.id)}
                className={`h-12 rounded-xl px-5 font-semibold ${a.id === area?.id ? "bg-navy text-white" : "border border-line-strong bg-white text-ink"}`}
              >
                {a.name}
              </button>
            ))}
          </div>
          <ul className="flex gap-4 text-sm text-muted" aria-label="Chú thích">
            <li className="flex items-center gap-2">
              <span aria-hidden className="h-4 w-4 rounded border-2 border-line-strong bg-white" /> Trống
            </li>
            <li className="flex items-center gap-2">
              <span aria-hidden className="h-4 w-4 rounded bg-navy" /> Có khách
            </li>
            <li className="flex items-center gap-2">
              <span aria-hidden className="h-4 w-4 rounded bg-[#ffe8a3]" /> Chờ thanh toán
            </li>
          </ul>
        </div>
        {create.error && (
          <p role="alert" className="font-semibold text-bad">
            {errorMessage(create.error)}
          </p>
        )}
        <div className="grid grid-cols-4 gap-4 overflow-y-auto pb-4" role="tabpanel" aria-label={area?.name}>
          {area?.tables.map((t) => (
            <TableCard key={t.id} table={t} now={now} onOpen={() => openTable(t)} disabled={create.isPending || (!online && !t.openOrder)} />
          ))}
        </div>
      </section>
      <aside className="flex w-[320px] shrink-0 flex-col gap-4 border-l border-line bg-white p-5">
        <button
          type="button"
          onClick={() => create.mutate({ type: "takeaway" }, { onSuccess: (o) => router.push(`/pos/don/${o.id}`) })}
          disabled={create.isPending || !online}
          className="h-16 rounded-[14px] bg-gold text-lg font-bold text-navy disabled:bg-subtle disabled:text-muted"
        >
          + Đơn mang đi mới
        </button>
        <h2 className="font-bold">Đơn mang đi đang mở</h2>
        <ul className="flex min-h-0 flex-1 flex-col gap-2 overflow-y-auto">
          {openTakeaways.length === 0 && <li className="text-sm text-muted">Chưa có đơn mang đi.</li>}
          {openTakeaways.map((o) => (
            <li key={o.id}>
              <Link href={`/pos/don/${o.id}`} className="flex min-h-14 items-center justify-between rounded-xl border border-line px-3 py-2 hover:bg-subtle">
                <span>
                  <span className="block font-semibold">Mang đi #{o.code.split("-").pop()}</span>
                  <span className="block text-xs text-muted">
                    {formatTime(o.createdAt)} · {o.items.length} món
                  </span>
                </span>
                <span className="font-bold">{formatVND(o.total)}</span>
              </Link>
            </li>
          ))}
        </ul>
        <nav className="flex flex-col gap-2 border-t border-line pt-4" aria-label="Chức năng">
          <Link href="/pos/don-trong-ca" className="flex h-12 items-center rounded-xl px-3 font-semibold hover:bg-subtle">
            Đơn trong ca
          </Link>
          <Link href="/pos/het-mon" className="flex h-12 items-center rounded-xl px-3 font-semibold hover:bg-subtle">
            Báo hết món
          </Link>
          <Link href="/pos/chot-ca" className="flex h-12 items-center rounded-xl px-3 font-semibold text-bad hover:bg-bad-soft">
            Chốt ca
          </Link>
        </nav>
      </aside>
    </div>
  );
}
