.PHONY: help dev observability-up
.PHONY: test test-all test-property test-cover test-integration test-containers
.PHONY: lint lint-all check check-fast ci
.PHONY: build run migrate seed bench audit clean generate simplify deadcode
.PHONY: e2e sync-alert-rules check-repo-layout security-check check-protocol-sync

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
	docker compose up -d postgres redis redis-ephemeral
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
	@command -v golangci-lint >/dev/null 2>&1 || go install github.com/golangci/golangci-lint/v2/cmd/golangci-lint@latest
	cd backend && golangci-lint run --allow-parallel-runners=false

lint-all: lint
	cd frontend && npm run lint
	cd frontend && npm run typecheck

check: lint-all test-cover check-protocol-sync

check-fast: lint-all test

ci: check test-containers audit check-repo-layout

check-repo-layout:
	go run scripts/ci/check-repo-layout.go

check-protocol-sync:
	cd backend && go generate ./internal/protocol/...
	@git diff --exit-code frontend/src/shared/game/constants.ts || (echo "generated protocol constants out of sync; run make generate" >&2; exit 1)

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

simplify:
	cd backend && gofmt -w .
	$(TOOL_BUILD)/goimports golang.org/x/tools/cmd/goimports
	cd backend && ./bin/goimports -w .
	cd backend && golangci-lint run --fix --allow-parallel-runners=false

clean:
	rm -rf backend/bin frontend/dist bin
	rm -f backend/*.out backend/*cov* backend/*cover*
	docker compose down -v

# sync-alert-rules: 生成 deploy/alertmanager/rules-configmap.yaml（v2-C-30/C-31/C-33）。
# 单一真相源 deploy/prometheus/alerts.yml → ConfigMap YAML，供 Prometheus StatefulSet
# 通过 configMap `alertmanager-rules` 挂载到 /etc/prometheus/rules/。
# 见 deploy/prometheus/deployment.yaml volume `rules` 与 deploy/kustomization.yaml configMapGenerator。
sync-alert-rules:
	@bash scripts/ci/sync-alert-rules.sh
	@echo "==> alertmanager rules ConfigMap synced to deploy/alertmanager/rules-configmap.yaml"
