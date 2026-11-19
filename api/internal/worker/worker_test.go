package worker_test

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/ndhuy-sg/first-claude-app/api/internal/taxgateway"
	"github.com/ndhuy-sg/first-claude-app/api/internal/testutil"
	"github.com/ndhuy-sg/first-claude-app/api/internal/worker"
)

type recMailer struct {
	mu   sync.Mutex
	sent []worker.Email
	fail bool
}

func (m *recMailer) Send(_ context.Context, e worker.Email) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.fail {
		return errors.New("smtp down")
	}
	m.sent = append(m.sent, e)
	return nil
}

// paidInvoice pays the Bàn 5 order in cash and returns the invoice id.
func paidInvoice(t *testing.T, env *testutil.Env, shift int64) int64 {
	t.Helper()
	oid := env.InsertBan5Order(shift)
	var res struct {
		Order struct {
			Invoice struct{ ID int64 } `json:"invoice"`
		} `json:"order"`
	}
	env.Tn1().Post(fmt.Sprintf("/api/v1/orders/%d/payments", oid), map[string]any{"method": "cash", "amount": 128_000, "cashReceived": 128_000}).MustStatus(t, 201).JSON(t, &res)
	return res.Order.Invoice.ID
}

func enableTax(t *testing.T, env *testutil.Env) {
	t.Helper()
	if _, err := env.Pool.Exec(t.Context(), `UPDATE app_settings SET tax_submission_enabled = true, tax_gateway = 'fake'`); err != nil {
		t.Fatal(err)
	}
}

func invoiceState(t *testing.T, env *testutil.Env, id int64) (status string, sentAt *time.Time, submissions int) {
	t.Helper()
	if err := env.Pool.QueryRow(t.Context(), `SELECT status, sent_at, (SELECT count(*) FROM tax_submissions WHERE invoice_id = $1) FROM invoices WHERE id = $1`, id).Scan(&status, &sentAt, &submissions); err != nil {
		t.Fatal(err)
	}
	return
}

func newWorker(env *testutil.Env, gw taxgateway.Gateway, m worker.Mailer) *worker.Worker {
	return worker.New(env.Deps, worker.Options{
		Gateway:       func(string) taxgateway.Gateway { return gw },
		Mailer:        m,
		SubmitTimeout: 50 * time.Millisecond,
	})
}

func TestE13_SwitchOffKeepsNotSentThenSendsWhenOn(t *testing.T) {
	env := testutil.NewEnv(t)
	shift := env.OpenShiftCN1()
	inv := paidInvoice(t, env, shift)
	fake := taxgateway.NewFake()
	w := newWorker(env, fake, &recMailer{})
	now := env.Clock.Now()
	for i := 0; i < 3; i++ {
		if _, err := w.RunOnce(t.Context(), now.Add(time.Duration(i)*2*time.Hour)); err != nil {
			t.Fatal(err)
		}
	}
	status, _, subs := invoiceState(t, env, inv)
	if status != "not_sent" || subs != 0 || fake.Calls != 0 {
		t.Fatalf("switch off: status %s, submissions %d, calls %d", status, subs, fake.Calls)
	}
	// still visible to the customer
	var code string
	_ = env.Pool.QueryRow(t.Context(), `SELECT lookup_code FROM invoices WHERE id = $1`, inv).Scan(&code)
	env.Anon().Get("/api/v1/public/invoices/"+code).MustStatus(t, 200)

	// an invoice whose job was lost is also picked up once the switch is on
	inv2 := paidInvoice(t, env, shift)
	if _, err := env.Pool.Exec(t.Context(), `DELETE FROM jobs WHERE dedupe_key = $1`, fmt.Sprintf("invoice.submit:%d", inv2)); err != nil {
		t.Fatal(err)
	}
	enableTax(t, env)
	if _, err := w.RunOnce(t.Context(), now.Add(6*time.Hour+time.Minute)); err != nil {
		t.Fatal(err)
	}
	for _, id := range []int64{inv, inv2} {
		if status, sentAt, subs := invoiceState(t, env, id); status != "sent" || sentAt == nil || subs != 1 {
			t.Fatalf("invoice %d after switch on: %s %v %d", id, status, sentAt, subs)
		}
	}
}

