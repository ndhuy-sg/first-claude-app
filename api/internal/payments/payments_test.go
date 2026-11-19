package payments_test

import (
	"fmt"
	"reflect"
	"strings"
	"sync"
	"testing"

	"github.com/ndhuy-sg/first-claude-app/api/internal/domain"
	"github.com/ndhuy-sg/first-claude-app/api/internal/payments"
	"github.com/ndhuy-sg/first-claude-app/api/internal/testutil"
)

func payPath(orderID int64) string { return fmt.Sprintf("/api/v1/orders/%d/payments", orderID) }

func cash(amount, received int64) map[string]any {
	return map[string]any{"method": "cash", "amount": amount, "cashReceived": received}
}

func qr(amount int64) map[string]any { return map[string]any{"method": "vietqr", "amount": amount} }

func decode(t *testing.T, r *testutil.Resp) payments.Result {
	t.Helper()
	var res payments.Result
	r.JSON(t, &res)
	return res
}

func count(t *testing.T, env *testutil.Env, sql string, args ...any) int {
	t.Helper()
	var n int
	if err := env.Pool.QueryRow(t.Context(), sql, args...).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}

func setup(t *testing.T) (*testutil.Env, *testutil.Client, int64, int64) {
	env := testutil.NewEnv(t)
	shift := env.OpenShiftCN1()
	return env, env.Tn1(), shift, env.InsertBan5Order(shift)
}

func TestP01_CashExactAmount(t *testing.T) {
	env, tn1, shift, oid := setup(t)
	res := decode(t, tn1.Post(payPath(oid), cash(128_000, 128_000)).MustStatus(t, 201))
	if res.Payment.Status != "confirmed" || res.Payment.Method != "cash" || *res.Payment.ChangeAmount != 0 || *res.Payment.CashReceived != 128_000 {
		t.Fatalf("payment: %+v", res.Payment)
	}
	o := res.Order
	if o.Status != "paid" || o.PaidAmount != 128_000 || o.PaidAt == nil || o.Invoice == nil {
		t.Fatalf("order: %+v", o)
	}
	var paidShift int64
	_ = env.Pool.QueryRow(t.Context(), `SELECT paid_shift_id FROM orders WHERE id = $1`, oid).Scan(&paidShift)
	if paidShift != shift {
		t.Fatalf("paid_shift_id = %d, want %d", paidShift, shift)
	}
	// the table is free again: no open order on Bàn 5
	if n := count(t, env, `SELECT count(*) FROM orders WHERE table_id = $1 AND status = 'open'`, env.Fx.TablesCN1[5]); n != 0 {
		t.Fatalf("open orders on table 5 = %d", n)
	}
}

func TestP02_CashWithChange(t *testing.T) {
	_, tn1, _, oid := setup(t)
	res := decode(t, tn1.Post(payPath(oid), cash(128_000, 200_000)).MustStatus(t, 201))
	if *res.Payment.CashReceived != 200_000 || *res.Payment.ChangeAmount != 72_000 || res.Order.Status != "paid" {
		t.Fatalf("result: %+v", res)
	}
}

func TestP03_CashLessThanAmount(t *testing.T) {
	env, tn1, _, oid := setup(t)
	if code := tn1.Post(payPath(oid), cash(128_000, 100_000)).MustStatus(t, 400).ErrCode(); code != "INSUFFICIENT_CASH" {
		t.Fatalf("code = %s", code)
	}
	tn1.Post(payPath(oid), map[string]any{"method": "cash", "amount": 128_000}).MustStatus(t, 400) // cashReceived missing
	if n := count(t, env, `SELECT count(*) FROM payments`); n != 0 {
		t.Fatalf("payments = %d", n)
	}
}

