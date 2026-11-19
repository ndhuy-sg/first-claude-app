import { scopeToBranchId, type Scope } from "./scope";

export type SalesGroupBy = "day" | "product" | "hour" | "payment_method" | "staff" | "shift" | "branch";

export type SalesFilters = { scope: Scope; from: string; to: string };

export type SalesQuery = { branchId?: number; from: string; to: string; groupBy: SalesGroupBy };

/** Query of GET /reports/sales for the report filters (W-13). from/to are swapped if reversed. */
export function buildSalesQuery(f: SalesFilters, groupBy: SalesGroupBy): SalesQuery {
  const [from, to] = f.from <= f.to ? [f.from, f.to] : [f.to, f.from];
  const q: SalesQuery = { from, to, groupBy };
  const branchId = scopeToBranchId(f.scope);
  if (branchId !== undefined) q.branchId = branchId;
  return q;
}

/** "2026-11-19" + n days (calendar arithmetic on business dates, no time zone involved). */
export function addDays(date: string, n: number): string {
  const [y, m, d] = date.split("-").map(Number);
  const t = new Date(Date.UTC(y, m - 1, d + n));
  return t.toISOString().slice(0, 10);
}

/** Every date from..to inclusive (capped at 366 entries). */
export function daysBetween(from: string, to: string): string[] {
  const out: string[] = [];
  for (let d = from; d <= to && out.length < 366; d = addDays(d, 1)) out.push(d);
  return out;
}

/** "2026-11-19" → "19/11" */
export function shortDate(date: string): string {
  const [, m, d] = date.split("-");
  return `${d}/${m}`;
}

/** "2026-11-19" → "19/11/2026" (business date, no time zone conversion) */
export function longDate(date: string): string {
  const [y, m, d] = date.split("-");
  return `${d}/${m}/${y}`;
}

/** Average order value, rounded down to the dong. */
export function average(revenue: number, orders: number): number {
  return orders > 0 ? Math.floor(revenue / orders) : 0;
}

/** Share as a whole percent string: 0.4567 → "46%" */
export function percent(part: number, whole: number): string {
  if (whole <= 0) return "0%";
  return `${Math.round((part / whole) * 100)}%`;
}
