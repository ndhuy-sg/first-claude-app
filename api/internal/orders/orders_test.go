package orders_test

import (
	"context"
	"fmt"
	"sync"
	"testing"

	"github.com/ndhuy-sg/first-claude-app/api/internal/orderview"
	"github.com/ndhuy-sg/first-claude-app/api/internal/testutil"
)

func TestO01_CreateDineInOrderOccupiesTable(t *testing.T) {
	env := testutil.NewEnv(t)
	env.OpenShiftCN1()
	tn1 := env.Tn1()
	t5 := env.Fx.TablesCN1[5]

	if ft := floor(t, tn1)[t5]; ft.Status != "free" || ft.OpenOrder != nil {
		t.Fatalf("table 5 before = %+v", ft)
	}
	o := createDineIn(t, tn1, t5)
	if o.Status != "open" || o.Type != "dine_in" || o.TableID == nil || *o.TableID != t5 || o.TableName == nil || *o.TableName != "Bàn 5" {
		t.Fatalf("order = %+v", o)
	}
	if o.Code != "CN1-20261119-0001" || o.BranchID != env.Fx.Branches[1] || o.CreatedBy.ID != env.Fx.Users.Tn1 {
		t.Fatalf("code/branch/createdBy = %s %d %+v", o.Code, o.BranchID, o.CreatedBy)
	}
	if o.Items == nil || o.Payments == nil || o.Discount != nil {
		t.Fatalf("lists must be [] and discount null: %+v", o)
	}
	ft := floor(t, tn1)[t5]
	if ft.Status != "occupied" || ft.OpenOrder == nil || ft.OpenOrder.ID != o.ID || ft.OpenOrder.Code != o.Code {
		t.Fatalf("table 5 after = %+v", ft)
	}
	if !hasEvent(env, "order.updated") || !hasEvent(env, "table.updated") {
		t.Fatalf("events = %+v", env.Events.Events)
	}

	// Second order on the same table is refused; codes are sequential per branch and day (D-10).
	r := tn1.Post(base+"/orders", map[string]any{"type": "dine_in", "tableId": t5}).MustStatus(t, 409)
	if r.ErrCode() != "TABLE_OCCUPIED" {
		t.Fatalf("code = %s", r.ErrCode())
	}
	if o2 := createTakeaway(t, tn1); o2.Code != "CN1-20261119-0002" {
		t.Fatalf("second code = %s", o2.Code)
	}
	// Dine-in without a table is invalid; another branch's table is not found.
	tn1.Post(base+"/orders", map[string]any{"type": "dine_in"}).MustStatus(t, 400)
	tn1.Post(base+"/orders", map[string]any{"type": "dine_in", "tableId": env.Fx.TablesCN2[1]}).MustStatus(t, 404)
	tn1.Post(base+"/orders", map[string]any{"type": "delivery"}).MustStatus(t, 400)
}

func TestO01_TableAwaitingPayment(t *testing.T) {
	env := testutil.NewEnv(t)
	shift := env.OpenShiftCN1()
	id := env.InsertBan5Order(shift)
	exec(t, env, `INSERT INTO payments (order_id, branch_id, shift_id, method, status, amount, created_by) VALUES ($1, $2, $3, 'vietqr', 'pending', 128000, $4)`,
		id, env.Fx.Branches[1], shift, env.Fx.Users.Tn1)
	ft := floor(t, env.Tn1())[env.Fx.TablesCN1[5]]
	if ft.Status != "awaiting_payment" || ft.OpenOrder == nil || ft.OpenOrder.Total != 128_000 || ft.OpenOrder.ItemCount != 3 {
		t.Fatalf("table 5 = %+v", ft)
	}
}

func TestO02_CreateTakeawayOrder(t *testing.T) {
	env := testutil.NewEnv(t)
	env.OpenShiftCN1()
	o := decodeOrder(t, env.Tn1().Post(base+"/orders", map[string]any{"type": "takeaway", "tableId": env.Fx.TablesCN1[5]}).MustStatus(t, 201))
	if o.Type != "takeaway" || o.TableID != nil || o.TableName != nil || o.Status != "open" {
		t.Fatalf("order = %+v", o)
	}
	if ft := floor(t, env.Tn1())[env.Fx.TablesCN1[5]]; ft.Status != "free" {
		t.Fatalf("table 5 = %+v", ft)
	}
}

