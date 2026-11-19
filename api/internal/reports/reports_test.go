package reports_test

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/ndhuy-sg/first-claude-app/api/internal/testutil"
)

// ---------- test data builders (direct SQL: payments belong to another module) ----------

type salesReport struct {
	From    string `json:"from"`
	To      string `json:"to"`
	GroupBy string `json:"groupBy"`
	Totals  struct {
		Revenue    int64 `json:"revenue"`
		OrderCount int   `json:"orderCount"`
		Discount   int64 `json:"discount"`
		Refunds    int64 `json:"refunds"`
	} `json:"totals"`
	Rows []row `json:"rows"`
}

type row struct {
	Key        string `json:"key"`
	Label      string `json:"label"`
	Revenue    int64  `json:"revenue"`
	OrderCount int    `json:"orderCount"`
	Qty        int    `json:"qty"`
}

func (r salesReport) row(t *testing.T, key string) row {
	t.Helper()
	for _, x := range r.Rows {
		if x.Key == key {
			return x
		}
	}
	t.Fatalf("row %q not found in %+v", key, r.Rows)
	return row{}
}

// vn returns the instant of a Vietnam wall-clock time on November 2026.
func vn(day, hour, min int) time.Time {
	return time.Date(2026, 11, day, hour, min, 0, 0, time.FixedZone("ICT", 7*3600))
}

type sale struct {
	branch  int64
	shift   int64
	by      int64
	items   []testutil.ItemSpec
	paidAt  time.Time
	status  string // default "paid"
	method  string // default "cash"
	payment []payment
	disc    int64
}

type payment struct {
	method string
	amount int64
	status string
}

// insertSale inserts an order, sets it paid (or another final status) and adds payments.
func insertSale(t *testing.T, env *testutil.Env, s sale) int64 {
	t.Helper()
	ctx := context.Background()
	id := env.InsertOrder(testutil.OrderSpec{BranchID: s.branch, ShiftID: s.shift, CreatedBy: s.by, Items: s.items, CreatedAt: s.paidAt.Add(-10 * time.Minute)})
	status := s.status
	if status == "" {
		status = "paid"
	}
	var total int64
	if _, err := env.Pool.Exec(ctx,
		`UPDATE orders SET status = $2, discount_amount = $3, total = subtotal - $3,
		        paid_at = CASE WHEN $2 IN ('paid', 'refunded') THEN $4::timestamptz END,
		        paid_shift_id = CASE WHEN $2 IN ('paid', 'refunded') THEN shift_id END
		 WHERE id = $1`, id, status, s.disc, s.paidAt); err != nil {
		t.Fatalf("update order: %v", err)
	}
	if err := env.Pool.QueryRow(ctx, `SELECT total FROM orders WHERE id = $1`, id).Scan(&total); err != nil {
		t.Fatal(err)
	}
	pays := s.payment
	if pays == nil && (status == "paid" || status == "refunded") {
		m := s.method
		if m == "" {
			m = "cash"
		}
		pays = []payment{{method: m, amount: total, status: "confirmed"}}
	}
	for _, p := range pays {
		var cash any
		if p.method == "cash" {
			cash = p.amount
		}
		if _, err := env.Pool.Exec(ctx,
			`INSERT INTO payments (order_id, branch_id, shift_id, method, status, amount, cash_received, change_amount, created_by, created_at, confirmed_at)
			 VALUES ($1, $2, $3, $4, $5, $6, $7, 0, $8, $9, $9)`,
			id, s.branch, s.shift, p.method, p.status, p.amount, cash, s.by, s.paidAt); err != nil {
			t.Fatalf("insert payment: %v", err)
		}
	}
	return id
}

func items(env *testutil.Env, specs ...any) []testutil.ItemSpec {
	// specs: productID, unitPrice, qty, ...
	var out []testutil.ItemSpec
	for i := 0; i+2 < len(specs); i += 3 {
		out = append(out, testutil.ItemSpec{ProductID: specs[i].(int64), SizeCode: "M", UnitPrice: int64(specs[i+1].(int)), Qty: specs[i+2].(int)})
	}
	return out
}

