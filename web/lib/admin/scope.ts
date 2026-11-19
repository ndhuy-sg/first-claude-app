import type { Schemas } from "@/lib/api/client";
import type { Me } from "./permissions";

/** "all" = Toàn chuỗi (owner only), otherwise a branch id. */
export type Scope = "all" | number;

export type ScopeOption = { value: string; label: string };

type BranchLite = Pick<Schemas["Branch"], "id" | "code" | "name">;

/**
 * Options of the branch scope selector (W-12):
 * owner sees "Toàn chuỗi" + every branch; a branch-bound user sees only their own branch.
 */
export function scopeOptions(me: Me, branches: BranchLite[]): ScopeOption[] {
  const sorted = [...branches].sort((a, b) => a.code.localeCompare(b.code, "vi", { numeric: true }));
  if (me.user.role === "owner") {
    return [{ value: "all", label: "Toàn chuỗi" }, ...sorted.map((b) => ({ value: String(b.id), label: branchLabel(b) }))];
  }
  if (me.user.branchId === null) {
    // warehouse manager: chain-wide read where allowed, no "Toàn chuỗi" comparison
    return sorted.map((b) => ({ value: String(b.id), label: branchLabel(b) }));
  }
  return sorted.filter((b) => b.id === me.user.branchId).map((b) => ({ value: String(b.id), label: branchLabel(b) }));
}

export function branchLabel(b: BranchLite): string {
  return b.name && b.name !== b.code ? `${b.code} – ${b.name}` : b.code;
}

export function defaultScope(me: Me): Scope {
  if (me.user.role === "owner" || me.user.branchId === null) return "all";
  return me.user.branchId;
}

export function parseScope(value: string): Scope {
  return value === "all" ? "all" : Number(value);
}

/** Query parameter for the API: undefined = whole chain. */
export function scopeToBranchId(scope: Scope): number | undefined {
  return scope === "all" ? undefined : scope;
}
