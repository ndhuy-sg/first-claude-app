package invoices_test

import (
	"fmt"
	"regexp"
	"sort"
	"sync"
	"testing"

	"github.com/ndhuy-sg/first-claude-app/api/internal/domain"
	"github.com/ndhuy-sg/first-claude-app/api/internal/invoices"
	"github.com/ndhuy-sg/first-claude-app/api/internal/testutil"
)

type paid struct {
	Order struct {
		ID      int64 `json:"id"`
		Invoice struct {
			ID         int64  `json:"id"`
			Symbol     string `json:"symbol"`
			Number     string `json:"number"`
			LookupCode string `json:"lookupCode"`
		} `json:"invoice"`
	} `json:"order"`
}

func payCash(t *testing.T, c *testutil.Client, orderID, amount int64) paid {
	t.Helper()
	var p paid
	c.Post(fmt.Sprintf("/api/v1/orders/%d/payments", orderID), map[string]any{"method": "cash", "amount": amount, "cashReceived": amount}).MustStatus(t, 201).JSON(t, &p)
	return p
}

func getInvoice(t *testing.T, c *testutil.Client, id int64) invoices.Invoice {
	t.Helper()
	var inv invoices.Invoice
	c.Get(fmt.Sprintf("/api/v1/invoices/%d", id)).MustStatus(t, 200).JSON(t, &inv)
	return inv
}

func cn2Order(env *testutil.Env, shift int64, unit int64) int64 {
	return env.InsertOrder(testutil.OrderSpec{BranchID: env.Fx.Branches[2], ShiftID: shift, CreatedBy: env.Fx.Users.Tn2,
		Items: []testutil.ItemSpec{{ProductID: env.Fx.Products.CaPheSua, SizeCode: "M", UnitPrice: unit, Qty: 1}}})
}

func TestE02_InvoiceData(t *testing.T) {
	env := testutil.NewEnv(t)
	shift := env.OpenShiftCN1()
	p := payCash(t, env.Tn1(), env.InsertBan5Order(shift), 128_000)
	inv := getInvoice(t, env.Ql1(), p.Order.Invoice.ID)
	if inv.Symbol != "1C26MAA" || inv.Number != "00000001" || inv.Kind != "normal" || inv.Status != "not_sent" ||
		inv.BranchID != env.Fx.Branches[1] || inv.OrderCode != "CN1-20261119-0001" {
		t.Fatalf("header: %+v", inv.InvoiceSummary)
	}
	if inv.Seller.Name != "Chuỗi cà phê Mộc" || inv.Seller.TaxCode != "0100000000" || inv.Seller.Address != "Địa chỉ CN1" {
		t.Fatalf("seller: %+v", inv.Seller)
	}
	if inv.Buyer.Name != "Khách lẻ" || inv.Buyer.TaxCode != "" {
		t.Fatalf("buyer: %+v", inv.Buyer)
	}
	want := []invoices.Line{
		{LineNo: 1, Name: "Cà phê sữa (L)", Qty: 2, UnitPrice: 43_000, Amount: 86_000},
		{LineNo: 2, Name: "Trà đào cam sả (M)", Qty: 1, UnitPrice: 42_000, Amount: 42_000},
	}
	if fmt.Sprint(inv.Lines) != fmt.Sprint(want) {
		t.Fatalf("lines: %+v", inv.Lines)
	}
	if inv.VatRateBp != 800 || inv.AmountBeforeVat != 118_519 || inv.VatAmount != 9_481 || inv.Total != 128_000 ||
		inv.PaymentMethod != "Tiền mặt" || inv.TotalInWords != "Một trăm hai mươi tám nghìn đồng" || inv.Cancelled || inv.SentAt != nil {
		t.Fatalf("totals: %+v", inv)
	}
	if inv.LookupURL != "http://localhost:3000/tra-cuu/"+inv.LookupCode {
		t.Fatalf("lookup url %s", inv.LookupURL)
	}

	// A discounted order gets a negative "Chiết khấu thương mại" line; lines add up to the total.
	o2 := env.InsertBan5Order(shift)
	if _, err := env.Pool.Exec(t.Context(), `UPDATE orders SET discount_type = 'percent', discount_value = 10, discount_amount = 12800, discount_reason = 'Khách quen', total = 115200 WHERE id = $1`, o2); err != nil {
		t.Fatal(err)
	}
	p2 := payCash(t, env.Tn1(), o2, 115_200)
	inv2 := getInvoice(t, env.Ql1(), p2.Order.Invoice.ID)
	var sum int64
	for _, l := range inv2.Lines {
		sum += l.Amount
	}
	last := inv2.Lines[len(inv2.Lines)-1]
	if len(inv2.Lines) != 3 || last.Name != "Chiết khấu thương mại" || last.Amount != -12_800 || sum != 115_200 || inv2.Total != 115_200 || inv2.Number != "00000002" {
		t.Fatalf("discounted invoice: %+v", inv2)
	}
	b, v := domain.SplitVAT(115_200, 800)
	if inv2.AmountBeforeVat != b || inv2.VatAmount != v {
		t.Fatalf("vat split %d+%d", inv2.AmountBeforeVat, inv2.VatAmount)
	}
}