func TestP04_VietQRThenConfirm(t *testing.T) {
	env, tn1, _, oid := setup(t)
	res := decode(t, tn1.Post(payPath(oid), qr(128_000)).MustStatus(t, 201))
	if res.Payment.Status != "pending" || res.Order.Status != "open" || res.Order.PaidAmount != 0 || res.Order.Invoice != nil {
		t.Fatalf("after create: %+v", res)
	}
	top, err := domain.ParseTLV(res.Payment.QRPayload)
	if err != nil || top["54"] != "128000" || !strings.Contains(top["62"], "CN1-20261119-0001") || !strings.Contains(top["38"], "970436") {
		t.Fatalf("qr payload %q: %v", res.Payment.QRPayload, err)
	}
	// a pending QR reserves its amount: cash for the same amount is refused
	if code := tn1.Post(payPath(oid), cash(128_000, 128_000)).MustStatus(t, 400).ErrCode(); code != "AMOUNT_EXCEEDS_DUE" {
		t.Fatalf("code = %s", code)
	}
	conf := decode(t, tn1.Post(fmt.Sprintf("/api/v1/payments/%d/confirm", res.Payment.ID), nil).MustStatus(t, 200))
	if conf.Payment.Status != "confirmed" || conf.Payment.ConfirmedAt == nil || conf.Order.Status != "paid" || conf.Order.Invoice == nil {
		t.Fatalf("after confirm: %+v", conf)
	}
	// confirming twice → 409
	tn1.Post(fmt.Sprintf("/api/v1/payments/%d/confirm", res.Payment.ID), nil).MustStatus(t, 409)
	// CN2 cashier cannot see the payment (404)
	env.OpenShift(env.Fx.Branches[2], env.Fx.DeviceCN2, env.Fx.Users.Tn2, 0)
	env.Tn2().Post(fmt.Sprintf("/api/v1/payments/%d/cancel", res.Payment.ID), nil).MustStatus(t, 404)
}

func TestP05_CancelQRThenCash(t *testing.T) {
	env, tn1, _, oid := setup(t)
	res := decode(t, tn1.Post(payPath(oid), qr(128_000)).MustStatus(t, 201))
	can := decode(t, tn1.Post(fmt.Sprintf("/api/v1/payments/%d/cancel", res.Payment.ID), nil).MustStatus(t, 200))
	if can.Payment.Status != "cancelled" || can.Order.Status != "open" {
		t.Fatalf("after cancel: %+v", can)
	}
	paid := decode(t, tn1.Post(payPath(oid), cash(128_000, 150_000)).MustStatus(t, 201))
	if paid.Order.Status != "paid" || paid.Order.PaidAmount != 128_000 {
		t.Fatalf("order: %+v", paid.Order)
	}
	if n := count(t, env, `SELECT count(*) FROM payments WHERE order_id = $1 AND status = 'confirmed'`, oid); n != 1 {
		t.Fatalf("confirmed payments = %d", n)
	}
	// a cancelled QR cannot be confirmed afterwards
	tn1.Post(fmt.Sprintf("/api/v1/payments/%d/confirm", res.Payment.ID), nil).MustStatus(t, 409)
}

func TestP06_MixedCashAndQR(t *testing.T) {
	env, tn1, _, oid := setup(t)
	part := decode(t, tn1.Post(payPath(oid), cash(28_000, 50_000)).MustStatus(t, 201))
	if part.Order.Status != "open" || part.Order.PaidAmount != 28_000 || *part.Payment.ChangeAmount != 22_000 {
		t.Fatalf("after cash part: %+v", part)
	}
	if code := tn1.Post(payPath(oid), qr(100_001)).MustStatus(t, 400).ErrCode(); code != "AMOUNT_EXCEEDS_DUE" {
		t.Fatalf("code = %s", code)
	}
	q := decode(t, tn1.Post(payPath(oid), qr(100_000)).MustStatus(t, 201))
	if q.Order.Status != "open" {
		t.Fatal("order must stay open until the QR is confirmed")
	}
	done := decode(t, tn1.Post(fmt.Sprintf("/api/v1/payments/%d/confirm", q.Payment.ID), nil).MustStatus(t, 200))
	if done.Order.Status != "paid" || done.Order.PaidAmount != 128_000 || len(done.Order.Payments) != 2 {
		t.Fatalf("order: %+v", done.Order)
	}
	var method string
	_ = env.Pool.QueryRow(t.Context(), `SELECT payment_method FROM invoices WHERE order_id = $1`, oid).Scan(&method)
	if method != "Tiền mặt/Chuyển khoản" {
		t.Fatalf("invoice payment method = %q", method)
	}
}

