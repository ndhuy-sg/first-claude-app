"use client";
import { useState } from "react";
import type { Schemas } from "@/lib/api/client";
import { formatVND } from "@/lib/format";

type Product = Schemas["MenuProduct"];

function normalize(s: string) {
  return s
    .normalize("NFD")
    .replace(/[̀-ͯ]/g, "")
    .replace(/đ/g, "d")
    .replace(/Đ/g, "D")
    .toLowerCase();
}

/** Category rail (132px) + search + 3-column product grid. Sold-out items are greyed and disabled (W-05). */
export function ProductGrid({ menu, onPick, disabled }: { menu: Schemas["Menu"]; onPick: (p: Product) => void; disabled?: boolean }) {
  const [catId, setCatId] = useState<number | null>(null);
  const [q, setQ] = useState("");
  const cat = menu.categories.find((c) => c.id === catId) ?? menu.categories[0];
  const query = normalize(q.trim());
  const products = query
    ? menu.categories.flatMap((c) => c.products).filter((p) => normalize(p.name).includes(query))
    : (cat?.products ?? []);

  return (
    <div className={`flex min-w-0 flex-1 ${disabled ? "opacity-60" : ""}`}>
      <nav aria-label="Nhóm món" className="flex w-[132px] shrink-0 flex-col gap-2 overflow-y-auto border-r border-line bg-white p-3">
        {menu.categories.map((c) => (
          <button
            key={c.id}
            type="button"
            aria-pressed={!query && c.id === cat?.id}
            onClick={() => {
              setCatId(c.id);
              setQ("");
            }}
            className={`flex h-[72px] items-center justify-center rounded-xl px-2 text-center font-semibold ${
              !query && c.id === cat?.id ? "bg-navy text-white" : "bg-surface text-ink hover:bg-subtle"
            }`}
          >
            {c.name}
          </button>
        ))}
      </nav>
      <div className="flex min-w-0 flex-1 flex-col gap-3 p-4">
        <label className="sr-only" htmlFor="product-search">
          Tìm món
        </label>
        <input
          id="product-search"
          type="search"
          value={q}
          onChange={(e) => setQ(e.target.value)}
          placeholder="Tìm món…"
          className="h-12 rounded-xl border border-line-strong bg-white px-4"
        />
        <div className="grid grid-cols-3 content-start gap-3 overflow-y-auto pb-4" aria-label="Danh sách món">
          {products.map((p) => (
            <button
              key={p.id}
              type="button"
              disabled={p.soldOut || disabled}
              aria-label={`${p.name} · ${formatVND(p.basePrice)}${p.soldOut ? " · hết món" : ""}`}
              onClick={() => onPick(p)}
              className={`relative flex h-[112px] flex-col justify-between rounded-2xl border p-3.5 text-left ${
                p.soldOut ? "cursor-not-allowed border-line bg-subtle text-muted opacity-60" : "border-line bg-white hover:border-navy active:scale-[0.98]"
              }`}
            >
              <span className="font-semibold leading-snug">{p.name}</span>
              <span className="flex items-center justify-between">
                <span className="font-bold">{formatVND(p.basePrice)}</span>
                {p.soldOut && <span className="rounded-full bg-bad-soft px-2 py-0.5 text-xs font-bold text-[#7a2a1e]">Hết món</span>}
              </span>
            </button>
          ))}
          {products.length === 0 && <p className="col-span-3 text-muted">Không có món phù hợp.</p>}
        </div>
      </div>
    </div>
  );
}