func TestE06_CompanyBuyer(t *testing.T) {
	env := testutil.NewEnv(t)
	oid := env.InsertBan5Order(env.OpenShiftCN1())
	if _, err := env.Pool.Exec(t.Context(), `UPDATE orders SET buyer_name = 'Công ty TNHH ABC', buyer_tax_code = '0101234567-001', buyer_address = '1 Lê Lợi, Q1', buyer_email = 'ketoan@abc.vn' WHERE id = $1`, oid); err != nil {
		t.Fatal(err)
	}
	p := payCash(t, env.Tn1(), oid, 128_000)
	inv := getInvoice(t, env.Tn1(), p.Order.Invoice.ID) // the POS may open the invoice it issued
	if inv.Buyer.Name != "Công ty TNHH ABC" || inv.Buyer.TaxCode != "0101234567-001" || inv.Buyer.Address != "1 Lê Lợi, Q1" || inv.Buyer.Email != "ketoan@abc.vn" {
		t.Fatalf("buyer: %+v", inv.Buyer)
	}
	var n int
	_ = env.Pool.QueryRow(t.Context(), `SELECT count(*) FROM jobs WHERE kind = 'invoice.email' AND payload->>'email' = 'ketoan@abc.vn'`).Scan(&n)
	if n != 1 {
		t.Fatalf("email jobs = %d", n)
	}
}

func TestE07_CN2InvoiceUsesCN2Series(t *testing.T) {
	env := testutil.NewEnv(t)
	payCash(t, env.Tn1(), env.InsertBan5Order(env.OpenShiftCN1()), 128_000)
	s2 := env.OpenShift(env.Fx.Branches[2], env.Fx.DeviceCN2, env.Fx.Users.Tn2, 0)
	p := payCash(t, env.Tn2(), cn2Order(env, s2, 29_000), 29_000)
	if p.Order.Invoice.Symbol != "1C26MAB" || p.Order.Invoice.Number != "00000001" {
		t.Fatalf("CN2 invoice: %+v", p.Order.Invoice)
	}
	inv := getInvoice(t, env.Owner(), p.Order.Invoice.ID)
	if inv.Seller.Address != "Địa chỉ CN2" || inv.BranchID != env.Fx.Branches[2] {
		t.Fatalf("seller: %+v", inv.Seller)
	}
	// ql1 cannot open a CN2 invoice (404)
	env.Ql1().Get(fmt.Sprintf("/api/v1/invoices/%d", inv.ID)).MustStatus(t, 404)
}