func TestO03_ServerComputesPrice(t *testing.T) {
	env := testutil.NewEnv(t)
	env.OpenShiftCN1()
	tn1 := env.Tn1()
	o := createTakeaway(t, tn1)
	r := tn1.Post(orderPath(o.ID, "/items"), map[string]any{
		"productId": env.Fx.Products.CaPheSua, "sizeCode": "L", "modifierIds": []int64{env.Fx.Modifiers.ThemShot},
		"qty": 1, "unitPrice": 1000, "note": "ít ngọt",
	}).MustStatus(t, 201)
	o = decodeOrder(t, r)
	if len(o.Items) != 1 {
		t.Fatalf("items = %+v", o.Items)
	}
	it := o.Items[0]
	if it.UnitPrice != 43_000 || it.LineTotal != 43_000 || it.Name != "Cà phê sữa" || it.SizeCode != "L" || it.Note != "ít ngọt" {
		t.Fatalf("item = %+v", it)
	}
	if len(it.Modifiers) != 1 || it.Modifiers[0].Name != "Thêm shot" || it.Modifiers[0].Price != 8_000 {
		t.Fatalf("modifiers = %+v", it.Modifiers)
	}
	if o.Subtotal != 43_000 || o.Total != 43_000 {
		t.Fatalf("subtotal/total = %d/%d", o.Subtotal, o.Total)
	}
	// qty 2 → 86.000đ (D-01 through the API)
	o = mustAdd(t, tn1, o.ID, env.Fx.Products.CaPheSua, "L", 2, env.Fx.Modifiers.ThemShot, env.Fx.Modifiers.ItDa)
	if o.Items[1].LineTotal != 86_000 || o.Total != 129_000 {
		t.Fatalf("line/total = %d/%d", o.Items[1].LineTotal, o.Total)
	}
	// Product without sizes: no size needed; product with sizes: size required and must exist.
	o = mustAdd(t, tn1, o.ID, env.Fx.Products.Croissant, "", 1)
	if o.Items[2].UnitPrice != 35_000 {
		t.Fatalf("croissant = %+v", o.Items[2])
	}
	addItem(tn1, o.ID, env.Fx.Products.CaPheSua, "", 1).MustStatus(t, 400)
	addItem(tn1, o.ID, env.Fx.Products.CaPheSua, "XL", 1).MustStatus(t, 400)
	addItem(tn1, o.ID, env.Fx.Products.CaPheSua, "M", 0).MustStatus(t, 400)
	addItem(tn1, o.ID, 999999, "M", 1).MustStatus(t, 404)
}

func TestO04_B04_SoldOutPerBranch(t *testing.T) {
	env := testutil.NewEnv(t)
	env.OpenShiftCN1()
	env.OpenShift(env.Fx.Branches[2], env.Fx.DeviceCN2, env.Fx.Users.Tn2, 500_000)
	exec(t, env, `INSERT INTO branch_sold_out (branch_id, product_id, marked_by) VALUES ($1, $2, $3)`,
		env.Fx.Branches[1], env.Fx.Products.CaPheSua, env.Fx.Users.Ql1)

	tn1 := env.Tn1()
	o := createTakeaway(t, tn1)
	r := addItem(tn1, o.ID, env.Fx.Products.CaPheSua, "M", 1).MustStatus(t, 409)
	if r.ErrCode() != "PRODUCT_SOLD_OUT" {
		t.Fatalf("code = %s", r.ErrCode())
	}
	// Other products still sell at CN1, and CN2 can still sell Cà phê sữa.
	mustAdd(t, tn1, o.ID, env.Fx.Products.CaPheDen, "M", 1)
	tn2 := env.Tn2()
	o2 := createTakeaway(t, tn2)
	if o2.Code != "CN2-20261119-0001" {
		t.Fatalf("CN2 code = %s", o2.Code)
	}
	mustAdd(t, tn2, o2.ID, env.Fx.Products.CaPheSua, "M", 1)
}

func TestO05_InvalidModifier(t *testing.T) {
	env := testutil.NewEnv(t)
	env.OpenShiftCN1()
	tn1 := env.Tn1()
	o := createTakeaway(t, tn1)
	var tranChau int64
	if err := env.Pool.QueryRow(context.Background(), `SELECT id FROM modifiers WHERE product_id = $1 AND name = 'Trân châu trắng'`, env.Fx.Products.TraDao).Scan(&tranChau); err != nil {
		t.Fatal(err)
	}
	r := addItem(tn1, o.ID, env.Fx.Products.CaPheSua, "M", 1, tranChau).MustStatus(t, 400)
	if r.ErrCode() != "INVALID_MODIFIER" {
		t.Fatalf("code = %s", r.ErrCode())
	}
	addItem(tn1, o.ID, env.Fx.Products.CaPheSua, "M", 1, 99999999).MustStatus(t, 400)
	exec(t, env, `UPDATE modifiers SET is_active = false WHERE id = $1`, env.Fx.Modifiers.ItNgot)
	if r := addItem(tn1, o.ID, env.Fx.Products.CaPheSua, "M", 1, env.Fx.Modifiers.ItNgot).MustStatus(t, 400); r.ErrCode() != "INVALID_MODIFIER" {
		t.Fatalf("inactive code = %s", r.ErrCode())
	}
	if got := decodeOrder(t, tn1.Get(orderPath(o.ID, ""))); len(got.Items) != 0 || got.Total != 0 {
		t.Fatalf("nothing should be added: %+v", got)
	}
}