func TestE03_AcceptedInvoiceIsSent(t *testing.T) {
	env := testutil.NewEnv(t)
	enableTax(t, env)
	inv := paidInvoice(t, env, env.OpenShiftCN1())
	fake := taxgateway.NewFake()
	n, err := newWorker(env, fake, &recMailer{}).RunOnce(t.Context(), env.Clock.Now())
	if err != nil || n != 1 {
		t.Fatalf("processed %d, err %v", n, err)
	}
	status, sentAt, subs := invoiceState(t, env, inv)
	if status != "sent" || sentAt == nil || subs != 1 || fake.Accepted() != 1 {
		t.Fatalf("status %s sentAt %v subs %d", status, sentAt, subs)
	}
	var ok bool
	var gateway string
	_ = env.Pool.QueryRow(t.Context(), `SELECT success, gateway FROM tax_submissions WHERE invoice_id = $1`, inv).Scan(&ok, &gateway)
	if !ok || gateway != "fake" {
		t.Fatalf("submission: %v %s", ok, gateway)
	}
	// the payload carries the branch's data (E-02)
	x := fake.Received[fmt.Sprintf("MOC-INV-%d", inv)]
	for _, want := range []string{"<KHHDon>C26MAA</KHHDon>", "<MST>0100000000</MST>", "<Ten>Khách lẻ</Ten>", "<THHDVu>Cà phê sữa (L)</THHDVu>", "<TgTTTBSo>128000</TgTTTBSo>", "<TgTThue>9481</TgTThue>", "<HTTToan>Tiền mặt</HTTToan>"} {
		if !contains(string(x), want) {
			t.Fatalf("payload lacks %s:\n%s", want, x)
		}
	}
	// the API shows the new status
	var got struct {
		Status string     `json:"status"`
		SentAt *time.Time `json:"sentAt"`
	}
	env.Ql1().Get(fmt.Sprintf("/api/v1/invoices/%d", inv)).MustStatus(t, 200).JSON(t, &got)
	if got.Status != "sent" || got.SentAt == nil {
		t.Fatalf("api: %+v", got)
	}
}

func contains(s, sub string) bool { return strings.Contains(s, sub) }

func TestE04_RetriesWithBackoffThenFails(t *testing.T) {
	env := testutil.NewEnv(t)
	enableTax(t, env)
	shift := env.OpenShiftCN1()
	inv := paidInvoice(t, env, shift)
	fake := taxgateway.NewFake()
	fake.SetMode(taxgateway.FakeTimeout) // first: no answer
	w := newWorker(env, fake, &recMailer{})
	now := env.Clock.Now()
	waits := []time.Duration{time.Minute, 5 * time.Minute, 15 * time.Minute, time.Hour, time.Hour, time.Hour, time.Hour, time.Hour, time.Hour}
	for attempt := 1; attempt <= 10; attempt++ {
		if attempt == 2 {
			fake.SetMode(taxgateway.FakeError)
		}
		if n, err := w.RunOnce(t.Context(), now); err != nil || n != 1 {
			t.Fatalf("attempt %d: processed %d, err %v", attempt, n, err)
		}
		var runAt time.Time
		var jobStatus string
		_ = env.Pool.QueryRow(t.Context(), `SELECT run_at, status FROM jobs WHERE dedupe_key = $1`, fmt.Sprintf("invoice.submit:%d", inv)).Scan(&runAt, &jobStatus)
		if attempt < 10 {
			if jobStatus != "queued" || !runAt.Equal(now.Add(waits[attempt-1])) {
				t.Fatalf("attempt %d: job %s run_at %v, want +%v", attempt, jobStatus, runAt, waits[attempt-1])
			}
			// nothing happens before the wait is over
			if n, _ := w.RunOnce(t.Context(), runAt.Add(-time.Second)); n != 0 {
				t.Fatalf("attempt %d: job ran early", attempt)
			}
			if st, _, _ := invoiceState(t, env, inv); st != "sending" {
				t.Fatalf("attempt %d: invoice %s", attempt, st)
			}
			now = runAt
		} else if jobStatus != "failed" {
			t.Fatalf("job after 10 attempts: %s", jobStatus)
		}
	}
	status, sentAt, subs := invoiceState(t, env, inv)
	if status != "failed" || sentAt != nil || subs != 10 {
		t.Fatalf("final: %s %v %d", status, sentAt, subs)
	}
	// selling still works while the tax authority is down
	paidInvoice(t, env, shift)
	// shown to the admin as failed, and "Gửi lại" works
	var l struct {
		Counts struct {
			Failed int `json:"failed"`
		} `json:"counts"`
	}
	env.Owner().Get("/api/v1/invoices").MustStatus(t, 200).JSON(t, &l)
	if l.Counts.Failed != 1 {
		t.Fatalf("failed count %d", l.Counts.Failed)
	}
	env.Owner().Post(fmt.Sprintf("/api/v1/invoices/%d/resend", inv), nil).MustStatus(t, 202)
	fake.SetMode(taxgateway.FakeSuccess)
	if _, err := w.RunOnce(t.Context(), now.Add(time.Minute)); err != nil {
		t.Fatal(err)
	}
	if status, _, subs := invoiceState(t, env, inv); status != "sent" || subs != 11 {
		t.Fatalf("after resend: %s %d", status, subs)
	}
}

// flaky accepts the invoice at the tax authority but loses the answer the first time.
type flaky struct {
	*taxgateway.Fake
	mu     sync.Mutex
	failed bool
}

func (f *flaky) Submit(ctx context.Context, inv taxgateway.Invoice) (taxgateway.Result, error) {
	res, err := f.Fake.Submit(ctx, inv)
	f.mu.Lock()
	defer f.mu.Unlock()
	if !f.failed {
		f.failed = true
		return taxgateway.Result{}, errors.New("connection reset")
	}
	return res, err
}

