"use client";
import { useRouter } from "next/navigation";
import { useState } from "react";
import type { Schemas } from "@/lib/api/client";
import { formatVND } from "@/lib/format";
import { itemDetail, orderTitle } from "@/lib/pos/money";
import { useMergeOrder, useMoveOrder, useSplitOrder } from "@/lib/pos/mutations";
import { useTables } from "@/lib/pos/queries";
import { errorMessage } from "@/lib/pos/util";
import { Dialog } from "./Dialog";

export type TableOpsTab = "move" | "merge" | "split";
const TABS: [TableOpsTab, string][] = [
  ["move", "Chuyển bàn"],
  ["merge", "Gộp bàn"],
  ["split", "Tách hoá đơn"],
];

/** Screen 10 – move to another table, merge into another table's order, or split the bill by tapping items. */
export function TableOpsDialog({ order, initialTab, onClose }: { order: Schemas["Order"]; initialTab: TableOpsTab; onClose: () => void }) {
  const router = useRouter();
  const [tab, setTab] = useState<TableOpsTab>(order.type === "takeaway" && initialTab === "move" ? "split" : initialTab);
  const tables = useTables();
  const move = useMoveOrder(order.id);
  const merge = useMergeOrder(order.id);
  const split = useSplitOrder(order.id);
  const [moved, setMoved] = useState<number[]>([]);
  const err = move.error ?? merge.error ?? split.error;

  const allTables = (tables.data ?? []).flatMap((a) => a.tables.map((t) => ({ ...t, areaName: a.name })));
  const free = allTables.filter((t) => t.status === "free");
  const others = allTables.filter((t) => t.openOrder && t.openOrder.id !== order.id);

  const left = order.items.filter((i) => !moved.includes(i.id));
  const right = order.items.filter((i) => moved.includes(i.id));
  const sum = (items: Schemas["OrderItem"][]) => items.reduce((s, i) => s + i.lineTotal, 0);
  const toggle = (id: number) => setMoved((m) => (m.includes(id) ? m.filter((x) => x !== id) : [...m, id]));

  return (
    <Dialog title={`${orderTitle(order)} · ${order.code}`} onClose={onClose} width={900}>
      <div className="flex flex-col gap-4">
        <div role="tablist" aria-label="Thao tác bàn" className="inline-flex self-start rounded-xl border border-line-strong p-1">
          {TABS.filter(([t]) => order.type === "dine_in" || t !== "move").map(([t, label]) => (
            <button
              key={t}
              type="button"
              role="tab"
              aria-selected={tab === t}
              onClick={() => setTab(t)}
              className={`h-12 rounded-lg px-5 font-semibold ${tab === t ? "bg-navy text-white" : ""}`}
            >
              {label}
            </button>
          ))}
        </div>
        {err && (
          <p role="alert" className="font-semibold text-bad">
            {errorMessage(err)}
          </p>
        )}

        {tab === "move" && (
          <div>
            <p className="mb-3 text-muted">Chọn bàn trống để chuyển cả đơn sang.</p>
            <div className="grid grid-cols-5 gap-3">
              {free.map((t) => (
                <button
                  key={t.id}
                  type="button"
                  disabled={move.isPending}
                  onClick={() => move.mutate(t.id, { onSuccess: onClose })}
                  className="flex h-20 flex-col items-center justify-center rounded-xl border-2 border-line-strong bg-white font-bold hover:border-navy"
                >
                  {t.name}
                  <span className="text-xs font-normal text-muted">{t.areaName}</span>
                </button>
              ))}
              {free.length === 0 && <p className="col-span-5 text-muted">Không còn bàn trống.</p>}
            </div>
          </div>
        )}

        {tab === "merge" && (
          <div>
            <p className="mb-3 text-muted">Chọn bàn để gộp các món của đơn này vào. Đơn này sẽ đóng và bàn được trả lại.</p>
            <div className="grid grid-cols-4 gap-3">
              {others.map((t) => (
                <button
                  key={t.id}
                  type="button"
                  disabled={merge.isPending}
                  onClick={() => merge.mutate(t.openOrder!.id, { onSuccess: (target) => router.replace(`/pos/don/${target.id}`) })}
                  className="flex h-24 flex-col items-start justify-between rounded-xl border-2 border-navy bg-navy p-3 text-left text-white"
                >
                  <span className="font-bold">Gộp vào {t.name}</span>
                  <span className="text-sm">
                    {t.openOrder!.itemCount} món · {formatVND(t.openOrder!.total)}
                  </span>
                </button>
              ))}
              {others.length === 0 && <p className="col-span-4 text-muted">Không có bàn nào khác đang có khách.</p>}
            </div>
          </div>
        )}

        {tab === "split" && (
          <div className="flex flex-col gap-3">
            <p className="text-muted">Chạm vào món để chuyển sang hoá đơn mới, chạm lại để trả về.</p>
            <div className="grid grid-cols-2 gap-4">
              {(
                [
                  ["Hoá đơn hiện tại", left],
                  ["Hoá đơn mới", right],
                ] as const
              ).map(([title, items]) => (
                <section key={title} aria-label={title} className="flex flex-col rounded-xl border border-line">
                  <h3 className="border-b border-line px-4 py-2 font-bold">{title}</h3>
                  <ul className="flex min-h-40 flex-col">
                    {items.map((i) => (
                      <li key={i.id}>
                        <button type="button" onClick={() => toggle(i.id)} className="flex min-h-14 w-full items-center justify-between px-4 py-2 text-left hover:bg-subtle">
                          <span>
                            <span className="block font-semibold">
                              {i.qty} × {i.name}
                            </span>
                            <span className="block text-xs text-muted">{itemDetail(i)}</span>
                          </span>
                          <span className="font-semibold">{formatVND(i.lineTotal)}</span>
                        </button>
                      </li>
                    ))}
                  </ul>
                  <p className="mt-auto flex justify-between border-t border-line px-4 py-2 font-bold">
                    <span>Tạm tính</span> <span>{formatVND(sum(items))}</span>
                  </p>
                </section>
              ))}
            </div>
            <button
              type="button"
              disabled={right.length === 0 || left.length === 0 || split.isPending}
              onClick={() => split.mutate(moved, { onSuccess: onClose })}
              className="h-14 rounded-xl bg-gold text-lg font-bold text-navy disabled:bg-subtle disabled:text-muted"
            >
              Tách hoá đơn
            </button>
          </div>
        )}
      </div>
    </Dialog>
  );
}