func TestO06_PriceSnapshot(t *testing.T) {
	env := testutil.NewEnv(t)
	env.OpenShiftCN1()
	tn1 := env.Tn1()
	o := createTakeaway(t, tn1)
	mustAdd(t, tn1, o.ID, env.Fx.Products.CaPheSua, "L", 1, env.Fx.Modifiers.ThemShot)
	exec(t, env, `UPDATE products SET base_price = 31000, name = 'Cà phê sữa đá' WHERE id = $1`, env.Fx.Products.CaPheSua)
	exec(t, env, `UPDATE modifiers SET price = 10000 WHERE id = $1`, env.Fx.Modifiers.ThemShot)
	o = mustAdd(t, tn1, o.ID, env.Fx.Products.CaPheSua, "L", 1, env.Fx.Modifiers.ThemShot)
	old, nw := o.Items[0], o.Items[1]
	if old.UnitPrice != 43_000 || old.Name != "Cà phê sữa" || old.Modifiers[0].Price != 8_000 {
		t.Fatalf("old line changed: %+v", old)
	}
	if nw.UnitPrice != 47_000 || nw.Name != "Cà phê sữa đá" || nw.Modifiers[0].Price != 10_000 {
		t.Fatalf("new line = %+v", nw)
	}
	if o.Total != 90_000 {
		t.Fatalf("total = %d", o.Total)
	}
	// Editing the old line's qty keeps its snapshot price.
	o = decodeOrder(t, tn1.Patch(orderPath(o.ID, fmt.Sprintf("/items/%d", old.ID)), map[string]any{"qty": 2}).MustStatus(t, 200))
	if o.Items[0].UnitPrice != 43_000 || o.Items[0].LineTotal != 86_000 || o.Total != 133_000 {
		t.Fatalf("after qty change: %+v total %d", o.Items[0], o.Total)
	}
}

func TestO07_EditAndDeleteItems(t *testing.T) {
	env := testutil.NewEnv(t)
	shift := env.OpenShiftCN1()
	tn1 := env.Tn1()
	id := env.InsertBan5Order(shift) // 2×43.000 + 1×42.000 = 128.000
	o := decodeOrder(t, tn1.Get(orderPath(id, "")).MustStatus(t, 200))
	cf, tra := o.Items[0], o.Items[1]

	o = decodeOrder(t, tn1.Patch(orderPath(id, fmt.Sprintf("/items/%d", cf.ID)), map[string]any{"qty": 3, "note": "nóng"}).MustStatus(t, 200))
	if o.Items[0].Qty != 3 || o.Items[0].LineTotal != 129_000 || o.Items[0].Note != "nóng" || o.Subtotal != 171_000 || o.Total != 171_000 {
		t.Fatalf("after patch: %+v subtotal %d", o.Items[0], o.Subtotal)
	}
	// note only keeps qty
	o = decodeOrder(t, tn1.Patch(orderPath(id, fmt.Sprintf("/items/%d", cf.ID)), map[string]any{"note": ""}).MustStatus(t, 200))
	if o.Items[0].Qty != 3 || o.Items[0].Note != "" {
		t.Fatalf("note only: %+v", o.Items[0])
	}
	tn1.Patch(orderPath(id, fmt.Sprintf("/items/%d", cf.ID)), map[string]any{"qty": 0}).MustStatus(t, 400)

	o = decodeOrder(t, tn1.Delete(orderPath(id, fmt.Sprintf("/items/%d", tra.ID))).MustStatus(t, 200))
	if len(o.Items) != 1 || o.Total != 129_000 {
		t.Fatalf("after delete: %+v", o)
	}
	tn1.Delete(orderPath(id, fmt.Sprintf("/items/%d", tra.ID))).MustStatus(t, 404)

	// An item of another order is not reachable through this order.
	other := createTakeaway(t, tn1)
	other = mustAdd(t, tn1, other.ID, env.Fx.Products.Croissant, "", 1)
	tn1.Patch(orderPath(id, fmt.Sprintf("/items/%d", other.Items[0].ID)), map[string]any{"qty": 2}).MustStatus(t, 404)

	// A discount in percent is recomputed after item changes.
	tn1.Post(orderPath(id, "/discount"), map[string]any{"type": "percent", "value": 10, "reason": "Khách quen"}).MustStatus(t, 200)
	o = decodeOrder(t, tn1.Patch(orderPath(id, fmt.Sprintf("/items/%d", cf.ID)), map[string]any{"qty": 2}).MustStatus(t, 200))
	if o.Subtotal != 86_000 || o.Discount == nil || o.Discount.Amount != 8_600 || o.Total != 77_400 {
		t.Fatalf("percent discount after change: subtotal %d discount %+v total %d", o.Subtotal, o.Discount, o.Total)
	}
}