func TestP07_PaidOrCancelledOrderIsNotOpen(t *testing.T) {
	env, tn1, shift, oid := setup(t)
	tn1.Post(payPath(oid), cash(128_000, 128_000)).MustStatus(t, 201)
	if code := tn1.Post(payPath(oid), cash(1, 1)).MustStatus(t, 409).ErrCode(); code != "ORDER_NOT_OPEN" {
		t.Fatalf("code = %s", code)
	}
	o2 := env.InsertOrder(testutil.OrderSpec{BranchID: env.Fx.Branches[1], ShiftID: shift, CreatedBy: env.Fx.Users.Tn1,
		Items: []testutil.ItemSpec{{ProductID: env.Fx.Products.CaPheDen, SizeCode: "M", UnitPrice: 25_000, Qty: 1}}})
	if _, err := env.Pool.Exec(t.Context(), `UPDATE orders SET status = 'cancelled', cancel_reason = 'x' WHERE id = $1`, o2); err != nil {
		t.Fatal(err)
	}
	tn1.Post(payPath(o2), cash(25_000, 25_000)).MustStatus(t, 409)
}

func TestP08_DoubleSubmitRecordsOnePayment(t *testing.T) {
	env, tn1, shift, oid := setup(t)
	r1 := tn1.Do("POST", payPath(oid), cash(128_000, 200_000), "Idempotency-Key", "pay-ban5-1")
	r1.MustStatus(t, 201)
	r2 := tn1.Do("POST", payPath(oid), cash(128_000, 200_000), "Idempotency-Key", "pay-ban5-1")
	r2.MustStatus(t, 201)
	if string(r1.Body) != string(r2.Body) || r2.Header.Get("Idempotent-Replay") != "true" {
		t.Fatalf("replay differs:\n%s\n%s", r1.Body, r2.Body)
	}
	if n := count(t, env, `SELECT count(*) FROM payments WHERE order_id = $1`, oid); n != 1 {
		t.Fatalf("payments = %d", n)
	}

	// concurrent duplicates without a key: the order row lock lets exactly one through
	o2 := env.InsertOrder(testutil.OrderSpec{BranchID: env.Fx.Branches[1], ShiftID: shift, CreatedBy: env.Fx.Users.Tn1,
		Items: []testutil.ItemSpec{{ProductID: env.Fx.Products.CaPheDen, SizeCode: "M", UnitPrice: 25_000, Qty: 2}}})
	clients := make([]*testutil.Client, 6)
	for i := range clients {
		clients[i] = env.Tn1()
	}
	statuses := make([]int, len(clients))
	var wg sync.WaitGroup
	for i, c := range clients {
		wg.Add(1)
		go func(i int, c *testutil.Client) {
			defer wg.Done()
			statuses[i] = c.Post(payPath(o2), cash(50_000, 50_000)).Status
		}(i, c)
	}
	wg.Wait()
	ok := 0
	for _, s := range statuses {
		switch s {
		case 201:
			ok++
		case 409:
		default:
			t.Fatalf("unexpected status %d in %v", s, statuses)
		}
	}
	if ok != 1 || count(t, env, `SELECT count(*) FROM payments WHERE order_id = $1`, o2) != 1 ||
		count(t, env, `SELECT count(*) FROM invoices WHERE order_id = $1`, o2) != 1 {
		t.Fatalf("statuses %v", statuses)
	}
}

