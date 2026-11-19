package shifts_test

import (
	"fmt"
	"testing"

	"github.com/ndhuy-sg/first-claude-app/api/internal/shifts"
	"github.com/ndhuy-sg/first-claude-app/api/internal/testutil"
)

func payCash(t *testing.T, c *testutil.Client, orderID, amount, received int64) {
	t.Helper()
	c.Post(fmt.Sprintf("/api/v1/orders/%d/payments", orderID), map[string]any{"method": "cash", "amount": amount, "cashReceived": received}).MustStatus(t, 201)
}

func openViaAPI(t *testing.T, c *testutil.Client, cash int64) shifts.Shift {
	t.Helper()
	var s shifts.Shift
	c.Post("/api/v1/shifts/open", map[string]any{"openingCash": cash}).MustStatus(t, 201).JSON(t, &s)
	return s
}

func TestS01_OneOpenShiftPerDevice(t *testing.T) {
	env := testutil.NewEnv(t)
	tn1 := env.Tn1()
	r := tn1.Get("/api/v1/shifts/current").MustStatus(t, 200)
	if string(r.Body) != "null" {
		t.Fatalf("current before open = %s, want null", r.Body)
	}
	s := openViaAPI(t, tn1, 1_000_000)
	if s.Status != "open" || s.OpeningCash != 1_000_000 || s.DeviceID != env.Fx.DeviceCN1 || s.BranchID != env.Fx.Branches[1] || s.OpenedBy.ID != env.Fx.Users.Tn1 {
		t.Fatalf("unexpected shift: %+v", s)
	}
	if code := tn1.Post("/api/v1/shifts/open", map[string]any{"openingCash": 500_000}).MustStatus(t, 409).ErrCode(); code != "SHIFT_ALREADY_OPEN" {
		t.Fatalf("code = %s", code)
	}
	// another user on the same device also gets 409
	env.Ql1().Post("/api/v1/shifts/open", map[string]any{"openingCash": 0}).MustStatus(t, 409)
	var cur shifts.Shift
	tn1.Get("/api/v1/shifts/current").MustStatus(t, 200).JSON(t, &cur)
	if cur.ID != s.ID {
		t.Fatalf("current = %d, want %d", cur.ID, s.ID)
	}
	// CN2 device is independent
	openViaAPI(t, env.Tn2(), 0)
	// a session without a POS device cannot open a shift
	env.Owner().Post("/api/v1/shifts/open", map[string]any{"openingCash": 0}).MustStatus(t, 401)
}

func TestS02_CloseShiftComputesTotals(t *testing.T) {
	env := testutil.NewEnv(t)
	tn1 := env.Tn1()
	s := openViaAPI(t, tn1, 1_000_000)
	o1 := env.InsertBan5Order(s.ID) // 128.000
	payCash(t, tn1, o1, 128_000, 200_000)
	o2 := env.InsertOrder(testutil.OrderSpec{BranchID: env.Fx.Branches[1], ShiftID: s.ID, CreatedBy: env.Fx.Users.Tn1,
		Items: []testutil.ItemSpec{{ProductID: env.Fx.Products.TraDao, SizeCode: "M", UnitPrice: 42_000, Qty: 1}}})
	var res struct {
		Payment struct{ ID int64 } `json:"payment"`
	}
	tn1.Post(fmt.Sprintf("/api/v1/orders/%d/payments", o2), map[string]any{"method": "vietqr", "amount": 42_000}).MustStatus(t, 201).JSON(t, &res)
	tn1.Post(fmt.Sprintf("/api/v1/payments/%d/confirm", res.Payment.ID), nil).MustStatus(t, 200)

	var before shifts.Summary
	tn1.Get(fmt.Sprintf("/api/v1/shifts/%d/summary", s.ID)).MustStatus(t, 200).JSON(t, &before)
	if before.CountedCash != nil || before.CashDiff != nil || before.ExpectedCash != 1_128_000 {
		t.Fatalf("summary before close: %+v", before)
	}

	var sum shifts.Summary
	tn1.Post(fmt.Sprintf("/api/v1/shifts/%d/close", s.ID), map[string]any{"countedCash": 1_120_000}).MustStatus(t, 200).JSON(t, &sum)
	if sum.CashSales != 128_000 || sum.QrSales != 42_000 || sum.OrderCount != 2 || sum.Revenue != 170_000 ||
		sum.ExpectedCash != 1_128_000 || sum.CountedCash == nil || *sum.CountedCash != 1_120_000 || sum.CashDiff == nil || *sum.CashDiff != -8_000 ||
		sum.Shift.Status != "closed" || sum.Shift.ClosedAt == nil || sum.InvoicesNotSent != 2 {
		t.Fatalf("summary: %+v", sum)
	}
	// closing again → 409 SHIFT_CLOSED; current is null again
	if code := tn1.Post(fmt.Sprintf("/api/v1/shifts/%d/close", s.ID), map[string]any{"countedCash": 0}).MustStatus(t, 409).ErrCode(); code != "SHIFT_CLOSED" {
		t.Fatalf("code = %s", code)
	}
	if r := tn1.Get("/api/v1/shifts/current").MustStatus(t, 200); string(r.Body) != "null" {
		t.Fatalf("current after close = %s", r.Body)
	}
	var n int
	_ = env.Pool.QueryRow(t.Context(), `SELECT count(*) FROM audit_logs WHERE action = 'shift.close' AND entity_id = $1`, fmt.Sprint(s.ID)).Scan(&n)
	if n != 1 {
		t.Fatalf("audit shift.close rows = %d", n)
	}
	found := false
	for _, e := range env.Events.Events {
		if e.Type == "shift.closed" && e.BranchID == env.Fx.Branches[1] {
			found = true
		}
	}
	if !found {
		t.Fatal("shift.closed event not published")
	}
	// another branch's shift is hidden (404)
	env.Tn2().Get(fmt.Sprintf("/api/v1/shifts/%d/summary", s.ID)).MustStatus(t, 404)
}