func TestO08_CashierDiscountWithinLimit(t *testing.T) {
	env := testutil.NewEnv(t)
	shift := env.OpenShiftCN1()
	tn1 := env.Tn1()
	id := env.InsertBan5Order(shift)

	if r := tn1.Post(orderPath(id, "/discount"), map[string]any{"type": "percent", "value": 10, "reason": " "}).MustStatus(t, 400); r.ErrCode() != "VALIDATION_ERROR" {
		t.Fatalf("missing reason code = %s", r.ErrCode())
	}
	o := decodeOrder(t, tn1.Post(orderPath(id, "/discount"), map[string]any{"type": "percent", "value": 20, "reason": "Khách quen"}).MustStatus(t, 200))
	if o.Discount == nil || o.Discount.Type != "percent" || o.Discount.Value != 20 || o.Discount.Amount != 25_600 || o.Discount.Reason != "Khách quen" || o.Discount.ApprovedBy != nil {
		t.Fatalf("discount = %+v", o.Discount)
	}
	if o.Subtotal != 128_000 || o.Total != 102_400 {
		t.Fatalf("subtotal/total = %d/%d", o.Subtotal, o.Total)
	}
	a := audits(t, env, "order.discount", id)
	if len(a) != 1 || a[0].ActorID == nil || *a[0].ActorID != env.Fx.Users.Tn1 || a[0].ApproverID != nil {
		t.Fatalf("audit = %+v", a)
	}

	// Amount discount ≤ 20%, and more than the subtotal is refused (D-04).
	o = decodeOrder(t, tn1.Post(orderPath(id, "/discount"), map[string]any{"type": "amount", "value": 20_000, "reason": "Đền bù"}).MustStatus(t, 200))
	if o.Discount.Amount != 20_000 || o.Total != 108_000 {
		t.Fatalf("amount discount = %+v total %d", o.Discount, o.Total)
	}
	tn1.Post(orderPath(id, "/discount"), map[string]any{"type": "amount", "value": 200_000, "reason": "x"}).MustStatus(t, 400)
	tn1.Post(orderPath(id, "/discount"), map[string]any{"type": "percent", "value": 101, "reason": "x"}).MustStatus(t, 400)
	tn1.Post(orderPath(id, "/discount"), map[string]any{"type": "bogus", "value": 1, "reason": "x"}).MustStatus(t, 400)

	// value 0 removes the discount.
	o = decodeOrder(t, tn1.Post(orderPath(id, "/discount"), map[string]any{"type": "percent", "value": 0, "reason": "Bỏ giảm"}).MustStatus(t, 200))
	if o.Discount != nil || o.Total != 128_000 {
		t.Fatalf("removed discount = %+v total %d", o.Discount, o.Total)
	}
}

func TestO09_DiscountAboveLimitNeedsManagerPIN(t *testing.T) {
	env := testutil.NewEnv(t)
	shift := env.OpenShiftCN1()
	tn1 := env.Tn1()
	id := env.InsertBan5Order(shift)
	req := func(extra map[string]any) map[string]any {
		b := map[string]any{"type": "percent", "value": 30, "reason": "Sự cố phục vụ"}
		for k, v := range extra {
			b[k] = v
		}
		return b
	}
	if r := tn1.Post(orderPath(id, "/discount"), req(nil)).MustStatus(t, 403); r.ErrCode() != "APPROVAL_REQUIRED" {
		t.Fatalf("no PIN code = %s", r.ErrCode())
	}
	if r := tn1.Post(orderPath(id, "/discount"), req(map[string]any{"managerId": env.Fx.Users.Ql1, "managerPin": "0000"})).MustStatus(t, 403); r.ErrCode() != "INVALID_MANAGER_PIN" {
		t.Fatalf("wrong PIN code = %s", r.ErrCode())
	}
	// Manager of another branch, and a cashier, cannot approve.
	if r := tn1.Post(orderPath(id, "/discount"), req(map[string]any{"managerId": env.Fx.Users.Ql2, "managerPin": "8888"})).MustStatus(t, 403); r.ErrCode() != "INVALID_MANAGER_PIN" {
		t.Fatalf("other branch manager code = %s", r.ErrCode())
	}
	if r := tn1.Post(orderPath(id, "/discount"), req(map[string]any{"managerId": env.Fx.Users.Tn1, "managerPin": "1234"})).MustStatus(t, 403); r.ErrCode() != "INVALID_MANAGER_PIN" {
		t.Fatalf("cashier approver code = %s", r.ErrCode())
	}
	// An amount above 20% of the subtotal also needs approval.
	if r := tn1.Post(orderPath(id, "/discount"), map[string]any{"type": "amount", "value": 30_000, "reason": "x"}).MustStatus(t, 403); r.ErrCode() != "APPROVAL_REQUIRED" {
		t.Fatalf("amount code = %s", r.ErrCode())
	}
	if got := decodeOrder(t, tn1.Get(orderPath(id, ""))); got.Discount != nil || len(audits(t, env, "order.discount", id)) != 0 {
		t.Fatalf("refused discounts must not change anything: %+v", got.Discount)
	}

	o := decodeOrder(t, tn1.Post(orderPath(id, "/discount"), req(map[string]any{"managerId": env.Fx.Users.Ql1, "managerPin": "9999"})).MustStatus(t, 200))
	if o.Discount == nil || o.Discount.Amount != 38_400 || o.Total != 89_600 || o.Discount.ApprovedBy == nil || o.Discount.ApprovedBy.ID != env.Fx.Users.Ql1 || o.Discount.ApprovedBy.Name != "Tuấn" {
		t.Fatalf("approved discount = %+v total %d", o.Discount, o.Total)
	}
	a := audits(t, env, "order.discount", id)
	if len(a) != 1 || *a[0].ActorID != env.Fx.Users.Tn1 || a[0].ApproverID == nil || *a[0].ApproverID != env.Fx.Users.Ql1 {
		t.Fatalf("audit = %+v", a)
	}

	// A branch manager discounting on the POS approves it themselves.
	o = decodeOrder(t, env.Ql1().Post(orderPath(id, "/discount"), req(map[string]any{"value": 50})).MustStatus(t, 200))
	if o.Discount.ApprovedBy == nil || o.Discount.ApprovedBy.ID != env.Fx.Users.Ql1 || o.Total != 64_000 {
		t.Fatalf("manager discount = %+v", o.Discount)
	}
}