func TestP09_RefundNeedsManagerAndIssuesCancelInvoice(t *testing.T) {
	env, tn1, shift, oid := setup(t)
	tn1.Post(payPath(oid), cash(128_000, 200_000)).MustStatus(t, 201)
	path := fmt.Sprintf("/api/v1/orders/%d/refund", oid)

	if code := tn1.Post(path, map[string]any{"reason": "Khách đổi ý"}).MustStatus(t, 403).ErrCode(); code != "APPROVAL_REQUIRED" {
		t.Fatalf("code = %s", code)
	}
	if code := tn1.Post(path, map[string]any{"reason": "Khách đổi ý", "managerId": env.Fx.Users.Ql1, "managerPin": "0000"}).MustStatus(t, 403).ErrCode(); code != "INVALID_MANAGER_PIN" {
		t.Fatalf("code = %s", code)
	}
	// a cashier's own PIN is not a manager approval; neither is CN2's manager
	tn1.Post(path, map[string]any{"reason": "x", "managerId": env.Fx.Users.Tn1, "managerPin": "1234"}).MustStatus(t, 403)
	tn1.Post(path, map[string]any{"reason": "x", "managerId": env.Fx.Users.Ql2, "managerPin": "8888"}).MustStatus(t, 403)
	tn1.Post(path, map[string]any{"reason": "  ", "managerId": env.Fx.Users.Ql1, "managerPin": "9999"}).MustStatus(t, 400)

	var o struct {
		Status string `json:"status"`
	}
	tn1.Post(path, map[string]any{"reason": "Khách đổi ý", "managerId": env.Fx.Users.Ql1, "managerPin": "9999"}).MustStatus(t, 200).JSON(t, &o)
	if o.Status != "refunded" {
		t.Fatalf("status = %s", o.Status)
	}
	var amount, cashAmount, approvedBy int64
	var refundShift *int64
	if err := env.Pool.QueryRow(t.Context(), `SELECT amount, cash_amount, approved_by, shift_id FROM refunds WHERE order_id = $1`, oid).Scan(&amount, &cashAmount, &approvedBy, &refundShift); err != nil {
		t.Fatal(err)
	}
	if amount != 128_000 || cashAmount != 128_000 || approvedBy != env.Fx.Users.Ql1 || refundShift == nil || *refundShift != shift {
		t.Fatalf("refund row: %d %d %d %v", amount, cashAmount, approvedBy, refundShift)
	}
	if n := count(t, env, `SELECT count(*) FROM audit_logs WHERE action = 'order.refund' AND entity_id = $1 AND approver_id = $2 AND note = 'Khách đổi ý'`, fmt.Sprint(oid), env.Fx.Users.Ql1); n != 1 {
		t.Fatalf("audit rows = %d", n)
	}
	// E-08: a cancel invoice referencing the original, with its own submit job
	var cancelID, origID int64
	if err := env.Pool.QueryRow(t.Context(), `SELECT c.id, c.original_invoice_id FROM invoices c WHERE c.order_id = $1 AND c.kind = 'cancel'`, oid).Scan(&cancelID, &origID); err != nil {
		t.Fatal(err)
	}
	if n := count(t, env, `SELECT count(*) FROM invoices WHERE id = $1 AND kind = 'normal' AND order_id = $2`, origID, oid); n != 1 {
		t.Fatal("original invoice mismatch")
	}
	if n := count(t, env, `SELECT count(*) FROM jobs WHERE kind = 'invoice.submit' AND dedupe_key = $1`, fmt.Sprintf("invoice.submit:%d", cancelID)); n != 1 {
		t.Fatal("cancel invoice submit job missing")
	}
	// refund twice / refund an open order → 409
	if code := tn1.Post(path, map[string]any{"reason": "again", "managerId": env.Fx.Users.Ql1, "managerPin": "9999"}).MustStatus(t, 409).ErrCode(); code != "ORDER_NOT_PAID" {
		t.Fatalf("code = %s", code)
	}
	// a manager refunds directly; the drawer of the shift loses the cash
	o2 := env.InsertBan5Order(shift)
	tn1.Post(payPath(o2), cash(128_000, 128_000)).MustStatus(t, 201)
	env.Ql1().Post(fmt.Sprintf("/api/v1/orders/%d/refund", o2), map[string]any{"reason": "Sai món"}).MustStatus(t, 200)
	var sum struct {
		CashSales    int64 `json:"cashSales"`
		CashRefunds  int64 `json:"cashRefunds"`
		ExpectedCash int64 `json:"expectedCash"`
	}
	tn1.Get(fmt.Sprintf("/api/v1/shifts/%d/summary", shift)).MustStatus(t, 200).JSON(t, &sum)
	if sum.CashSales != 256_000 || sum.CashRefunds != 256_000 || sum.ExpectedCash != 1_000_000 {
		t.Fatalf("summary: %+v", sum)
	}
}

