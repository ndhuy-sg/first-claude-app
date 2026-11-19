// Package domain holds pure business rules with no database or HTTP access.
// Everything here must be unit-testable without PostgreSQL.
package domain

// Role is a user role. Values match the user_role enum in the database.
type Role string

const (
	RoleOwner            Role = "owner"
	RoleWarehouseManager Role = "warehouse_manager"
	RoleBranchManager    Role = "branch_manager"
	RoleCashier          Role = "cashier"
	RoleBarista          Role = "barista"
)

func (r Role) Valid() bool {
	switch r {
	case RoleOwner, RoleWarehouseManager, RoleBranchManager, RoleCashier, RoleBarista:
		return true
	}
	return false
}

// Scope says where a role may perform an action.
type Scope int

const (
	ScopeNone      Scope = iota // not allowed
	ScopeOwnBranch              // only in the user's own branch
	ScopeWarehouse              // only for the central warehouse
	ScopeChain                  // every branch and the warehouse
)

// Action is something a user can do. The matrix below mirrors the
// "Ma trận phân quyền" board on the design canvas.
type Action string

const (
	ActSell             Action = "sell"              // create orders, add items, take payment
	ActDiscount         Action = "discount"          // discount up to the max % without approval
	ActApprove          Action = "approve"           // approve discount above max %, refund
	ActSoldOut          Action = "sold_out"          // toggle sold-out
	ActBarScreen        Action = "bar_screen"        // barista display
	ActShift            Action = "shift"             // open/close shift
	ActViewOrders       Action = "view_orders"       // list/view orders
	ActViewSalesReport  Action = "view_sales_report" // reports and dashboard
	ActViewInvoices     Action = "view_invoices"     // list invoices, resend
	ActEditMenu         Action = "edit_menu"         // products, prices, recipes
	ActManageTables     Action = "manage_tables"     // areas and tables
	ActManageBranches   Action = "manage_branches"   // create branch, tax info, VietQR account
	ActWarehouseReceipt Action = "warehouse_receipt" // goods receipt into warehouse, production
	ActApproveTransfer  Action = "approve_transfer"  // approve and ship transfers
	ActRequestReceive   Action = "request_receive"   // branch requests and receives goods
	ActLocalPurchase    Action = "local_purchase"    // branch buys locally
	ActStocktake        Action = "stocktake"         // stocktake, waste
	ActApproveStocktake Action = "approve_stocktake" // approve stocktake variance above threshold
	ActManageStaff      Action = "manage_staff"      // users, PIN reset, lock
	ActManageDevices    Action = "manage_devices"    // activate/revoke POS devices
	ActSettings         Action = "settings"          // system settings, tax switch
	ActViewAudit        Action = "view_audit"        // audit log
)

// Columns: owner, warehouse_manager, branch_manager, cashier, barista.
// a = chain, k = warehouse, o = own branch, x = none.
var matrix = map[Action]string{
	ActSell:             "axoox",
	ActDiscount:         "axoox",
	ActApprove:          "axoxx",
	ActSoldOut:          "axoxo",
	ActBarScreen:        "axooo",
	ActShift:            "axoox",
	ActViewOrders:       "axoox",
	ActViewSalesReport:  "axoxx",
	ActViewInvoices:     "axoxx",
	ActEditMenu:         "axxxx",
	ActManageTables:     "axoxx",
	ActManageBranches:   "axxxx",
	ActWarehouseReceipt: "akxxx",
	ActApproveTransfer:  "akxxx",
	ActRequestReceive:   "axoxx",
	ActLocalPurchase:    "axoxx",
	ActStocktake:        "akoxx",
	ActApproveStocktake: "akxxx",
	ActManageStaff:      "axoxx",
	ActManageDevices:    "axoxx",
	ActSettings:         "axxxx",
	ActViewAudit:        "akoxx",
}

var roleColumn = map[Role]int{
	RoleOwner: 0, RoleWarehouseManager: 1, RoleBranchManager: 2, RoleCashier: 3, RoleBarista: 4,
}

// Can returns the scope in which role may perform action.
func Can(role Role, action Action) Scope {
	row, ok := matrix[action]
	col, ok2 := roleColumn[role]
	if !ok || !ok2 {
		return ScopeNone
	}
	switch row[col] {
	case 'a':
		return ScopeChain
	case 'k':
		return ScopeWarehouse
	case 'o':
		return ScopeOwnBranch
	}
	return ScopeNone
}

// AllActions lists every action (used by tests that walk the whole matrix).
func AllActions() []Action {
	out := make([]Action, 0, len(matrix))
	for a := range matrix {
		out = append(out, a)
	}
	return out
}