func TestO10_MoveTable(t *testing.T) {
	env := testutil.NewEnv(t)
	env.OpenShiftCN1()
	tn1 := env.Tn1()
	t5, t6, t7 := env.Fx.TablesCN1[5], env.Fx.TablesCN1[6], env.Fx.TablesCN1[7]
	o5 := createDineIn(t, tn1, t5)
	createDineIn(t, tn1, t7)

	o := decodeOrder(t, tn1.Post(orderPath(o5.ID, "/move"), map[string]any{"tableId": t6}).MustStatus(t, 200))
	if o.TableID == nil || *o.TableID != t6 || *o.TableName != "Bàn 6" {
		t.Fatalf("moved order = %+v", o)
	}
	fl := floor(t, tn1)
	if fl[t5].Status != "free" || fl[t6].Status != "occupied" || fl[t6].OpenOrder.ID != o5.ID {
		t.Fatalf("floor: 5=%+v 6=%+v", fl[t5], fl[t6])
	}
	if r := tn1.Post(orderPath(o5.ID, "/move"), map[string]any{"tableId": t7}).MustStatus(t, 409); r.ErrCode() != "TABLE_OCCUPIED" {
		t.Fatalf("code = %s", r.ErrCode())
	}
	tn1.Post(orderPath(o5.ID, "/move"), map[string]any{"tableId": env.Fx.TablesCN2[1]}).MustStatus(t, 404)
	// A takeaway order can be seated at a table.
	ta := createTakeaway(t, tn1)
	o = decodeOrder(t, tn1.Post(orderPath(ta.ID, "/move"), map[string]any{"tableId": t5}).MustStatus(t, 200))
	if o.Type != "dine_in" || *o.TableID != t5 {
		t.Fatalf("seated takeaway = %+v", o)
	}
}

func TestO11_MergeTables(t *testing.T) {
	env := testutil.NewEnv(t)
	shift := env.OpenShiftCN1()
	tn1 := env.Tn1()
	t5, t6 := env.Fx.TablesCN1[5], env.Fx.TablesCN1[6]
	id5 := env.InsertBan5Order(shift) // 128.000
	o6 := createDineIn(t, tn1, t6)
	mustAdd(t, tn1, o6.ID, env.Fx.Products.Croissant, "", 2) // 70.000

	target := decodeOrder(t, tn1.Post(orderPath(id5, "/merge"), map[string]any{"targetOrderId": o6.ID}).MustStatus(t, 200))
	if target.ID != o6.ID || len(target.Items) != 3 || target.Subtotal != 198_000 || target.Total != 198_000 || *target.TableID != t6 {
		t.Fatalf("target = %+v", target)
	}
	src := decodeOrder(t, tn1.Get(orderPath(id5, "")).MustStatus(t, 200))
	if src.Status != "merged" || len(src.Items) != 0 || src.Total != 0 {
		t.Fatalf("source = %+v", src)
	}
	var into *int64
	if err := env.Pool.QueryRow(context.Background(), `SELECT merged_into_order_id FROM orders WHERE id = $1`, id5).Scan(&into); err != nil || into == nil || *into != o6.ID {
		t.Fatalf("merged_into = %v %v", into, err)
	}
	fl := floor(t, tn1)
	if fl[t5].Status != "free" || fl[t6].OpenOrder == nil || fl[t6].OpenOrder.ItemCount != 5 {
		t.Fatalf("floor: 5=%+v 6=%+v", fl[t5], fl[t6])
	}
	// Merging a closed order, or into itself, is refused.
	if r := tn1.Post(orderPath(id5, "/merge"), map[string]any{"targetOrderId": o6.ID}).MustStatus(t, 409); r.ErrCode() != "ORDER_NOT_OPEN" {
		t.Fatalf("code = %s", r.ErrCode())
	}
	tn1.Post(orderPath(o6.ID, "/merge"), map[string]any{"targetOrderId": o6.ID}).MustStatus(t, 400)
}