func TestE05_RetryNeverDuplicatesInvoice(t *testing.T) {
	env := testutil.NewEnv(t)
	enableTax(t, env)
	inv := paidInvoice(t, env, env.OpenShiftCN1())
	gw := &flaky{Fake: taxgateway.NewFake()}
	w := newWorker(env, gw, &recMailer{})
	now := env.Clock.Now()
	if _, err := w.RunOnce(t.Context(), now); err != nil {
		t.Fatal(err)
	}
	if _, err := w.RunOnce(t.Context(), now.Add(time.Minute)); err != nil {
		t.Fatal(err)
	}
	if gw.Calls != 2 || gw.Accepted() != 1 {
		t.Fatalf("calls %d, accepted %d", gw.Calls, gw.Accepted())
	}
	status, _, subs := invoiceState(t, env, inv)
	if status != "sent" || subs != 2 {
		t.Fatalf("invoice %s, submissions %d", status, subs)
	}
	var invoicesForOrder int
	_ = env.Pool.QueryRow(t.Context(), `SELECT count(*) FROM invoices WHERE order_id = (SELECT order_id FROM invoices WHERE id = $1)`, inv).Scan(&invoicesForOrder)
	if invoicesForOrder != 1 {
		t.Fatalf("invoices for order = %d", invoicesForOrder)
	}
	// a resend of an already-sent invoice (job re-queued by hand) does not submit again
	if _, err := env.Pool.Exec(t.Context(), `UPDATE jobs SET status = 'queued', run_at = $1 WHERE dedupe_key = $2`, now, fmt.Sprintf("invoice.submit:%d", inv)); err != nil {
		t.Fatal(err)
	}
	if _, err := w.RunOnce(t.Context(), now.Add(2*time.Minute)); err != nil {
		t.Fatal(err)
	}
	if gw.Calls != 2 {
		t.Fatalf("sent again: calls %d", gw.Calls)
	}
}

func TestE08_CancelInvoiceIsSubmitted(t *testing.T) {
	env := testutil.NewEnv(t)
	enableTax(t, env)
	shift := env.OpenShiftCN1()
	inv := paidInvoice(t, env, shift)
	var orderID int64
	_ = env.Pool.QueryRow(t.Context(), `SELECT order_id FROM invoices WHERE id = $1`, inv).Scan(&orderID)
	env.Ql1().Post(fmt.Sprintf("/api/v1/orders/%d/refund", orderID), map[string]any{"reason": "Khách trả lại"}).MustStatus(t, 200)
	fake := taxgateway.NewFake()
	if n, err := newWorker(env, fake, &recMailer{}).RunOnce(t.Context(), env.Clock.Now()); err != nil || n != 2 {
		t.Fatalf("processed %d, err %v", n, err)
	}
	var cancelID int64
	var status string
	_ = env.Pool.QueryRow(t.Context(), `SELECT id, status FROM invoices WHERE original_invoice_id = $1 AND kind = 'cancel'`, inv).Scan(&cancelID, &status)
	if status != "sent" {
		t.Fatalf("cancel invoice status %s", status)
	}
	x := string(fake.Received[fmt.Sprintf("MOC-INV-%d", cancelID)])
	if !contains(x, "<TCHDon>Hủy</TCHDon>") || !contains(x, "<SHDCLQuan>1</SHDCLQuan>") {
		t.Fatalf("cancel payload:\n%s", x)
	}
	var got struct {
		Cancelled bool `json:"cancelled"`
	}
	env.Ql1().Get(fmt.Sprintf("/api/v1/invoices/%d", inv)).MustStatus(t, 200).JSON(t, &got)
	if !got.Cancelled {
		t.Fatal("original invoice not marked cancelled")
	}
}

func TestInvoiceEmailJob(t *testing.T) {
	env := testutil.NewEnv(t)
	inv := paidInvoice(t, env, env.OpenShiftCN1())
	env.Ql1().Post(fmt.Sprintf("/api/v1/invoices/%d/email", inv), map[string]any{"email": "khach@example.com"}).MustStatus(t, 202)
	m := &recMailer{fail: true}
	w := newWorker(env, taxgateway.NewFake(), m)
	now := env.Clock.Now()
	if _, err := w.RunOnce(t.Context(), now); err != nil {
		t.Fatal(err)
	}
	m.fail = false
	if _, err := w.RunOnce(t.Context(), now.Add(time.Minute)); err != nil {
		t.Fatal(err)
	}
	if len(m.sent) != 1 || m.sent[0].To != "khach@example.com" || !contains(m.sent[0].Body, "http://localhost:3000/tra-cuu/") {
		t.Fatalf("sent: %+v", m.sent)
	}
	var st string
	_ = env.Pool.QueryRow(t.Context(), `SELECT status FROM jobs WHERE kind = 'invoice.email'`).Scan(&st)
	if st != "done" {
		t.Fatalf("email job %s", st)
	}
}

func TestBackoff(t *testing.T) {
	want := []time.Duration{time.Minute, 5 * time.Minute, 15 * time.Minute, time.Hour, time.Hour, time.Hour}
	for i, w := range want {
		if got := worker.Backoff(i + 1); got != w {
			t.Errorf("Backoff(%d) = %v, want %v", i+1, got, w)
		}
	}
}