func shiftCN2(env *testutil.Env) int64 {
	return env.OpenShift(env.Fx.Branches[2], env.Fx.DeviceCN2, env.Fx.Users.Tn2, 0)
}

func getReport(t *testing.T, c *testutil.Client, q string) salesReport {
	t.Helper()
	var r salesReport
	c.Get("/api/v1/reports/sales?" + q).MustStatus(t, 200).JSON(t, &r)
	return r
}

// ---------- R-01 ----------

func TestR01_OnlyPaidOrdersCount(t *testing.T) {
	env := testutil.NewEnv(t)
	sh := env.OpenShiftCN1()
	cn1, tn1 := env.Fx.Branches[1], env.Fx.Users.Tn1
	p := env.Fx.Products
	insertSale(t, env, sale{branch: cn1, shift: sh, by: tn1, items: items(env, p.CaPheSua, 30000, 2), paidAt: vn(19, 8, 0)})
	insertSale(t, env, sale{branch: cn1, shift: sh, by: tn1, items: items(env, p.TraDao, 42000, 1), paidAt: vn(19, 9, 0), disc: 2000})
	insertSale(t, env, sale{branch: cn1, shift: sh, by: tn1, items: items(env, p.BacXiu, 35000, 1), paidAt: vn(19, 9, 30), status: "cancelled"})
	insertSale(t, env, sale{branch: cn1, shift: sh, by: tn1, items: items(env, p.ColdBrew, 45000, 1), paidAt: vn(19, 10, 0), status: "refunded"})
	insertSale(t, env, sale{branch: cn1, shift: sh, by: tn1, items: items(env, p.ColdBrew, 45000, 1), paidAt: vn(19, 10, 0), status: "merged"})
	insertSale(t, env, sale{branch: cn1, shift: sh, by: tn1, items: items(env, p.ColdBrew, 45000, 1), paidAt: vn(19, 10, 0), status: "open"})

	r := getReport(t, env.Ql1(), fmt.Sprintf("branchId=%d&from=2026-11-19&to=2026-11-19&groupBy=day", cn1))
	if r.Totals.Revenue != 60000+40000 || r.Totals.OrderCount != 2 || r.Totals.Discount != 2000 {
		t.Fatalf("totals = %+v", r.Totals)
	}
	if r.From != "2026-11-19" || r.To != "2026-11-19" || r.GroupBy != "day" {
		t.Fatalf("header = %s %s %s", r.From, r.To, r.GroupBy)
	}
	d := r.row(t, "2026-11-19")
	if d.Revenue != 100000 || d.OrderCount != 2 {
		t.Fatalf("day row = %+v", d)
	}
}

// ---------- R-02 ----------

func TestR02_ChainEqualsSumOfBranches(t *testing.T) {
	env := testutil.NewEnv(t)
	s1, s2 := env.OpenShiftCN1(), shiftCN2(env)
	p := env.Fx.Products
	insertSale(t, env, sale{branch: env.Fx.Branches[1], shift: s1, by: env.Fx.Users.Tn1, items: items(env, p.CaPheSua, 30000, 3), paidAt: vn(19, 8, 0)})
	insertSale(t, env, sale{branch: env.Fx.Branches[2], shift: s2, by: env.Fx.Users.Tn2, items: items(env, p.TraDao, 42000, 2), paidAt: vn(19, 8, 30)})
	insertSale(t, env, sale{branch: env.Fx.Branches[2], shift: s2, by: env.Fx.Users.Tn2, items: items(env, p.TraDao, 42000, 1), paidAt: vn(19, 9, 30)})

	owner := env.Owner()
	q := "from=2026-11-19&to=2026-11-19&groupBy=branch"
	chain := getReport(t, owner, q)
	var sum int64
	var cnt int
	for i := 1; i <= 5; i++ {
		b := getReport(t, owner, fmt.Sprintf("branchId=%d&%s", env.Fx.Branches[i], q))
		sum += b.Totals.Revenue
		cnt += b.Totals.OrderCount
	}
	if chain.Totals.Revenue != sum || chain.Totals.OrderCount != cnt || sum != 90000+84000+42000 {
		t.Fatalf("chain %+v, sum of branches %d/%d", chain.Totals, sum, cnt)
	}
	if len(chain.Rows) != 5 {
		t.Fatalf("branch rows = %d, want 5 (all branches, also without sales)", len(chain.Rows))
	}
	var rowsSum int64
	for _, r := range chain.Rows {
		rowsSum += r.Revenue
	}
	if rowsSum != chain.Totals.Revenue {
		t.Fatalf("rows sum %d != total %d", rowsSum, chain.Totals.Revenue)
	}
}

