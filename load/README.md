# Load test N-01 (k6)

`load/k6/pos.js` simulates **50 POS tablets** selling at the same time: each virtual user is one
activated device with a cashier logged in by PIN and an open shift, and loops
*new takeaway order → add Cà phê sữa (L + Thêm shot) → add Croissant bơ → pay cash*,
with 0.5–1.5 s of "cashier think time" between taps.

Pass criteria (docs/test-cases/phase-1-pos.md, N-01; docs/ARCHITECTURE.md §13):

| Threshold | Meaning |
|---|---|
| `http_req_duration{flow:sale}` p(95) < 200 ms | 95% of sale requests finish under 200 ms |
| `http_req_failed{flow:sale}` rate == 0 | no failed request |
| `checks` rate == 1 | every order priced right (43.000đ line), paid, invoice issued |

Setup and teardown requests (logins hash with bcrypt on purpose) are tagged separately and do not count.

## Run

**Never against production.** Setup activates 50 extra POS devices and creates real orders and invoices.
Use a local stack or a scratch copy of the VM.

1. Install k6: https://grafana.com/docs/k6/latest/set-up/install-k6/ (or `docker run --rm -i --network host grafana/k6 run - <load/k6/pos.js`).
2. Start a seeded API with plain-HTTP cookies, e.g. the e2e stack:
   ```bash
   api/scripts/test-db.sh start
   E2E_DB_NAME=mocpos_load web/e2e/support/start-stack.sh      # API on :8080, COOKIE_SECURE=false
   ```
   (Or `docker compose -f docker-compose.yml -f docker-compose.dev.yml up` then `… run --rm api seed`.)
3. Run:
   ```bash
   k6 run load/k6/pos.js                                   # 50 VUs, 3 minutes, against :8080
   k6 run -e VUS=50 -e DURATION=10m -e BASE_URL=http://localhost:8080 load/k6/pos.js
   ```

| Variable | Default | |
|---|---|---|
| `BASE_URL` | `http://localhost:8080` | API (direct) or `http://localhost:3000` (through Next.js) |
| `ORIGIN` | `http://localhost:3000` | sent as `Origin`; must be in the API's `ALLOWED_ORIGINS` |
| `VUS` / `DURATION` | `50` / `3m` | |
| `THINK_MIN` / `THINK_MAX` | `0.5` / `1.5` | seconds between actions |
| `PASSWORD` | `matkhau123` | seed password of ql1/ql2 |

k6 exits non-zero when a threshold fails, so the command can gate a release.

## Notes
- Cookies are `Secure` when `COOKIE_SECURE=true` (production default); over plain HTTP k6 would not
  send them, so test with `COOKIE_SECURE=false` or over HTTPS.
- The VM target is Oracle Always Free ARM (2 OCPU / 12 GB). To measure that box, run k6 from another
  machine against a staging copy restored from backup (`deploy/backup/restore.sh`), not from the VM itself.
- If a run is interrupted mid-order, teardown may fail to close a shift (open orders). Recreate the
  scratch database before the next run.
