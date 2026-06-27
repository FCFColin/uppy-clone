#!/usr/bin/env bash
# Verify ADR-021 repo layout: no legacy flat paths, canonical subdirs only.
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
cd "$ROOT"

fail=0

assert_missing() {
  local path="$1"
  if [[ -e "$path" ]]; then
    echo "legacy path must not exist: $path" >&2
    fail=1
  fi
}

assert_exists() {
  local path="$1"
  if [[ ! -e "$path" ]]; then
    echo "required path missing: $path" >&2
    fail=1
  fi
}

# Legacy flat docs (nested copies are canonical)
for f in \
  docs/runbook.md docs/slo.md docs/architecture.md docs/openapi.yaml \
  docs/asyncapi.yaml docs/ws-protocol.md docs/coverage-policy.md \
  docs/benchmarks-v2.md docs/environments.md docs/logging-policy.md \
  docs/threat-model.md docs/multi-region-topology.md docs/cockroachdb-migration.md \
  docs/db-query-analysis.md docs/capacity-planning.md docs/continuous-profiling.md \
  docs/chaos-experiments.md
do
  assert_missing "$f"
done

# Legacy backend server package in cmd/
for f in "$ROOT"/backend/cmd/server/*.go; do
  [[ "$(basename "$f")" == "main.go" ]] && continue
  echo "legacy server file must not exist: $f" >&2
  fail=1
done

# Legacy infra flat layout
for p in \
  infra/base infra/overlays infra/global \
  infra/main.tf infra/variables.tf infra/outputs.tf infra/service.yaml
do
  assert_missing "$p"
done

# Legacy scripts at repo scripts/ root
for f in \
  scripts/check-coverage.sh scripts/check-docker-digests.sh scripts/pin-digests.sh \
  scripts/k6 scripts/merge_go_tests.py scripts/merge-package-tests.py
do
  assert_missing "$f"
done

# Legacy docker / frontend / rbac paths
assert_missing docker/init-scripts
assert_missing frontend/play.css
assert_missing frontend/src/index_fetch.ts
assert_missing backend/internal/rbac/model.conf
assert_missing backend/internal/rbac/policy.csv

# One-off maintenance scripts must not return to scripts/archive
assert_missing scripts/archive

# Required canonical paths
for p in \
  backend/internal/server \
  infra/k8s/base infra/terraform \
  deploy/local \
  scripts/ci scripts/load \
  docker/postgres/init \
  docs/operations/runbook.md \
  docs/development/benchmarks-go-microbench.md \
  docs/development/benchmarks-k6-room-slo.md
do
  assert_exists "$p"
done

# cmd/server must contain only main.go
shopt -s nullglob
server_go=(backend/cmd/server/*.go)
if [[ ${#server_go[@]} -ne 1 || "$(basename "${server_go[0]}")" != "main.go" ]]; then
  echo "backend/cmd/server must contain only main.go (found: ${server_go[*]:-<none>})" >&2
  fail=1
fi

# Alert rules ConfigMap must be generated from rules.yml
if ! grep -q '^# Generated from deploy/alertmanager/rules.yml' deploy/alertmanager/rules-configmap.yaml; then
  echo "deploy/alertmanager/rules-configmap.yaml must be generated (run: make sync-alert-rules)" >&2
  fail=1
fi

if [[ "$fail" -ne 0 ]]; then
  exit 1
fi

echo "repo layout OK (ADR-021)"
