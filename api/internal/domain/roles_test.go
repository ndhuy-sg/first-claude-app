package domain

import "testing"

// TestD12_PermissionMatrix walks every (role, action) pair against the
// "Ma trận phân quyền" board, copied here as an explicit table.
// Letters per role: a = toàn chuỗi, k = chỉ kho tổng, o = chi nhánh của mình, x = không được.
func TestD12_PermissionMatrix(t *testing.T) {
	roles := []Role{RoleOwner, RoleWarehouseManager, RoleBranchManager, RoleCashier, RoleBarista}
	// columns:                     chủ chuỗi, QL kho, QL chi nhánh, thu ngân, pha chế
	expected := map[Action][5]byte{
		ActSell:             {'a', 'x', 'o', 'o', 'x'}, // bán hàng, thanh toán
		ActDiscount:         {'a', 'x', 'o', 'o', 'x'}, // giảm giá trong hạn mức
		ActApprove:          {'a', 'x', 'o', 'x', 'x'}, // duyệt giảm giá vượt mức, hoàn tiền
		ActSoldOut:          {'a', 'x', 'o', 'x', 'o'}, // bật/tắt hết món
		ActBarScreen:        {'a', 'x', 'o', 'o', 'o'}, // màn hình quầy pha chế
		ActShift:            {'a', 'x', 'o', 'o', 'x'}, // mở/chốt ca
		ActViewOrders:       {'a', 'x', 'o', 'o', 'x'}, // xem đơn
		ActViewSalesReport:  {'a', 'x', 'o', 'x', 'x'}, // báo cáo bán hàng
		ActViewInvoices:     {'a', 'x', 'o', 'x', 'x'}, // hoá đơn điện tử
		ActEditMenu:         {'a', 'x', 'x', 'x', 'x'}, // sửa menu, giá
		ActManageTables:     {'a', 'x', 'o', 'x', 'x'}, // khu vực, bàn
		ActManageBranches:   {'a', 'x', 'x', 'x', 'x'}, // chi nhánh, thông tin thuế
		ActWarehouseReceipt: {'a', 'k', 'x', 'x', 'x'}, // nhập kho tổng, sản xuất
		ActApproveTransfer:  {'a', 'k', 'x', 'x', 'x'}, // duyệt, xuất chuyển kho
		ActRequestReceive:   {'a', 'x', 'o', 'x', 'x'}, // yêu cầu, nhận hàng
		ActLocalPurchase:    {'a', 'x', 'o', 'x', 'x'}, // mua tại chỗ
		ActStocktake:        {'a', 'k', 'o', 'x', 'x'}, // kiểm kê, hao hụt
		ActApproveStocktake: {'a', 'k', 'x', 'x', 'x'}, // duyệt chênh lệch kiểm kê
		ActManageStaff:      {'a', 'x', 'o', 'x', 'x'}, // nhân viên
		ActManageDevices:    {'a', 'x', 'o', 'x', 'x'}, // máy POS
		ActSettings:         {'a', 'x', 'x', 'x', 'x'}, // cài đặt hệ thống
		ActViewAudit:        {'a', 'k', 'o', 'x', 'x'}, // nhật ký thao tác
	}
	scopeOf := map[byte]Scope{'a': ScopeChain, 'k': ScopeWarehouse, 'o': ScopeOwnBranch, 'x': ScopeNone}

	all := AllActions()
	if len(all) != len(expected) {
		t.Fatalf("matrix has %d actions, expected table has %d", len(all), len(expected))
	}
	for _, a := range all {
		row, ok := expected[a]
		if !ok {
			t.Errorf("action %q is not in the expected table", a)
			continue
		}
		for i, r := range roles {
			if got, want := Can(r, a), scopeOf[row[i]]; got != want {
				t.Errorf("Can(%s, %s) = %d, want %d (%c)", r, a, got, want, row[i])
			}
		}
	}
	if Can(Role("guest"), ActSell) != ScopeNone || Can(RoleOwner, Action("nope")) != ScopeNone {
		t.Error("unknown role or action must be ScopeNone")
	}
	for _, r := range roles {
		if !r.Valid() {
			t.Errorf("%s should be valid", r)
		}
	}
	if Role("guest").Valid() {
		t.Error("guest must be invalid")
	}
}
