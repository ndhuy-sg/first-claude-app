// Vietnamese formatting helpers. Always use these, never toLocaleString directly.

/** 1250000 → "1.250.000đ" */
export function formatVND(n: number): string {
  const sign = n < 0 ? "−" : "";
  return sign + String(Math.abs(Math.trunc(n))).replace(/\B(?=(\d{3})+(?!\d))/g, ".") + "đ";
}

const TZ = "Asia/Ho_Chi_Minh";

/** ISO → "19/11/2026" in Vietnam time */
export function formatDate(iso: string | Date): string {
  const d = typeof iso === "string" ? new Date(iso) : iso;
  const p = new Intl.DateTimeFormat("en-GB", { timeZone: TZ, day: "2-digit", month: "2-digit", year: "numeric" }).format(d);
  return p; // en-GB gives dd/mm/yyyy
}

/** ISO → "07:12" (24h) in Vietnam time */
export function formatTime(iso: string | Date): string {
  const d = typeof iso === "string" ? new Date(iso) : iso;
  return new Intl.DateTimeFormat("en-GB", { timeZone: TZ, hour: "2-digit", minute: "2-digit", hour12: false }).format(d);
}

/** Today's business date in Vietnam as YYYY-MM-DD */
export function todayVN(now = new Date()): string {
  return new Intl.DateTimeFormat("en-CA", { timeZone: TZ, year: "numeric", month: "2-digit", day: "2-digit" }).format(now);
}