// ---------- R-03 ----------

func TestR03_ReportAccess(t *testing.T) {
	env := testutil.NewEnv(t)
	s1, s2 := env.OpenShiftCN1(), shiftCN2(env)
	p := env.Fx.Products
	insertSale(t, env, sale{branch: env.Fx.Branches[1], shift: s1, by: env.Fx.Users.Tn1, items: items(env, p.CaPheSua, 30000, 1), paidAt: vn(19, 8, 0)})
	insertSale(t, env, sale{branch: env.Fx.Branches[2], shift: s2, by: env.Fx.Users.Tn2, items: items(env, p.TraDao, 42000, 1), paidAt: vn(19, 8, 30)})

	// ql1 asking for CN2 (or for the whole chain) still gets CN1 only.
	ql1 := env.Ql1()
	for _, q := range []string{fmt.Sprintf("branchId=%d&", env.Fx.Branches[2]), ""} {
		r := getReport(t, ql1, q+"from=2026-11-19&to=2026-11-19&groupBy=branch")
		if r.Totals.Revenue != 30000 || len(r.Rows) != 1 || r.Rows[0].Key != fmt.Sprint(env.Fx.Branches[1]) {
			t.Fatalf("ql1 %q: %+v", q, r)
		}
	}
	var d dashboard
	ql1.Get(fmt.Sprintf("/api/v1/reports/dashboard?branchId=%d&date=2026-11-19", env.Fx.Branches[2])).MustStatus(t, 200).JSON(t, &d)
	if d.Revenue != 30000 || len(d.ByBranch) != 1 || d.ByBranch[0].BranchCode != "CN1" {
		t.Fatalf("ql1 dashboard: %+v", d)
	}

	// owner sees everything and compares branches.
	r := getReport(t, env.Owner(), "from=2026-11-19&to=2026-11-19&groupBy=branch")
	if r.Totals.Revenue != 72000 || r.row(t, fmt.Sprint(env.Fx.Branches[2])).Revenue != 42000 || r.row(t, fmt.Sprint(env.Fx.Branches[1])).Label == "" {
		t.Fatalf("owner: %+v", r)
	}

	// cashier and anonymous may not.
	env.Tn1().Get("/api/v1/reports/sales?from=2026-11-19&to=2026-11-19&groupBy=day").MustStatus(t, 403)
	env.Tn1().Get("/api/v1/reports/dashboard").MustStatus(t, 403)
	env.Anon().Get("/api/v1/reports/sales?from=2026-11-19&to=2026-11-19&groupBy=day").MustStatus(t, 401)
}

func TestR03_Validation(t *testing.T) {
	env := testutil.NewEnv(t)
	ql1 := env.Ql1()
	for _, q := range []string{
		"from=2026-11-19&to=2026-11-19",               // no groupBy
		"from=2026-11-19&to=2026-11-19&groupBy=table", // unknown groupBy
		"from=19/11/2026&to=2026-11-19&groupBy=day",   // bad date
		"from=2026-11-20&to=2026-11-19&groupBy=day",   // to before from
		"to=2026-11-19&groupBy=day",                   // no from
		"from=2025-01-01&to=2026-11-19&groupBy=day",   // range too long
	} {
		res := ql1.Get("/api/v1/reports/sales?" + q)
		if res.Status != 400 || res.ErrCode() != "VALIDATION_ERROR" {
			t.Errorf("%s: %d %s", q, res.Status, res.Body)
		}
	}
	if res := ql1.Get("/api/v1/reports/dashboard?date=xx"); res.Status != 400 {
		t.Errorf("dashboard bad date: %d", res.Status)
	}
}

