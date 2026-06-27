# Coverage Policy (Unit 100% / Integration 80% / Important 90% / Per-file 60%)

## Layers

| Layer | Command | Total gate |
|-------|---------|------------|
| Backend unit | `go test $(go list ./... \| grep -v tests/integration) -short -coverprofile=unit.out -covermode=atomic` | **lines/branches/functions ≥ 100%** |
| Backend integration | `go test ./tests/integration/... -coverprofile=int.out -covermode=atomic` | **lines ≥ 80%** |
| Frontend Vitest | `cd frontend && npm run test:frontend` | **lines/branches/functions/statements ≥ 100%** |

Governance script: [`scripts/check-coverage.sh`](../../scripts/check-coverage.sh)

```bash
make test-cover          # unit + integration + frontend profiles
bash scripts/check-coverage.sh unit
bash scripts/check-coverage.sh integration
bash scripts/check-coverage.sh frontend
bash scripts/check-coverage.sh all
```

## Exclusion rules (automated, pattern-based)

Files matching these patterns are **fully excluded** from coverage gates (not counted at all):

### Frontend (vitest.config.ts exclude)
| Pattern | Rationale |
|---------|-----------|
| `src/**/*_types.ts`, `src/**/*.d.ts` | Pure type definitions |
| `src/**/constants.ts` | Pure constants |
| `src/main.ts`, `src/index.ts` | Entry glue |
| `src/game/renderer*.ts`, `src/game/ui*.ts` | Low ROI (visual rendering/UI; covered by E2E) |

### Backend (check-coverage.sh EXCLUDE_PATTERNS)
| Pattern | Rationale |
|---------|-----------|
| `*_types.ts`, `*.d.ts` | Pure type definitions |
| `constants.ts`, `constants.go` | Pure constants |
| `/main.ts`, `/index.ts` | Entry glue |
| `testutil/` | Test helpers |
| `degradation_deps.go` | Dependency injection glue |

## Per-file rules

- **Important paths ≥ 90%** — see script `IMPORTANT_*` lists (auth, crypto, audit, rbac, validate, store, cmd/server, handler, middleware, protocol, game, worker, domain; frontend ws, auth, protocol, input, state, phase_sync, session).
- **Any non-excluded source file ≥ 60%**.
- **Excluded files** (matching patterns above) have **no floor requirement**.

## Test quality (required in PRs)

Each new test file must include at least one **adversarial / failure-path** case with a comment explaining the threat model (`// 企业为何需要`, `// SECURITY:`, or `// Adversarial:`).

Categories to cover per module:

1. **Common** — happy path
2. **Edge** — empty input, bounds, concurrency, timeouts
3. **Malicious** — XSS nicknames, header spoofing, revoked JWT, SQL injection attempts, oversized bodies

Do **not** add tests that only mock success with `err == nil` and no behavioral assertion.

## Playwright E2E

End-to-end specs under `tests/e2e/` supplement behavior validation and are **not** counted in Vitest percentage gates.

Renderer (`src/game/renderer*.ts`) and UI (`src/game/ui*.ts`) are excluded from unit coverage and rely on E2E for behavioral validation.