func TestE14_BranchTaxInfo(t *testing.T) {
	env := testutil.NewEnv(t)
	// CN2 has its own tax code and 10% VAT; CN1 keeps the company code
	if _, err := env.Pool.Exec(t.Context(), `UPDATE branches SET tax_code = '0311111111', vat_rate_bp = 1000 WHERE id = $1`, env.Fx.Branches[2]); err != nil {
		t.Fatal(err)
	}
	p1 := payCash(t, env.Tn1(), env.InsertBan5Order(env.OpenShiftCN1()), 128_000)
	s2 := env.OpenShift(env.Fx.Branches[2], env.Fx.DeviceCN2, env.Fx.Users.Tn2, 0)
	p2 := payCash(t, env.Tn2(), cn2Order(env, s2, 86_000), 86_000)
	owner := env.Owner()
	i1, i2 := getInvoice(t, owner, p1.Order.Invoice.ID), getInvoice(t, owner, p2.Order.Invoice.ID)
	if i1.Seller.TaxCode != "0100000000" || i1.VatRateBp != 800 {
		t.Fatalf("CN1: %+v", i1.Seller)
	}
	if i2.Seller.TaxCode != "0311111111" || i2.VatRateBp != 1000 || i2.AmountBeforeVat != 78_182 || i2.VatAmount != 7_818 {
		t.Fatalf("CN2: %+v %d %d", i2.Seller, i2.AmountBeforeVat, i2.VatAmount)
	}
}

func TestE09_ListAndResend(t *testing.T) {
	env := testutil.NewEnv(t)
	shift := env.OpenShiftCN1()
	tn1 := env.Tn1()
	a := payCash(t, tn1, env.InsertBan5Order(shift), 128_000)
	payCash(t, tn1, env.InsertBan5Order(shift), 128_000)
	s2 := env.OpenShift(env.Fx.Branches[2], env.Fx.DeviceCN2, env.Fx.Users.Tn2, 0)
	b := payCash(t, env.Tn2(), cn2Order(env, s2, 29_000), 29_000)
	if _, err := env.Pool.Exec(t.Context(), `UPDATE invoices SET status = 'failed' WHERE id = $1`, a.Order.Invoice.ID); err != nil {
		t.Fatal(err)
	}

	type list struct {
		Items  []invoices.Invoice `json:"items"`
		Total  int                `json:"total"`
		Counts struct {
			IssuedToday, NotSent, Failed, Cancelled int
		} `json:"counts"`
	}
	var l list
	env.Ql1().Get("/api/v1/invoices?branchId=" + fmt.Sprint(env.Fx.Branches[2])).MustStatus(t, 200).JSON(t, &l)
	if l.Total != 2 || len(l.Items) != 2 || l.Counts.IssuedToday != 2 || l.Counts.Failed != 1 || l.Counts.NotSent != 1 {
		t.Fatalf("ql1 list: total %d counts %+v", l.Total, l.Counts)
	}
	for _, it := range l.Items {
		if it.BranchID != env.Fx.Branches[1] {
			t.Fatalf("ql1 sees branch %d", it.BranchID)
		}
	}
	env.Owner().Get("/api/v1/invoices").MustStatus(t, 200).JSON(t, &l)
	if l.Total != 3 || l.Counts.IssuedToday != 3 {
		t.Fatalf("owner list: %d %+v", l.Total, l.Counts)
	}
	env.Owner().Get(fmt.Sprintf("/api/v1/invoices?branchId=%d", env.Fx.Branches[2])).MustStatus(t, 200).JSON(t, &l)
	if l.Total != 1 || l.Items[0].ID != b.Order.Invoice.ID {
		t.Fatalf("owner CN2 list: %+v", l)
	}
	env.Owner().Get("/api/v1/invoices?status=failed").MustStatus(t, 200).JSON(t, &l)
	if l.Total != 1 || l.Items[0].ID != a.Order.Invoice.ID || l.Items[0].Status != "failed" {
		t.Fatalf("failed filter: %+v", l)
	}
	env.Owner().Get("/api/v1/invoices?date=2026-11-19&pageSize=1").MustStatus(t, 200).JSON(t, &l)
	if l.Total != 3 || len(l.Items) != 1 {
		t.Fatalf("date filter: %d items, total %d", len(l.Items), l.Total)
	}
	env.Owner().Get("/api/v1/invoices?date=2026-11-20").MustStatus(t, 200).JSON(t, &l)
	if l.Total != 0 || l.Items == nil {
		t.Fatalf("next day: %+v", l)
	}
	env.Owner().Get("/api/v1/invoices?status=bogus").MustStatus(t, 400)
	tn1.Get("/api/v1/invoices").MustStatus(t, 403) // cashier has no invoice list

	// resend a failed invoice: back to not_sent with a fresh job
	env.Ql1().Post(fmt.Sprintf("/api/v1/invoices/%d/resend", a.Order.Invoice.ID), nil).MustStatus(t, 202)
	var status string
	var attempts int
	var jobStatus string
	_ = env.Pool.QueryRow(t.Context(), `SELECT i.status, j.status, j.attempts FROM invoices i JOIN jobs j ON j.dedupe_key = 'invoice.submit:' || i.id WHERE i.id = $1`, a.Order.Invoice.ID).Scan(&status, &jobStatus, &attempts)
	if status != "not_sent" || jobStatus != "queued" || attempts != 0 {
		t.Fatalf("after resend: %s %s %d", status, jobStatus, attempts)
	}
	// a sent invoice cannot be resent; CN2's invoice is hidden from ql1
	if _, err := env.Pool.Exec(t.Context(), `UPDATE invoices SET status = 'sent' WHERE id = $1`, a.Order.Invoice.ID); err != nil {
		t.Fatal(err)
	}
	env.Ql1().Post(fmt.Sprintf("/api/v1/invoices/%d/resend", a.Order.Invoice.ID), nil).MustStatus(t, 409)
	env.Ql1().Post(fmt.Sprintf("/api/v1/invoices/%d/resend", b.Order.Invoice.ID), nil).MustStatus(t, 404)

	// email the invoice
	env.Ql1().Post(fmt.Sprintf("/api/v1/invoices/%d/email", a.Order.Invoice.ID), map[string]any{"email": "khach@example.com"}).MustStatus(t, 202)
	env.Ql1().Post(fmt.Sprintf("/api/v1/invoices/%d/email", a.Order.Invoice.ID), map[string]any{"email": "not-an-email"}).MustStatus(t, 400)
	var n int
	_ = env.Pool.QueryRow(t.Context(), `SELECT count(*) FROM jobs WHERE kind = 'invoice.email' AND payload->>'email' = 'khach@example.com'`).Scan(&n)
	if n != 1 {
		t.Fatalf("email jobs = %d", n)
	}
}

