// Package worker runs background jobs from the jobs table (FOR UPDATE SKIP LOCKED):
//
//   - invoice.submit: send an invoice to the tax authority through taxgateway.
//     While app_settings.tax_submission_enabled is OFF (default) nothing is sent:
//     the job is postponed and the invoice stays 'not_sent' (E-13). When the switch
//     is ON, every 'not_sent' invoice gets a due submit job (dedupe key
//     invoice.submit:<id>). Failures are retried after 1, 5, 15, 60 minutes, then
//     hourly; after max_attempts (10) the invoice becomes 'failed' (E-04). Every
//     attempt is recorded in tax_submissions. The same internal id is sent on
//     every attempt and an already 'sent' invoice is never sent again (E-05).
//   - invoice.email: email the invoice lookup link through a Mailer.
//
// A job is processed inside the transaction that claimed it, so a crash simply
// releases the row lock and the job is picked up again.
package worker

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/ndhuy-sg/first-claude-app/api/internal/invoices"
	"github.com/ndhuy-sg/first-claude-app/api/internal/orderview"
	"github.com/ndhuy-sg/first-claude-app/api/internal/platform"
	"github.com/ndhuy-sg/first-claude-app/api/internal/store"
	"github.com/ndhuy-sg/first-claude-app/api/internal/taxgateway"
)

// Email is one outgoing message.
type Email struct {
	From, To, Subject, Body string
}

// Mailer sends emails. The real SMTP/API sender is configured later.
type Mailer interface {
	Send(ctx context.Context, e Email) error
}

// LogMailer only logs the email (development, and until a mail provider is chosen).
type LogMailer struct{}

func (LogMailer) Send(ctx context.Context, e Email) error {
	slog.InfoContext(ctx, "email (log only)", "to", e.To, "subject", e.Subject)
	return nil
}

// SwitchOffDelay is how long a submit job waits while the tax switch is off.
// Turning the switch on pulls waiting jobs forward immediately.
const SwitchOffDelay = time.Hour

// Backoff returns the wait before the next try after `failures` failed attempts:
// 1, 5, 15, 60 minutes, then hourly.
func Backoff(failures int) time.Duration {
	steps := []time.Duration{time.Minute, 5 * time.Minute, 15 * time.Minute, time.Hour}
	if failures < 1 {
		return steps[0]
	}
	if failures > len(steps) {
		return time.Hour
	}
	return steps[failures-1]
}

// Options customise a Worker (tests inject a fake gateway and mailer).
type Options struct {
	// Gateway returns the transport for app_settings.tax_gateway. Default: taxgateway.ForName.
	Gateway       func(name string) taxgateway.Gateway
	Mailer        Mailer
	SubmitTimeout time.Duration // default 30s
	PollInterval  time.Duration // default 5s
}

type Worker struct {
	d   *platform.Deps
	opt Options
}

func New(d *platform.Deps, opt Options) *Worker {
	if opt.Gateway == nil {
		opt.Gateway = func(name string) taxgateway.Gateway { return taxgateway.ForName(name, nil) }
	}
	if opt.Mailer == nil {
		opt.Mailer = LogMailer{}
	}
	if opt.SubmitTimeout == 0 {
		opt.SubmitTimeout = 30 * time.Second
	}
	if opt.PollInterval == 0 {
		opt.PollInterval = 5 * time.Second
	}
	return &Worker{d: d, opt: opt}
}

// Run processes jobs until ctx is cancelled.
func Run(ctx context.Context, d *platform.Deps) error {
	return New(d, Options{}).Run(ctx)
}

func (w *Worker) Run(ctx context.Context) error {
	t := time.NewTicker(w.opt.PollInterval)
	defer t.Stop()
	for {
		if _, err := w.RunOnce(ctx, w.d.Clock.Now()); err != nil && ctx.Err() == nil {
			slog.ErrorContext(ctx, "worker", "err", err)
		}
		select {
		case <-ctx.Done():
			return nil
		case <-t.C:
		}
	}
}

// RunOnce processes every job due at now and returns how many it handled.
func RunOnce(ctx context.Context, d *platform.Deps, now time.Time) (int, error) {
	return New(d, Options{}).RunOnce(ctx, now)
}

func (w *Worker) RunOnce(ctx context.Context, now time.Time) (int, error) {
	q := store.New(w.d.Pool)
	st, err := q.GetSettings(ctx)
	if err != nil {
		return 0, err
	}
	if st.TaxSubmissionEnabled {
		if err := q.WorkerEnsureSubmitJobsForNotSent(ctx, now); err != nil {
			return 0, err
		}
	}
	n := 0
	for {
		did, err := w.processOne(ctx, now)
		if err != nil {
			return n, err
		}
		if !did {
			return n, nil
		}
		n++
	}
}