func TestS03_CloseWithOpenOrders(t *testing.T) {
	env := testutil.NewEnv(t)
	tn1 := env.Tn1()
	s := openViaAPI(t, tn1, 500_000)
	oid := env.InsertBan5Order(s.ID)
	r := tn1.Post(fmt.Sprintf("/api/v1/shifts/%d/close", s.ID), map[string]any{"countedCash": 500_000}).MustStatus(t, 409)
	var body struct {
		Error struct {
			Code    string `json:"code"`
			Details struct {
				Orders []struct {
					ID        int64   `json:"id"`
					Code      string  `json:"code"`
					TableName *string `json:"tableName"`
					Total     int64   `json:"total"`
				} `json:"orders"`
			} `json:"details"`
		} `json:"error"`
	}
	r.JSON(t, &body)
	if body.Error.Code != "OPEN_ORDERS" || len(body.Error.Details.Orders) != 1 || body.Error.Details.Orders[0].ID != oid ||
		body.Error.Details.Orders[0].Code != "CN1-20261119-0001" || body.Error.Details.Orders[0].TableName == nil || *body.Error.Details.Orders[0].TableName != "Bàn 5" {
		t.Fatalf("body = %s", r.Body)
	}
	// after paying, the shift closes
	payCash(t, tn1, oid, 128_000, 128_000)
	tn1.Post(fmt.Sprintf("/api/v1/shifts/%d/close", s.ID), map[string]any{"countedCash": 628_000}).MustStatus(t, 200)
}

func TestS04_LargeDifferenceNeedsNote(t *testing.T) {
	env := testutil.NewEnv(t)
	tn1 := env.Tn1()
	s := openViaAPI(t, tn1, 1_000_000)
	path := fmt.Sprintf("/api/v1/shifts/%d/close", s.ID)
	// the rule is |diff| > 50.000: 50.001 short needs a note
	if code := tn1.Post(path, map[string]any{"countedCash": 949_999}).MustStatus(t, 400).ErrCode(); code != "NOTE_REQUIRED" {
		t.Fatalf("code = %s", code)
	}
	if code := tn1.Post(path, map[string]any{"countedCash": 1_060_000, "note": "  "}).MustStatus(t, 400).ErrCode(); code != "NOTE_REQUIRED" {
		t.Fatalf("blank note: code = %s", code)
	}
	var sum shifts.Summary
	tn1.Post(path, map[string]any{"countedCash": 949_999, "note": "Thiếu tiền, đã báo quản lý"}).MustStatus(t, 200).JSON(t, &sum)
	if *sum.CashDiff != -50_001 || sum.Note != "Thiếu tiền, đã báo quản lý" {
		t.Fatalf("summary: %+v", sum)
	}
	// exactly the threshold needs no note
	s2 := openViaAPI(t, tn1, 1_000_000)
	tn1.Post(fmt.Sprintf("/api/v1/shifts/%d/close", s2.ID), map[string]any{"countedCash": 950_000}).MustStatus(t, 200)
}

func TestS05_PaidOrderBelongsToShiftOpenAtPayment(t *testing.T) {
	env := testutil.NewEnv(t)
	tn1 := env.Tn1()
	shiftA := env.OpenShiftCN1()
	oid := env.InsertBan5Order(shiftA)
	// shift A ends (handover) while the table is still open; shift B starts on the same device
	if _, err := env.Pool.Exec(t.Context(), `UPDATE shifts SET status = 'closed', closed_at = $2 WHERE id = $1`, shiftA, env.Clock.Now()); err != nil {
		t.Fatal(err)
	}
	shiftB := openViaAPI(t, tn1, 200_000)
	payCash(t, tn1, oid, 128_000, 150_000)

	var paidShift, payShift int64
	if err := env.Pool.QueryRow(t.Context(), `SELECT o.paid_shift_id, p.shift_id FROM orders o JOIN payments p ON p.order_id = o.id WHERE o.id = $1`, oid).Scan(&paidShift, &payShift); err != nil {
		t.Fatal(err)
	}
	if paidShift != shiftB.ID || payShift != shiftB.ID {
		t.Fatalf("paid_shift_id = %d, payment shift = %d, want %d", paidShift, payShift, shiftB.ID)
	}
	var a, b shifts.Summary
	tn1.Get(fmt.Sprintf("/api/v1/shifts/%d/summary", shiftA)).MustStatus(t, 200).JSON(t, &a)
	tn1.Get(fmt.Sprintf("/api/v1/shifts/%d/summary", shiftB.ID)).MustStatus(t, 200).JSON(t, &b)
	if a.OrderCount != 0 || a.CashSales != 0 || b.OrderCount != 1 || b.CashSales != 128_000 || b.ExpectedCash != 328_000 {
		t.Fatalf("A = %+v\nB = %+v", a, b)
	}
}
