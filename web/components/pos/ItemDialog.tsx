"use client";
import { useState } from "react";
import type { Schemas } from "@/lib/api/client";
import { formatVND } from "@/lib/format";
import { unitPrice } from "@/lib/pos/money";
import { Dialog } from "./Dialog";

type Product = Schemas["MenuProduct"];

/** Item dialog of screen 3: size, modifiers, note, quantity (W-04). Price shown is a preview; the server computes it. */
export function ItemDialog({
  product,
  onAdd,
  onClose,
  busy,
  error,
}: {
  product: Product;
  onAdd: (req: Schemas["AddItemRequest"]) => void;
  onClose: () => void;
  busy?: boolean;
  error?: string;
}) {
  const [sizeCode, setSizeCode] = useState<string | null>(product.sizes[0]?.code ?? null);
  const [mods, setMods] = useState<number[]>([]);
  const [note, setNote] = useState("");
  const [qty, setQty] = useState(1);
  const unit = unitPrice(product, sizeCode, mods);
  const total = unit * qty;

  const toggle = (id: number) => setMods((m) => (m.includes(id) ? m.filter((x) => x !== id) : [...m, id]));

  return (
    <Dialog
      title={product.name}
      onClose={onClose}
      footer={
        <div className="flex flex-col gap-2">
          {error && (
            <p role="alert" className="font-semibold text-bad">
              {error}
            </p>
          )}
          <button
            type="button"
            disabled={busy}
            onClick={() =>
              onAdd({
                productId: product.id,
                sizeCode: sizeCode ?? undefined,
                modifierIds: mods,
                qty,
                note: note.trim() || undefined,
              })
            }
            className="h-16 w-full rounded-[14px] bg-gold text-lg font-bold text-navy disabled:bg-subtle disabled:text-muted"
          >
            Thêm vào đơn · {formatVND(total)}
          </button>
        </div>
      }
    >
      <div className="flex flex-col gap-5">
        <p className="text-muted">
          Giá gốc <span className="font-semibold text-ink">{formatVND(product.basePrice)}</span>
        </p>
        {product.sizes.length > 0 && (
          <fieldset>
            <legend className="mb-2 font-semibold">Size</legend>
            <div role="radiogroup" aria-label="Size" className="inline-flex rounded-xl border border-line-strong p-1">
              {product.sizes.map((s) => (
                <button
                  key={s.code}
                  type="button"
                  role="radio"
                  aria-checked={s.code === sizeCode}
                  onClick={() => setSizeCode(s.code)}
                  className={`h-12 min-w-28 rounded-lg px-4 font-semibold ${s.code === sizeCode ? "bg-navy text-white" : "text-ink"}`}
                >
                  {s.name}
                  {s.priceDelta > 0 ? ` +${formatVND(s.priceDelta)}` : ""}
                </button>
              ))}
            </div>
          </fieldset>
        )}
        {product.modifiers.length > 0 && (
          <fieldset>
            <legend className="mb-2 font-semibold">Tuỳ chọn</legend>
            <div className="flex flex-wrap gap-2">
              {product.modifiers.map((m) => {
                const on = mods.includes(m.id);
                return (
                  <button
                    key={m.id}
                    type="button"
                    aria-pressed={on}
                    onClick={() => toggle(m.id)}
                    className={`h-12 rounded-full border px-4 font-semibold ${on ? "border-navy bg-navy text-white" : "border-line-strong bg-white text-ink"}`}
                  >
                    {on ? "✓ " : ""}
                    {m.name}
                    {m.price > 0 ? ` +${formatVND(m.price)}` : ""}
                  </button>
                );
              })}
            </div>
          </fieldset>
        )}
        <label className="flex flex-col gap-1.5 font-semibold">
          Ghi chú
          <input
            value={note}
            onChange={(e) => setNote(e.target.value)}
            placeholder="VD: ít ngọt, mang ra sau"
            className="h-12 rounded-xl border border-line-strong px-3.5 font-normal"
          />
        </label>
        <div className="flex items-center gap-4">
          <span className="font-semibold">Số lượng</span>
          <button type="button" aria-label="Giảm số lượng" onClick={() => setQty((q) => Math.max(1, q - 1))} className="h-12 w-12 rounded-xl border border-line-strong text-2xl">
            −
          </button>
          <span className="w-10 text-center text-2xl font-bold" aria-live="polite" data-testid="dialog-qty">
            {qty}
          </span>
          <button type="button" aria-label="Tăng số lượng" onClick={() => setQty((q) => q + 1)} className="h-12 w-12 rounded-xl border border-line-strong text-2xl">
            +
          </button>
        </div>
      </div>
    </Dialog>
  );
}