func TestE10_ConcurrentNumbering(t *testing.T) {
	env := testutil.NewEnv(t)
	shift := env.OpenShiftCN1()
	const n = 50
	ids := make([]int64, n)
	clients := make([]*testutil.Client, n)
	for i := range ids {
		ids[i] = env.InsertOrder(testutil.OrderSpec{BranchID: env.Fx.Branches[1], ShiftID: shift, CreatedBy: env.Fx.Users.Tn1,
			Items: []testutil.ItemSpec{{ProductID: env.Fx.Products.CaPheDen, SizeCode: "M", UnitPrice: 25_000, Qty: 1}}})
		clients[i] = env.Tn1()
	}
	s2 := env.OpenShift(env.Fx.Branches[2], env.Fx.DeviceCN2, env.Fx.Users.Tn2, 0)
	cn2 := []int64{cn2Order(env, s2, 29_000), cn2Order(env, s2, 29_000)}
	tn2 := env.Tn2()

	var wg sync.WaitGroup
	statuses := make([]int, n)
	for i := range ids {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			statuses[i] = clients[i].Post(fmt.Sprintf("/api/v1/orders/%d/payments", ids[i]), map[string]any{"method": "cash", "amount": 25_000, "cashReceived": 25_000}).Status
		}(i)
	}
	for _, id := range cn2 {
		payCash(t, tn2, id, 29_000)
	}
	wg.Wait()
	for i, s := range statuses {
		if s != 201 {
			t.Fatalf("payment %d status %d", i, s)
		}
	}
	rows, err := env.Pool.Query(t.Context(), `SELECT number FROM invoices WHERE branch_id = $1 ORDER BY number`, env.Fx.Branches[1])
	if err != nil {
		t.Fatal(err)
	}
	var nums []int64
	for rows.Next() {
		var v int64
		_ = rows.Scan(&v)
		nums = append(nums, v)
	}
	rows.Close()
	if len(nums) != n {
		t.Fatalf("CN1 invoices = %d", len(nums))
	}
	for i, v := range nums {
		if v != int64(i+1) {
			t.Fatalf("CN1 numbers not consecutive: %v", nums)
		}
	}
	var next int64
	_ = env.Pool.QueryRow(t.Context(), `SELECT next_number FROM invoice_series WHERE branch_id = $1`, env.Fx.Branches[1]).Scan(&next)
	if next != n+1 {
		t.Fatalf("next_number = %d", next)
	}
	var cn2nums []int64
	rows, _ = env.Pool.Query(t.Context(), `SELECT number FROM invoices WHERE branch_id = $1`, env.Fx.Branches[2])
	for rows.Next() {
		var v int64
		_ = rows.Scan(&v)
		cn2nums = append(cn2nums, v)
	}
	rows.Close()
	sort.Slice(cn2nums, func(i, j int) bool { return cn2nums[i] < cn2nums[j] })
	if fmt.Sprint(cn2nums) != "[1 2]" {
		t.Fatalf("CN2 numbers = %v", cn2nums)
	}
}

