// UI mirror of the role matrix in api/internal/domain/roles.go.
// The API is the source of truth (it returns 403/404); this only decides what the admin UI shows.
import type { Schemas } from "@/lib/api/client";

export type Role = Schemas["Role"];
export type Me = Schemas["Me"];
export type AccessScope = "chain" | "warehouse" | "own" | null;

export type AdminAction =
  | "view_orders"
  | "view_sales_report"
  | "view_invoices"
  | "edit_menu"
  | "sold_out"
  | "manage_tables"
  | "manage_branches"
  | "manage_staff"
  | "manage_devices"
  | "settings"
  | "view_audit";

// Columns: owner, warehouse_manager, branch_manager, cashier, barista (same letters as roles.go).
const matrix: Record<AdminAction, string> = {
  view_orders: "axoox",
  view_sales_report: "axoxx",
  view_invoices: "axoxx",
  edit_menu: "axxxx",
  sold_out: "axoxo",
  manage_tables: "axoxx",
  manage_branches: "axxxx",
  manage_staff: "axoxx",
  manage_devices: "axoxx",
  settings: "axxxx",
  view_audit: "akoxx",
};

const column: Record<Role, number> = { owner: 0, warehouse_manager: 1, branch_manager: 2, cashier: 3, barista: 4 };

export function can(role: Role, action: AdminAction): AccessScope {
  const c = matrix[action][column[role]];
  if (c === "a") return "chain";
  if (c === "k") return "warehouse";
  if (c === "o") return "own";
  return null;
}

/** Roles that may sign in to the admin pages (cashiers and baristas use the POS only). */
export function isAdminRole(role: Role): boolean {
  return role === "owner" || role === "warehouse_manager" || role === "branch_manager";
}

/** Users bound to one branch always act on that branch (W-12, A-12). */
export function isBranchBound(me: Me): boolean {
  return me.user.role !== "owner" && me.user.branchId !== null;
}
