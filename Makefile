# Common tasks. See docs/DEV.md.
export TEST_DATABASE_URL ?= postgres://postgres@localhost:54329/postgres?sslmode=disable

.PHONY: test-db test test-api test-web gen lint
test-db:
	api/scripts/test-db.sh start
test-api:
	cd api && go test ./...
test-web:
	cd web && npm test
test: test-api test-web
gen:
	cd api && sqlc generate
	cd web && npm run gen:api
lint:
	cd api && go vet ./... && gofmt -l . | (! grep .)
	cd web && npm run lint && npm run typecheck
