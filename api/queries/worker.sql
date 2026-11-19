-- Background worker: job queue and tax submissions.

-- name: WorkerClaimJob :one
SELECT * FROM jobs
WHERE status = 'queued' AND run_at <= $1
ORDER BY run_at, id
LIMIT 1
FOR UPDATE SKIP LOCKED;

-- name: WorkerUpdateJob :exec
UPDATE jobs SET status = $2, attempts = $3, run_at = $4, last_error = $5, updated_at = now() WHERE id = $1;

-- name: WorkerInsertTaxSubmission :exec
INSERT INTO tax_submissions (invoice_id, attempt, gateway, success, response, error, created_at)
VALUES ($1, $2, $3, $4, $5, $6, $7);

-- name: WorkerEnsureSubmitJobsForNotSent :exec
-- When the tax switch is on: every not_sent invoice gets a queued submit job, due now.
INSERT INTO jobs (kind, payload, dedupe_key, run_at, max_attempts)
SELECT 'invoice.submit', jsonb_build_object('invoiceId', i.id), 'invoice.submit:' || i.id, sqlc.arg(now)::timestamptz, 10
FROM invoices i WHERE i.status = 'not_sent'
ON CONFLICT (dedupe_key) DO UPDATE SET run_at = LEAST(jobs.run_at, EXCLUDED.run_at), updated_at = now()
WHERE jobs.status = 'queued';