func TestO12_SplitBill(t *testing.T) {
	env := testutil.NewEnv(t)
	shift := env.OpenShiftCN1()
	tn1 := env.Tn1()
	id := env.InsertBan5Order(shift)
	orig := decodeOrder(t, tn1.Get(orderPath(id, "")))
	tra := orig.Items[1]

	var res struct {
		Original orderview.Order `json:"original"`
		Created  orderview.Order `json:"created"`
	}
	tn1.Post(orderPath(id, "/split"), map[string]any{"itemIds": []int64{tra.ID}}).MustStatus(t, 200).JSON(t, &res)
	if res.Original.ID != id || len(res.Original.Items) != 1 || res.Original.Total != 86_000 {
		t.Fatalf("original = %+v", res.Original)
	}
	c := res.Created
	if c.ID == id || c.Status != "open" || len(c.Items) != 1 || c.Items[0].ID != tra.ID || c.Total != 42_000 || c.TableID == nil || *c.TableID != env.Fx.TablesCN1[5] {
		t.Fatalf("created = %+v", c)
	}
	if res.Original.Total+c.Total != orig.Total || c.Code != "CN1-20261119-0002" {
		t.Fatalf("totals %d + %d != %d, code %s", res.Original.Total, c.Total, orig.Total, c.Code)
	}
	var from *int64
	if err := env.Pool.QueryRow(context.Background(), `SELECT split_from_order_id FROM orders WHERE id = $1`, c.ID).Scan(&from); err != nil || from == nil || *from != id {
		t.Fatalf("split_from = %v %v", from, err)
	}
	// Moving every item, an unknown item, or nothing is refused.
	tn1.Post(orderPath(id, "/split"), map[string]any{"itemIds": []int64{res.Original.Items[0].ID}}).MustStatus(t, 400)
	tn1.Post(orderPath(id, "/split"), map[string]any{"itemIds": []int64{tra.ID}}).MustStatus(t, 400)
	tn1.Post(orderPath(id, "/split"), map[string]any{"itemIds": []int64{}}).MustStatus(t, 400)

	// Takeaway split stays takeaway.
	ta := createTakeaway(t, tn1)
	mustAdd(t, tn1, ta.ID, env.Fx.Products.Croissant, "", 1)
	ta = mustAdd(t, tn1, ta.ID, env.Fx.Products.CaPheDen, "M", 1)
	tn1.Post(orderPath(ta.ID, "/split"), map[string]any{"itemIds": []int64{ta.Items[1].ID}}).MustStatus(t, 200).JSON(t, &res)
	if res.Created.Type != "takeaway" || res.Created.TableID != nil || res.Created.Total != 25_000 || res.Original.Total != 35_000 {
		t.Fatalf("takeaway split = %+v / %+v", res.Original, res.Created)
	}
}

func TestO13_CancelOrder(t *testing.T) {
	env := testutil.NewEnv(t)
	shift := env.OpenShiftCN1()
	tn1 := env.Tn1()
	id := env.InsertBan5Order(shift)

	tn1.Post(orderPath(id, "/cancel"), map[string]any{"reason": ""}).MustStatus(t, 400)
	tn1.Post(orderPath(id, "/cancel"), map[string]any{}).MustStatus(t, 400)
	o := decodeOrder(t, tn1.Post(orderPath(id, "/cancel"), map[string]any{"reason": "Khách đổi ý"}).MustStatus(t, 200))
	if o.Status != "cancelled" {
		t.Fatalf("status = %s", o.Status)
	}
	a := audits(t, env, "order.cancel", id)
	if len(a) != 1 || *a[0].ActorID != env.Fx.Users.Tn1 || a[0].Note != "Khách đổi ý" {
		t.Fatalf("audit = %+v", a)
	}
	if ft := floor(t, tn1)[env.Fx.TablesCN1[5]]; ft.Status != "free" {
		t.Fatalf("table 5 = %+v", ft)
	}
	// A cancelled order takes no more changes.
	if r := addItem(tn1, id, env.Fx.Products.CaPheDen, "M", 1).MustStatus(t, 409); r.ErrCode() != "ORDER_NOT_OPEN" {
		t.Fatalf("add to cancelled code = %s", r.ErrCode())
	}
	if r := tn1.Post(orderPath(id, "/cancel"), map[string]any{"reason": "x"}).MustStatus(t, 409); r.ErrCode() != "ORDER_NOT_OPEN" {
		t.Fatalf("cancel twice code = %s", r.ErrCode())
	}
	// A paid order cannot be cancelled.
	paid := env.InsertBan5Order(shift)
	exec(t, env, `UPDATE orders SET status = 'paid', paid_at = now(), paid_shift_id = $2 WHERE id = $1`, paid, shift)
	if r := tn1.Post(orderPath(paid, "/cancel"), map[string]any{"reason": "x"}).MustStatus(t, 409); r.ErrCode() != "ORDER_PAID" {
		t.Fatalf("cancel paid code = %s", r.ErrCode())
	}
	for _, r := range []*testutil.Resp{
		addItem(tn1, paid, env.Fx.Products.CaPheDen, "M", 1),
		tn1.Post(orderPath(paid, "/discount"), map[string]any{"type": "percent", "value": 5, "reason": "x"}),
		tn1.Post(orderPath(paid, "/move"), map[string]any{"tableId": env.Fx.TablesCN1[9]}),
		tn1.Put(orderPath(paid, "/buyer"), map[string]any{"name": "A"}),
	} {
		if r.Status != 409 || r.ErrCode() != "ORDER_NOT_OPEN" {
			t.Fatalf("change paid order: %d %s", r.Status, r.Body)
		}
	}
}

