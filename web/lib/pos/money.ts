// Pure POS helpers for what the screen *shows*. The server always recomputes prices,
// discounts and totals (O-03); these only preview them so the cashier sees the number first.
import type { Schemas } from "@/lib/api/client";

type MenuProduct = Schemas["MenuProduct"];

/** Unit price preview of a product with the chosen size and modifiers (D-01). */
export function unitPrice(product: Pick<MenuProduct, "basePrice" | "sizes" | "modifiers">, sizeCode: string | null, modifierIds: number[]): number {
  const size = product.sizes.find((s) => s.code === sizeCode);
  const mods = product.modifiers.filter((m) => modifierIds.includes(m.id));
  return product.basePrice + (size?.priceDelta ?? 0) + mods.reduce((sum, m) => sum + m.price, 0);
}

/** Discount amount preview, rounded down to the dong (D-03). */
export function discountAmount(subtotal: number, type: "percent" | "amount", value: number): number {
  if (value <= 0) return 0;
  if (type === "percent") return Math.floor((subtotal * value) / 100);
  return value;
}

/** Discount percentage of the subtotal, used to decide if manager approval is needed (D-05). */
export function discountPct(subtotal: number, type: "percent" | "amount", value: number): number {
  if (type === "percent") return value;
  if (subtotal <= 0) return 0;
  return (value * 100) / subtotal;
}

export function needsApproval(subtotal: number, type: "percent" | "amount", value: number, maxPct: number): boolean {
  return discountPct(subtotal, type, value) > maxPct;
}

/** VAT contained in a VAT-inclusive total (D-08). vatRateBp: 800 = 8%. */
export function vatFromGross(total: number, vatRateBp: number): { beforeVat: number; vat: number } {
  const beforeVat = Math.round((total * 10000) / (10000 + vatRateBp));
  return { beforeVat, vat: total - beforeVat };
}

/** Buyer tax code: 10 digits, or 10 digits + "-" + 3 digits (D-14, W-09). */
export function isValidTaxCode(s: string): boolean {
  return /^\d{10}(-\d{3})?$/.test(s.trim());
}

export function isValidEmail(s: string): boolean {
  return /^[^\s@]+@[^\s@]+\.[^\s@]+$/.test(s.trim());
}

/** Cash denominations counted at shift close, largest first. */
export const DENOMINATIONS = [500_000, 200_000, 100_000, 50_000, 20_000, 10_000, 5_000] as const;

export function countedTotal(counts: Record<number, number>): number {
  return DENOMINATIONS.reduce((sum, d) => sum + d * (counts[d] ?? 0), 0);
}

/** Shift note is required when |difference| is above the threshold (S-04, default 50.000đ). */
export const SHIFT_NOTE_THRESHOLD = 50_000;

/** "Ca sáng" / "Ca chiều" / "Ca tối" from the shift opening time (Vietnam time). */
export function shiftName(openedAt: string): string {
  const h = Number(new Intl.DateTimeFormat("en-GB", { timeZone: "Asia/Ho_Chi_Minh", hour: "2-digit", hour12: false }).format(new Date(openedAt)));
  if (h < 12) return "Ca sáng";
  if (h < 18) return "Ca chiều";
  return "Ca tối";
}

/** Whole minutes elapsed since an ISO time. */
export function minutesSince(iso: string, now: number): number {
  return Math.max(0, Math.floor((now - new Date(iso).getTime()) / 60_000));
}

/** Appends keypad input to an amount string (digits, "000", "⌫"). Max 10 digits. */
export function keypadAppend(current: string, key: string): string {
  if (key === "⌫") return current.slice(0, -1);
  const next = (current === "0" ? "" : current) + key;
  const trimmed = next.replace(/^0+(?=\d)/, "");
  return trimmed.length > 10 ? current : trimmed;
}

/** Text for how an order was paid, from its confirmed payments. */
export function paymentMethodsLabel(payments: Schemas["Payment"][]): string {
  const ok = payments.filter((p) => p.status === "confirmed");
  const cash = ok.some((p) => p.method === "cash");
  const qr = ok.some((p) => p.method === "vietqr");
  if (cash && qr) return "Tiền mặt + Chuyển khoản QR";
  if (qr) return "Chuyển khoản QR";
  return "Tiền mặt";
}

/** "Size L · Thêm shot · Ít đá" */
export function itemDetail(item: Pick<Schemas["OrderItem"], "sizeName" | "modifiers">): string {
  const parts: string[] = [];
  if (item.sizeName) parts.push(`Size ${item.sizeName}`);
  for (const m of item.modifiers) parts.push(m.name);
  return parts.join(" · ");
}

export function orderTitle(order: Pick<Schemas["Order"], "type" | "tableName">): string {
  return order.type === "dine_in" ? (order.tableName ?? "Tại bàn") : "Mang đi";
}

export function itemCount(order: Pick<Schemas["Order"], "items">): number {
  return order.items.reduce((n, i) => n + i.qty, 0);
}