// ---------- R-04 ----------

func TestR04_ByProduct(t *testing.T) {
	env := testutil.NewEnv(t)
	sh := env.OpenShiftCN1()
	cn1, tn1 := env.Fx.Branches[1], env.Fx.Users.Tn1
	p := env.Fx.Products
	insertSale(t, env, sale{branch: cn1, shift: sh, by: tn1, items: items(env, p.CaPheSua, 43000, 2, p.TraDao, 42000, 1), paidAt: vn(19, 8, 0)})
	insertSale(t, env, sale{branch: cn1, shift: sh, by: tn1, items: items(env, p.CaPheSua, 35000, 1), paidAt: vn(19, 9, 0), disc: 5000})
	insertSale(t, env, sale{branch: cn1, shift: sh, by: tn1, items: items(env, p.CaPheSua, 35000, 5), paidAt: vn(19, 9, 0), status: "cancelled"})

	r := getReport(t, env.Ql1(), "from=2026-11-19&to=2026-11-19&groupBy=product")
	cps := r.row(t, fmt.Sprint(p.CaPheSua))
	if cps.Qty != 3 || cps.Revenue != 86000+35000 || cps.OrderCount != 2 || cps.Label != "Cà phê sữa" {
		t.Fatalf("cà phê sữa = %+v", cps)
	}
	td := r.row(t, fmt.Sprint(p.TraDao))
	if td.Qty != 1 || td.Revenue != 42000 || td.OrderCount != 1 {
		t.Fatalf("trà đào = %+v", td)
	}
	if len(r.Rows) != 2 || r.Rows[0].Key != fmt.Sprint(p.CaPheSua) {
		t.Fatalf("rows (sorted by qty) = %+v", r.Rows)
	}
	// Product revenue is before order-level discounts: rows sum = revenue + discount.
	if r.Totals.Revenue != 128000+30000 || r.Totals.Discount != 5000 || cps.Revenue+td.Revenue != r.Totals.Revenue+r.Totals.Discount {
		t.Fatalf("totals = %+v", r.Totals)
	}
}

// ---------- R-05 ----------

func TestR05_VietnamDayBoundary(t *testing.T) {
	env := testutil.NewEnv(t)
	sh := env.OpenShiftCN1()
	cn1, tn1 := env.Fx.Branches[1], env.Fx.Users.Tn1
	p := env.Fx.Products
	insertSale(t, env, sale{branch: cn1, shift: sh, by: tn1, items: items(env, p.CaPheSua, 30000, 1), paidAt: vn(19, 23, 30)})
	insertSale(t, env, sale{branch: cn1, shift: sh, by: tn1, items: items(env, p.TraDao, 42000, 1), paidAt: vn(20, 0, 30)})
	insertSale(t, env, sale{branch: cn1, shift: sh, by: tn1, items: items(env, p.BacXiu, 35000, 1), paidAt: vn(19, 0, 0)}) // first instant of 19/11

	ql1 := env.Ql1()
	r := getReport(t, ql1, "from=2026-11-19&to=2026-11-20&groupBy=day")
	if d := r.row(t, "2026-11-19"); d.Revenue != 65000 || d.OrderCount != 2 {
		t.Fatalf("19/11 = %+v", d)
	}
	if d := r.row(t, "2026-11-20"); d.Revenue != 42000 || d.OrderCount != 1 {
		t.Fatalf("20/11 = %+v", d)
	}
	if one := getReport(t, ql1, "from=2026-11-19&to=2026-11-19&groupBy=day"); one.Totals.Revenue != 65000 {
		t.Fatalf("only 19/11: %+v", one.Totals)
	}
	var d dashboard
	ql1.Get("/api/v1/reports/dashboard?date=2026-11-20").MustStatus(t, 200).JSON(t, &d)
	if d.Revenue != 42000 || d.Date != "2026-11-20" {
		t.Fatalf("dashboard 20/11: %+v", d)
	}
	// hour grouping uses VN hours as well
	h := getReport(t, ql1, "from=2026-11-19&to=2026-11-20&groupBy=hour")
	if h.row(t, "23").Revenue != 30000 || h.row(t, "00").Revenue != 77000 {
		t.Fatalf("hours = %+v", h.Rows)
	}
}

