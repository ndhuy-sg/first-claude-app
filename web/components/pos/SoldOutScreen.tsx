"use client";
import { formatVND } from "@/lib/format";
import { useOnline } from "@/lib/pos/connectivity";
import { useSetSoldOut } from "@/lib/pos/mutations";
import { useMenu } from "@/lib/pos/queries";
import { errorMessage } from "@/lib/pos/util";
import { PosLoading, usePosSession } from "./PosFrame";

/** Roles allowed to toggle sold-out (roles matrix: owner, branch manager, barista). */
const CAN_TOGGLE = new Set(["owner", "branch_manager", "barista"]);

/** Screen 12 – sold-out switches for this branch (B-04). Shared with other tablets via SSE product.sold_out. */
export function SoldOutScreen() {
  const { me } = usePosSession();
  const menu = useMenu();
  const online = useOnline();
  const branchId = me.device?.branchId ?? me.user.branchId ?? 0;
  const set = useSetSoldOut(branchId);
  const canToggle = CAN_TOGGLE.has(me.user.role);

  if (!menu.data) return <PosLoading />;
  const pendingId = set.isPending ? set.variables?.productId : undefined;

  return (
    <div className="flex min-w-0 flex-1 flex-col gap-4 overflow-y-auto p-6">
      <div>
        <h1 className="text-2xl font-bold">Báo hết món</h1>
        <p className="text-muted">Chỉ áp dụng cho chi nhánh {me.device?.branchCode}. Món “Hết” sẽ bị làm mờ trên mọi máy bán hàng của chi nhánh.</p>
        {!canToggle && (
          <p className="mt-2 rounded-xl bg-gold-soft p-3 font-semibold text-[#5c4400]">Chỉ quản lý chi nhánh hoặc pha chế được đổi trạng thái món.</p>
        )}
      </div>
      {set.error && (
        <p role="alert" className="font-semibold text-bad">
          {errorMessage(set.error)}
        </p>
      )}
      <div className="grid grid-cols-2 gap-5">
        {menu.data.categories.map((c) => (
          <section key={c.id} aria-label={c.name} className="rounded-2xl border border-line bg-white">
            <h2 className="border-b border-line px-5 py-3 text-lg font-bold">{c.name}</h2>
            <ul className="divide-y divide-line">
              {c.products.map((p) => {
                const selling = !p.soldOut;
                return (
                  <li key={p.id} className="flex items-center justify-between gap-3 px-5 py-2.5">
                    <span>
                      <span className={`block font-semibold ${selling ? "" : "text-muted line-through"}`}>{p.name}</span>
                      <span className="text-sm text-muted">{formatVND(p.basePrice)}</span>
                    </span>
                    <button
                      type="button"
                      role="switch"
                      aria-checked={selling}
                      aria-label={`${p.name}: ${selling ? "Đang bán" : "Hết"}`}
                      disabled={!canToggle || !online || pendingId === p.id}
                      onClick={() => set.mutate({ productId: p.id, soldOut: selling })}
                      className={`flex h-12 w-36 items-center justify-between rounded-full px-1.5 font-semibold transition disabled:opacity-60 ${
                        selling ? "bg-ok-soft text-ok" : "bg-bad-soft text-bad"
                      }`}
                    >
                      {selling ? (
                        <>
                          <span className="pl-3">Đang bán</span>
                          <span aria-hidden className="h-9 w-9 rounded-full bg-ok" />
                        </>
                      ) : (
                        <>
                          <span aria-hidden className="h-9 w-9 rounded-full bg-bad" />
                          <span className="pr-5">Hết</span>
                        </>
                      )}
                    </button>
                  </li>
                );
              })}
            </ul>
          </section>
        ))}
      </div>
    </div>
  );
}