func (w *Worker) processOne(ctx context.Context, now time.Time) (bool, error) {
	did := false
	err := store.InTx(ctx, w.d.Pool, func(q *store.Queries, _ pgx.Tx) error {
		job, err := q.WorkerClaimJob(ctx, now)
		if errors.Is(err, pgx.ErrNoRows) {
			return nil
		}
		if err != nil {
			return err
		}
		did = true
		switch job.Kind {
		case invoices.JobSubmit:
			return w.submit(ctx, q, job, now)
		case invoices.JobEmail:
			return w.email(ctx, q, job, now)
		}
		return q.WorkerUpdateJob(ctx, store.WorkerUpdateJobParams{
			ID: job.ID, Status: "failed", Attempts: job.Attempts, RunAt: job.RunAt, LastError: "unknown job kind " + job.Kind,
		})
	})
	return did, err
}

func done(ctx context.Context, q *store.Queries, job store.Job, attempts int32) error {
	return q.WorkerUpdateJob(ctx, store.WorkerUpdateJobParams{ID: job.ID, Status: "done", Attempts: attempts, RunAt: job.RunAt})
}

// retry records a failed attempt; returns true when the job gave up.
func retry(ctx context.Context, q *store.Queries, job store.Job, attempts int32, now time.Time, cause error) (bool, error) {
	status, runAt := "queued", now.Add(Backoff(int(attempts)))
	if attempts >= job.MaxAttempts {
		status, runAt = "failed", job.RunAt
	}
	err := q.WorkerUpdateJob(ctx, store.WorkerUpdateJobParams{ID: job.ID, Status: status, Attempts: attempts, RunAt: runAt, LastError: cause.Error()})
	return status == "failed", err
}

func (w *Worker) submit(ctx context.Context, q *store.Queries, job store.Job, now time.Time) error {
	var p invoices.SubmitPayload
	if err := json.Unmarshal(job.Payload, &p); err != nil {
		return err
	}
	st, err := q.GetSettings(ctx)
	if err != nil {
		return err
	}
	if !st.TaxSubmissionEnabled {
		return q.WorkerUpdateJob(ctx, store.WorkerUpdateJobParams{
			ID: job.ID, Status: "queued", Attempts: job.Attempts, RunAt: now.Add(SwitchOffDelay),
			LastError: "Công tắc gửi dữ liệu tới cơ quan thuế đang tắt",
		})
	}
	inv, err := q.InvLockInvoice(ctx, p.InvoiceID)
	if err != nil {
		return err
	}
	if inv.Status == "sent" { // already accepted: never send twice (E-05)
		return done(ctx, q, job, job.Attempts)
	}
	payload, err := invoices.LoadPayload(ctx, q, inv.ID)
	if err != nil {
		return err
	}
	gw := w.opt.Gateway(st.TaxGateway)
	sctx, cancel := context.WithTimeout(ctx, w.opt.SubmitTimeout)
	res, subErr := gw.Submit(sctx, payload)
	cancel()
	attempt := job.Attempts + 1
	rec := store.WorkerInsertTaxSubmissionParams{
		InvoiceID: inv.ID, Attempt: attempt, Gateway: gw.Name(), Success: subErr == nil, Response: res.Response, CreatedAt: now,
	}
	if subErr != nil {
		rec.Error = subErr.Error()
	}
	if err := q.WorkerInsertTaxSubmission(ctx, rec); err != nil {
		return err
	}
	if subErr == nil {
		if err := q.InvMarkInvoiceSent(ctx, store.InvMarkInvoiceSentParams{ID: inv.ID, SentAt: &now}); err != nil {
			return err
		}
		return done(ctx, q, job, attempt)
	}
	gaveUp, err := retry(ctx, q, job, attempt, now, subErr)
	if err != nil {
		return err
	}
	status := "sending" // retrying
	if gaveUp {
		status = "failed"
		slog.WarnContext(ctx, "invoice submission failed", "invoice", inv.ID, "attempts", attempt, "err", subErr)
	}
	return q.InvSetInvoiceStatus(ctx, store.InvSetInvoiceStatusParams{ID: inv.ID, Status: status})
}

func (w *Worker) email(ctx context.Context, q *store.Queries, job store.Job, now time.Time) error {
	var p invoices.EmailPayload
	if err := json.Unmarshal(job.Payload, &p); err != nil {
		return err
	}
	inv, err := invoices.Load(ctx, q, w.d.Cfg.PublicBaseURL, p.InvoiceID)
	if err != nil {
		return err
	}
	st, err := q.GetSettings(ctx)
	if err != nil {
		return err
	}
	e := Email{
		From:    st.InvoiceEmailFrom,
		To:      p.Email,
		Subject: fmt.Sprintf("Hoá đơn điện tử %s-%s – %s", inv.Symbol, inv.Number, inv.Seller.Name),
		Body: fmt.Sprintf("Kính gửi quý khách,\n\n%s gửi quý khách hoá đơn điện tử số %s (ký hiệu %s), tổng tiền %d đồng (%s).\n"+
			"Xem hoá đơn tại: %s\nMã tra cứu: %s\n",
			inv.Seller.Name, inv.Number, inv.Symbol, inv.Total, inv.TotalInWords,
			orderview.LookupURL(w.d.Cfg.PublicBaseURL, inv.LookupCode), inv.LookupCode),
	}
	attempt := job.Attempts + 1
	if err := w.opt.Mailer.Send(ctx, e); err != nil {
		_, err2 := retry(ctx, q, job, attempt, now, err)
		return err2
	}
	return done(ctx, q, job, attempt)
}