// ---------- R-06 ----------

func TestR06_ByHourPaymentStaffShift(t *testing.T) {
	env := testutil.NewEnv(t)
	ctx := context.Background()
	cn1, tn1, ql1ID := env.Fx.Branches[1], env.Fx.Users.Tn1, env.Fx.Users.Ql1
	p := env.Fx.Products

	shA := env.OpenShiftCN1()
	insertSale(t, env, sale{branch: cn1, shift: shA, by: tn1, items: items(env, p.CaPheSua, 30000, 1), paidAt: vn(19, 7, 10)})
	insertSale(t, env, sale{branch: cn1, shift: shA, by: tn1, items: items(env, p.TraDao, 42000, 1), paidAt: vn(19, 7, 50), method: "vietqr"})
	insertSale(t, env, sale{branch: cn1, shift: shA, by: ql1ID, items: items(env, p.ColdBrew, 50000, 2), paidAt: vn(19, 9, 5),
		payment: []payment{{"cash", 40000, "confirmed"}, {"vietqr", 60000, "confirmed"}, {"vietqr", 60000, "cancelled"}}})
	if _, err := env.Pool.Exec(ctx, `UPDATE shifts SET status = 'closed', closed_at = $2 WHERE id = $1`, shA, vn(19, 12, 0)); err != nil {
		t.Fatal(err)
	}
	shB := env.OpenShift(cn1, env.Fx.DeviceCN1, ql1ID, 500000)
	insertSale(t, env, sale{branch: cn1, shift: shB, by: tn1, items: items(env, p.BacXiu, 35000, 1), paidAt: vn(19, 14, 20)})

	ql1 := env.Ql1()
	q := "from=2026-11-19&to=2026-11-19&groupBy="

	h := getReport(t, ql1, q+"hour")
	if len(h.Rows) != 3 || h.Rows[0].Key != "07" || h.row(t, "07").Revenue != 72000 || h.row(t, "07").OrderCount != 2 ||
		h.row(t, "09").Revenue != 100000 || h.row(t, "14").Revenue != 35000 || h.row(t, "07").Label != "07:00–08:00" {
		t.Fatalf("hour = %+v", h.Rows)
	}

	pm := getReport(t, ql1, q+"payment_method")
	if c := pm.row(t, "cash"); c.Revenue != 30000+40000+35000 || c.OrderCount != 3 || c.Label != "Tiền mặt" {
		t.Fatalf("cash = %+v", c)
	}
	if v := pm.row(t, "vietqr"); v.Revenue != 42000+60000 || v.OrderCount != 2 || v.Label != "Chuyển khoản" {
		t.Fatalf("vietqr = %+v", v)
	}

	st := getReport(t, ql1, q+"staff")
	if s := st.row(t, fmt.Sprint(tn1)); s.Revenue != 30000+42000+35000 || s.OrderCount != 3 || s.Label != "Lan" {
		t.Fatalf("tn1 = %+v", s)
	}
	if s := st.row(t, fmt.Sprint(ql1ID)); s.Revenue != 100000 || s.OrderCount != 1 {
		t.Fatalf("ql1 = %+v", s)
	}

	sf := getReport(t, ql1, q+"shift")
	if len(sf.Rows) != 2 || sf.Rows[0].Key != fmt.Sprint(shA) {
		t.Fatalf("shift rows = %+v", sf.Rows)
	}
	if a := sf.row(t, fmt.Sprint(shA)); a.Revenue != 172000 || a.OrderCount != 3 || a.Label == "" {
		t.Fatalf("shift A = %+v", a)
	}
	if b := sf.row(t, fmt.Sprint(shB)); b.Revenue != 35000 || b.OrderCount != 1 {
		t.Fatalf("shift B = %+v", b)
	}
	for _, r := range [][]row{h.Rows, pm.Rows, st.Rows, sf.Rows} {
		var sum int64
		for _, x := range r {
			sum += x.Revenue
		}
		if sum != 207000 {
			t.Fatalf("rows sum %d != 207000: %+v", sum, r)
		}
	}
}