func TestO16_PayEmptyOrder(t *testing.T) {
	env := testutil.NewEnv(t)
	shift := env.OpenShiftCN1()
	oid := env.InsertOrder(testutil.OrderSpec{BranchID: env.Fx.Branches[1], ShiftID: shift, TableID: &env.Fx.TablesCN1[3], CreatedBy: env.Fx.Users.Tn1})
	if code := env.Tn1().Post(payPath(oid), cash(1, 1)).MustStatus(t, 400).ErrCode(); code != "EMPTY_ORDER" {
		t.Fatalf("code = %s", code)
	}
}

func TestPaymentNeedsOpenShift(t *testing.T) {
	env := testutil.NewEnv(t)
	shift := env.OpenShiftCN1()
	oid := env.InsertBan5Order(shift)
	if _, err := env.Pool.Exec(t.Context(), `UPDATE shifts SET status = 'closed' WHERE id = $1`, shift); err != nil {
		t.Fatal(err)
	}
	if code := env.Tn1().Post(payPath(oid), cash(128_000, 128_000)).MustStatus(t, 409).ErrCode(); code != "NO_OPEN_SHIFT" {
		t.Fatalf("code = %s", code)
	}
}

func TestE01_PaymentAndInvoiceInOneTransaction(t *testing.T) {
	env, tn1, shift, oid := setup(t)
	res := decode(t, tn1.Post(payPath(oid), cash(128_000, 128_000)).MustStatus(t, 201))
	if res.Order.Invoice == nil || res.Order.Invoice.Status != "not_sent" || res.Order.Invoice.Number != "00000001" ||
		res.Order.Invoice.LookupURL != "http://localhost:3000/tra-cuu/"+res.Order.Invoice.LookupCode {
		t.Fatalf("invoice: %+v", res.Order.Invoice)
	}
	if n := count(t, env, `SELECT count(*) FROM jobs WHERE kind = 'invoice.submit' AND dedupe_key = $1`, fmt.Sprintf("invoice.submit:%d", res.Order.Invoice.ID)); n != 1 {
		t.Fatal("submit job missing")
	}
	// If issuing the invoice fails, the payment is rolled back too: no paid order without an invoice.
	o2 := env.InsertBan5Order(shift)
	if _, err := env.Pool.Exec(t.Context(), `DELETE FROM invoice_series WHERE branch_id = $1`, env.Fx.Branches[1]); err != nil {
		t.Fatal(err)
	}
	tn1.Post(payPath(o2), cash(128_000, 128_000)).MustStatus(t, 500)
	var status string
	_ = env.Pool.QueryRow(t.Context(), `SELECT status FROM orders WHERE id = $1`, o2).Scan(&status)
	if status != "open" || count(t, env, `SELECT count(*) FROM payments WHERE order_id = $1`, o2) != 0 {
		t.Fatalf("order %s, payments must be rolled back", status)
	}
}

func TestCashSuggestionsEndpoint(t *testing.T) {
	env := testutil.NewEnv(t)
	var got []int64
	env.Tn1().Get("/api/v1/payments/cash-suggestions?amount=86000").MustStatus(t, 200).JSON(t, &got)
	if !reflect.DeepEqual(got, []int64{86_000, 90_000, 100_000, 200_000, 500_000}) {
		t.Fatalf("got %v", got)
	}
	env.Tn1().Get("/api/v1/payments/cash-suggestions?amount=abc").MustStatus(t, 400)
	env.Anon().Get("/api/v1/payments/cash-suggestions?amount=1").MustStatus(t, 401)
}
