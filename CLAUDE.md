# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project status

Phase 1 (POS + self-issued e-invoices) is being implemented. Read in this order:
`docs/DEV.md` (layout, commands, coding rules), `docs/PLAN.md` (approved decisions, Vietnamese),
`docs/ARCHITECTURE.md`, `docs/SCREENS.md`, `docs/test-cases/phase-1-pos.md` (approved test cases; write tests first, named by test ID).
`api/openapi.yaml` is the API contract shared by the Go API and the Next.js app.

## Commands
```bash
api/scripts/test-db.sh start                    # local PostgreSQL for tests (port 54329)
export TEST_DATABASE_URL="postgres://postgres@localhost:54329/postgres?sslmode=disable"
cd api && go test ./...                         # Go tests; single: go test ./internal/orders -run TestO03 -v
cd api && ~/go/bin/sqlc generate                # after editing api/queries/*.sql
cd web && npm test                              # Vitest; single: npx vitest run lib/format.test.ts
cd web && npm run gen:api                       # after editing api/openapi.yaml
cd web && npm run lint && npm run typecheck && npm run build
```

## Architecture in one paragraph
Tablets (PWA) and admin browsers talk to Next.js (`web/`, UI only); `/api/*` goes to the Go API (`api/`, Gin) which owns
every business rule and talks to PostgreSQL via sqlc. Feature modules live in `api/internal/<module>` and register routes in
`internal/app/router.go`. Cross-cutting pieces: `auth` (cookie sessions, branch scoping, RBAC matrix in `domain/roles.go`),
`platform` (deps, audit log, job queue, SSE events), `worker` (jobs: invoice submission to the tax authority, off by default).
Payment, invoice numbering and job enqueueing happen in one DB transaction.

## Product and workflow
A POS system (no public website) for a 5-branch café chain with a central warehouse, used on iPad/Android tablets.
E-invoices are issued by the system itself (no invoice vendor). The UI/UX design and architecture diagrams are on a design
canvas linked from `docs/PLAN.md`. Workflow: plan → design → test cases → owner approval → write tests → implement until tests pass.