// ---------- Dashboard ----------

type dashboard struct {
	Date           string `json:"date"`
	Revenue        int64  `json:"revenue"`
	OrderCount     int    `json:"orderCount"`
	AvgOrderValue  int64  `json:"avgOrderValue"`
	InvoicesIssued int    `json:"invoicesIssued"`
	ByBranch       []struct {
		BranchID   int64  `json:"branchId"`
		BranchCode string `json:"branchCode"`
		Revenue    int64  `json:"revenue"`
		OrderCount int    `json:"orderCount"`
	} `json:"byBranch"`
	TopProducts []struct {
		ProductID int64  `json:"productId"`
		Name      string `json:"name"`
		Qty       int    `json:"qty"`
		Revenue   int64  `json:"revenue"`
	} `json:"topProducts"`
	Alerts []struct {
		Kind     string  `json:"kind"`
		Severity string  `json:"severity"`
		Message  string  `json:"message"`
		Link     *string `json:"link"`
	} `json:"alerts"`
}

func insertInvoice(t *testing.T, env *testutil.Env, orderID, branch, number int64, kind, status string, at time.Time) {
	t.Helper()
	if _, err := env.Pool.Exec(context.Background(),
		`INSERT INTO invoices (branch_id, order_id, kind, symbol, number, lookup_code, seller_name, seller_tax_code, seller_address,
		                       buyer_name, payment_method, vat_rate_bp, amount_before_vat, vat_amount, total, status, issued_at)
		 VALUES ($1, $2, $3, '1C26MAA', $4, $5, 'Mộc', '0101', 'HN', 'Khách lẻ', 'Tiền mặt', 800, 1000, 80, 1080, $6, $7)`,
		branch, orderID, kind, number, fmt.Sprintf("LK%d-%d", branch, number), status, at); err != nil {
		t.Fatalf("insert invoice: %v", err)
	}
}

