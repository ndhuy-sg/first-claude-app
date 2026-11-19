# Developer guide (conventions for humans and agents)

Read this before writing code. The product spec is `docs/PLAN.md`, `docs/ARCHITECTURE.md`,
`docs/SCREENS.md` and the approved test cases in `docs/test-cases/phase-1-pos.md`.
The UI design is on the canvas linked from `docs/PLAN.md` (navy `#14213D`, yellow `#F5B700`, white).

## Layout
```
api/                     Go + Gin API (module github.com/ndhuy-sg/first-claude-app/api)
  cmd/mocpos/            main: serve | worker | migrate | seed
  migrations/            goose SQL migrations (embedded)
  queries/               sqlc queries, one file per module (core.sql = shared)
  openapi.yaml           THE API contract (source of truth for web types)
  internal/
    app/                 router wiring (one Register line per module)
    platform/            Deps (pool, config, clock, events), Audit(), EnqueueJob()
    auth/                session middleware, principal, RequireAuth/RequireAction, ResolveBranch/CheckBranch, idempotency
    domain/              pure business rules (roles matrix, pricing, VAT, ...) – no DB
    store/               sqlc generated code + InTx helper (do not edit generated files)
    apperr/, httpx/      error type, JSON helpers
    seed/, testutil/     demo data and test harness
    <module>/            feature packages: handler.go + service.go + tests
web/                     Next.js 16 + Tailwind 4 (App Router, TypeScript)
  lib/api/               typed client (openapi-fetch) + generated schema.d.ts
  lib/format.ts          formatVND / formatDate / formatTime (always use these)
  components/ui/         shared primitives: Button, Card, Chip, Field
  app/pos/               POS (tablet) screens
  app/admin/             admin screens
  app/tra-cuu/           public invoice lookup
```

## Commands
```bash
# one-time: local PostgreSQL for tests (Docker is not required)
api/scripts/test-db.sh start
export TEST_DATABASE_URL="postgres://postgres@localhost:54329/postgres?sslmode=disable"

cd api
go test ./...                                  # all Go tests
go test ./internal/orders -run TestAddItem -v  # one test
sqlc generate                                  # after editing queries/*.sql (binary: ~/go/bin/sqlc)
go vet ./... && gofmt -l .                     # must be clean

cd web
npm run gen:api       # after editing api/openapi.yaml
npm test              # vitest unit/component tests
npm run typecheck && npm run lint
npm run build
```

## Go rules
- **Module package** `internal/<module>` exposes `func Register(g *gin.RouterGroup, d *platform.Deps)`; add one line in `internal/app/router.go`.
- **Queries** go in `queries/<module>.sql`, then `sqlc generate`. Never hand-edit `internal/store/*.sql.go`.
- **Schema**: never edit `migrations/00001_init.sql`. If you truly need a change, add a new migration file and say so in your report.
- **Writes** run inside `store.InTx`. Read the row you change with `SELECT ... FOR UPDATE` when concurrent requests could race (orders, invoice numbers, shifts).
- **Money** is `int64` VND, never float. Use `domain` helpers for prices, discounts, VAT, change.
- **Time**: use `d.Clock.Now()`, never `time.Now()` in business code (tests freeze time). Business dates via `domain.BusinessDate` (Asia/Ho_Chi_Minh).
- **Access**: protect routes with `auth.RequireAuth()` / `auth.RequireAction(domain.ActX)`. Pick the branch with
  `auth.ResolveBranch` (branch-bound users always get their own branch, A-12) and guard loaded rows with
  `auth.CheckBranch` (returns 404 for other branches, A-10).
- **Errors**: return `*apperr.Error` with a stable UPPER_SNAKE code and a Vietnamese message; render with `httpx.Fail`.
  Codes used by the contract are listed in `openapi.yaml` (e.g. ORDER_NOT_OPEN, TABLE_OCCUPIED, PRODUCT_SOLD_OUT).
- **Audit**: discount, cancel, refund, price change, sold-out, user lock, settings change → `platform.Audit(ctx, q, ...)` in the same transaction.
- **Events**: after commit, `d.Events.Publish(platform.Event{Type: "order.updated", BranchID: ..., Data: ...})`.
- **JSON**: camelCase field names exactly as in `openapi.yaml`. Nullable fields are JSON `null`, lists are `[]` not `null`.
- **Tests**: write the test first, named after the test-case ID (`TestO03_ServerIgnoresClientPrice`).
  HTTP tests live in the external package (`package orders_test`) and use `testutil.NewEnv(t)`:
  `env.Tn1()` (cashier CN1 on iPad CN1), `env.Tn2()`, `env.Ql1()`, `env.Owner()`, `env.As(userID)`, `env.Anon()`.
  Fixtures: `env.Fx.Products.CaPheSua`, `env.Fx.TablesCN1[5]`, `env.Fx.Modifiers.ThemShot`, … (see `internal/seed`).
  The clock is `env.Clock` (19/11/2026 07:00 VN). Each test gets its own database, so tests may run in parallel.

## Web rules
- Next.js 16 has breaking changes: read `web/node_modules/next/dist/docs/` before using an API you are unsure of.
- Call the API only through `api` from `@/lib/api/client` (typed). Wrap with TanStack Query. Use `unwrap()`.
- Send `Idempotency-Key: newIdempotencyKey()` on payment and other money writes; reuse the key on retry.
- Use the design tokens from `app/globals.css` (`bg-navy`, `bg-gold`, `text-muted`, `border-line`, …) and `components/ui/*`.
- POS screens are designed for 1180×820 and 1280×800 landscape tablets; touch targets ≥ 44px (POS buttons usually 56–64px).
- Every user-visible text is Vietnamese. Money via `formatVND`, dates via `formatDate`/`formatTime`.
- Accessibility: real `<button>`/`<a>`/`<input>` with labels, `aria-pressed` for toggles, never color alone for status.
- Tests: Vitest + Testing Library next to the component (`*.test.tsx`), mocking `fetch`. Playwright e2e lives in `web/e2e/`.

## Seed accounts (dev and tests)
| Key | Login | Role | Branch |
|---|---|---|---|
| owner | owner@moc.vn / matkhau123 | Chủ chuỗi | – |
| kho | kho@moc.vn / matkhau123 | Quản lý kho tổng | – |
| ql1 | ql1@moc.vn / matkhau123, PIN 9999 | Quản lý chi nhánh | CN1 |
| tn1 | PIN 1234 | Thu ngân | CN1 |
| pc1 | PIN 4321 | Pha chế | CN1 |
| ql2 | ql2@moc.vn / matkhau123, PIN 8888 | Quản lý chi nhánh | CN2 |
| tn2 | PIN 5678 | Thu ngân | CN2 |

## Merge rule (owner decision)
Nothing is merged into `claude/init-t81sv3` unless, after the merge, ALL checks pass on the merged tree:
`cd api && go vet ./... && test -z "$(gofmt -l .)" && go test ./...` and
`cd web && npm run lint && npm run typecheck && npm test && npm run build`.
A branch whose tests fail is sent back to its author to fix; it is never merged "to fix later".
