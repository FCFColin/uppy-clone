.PHONY: help dev observability-up test test-all test-property test-cover test-integration test-containers lint lint-all build run migrate seed bench audit clean generate simplify deadcode check check-fast ci e2e sync-alert-rules check-repo-layout security-check

help:
	@echo "Targets:"
	@echo "  dev              Start postgres/redis + backend air + frontend"
	@echo "  observability-up Start Prometheus + Grafana + Alertmanager (profile observability)"
	@echo "  test             Backend unit tests (-race -short)"
	@echo "  test-all         Backend + integration + frontend tests"
	@echo "  test-property    Run property-based tests"
	@echo "  test-containers  testcontainers tests (no -short)"
	@echo "  test-cover       Backend coverage + frontend tests"
	@echo "  test-integration Integration tests (testcontainers)"
	@echo "  lint             Backend golangci-lint"
	@echo "  lint-all         Backend lint + frontend lint + typecheck"
	@echo "  check-fast       lint-all + unit tests (-short)"
	@echo "  check            lint-all + test-cover"
	@echo "  security-check   Layer-1 security self-check (see docs/security/self-check-checklist.md)"
	@echo "  ci               check + test-containers + audit"
	@echo "  e2e              Playwright E2E (tests/e2e)"
	@echo "  build run migrate seed bench audit clean generate simplify deadcode"

TOOL_BUILD = cd backend && go build -o bin

dev:
	docker compose up -d postgres redis
	$(TOOL_BUILD)/air github.com/air-verse/air
	cd backend && ./bin/air &
	cd frontend && npm run dev

observability-up:
	docker compose --profile observability up -d prometheus grafana alertmanager

test:
	cd backend && go test ./... -race -short -timeout 60s

test-integration:
	cd backend && go test -tags=integration ./tests/integration/... -timeout 180s -v

test-containers:
	cd backend && go test -tags=integration ./tests/integration/... ./internal/outbox/... ./internal/worker/... -timeout 180s

test-all: test test-integration
	cd frontend && npm test

.PHONY: test-property
test-property:  ## Run property-based tests
	go test -short -count=1 -run "TestPhysics_|TestState_|TestProtocol_" ./internal/game/... ./internal/protocol/...
	cd frontend && npx vitest run src/**/*.property.test.ts

test-cover:
	cd backend && go test $$(go list ./internal/... | grep -v /internal/testutil | grep -v /internal/testsecrets) -short -p 1 -coverprofile=unit.out -covermode=atomic -timeout 180s
	cd backend && go test -tags=integration ./tests/integration/... ./internal/outbox/... ./internal/worker/... -p 1 -coverprofile=int.out -covermode=atomic -timeout 180s
	cd backend && go tool cover -func unit.out
	bash scripts/ci/check-coverage.sh unit backend/unit.out
	bash scripts/ci/check-coverage.sh integration backend/int.out
	cd frontend && npm run test:frontend
	bash scripts/ci/check-coverage.sh frontend

lint:
	cd backend && golangci-lint run --allow-parallel-runners=false

lint-all: lint
	cd frontend && npm run lint
	cd frontend && npm run typecheck

check: lint-all test-cover

check-fast: lint-all test

ci: check test-containers audit check-repo-layout

check-repo-layout:
	@bash scripts/ci/check-repo-layout.sh 2>/dev/null || powershell -NoProfile -ExecutionPolicy Bypass -File scripts/ci/check-repo-layout.ps1

security-check:
	@echo "==> detect-secrets (requires: pip install detect-secrets)"
	@detect-secrets scan --baseline .secrets.baseline
	@echo "==> docker digest check"
	@bash scripts/ci/check-docker-digests.sh Dockerfile
	@echo "==> repo layout"
	@$(MAKE) check-repo-layout
	@echo "==> backend lint"
	@$(MAKE) lint
	@echo "==> frontend npm audit"
	@cd frontend && npm audit --audit-level=high
	@echo "Security check complete. See docs/security/self-check-checklist.md for full layers."

e2e:
	npx playwright test

build:
	cd backend && go build -o bin/server ./cmd/server
	cd frontend && npm run build

run:
	cd backend && go run ./cmd/server

migrate:
	cd backend && go run ./cmd/server -migrate-only

seed:
	cd backend && go run ./cmd/seed

bench:
	cd backend && go test -bench=. -benchmem -run=^$$ ./internal/protocol/ ./internal/game/ -count=1 | tee ../docs/development/benchmarks-go-microbench.md

load-smoke:
	k6 run scripts/load/k6-smoke.js

load-ws-soak:
	k6 run scripts/load/k6-ws-soak.js

load-single-room:
	k6 run scripts/load/k6-single-room.js

audit:
	$(TOOL_BUILD)/govulncheck golang.org/x/vuln/cmd/govulncheck
	cd backend && ./bin/govulncheck ./...
	gitleaks detect --source . --report-path leaks.json
	trivy fs .

deadcode:
	$(TOOL_BUILD)/deadcode golang.org/x/tools/cmd/deadcode
	cd backend && ./bin/deadcode ./...

generate:
	go run scripts/codegen/generate_nicknames.go
	cd backend && go generate ./...

check-generated:
	go run scripts/codegen/generate_nicknames.go
	@git diff --exit-code backend/internal/nicknames/pools_gen.go frontend/src/shared/nickname_pools_gen.ts || (echo "generated nickname pools out of sync; run make generate" >&2; exit 1)

simplify:
	cd backend && gofmt -w .
	$(TOOL_BUILD)/goimports golang.org/x/tools/cmd/goimports
	cd backend && ./bin/goimports -w .
	cd backend && golangci-lint run --fix --allow-parallel-runners=false

clean:
	rm -rf backend/bin frontend/dist bin
	rm -f backend/*.out backend/*cov* backend/*cover*
	rm -f backend/migrate backend/seed backend/backfill backend/server backend/store backend/unit backend/handler backend/unit_focus 'backend/$$out'
	docker compose down -v