func TestDashboard(t *testing.T) {
	env := testutil.NewEnv(t)
	ctx := context.Background()
	s1, s2 := env.OpenShiftCN1(), shiftCN2(env)
	cn1, cn2 := env.Fx.Branches[1], env.Fx.Branches[2]
	p := env.Fx.Products
	o1 := insertSale(t, env, sale{branch: cn1, shift: s1, by: env.Fx.Users.Tn1, items: items(env, p.CaPheSua, 30000, 3, p.TraDao, 42000, 1), paidAt: vn(19, 8, 0)})
	o2 := insertSale(t, env, sale{branch: cn1, shift: s1, by: env.Fx.Users.Tn1, items: items(env, p.BacXiu, 35000, 1), paidAt: vn(19, 9, 0)})
	insertSale(t, env, sale{branch: cn2, shift: s2, by: env.Fx.Users.Tn2, items: items(env, p.ColdBrew, 45000, 2, p.Croissant, 25000, 1, p.CaPheDen, 25000, 1), paidAt: vn(19, 10, 0)})
	insertSale(t, env, sale{branch: cn2, shift: s2, by: env.Fx.Users.Tn2, items: items(env, p.ColdBrew, 45000, 1), paidAt: vn(18, 10, 0)}) // yesterday
	insertInvoice(t, env, o1, cn1, 1, "normal", "sent", vn(19, 8, 1))
	insertInvoice(t, env, o2, cn1, 2, "normal", "not_sent", vn(19, 9, 1))
	insertInvoice(t, env, o2, cn1, 3, "cancel", "not_sent", vn(19, 9, 2))
	insertInvoice(t, env, o2, cn1, 4, "normal", "sent", vn(18, 9, 1))
	// a closed CN2 shift with a cash difference above the threshold (50.000đ)
	if _, err := env.Pool.Exec(ctx, `UPDATE shifts SET status = 'closed', closed_at = $2, cash_diff = -60000 WHERE id = $1`, s2, vn(19, 11, 0)); err != nil {
		t.Fatal(err)
	}

	var d dashboard
	env.Clock.T = vn(19, 15, 0)
	env.Owner().Get("/api/v1/reports/dashboard").MustStatus(t, 200).JSON(t, &d) // date defaults to today (VN)
	if d.Date != "2026-11-19" || d.Revenue != 132000+35000+140000 || d.OrderCount != 3 || d.AvgOrderValue != 102333 || d.InvoicesIssued != 2 {
		t.Fatalf("dashboard = %+v", d)
	}
	if len(d.ByBranch) != 5 || d.ByBranch[0].BranchCode != "CN1" || d.ByBranch[0].Revenue != 167000 || d.ByBranch[1].Revenue != 140000 || d.ByBranch[1].OrderCount != 1 {
		t.Fatalf("byBranch = %+v", d.ByBranch)
	}
	if len(d.TopProducts) != 5 || d.TopProducts[0].ProductID != p.CaPheSua || d.TopProducts[0].Qty != 3 || d.TopProducts[1].ProductID != p.ColdBrew || d.TopProducts[1].Revenue != 90000 {
		t.Fatalf("topProducts = %+v", d.TopProducts)
	}
	kinds := map[string]string{}
	for _, a := range d.Alerts {
		kinds[a.Kind] = a.Severity
		if a.Message == "" {
			t.Fatalf("alert without message: %+v", a)
		}
		if a.Kind == "tax_submission_off" && (a.Link == nil || *a.Link != "/admin/hoa-don") {
			t.Fatalf("tax alert link: %+v", a)
		}
	}
	if kinds["tax_submission_off"] != "critical" || kinds["shift_cash_diff"] != "warning" {
		t.Fatalf("alerts = %+v", d.Alerts)
	}
	if _, ok := kinds["invoice_failed"]; ok {
		t.Fatalf("no failed invoices yet: %+v", d.Alerts)
	}

	// ql1: CN1 only, so no CN2 shift alert; a failed invoice adds a critical alert.
	insertInvoice(t, env, o2, cn1, 5, "normal", "failed", vn(19, 9, 3))
	var q dashboard
	env.Ql1().Get("/api/v1/reports/dashboard?date=2026-11-19").MustStatus(t, 200).JSON(t, &q)
	kinds = map[string]string{}
	for _, a := range q.Alerts {
		kinds[a.Kind] = a.Severity
	}
	if q.Revenue != 167000 || q.InvoicesIssued != 3 || kinds["invoice_failed"] != "critical" || kinds["tax_submission_off"] != "critical" {
		t.Fatalf("ql1 dashboard = %+v", q)
	}
	if _, ok := kinds["shift_cash_diff"]; ok {
		t.Fatalf("ql1 sees CN2 shift alert: %+v", q.Alerts)
	}

	// turning tax submission on removes that alert; an empty day has avg 0 and [] lists.
	if _, err := env.Pool.Exec(ctx, `UPDATE app_settings SET tax_submission_enabled = true`); err != nil {
		t.Fatal(err)
	}
	var e dashboard
	res := env.Owner().Get("/api/v1/reports/dashboard?date=2026-11-25").MustStatus(t, 200)
	res.JSON(t, &e)
	if e.Revenue != 0 || e.AvgOrderValue != 0 || e.TopProducts == nil || len(e.ByBranch) != 5 {
		t.Fatalf("empty day = %s", res.Body)
	}
	for _, a := range e.Alerts {
		if a.Kind == "tax_submission_off" {
			t.Fatalf("tax alert while enabled")
		}
	}
}
