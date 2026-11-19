"use client";
import { useRouter } from "next/navigation";
import { useEffect, useState } from "react";
import type { Schemas } from "@/lib/api/client";
import { useOnline } from "@/lib/pos/connectivity";
import { useAddItem, useCancelOrder, useDeleteItem, useUpdateItem } from "@/lib/pos/mutations";
import { useDevice, useMenu, useOrder } from "@/lib/pos/queries";
import { errorMessage } from "@/lib/pos/util";
import { Cart } from "./Cart";
import { Dialog } from "./Dialog";
import { DiscountDialog } from "./DiscountDialog";
import { ItemDialog } from "./ItemDialog";
import { PosLoading, usePosSession } from "./PosFrame";
import { ProductGrid } from "./ProductGrid";
import { TableOpsDialog, type TableOpsTab } from "./TableOpsDialog";

const CANCEL_REASONS = ["Khách đổi ý", "Tạo nhầm đơn", "Khác"];

/** Branch managers of the device, who may approve discounts and refunds with their PIN. */
export function useManagers() {
  const device = useDevice();
  return (device.data?.staff ?? []).filter((s) => s.role === "branch_manager");
}

/** Screen 3 – ordering: categories, product grid, cart (W-03, W-04, W-05). */
export function OrderScreen({ orderId }: { orderId: number }) {
  const router = useRouter();
  const { me } = usePosSession();
  const order = useOrder(orderId);
  const menu = useMenu();
  const online = useOnline();
  const managers = useManagers();
  const add = useAddItem(orderId);
  const update = useUpdateItem(orderId);
  const del = useDeleteItem(orderId);
  const cancel = useCancelOrder(orderId);
  const [picked, setPicked] = useState<Schemas["MenuProduct"] | null>(null);
  const [dialog, setDialog] = useState<null | "discount" | "cancel" | TableOpsTab>(null);
  const [cancelReason, setCancelReason] = useState(CANCEL_REASONS[0]);

  const status = order.data?.status;
  useEffect(() => {
    if (status === "paid") router.replace(`/pos/hoan-tat/${orderId}`);
    else if (status && status !== "open") router.replace("/pos/ban");
  }, [status, orderId, router]);

  if (order.isError) {
    return (
      <p role="alert" className="m-auto text-lg text-muted">
        {errorMessage(order.error)}
      </p>
    );
  }
  if (!order.data || !menu.data) return <PosLoading />;
  const o = order.data;
  const lineError = update.error ?? del.error;

  const pick = (p: Schemas["MenuProduct"]) => {
    add.reset();
    // Products without options are added straight away.
    if (p.sizes.length === 0 && p.modifiers.length === 0) add.mutate({ productId: p.id, qty: 1 });
    else setPicked(p);
  };

  return (
    <div className="flex min-w-0 flex-1">
      <div className="flex min-w-0 flex-1 flex-col">
        {(lineError || (add.error && !picked)) && (
          <p role="alert" className="border-b border-line bg-bad-soft px-4 py-2 font-semibold text-[#7a2a1e]">
            {errorMessage(lineError ?? add.error)}
          </p>
        )}
        <ProductGrid menu={menu.data} onPick={pick} disabled={!online} />
      </div>
      <Cart
        order={o}
        online={online}
        busy={update.isPending || del.isPending}
        onQty={(it, qty) => update.mutate({ itemId: it.id, qty })}
        onRemove={(it) => del.mutate(it.id)}
        onDiscount={() => setDialog("discount")}
        onTableOps={() => setDialog(o.type === "dine_in" ? "merge" : "split")}
        onMoveTable={() => setDialog("move")}
        onCancel={() => setDialog("cancel")}
        onPay={() => router.push(`/pos/thanh-toan/${o.id}`)}
      />

      {picked && (
        <ItemDialog
          product={picked}
          busy={add.isPending}
          error={add.error ? errorMessage(add.error) : undefined}
          onClose={() => setPicked(null)}
          onAdd={(req) => add.mutate(req, { onSuccess: () => setPicked(null) })}
        />
      )}
      {dialog === "discount" && (
        <DiscountDialog order={o} maxPct={me.settings.discountMaxPct} managers={managers} onClose={() => setDialog(null)} />
      )}
      {(dialog === "move" || dialog === "merge" || dialog === "split") && (
        <TableOpsDialog order={o} initialTab={dialog} onClose={() => setDialog(null)} />
      )}
      {dialog === "cancel" && (
        <Dialog
          title="Huỷ đơn"
          width={480}
          onClose={() => setDialog(null)}
          footer={
            <button
              type="button"
              disabled={cancel.isPending}
              onClick={() => cancel.mutate(cancelReason, { onSuccess: () => router.replace("/pos/ban") })}
              className="h-14 w-full rounded-xl border border-bad bg-white font-bold text-bad"
            >
              Huỷ đơn {o.code}
            </button>
          }
        >
          <label className="flex flex-col gap-1.5 font-semibold">
            Lý do huỷ
            <select value={cancelReason} onChange={(e) => setCancelReason(e.target.value)} className="h-12 rounded-xl border border-line-strong bg-white px-3 font-normal">
              {CANCEL_REASONS.map((r) => (
                <option key={r}>{r}</option>
              ))}
            </select>
          </label>
          {cancel.error && (
            <p role="alert" className="mt-3 font-semibold text-bad">
              {errorMessage(cancel.error)}
            </p>
          )}
        </Dialog>
      )}
    </div>
  );
}