var codeRe = regexp.MustCompile(`^[A-HJ-NP-Z2-9]{4}-[A-HJ-NP-Z2-9]{4}$`)

func TestE11_PublicLookup(t *testing.T) {
	env := testutil.NewEnv(t)
	shift := env.OpenShiftCN1()
	p1 := payCash(t, env.Tn1(), env.InsertBan5Order(shift), 128_000)
	p2 := payCash(t, env.Tn1(), env.InsertBan5Order(shift), 128_000)
	c1, c2 := p1.Order.Invoice.LookupCode, p2.Order.Invoice.LookupCode
	if !codeRe.MatchString(c1) || !codeRe.MatchString(c2) || c1 == c2 {
		t.Fatalf("codes %q %q", c1, c2)
	}
	anon := env.Anon()
	var inv invoices.Invoice
	anon.Get("/api/v1/public/invoices/" + c1).MustStatus(t, 200).JSON(t, &inv)
	if inv.ID != p1.Order.Invoice.ID || inv.Total != 128_000 || len(inv.Lines) != 2 || inv.TotalInWords == "" {
		t.Fatalf("public invoice: %+v", inv)
	}
	// lower case and without the dash also work
	loose := regexp.MustCompile("-").ReplaceAllString(c2, "")
	anon.Get("/api/v1/public/invoices/" + toLower(loose)).MustStatus(t, 200).JSON(t, &inv)
	if inv.ID != p2.Order.Invoice.ID {
		t.Fatal("normalised lookup failed")
	}
	if code := anon.Get("/api/v1/public/invoices/AAAA-AAAA").MustStatus(t, 404).ErrCode(); code != "INVOICE_NOT_FOUND" {
		t.Fatalf("code = %s", code)
	}
	anon.Get(fmt.Sprintf("/api/v1/invoices/%d", inv.ID)).MustStatus(t, 401)
	// many codes: all distinct and well-formed
	seen := map[string]bool{}
	for i := 0; i < 2000; i++ {
		c := invoices.NewLookupCode()
		if !codeRe.MatchString(c) || seen[c] {
			t.Fatalf("bad or duplicate code %q", c)
		}
		seen[c] = true
	}
}

func toLower(s string) string {
	b := []byte(s)
	for i, c := range b {
		if c >= 'A' && c <= 'Z' {
			b[i] = c + 32
		}
	}
	return string(b)
}

func TestLineNameAndMethodText(t *testing.T) {
	if got := invoices.LineName("Cà phê sữa", "L", []string{"Thêm shot", "Ít đá"}); got != "Cà phê sữa (L, Thêm shot, Ít đá)" {
		t.Fatal(got)
	}
	if got := invoices.LineName("Croissant bơ", "", nil); got != "Croissant bơ" {
		t.Fatal(got)
	}
	if invoices.PaymentMethodText(1, 0) != "Tiền mặt" || invoices.PaymentMethodText(0, 1) != "Chuyển khoản" || invoices.PaymentMethodText(1, 1) != "Tiền mặt/Chuyển khoản" {
		t.Fatal("payment method text")
	}
}