func TestO14_ConcurrentAddItems(t *testing.T) {
	env := testutil.NewEnv(t)
	env.OpenShiftCN1()
	o := createDineIn(t, env.Tn1(), env.Fx.TablesCN1[5])
	clients := []*testutil.Client{env.Tn1(), env.Ql1()}
	const perClient = 10
	var wg sync.WaitGroup
	statuses := make(chan int, len(clients)*perClient)
	for _, c := range clients {
		wg.Add(1)
		go func(c *testutil.Client) {
			defer wg.Done()
			for i := 0; i < perClient; i++ {
				statuses <- addItem(c, o.ID, env.Fx.Products.CaPheSua, "L", 1, env.Fx.Modifiers.ThemShot).Status
			}
		}(c)
	}
	wg.Wait()
	close(statuses)
	for s := range statuses {
		if s != 201 {
			t.Fatalf("status %d", s)
		}
	}
	got := decodeOrder(t, env.Tn1().Get(orderPath(o.ID, "")))
	if len(got.Items) != 2*perClient || got.Subtotal != 2*perClient*43_000 || got.Total != got.Subtotal {
		t.Fatalf("items %d subtotal %d total %d", len(got.Items), got.Subtotal, got.Total)
	}
}

func TestO14_ConcurrentCreateOrdersGetDistinctCodes(t *testing.T) {
	env := testutil.NewEnv(t)
	env.OpenShiftCN1()
	var wg sync.WaitGroup
	codes := make(chan string, 10)
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			r := env.Tn1().Post(base+"/orders", map[string]any{"type": "takeaway"})
			var o orderview.Order
			if r.Status == 201 {
				_ = jsonUnmarshal(r.Body, &o)
			}
			codes <- o.Code
		}()
	}
	wg.Wait()
	close(codes)
	seen := map[string]bool{}
	for c := range codes {
		if c == "" || seen[c] {
			t.Fatalf("missing or duplicate code %q", c)
		}
		seen[c] = true
	}
	for i := 1; i <= 10; i++ {
		if !seen[fmt.Sprintf("CN1-20261119-%04d", i)] {
			t.Fatalf("codes = %v", seen)
		}
	}
}

func TestO15_CreateOrderWithoutShift(t *testing.T) {
	env := testutil.NewEnv(t)
	tn1 := env.Tn1()
	r := tn1.Post(base+"/orders", map[string]any{"type": "takeaway"}).MustStatus(t, 409)
	if r.ErrCode() != "NO_OPEN_SHIFT" {
		t.Fatalf("code = %s", r.ErrCode())
	}
	// A shift open on another device (CN2) does not count, neither does a closed one.
	env.OpenShift(env.Fx.Branches[2], env.Fx.DeviceCN2, env.Fx.Users.Tn2, 0)
	shift := env.OpenShiftCN1()
	exec(t, env, `UPDATE shifts SET status = 'closed', closed_at = now() WHERE id = $1`, shift)
	tn1.Post(base+"/orders", map[string]any{"type": "takeaway"}).MustStatus(t, 409)
	// The owner (no POS device) cannot create orders either.
	if r := env.Owner().Post(base+"/orders", map[string]any{"type": "takeaway"}).MustStatus(t, 409); r.ErrCode() != "NO_OPEN_SHIFT" {
		t.Fatalf("owner code = %s", r.ErrCode())
	}
	env.OpenShiftCN1()
	createTakeaway(t, tn1)
}

func TestA12_BranchFromDeviceNotBody(t *testing.T) {
	env := testutil.NewEnv(t)
	env.OpenShiftCN1()
	env.OpenShift(env.Fx.Branches[2], env.Fx.DeviceCN2, env.Fx.Users.Tn2, 0)
	tn1 := env.Tn1()
	o := decodeOrder(t, tn1.Post(base+"/orders", map[string]any{"type": "takeaway", "branchId": env.Fx.Branches[2], "branchCode": "CN2"}).MustStatus(t, 201))
	if o.BranchID != env.Fx.Branches[1] || o.Code[:4] != "CN1-" {
		t.Fatalf("order branch = %d code %s", o.BranchID, o.Code)
	}
	cn2 := createTakeaway(t, env.Tn2())
	// tn1 cannot see or change CN2's order, and branchId in the list query is ignored.
	tn1.Get(orderPath(cn2.ID, "")).MustStatus(t, 404)
	addItem(tn1, cn2.ID, env.Fx.Products.CaPheDen, "M", 1).MustStatus(t, 404)
	tn1.Post(orderPath(cn2.ID, "/cancel"), map[string]any{"reason": "x"}).MustStatus(t, 404)
	var list struct {
		Items []orderview.Order `json:"items"`
		Total int               `json:"total"`
	}
	tn1.Get(fmt.Sprintf("%s/orders?branchId=%d", base, env.Fx.Branches[2])).MustStatus(t, 200).JSON(t, &list)
	if list.Total != 1 || len(list.Items) != 1 || list.Items[0].ID != o.ID {
		t.Fatalf("list = %+v", list)
	}
	// Merging into another branch's order is not possible either.
	tn1.Post(orderPath(o.ID, "/merge"), map[string]any{"targetOrderId": cn2.ID}).MustStatus(t, 404)
	// GET /tables of tn1 always shows CN1's floor.
	fl := floor(t, env.Tn1())
	if _, ok := fl[env.Fx.TablesCN1[5]]; !ok || len(fl) != 26 {
		t.Fatalf("floor has %d tables", len(fl))
	}
}

