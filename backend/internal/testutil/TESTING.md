# Test Boundary: Unit vs Integration (RO-037)

## Rule

| Aspect | Unit tests (`internal/**/*_test.go`) | Integration tests (`tests/integration/**`) |
|---|---|---|
| Build tag | **none** (default build) | `//go:build integration` |
| Docker / Postgres / Redis (testcontainers) | **MUST NOT** require | MAY require |
| miniredis (in-process Redis mock) | OK — no external dependency | OK |
| pgxmock / `newMockRepo` | OK — no external dependency | OK |
| Run command | `go test ./internal/...` | `go test -tags integration ./tests/...` |
| `-short` flag | Always runs | Skipped via `skipIfShort` |

## Naming convention

- Unit test files: `<feature>_test.go` (no build tag)
- Integration test files in `internal/`: `<feature>_integration_test.go` with `//go:build integration` tag
- Integration test files in `tests/integration/`: `<feature>_test.go` with `//go:build integration` tag

## Helpers (`testutil` package)

| Helper | External dep? | Usable in unit tests? |
|---|---|---|
| `SetupPostgres(t, opts...)` | Yes (testcontainers Postgres) | **No** — integration only |
| `SetupRedisStore(t)` | Yes (testcontainers Redis) | **No** — integration only |
| `SetupRedisClient(t)` | Yes (testcontainers Redis) | **No** — integration only |
| `SetupMiniredisStore(t)` | No (in-process) | **Yes** — unit-safe |
| `newMockRepo[T](t, newFn)` | No (pgxmock) | **Yes** — unit-safe (in `store` package) |

## CI expectations

- **PR CI** (no Docker): `go build ./... && go vet ./... && go test ./internal/... -timeout 180s`
  - Unit tests run; integration-tagged tests in `internal/` are excluded by the build tag.
- **Merge CI** (Docker available): `go test -tags integration ./tests/...`
  - Runs all integration tests with real Postgres/Redis via testcontainers.

## Known Type A tests (unit test requiring external resource, kept as-is)

These tests are in `internal/` but connect to `localhost:<port>` directly. They skip
gracefully when the service is unavailable, so `go test ./internal/...` does not fail.
They should eventually be converted to miniredis or moved to `tests/integration/`.

| File | Function | External dep | Skip behavior | Status |
|---|---|---|---|---|
| `internal/audit/audit_db_test.go` | `tryAuditPostgresPool` | Postgres (localhost:5432) | `t.Skipf` | Deferred — tests SQL correctness |
| `internal/migrateutil/migrateutil_test.go` | `tryPostgresConnString` | Postgres (localhost:5432) | `t.Skipf` | Deferred — tests migration SQL |
| `internal/health/health_test.go` | `TestReadyHandler_PostgresOKIntegration` | Postgres (localhost:5432) | `t.Skip` | Deferred — tests real PG health check |
| `internal/store/postgres_test.go` | `tryPostgresConnString` | Postgres (localhost:5432) | `t.Skipf` | RO-048 territory (do not touch) |
| `tests/integration/rate_limiter_test.go` | all tests | miniredis only (no external) | N/A | Should move to `internal/store/` — deferred (RO-048 conflict) |