func TestListOrders_Filters(t *testing.T) {
	env := testutil.NewEnv(t)
	shift := env.OpenShiftCN1()
	tn1 := env.Tn1()
	id5 := env.InsertBan5Order(shift)
	ta := createTakeaway(t, tn1)
	mustAdd(t, tn1, ta.ID, env.Fx.Products.Croissant, "", 1)
	tn1.Post(orderPath(ta.ID, "/cancel"), map[string]any{"reason": "Nhầm"}).MustStatus(t, 200)
	// yesterday's order
	env.InsertOrder(testutil.OrderSpec{BranchID: env.Fx.Branches[1], ShiftID: shift, CreatedBy: env.Fx.Users.Tn1,
		CreatedAt: env.Clock.Now().AddDate(0, 0, -1),
		Items:     []testutil.ItemSpec{{ProductID: env.Fx.Products.CaPheDen, UnitPrice: 25_000, Qty: 1}}})

	type list struct {
		Items []orderview.Order `json:"items"`
		Total int               `json:"total"`
	}
	get := func(q string) list {
		t.Helper()
		var l list
		tn1.Get(base+"/orders"+q).MustStatus(t, 200).JSON(t, &l)
		return l
	}
	if l := get(""); l.Total != 2 || len(l.Items) != 2 || l.Items[0].ID != ta.ID {
		t.Fatalf("today = %+v", l)
	}
	if l := get("?status=open"); l.Total != 1 || l.Items[0].ID != id5 {
		t.Fatalf("open = %+v", l)
	}
	if l := get("?date=2026-11-18"); l.Total != 1 {
		t.Fatalf("yesterday = %+v", l)
	}
	if l := get("?q=0001"); l.Total != 1 || l.Items[0].ID != id5 {
		t.Fatalf("q code = %+v", l)
	}
	if l := get("?q=" + "B%C3%A0n%205"); l.Total != 1 || l.Items[0].ID != id5 {
		t.Fatalf("q table = %+v", l)
	}
	if l := get("?q=128000"); l.Total != 1 || l.Items[0].ID != id5 {
		t.Fatalf("q amount = %+v", l)
	}
	if l := get(fmt.Sprintf("?shiftId=%d&pageSize=1&page=2", shift)); l.Total != 2 || len(l.Items) != 1 || l.Items[0].ID != id5 {
		t.Fatalf("paging = %+v", l)
	}
	tn1.Get(base+"/orders?status=bogus").MustStatus(t, 400)
	tn1.Get(base+"/orders?date=19-11-2026").MustStatus(t, 400)

	// Owner sees every branch, or one when branchId is given.
	var l list
	env.Owner().Get(base+"/orders").MustStatus(t, 200).JSON(t, &l)
	if l.Total != 2 {
		t.Fatalf("owner list = %+v", l)
	}
	env.Owner().Get(fmt.Sprintf("%s/orders?branchId=%d", base, env.Fx.Branches[2])).MustStatus(t, 200).JSON(t, &l)
	if l.Total != 0 || l.Items == nil {
		t.Fatalf("owner CN2 list = %+v", l)
	}
	// Barista cannot list orders; anonymous is 401.
	env.AsOnDevice(env.Fx.Users.Pc1, env.Fx.DeviceCN1, env.Fx.DeviceCN1Token).Get(base+"/orders").MustStatus(t, 403)
	env.Anon().Get(base+"/orders").MustStatus(t, 401)
}

func TestSetBuyer_TaxCode(t *testing.T) {
	env := testutil.NewEnv(t)
	shift := env.OpenShiftCN1()
	tn1 := env.Tn1()
	id := env.InsertBan5Order(shift)
	if r := tn1.Put(orderPath(id, "/buyer"), map[string]any{"name": "Cty A", "taxCode": "12345"}).MustStatus(t, 400); r.ErrCode() != "INVALID_TAX_CODE" {
		t.Fatalf("code = %s", r.ErrCode())
	}
	o := decodeOrder(t, tn1.Put(orderPath(id, "/buyer"), map[string]any{
		"name": "Công ty TNHH A", "taxCode": " 0101234567-001 ", "address": "1 Lê Lợi", "email": "ketoan@a.vn",
	}).MustStatus(t, 200))
	if o.Buyer.Name != "Công ty TNHH A" || o.Buyer.TaxCode != "0101234567-001" || o.Buyer.Address != "1 Lê Lợi" || o.Buyer.Email != "ketoan@a.vn" {
		t.Fatalf("buyer = %+v", o.Buyer)
	}
	tn1.Put(orderPath(id, "/buyer"), map[string]any{"email": "not-an-email"}).MustStatus(t, 400)
	// Empty body = retail customer.
	o = decodeOrder(t, tn1.Put(orderPath(id, "/buyer"), map[string]any{}).MustStatus(t, 200))
	if o.Buyer.Name != "" || o.Buyer.TaxCode != "" {
		t.Fatalf("cleared buyer = %+v", o.Buyer)
	}
}
