#!/usr/bin/env bash
# Layered coverage governance: total 80%, important paths 90%, per-file floor 60%.
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
BACKEND="${ROOT}/backend"
FRONTEND="${ROOT}/frontend"

UNIT_MIN="${UNIT_MIN:-80}"
INT_MIN="${INT_MIN:-80}"
FRONTEND_LINES_MIN="${FRONTEND_LINES_MIN:-85}"
FRONTEND_BRANCHES_MIN="${FRONTEND_BRANCHES_MIN:-80}"
FRONTEND_FUNCTIONS_MIN="${FRONTEND_FUNCTIONS_MIN:-85}"
FILE_MIN="${FILE_MIN:-60}"
IMPORTANT_MIN="${IMPORTANT_MIN:-90}"

UNIT_OUT="${COVER_UNIT:-${BACKEND}/unit.out}"
INT_OUT="${COVER_INT:-${BACKEND}/int.out}"
FRONTEND_LCOV="${FRONTEND}/coverage/lcov.info"

# Important paths (relative to backend/ or frontend/) — each must meet IMPORTANT_MIN.
IMPORTANT_BACKEND=(
  "internal/auth/"
  "internal/crypto/"
  "internal/audit/"
  "internal/rbac/"
  "internal/validate/"
  "internal/store/"
  "internal/server/"
  "internal/config/env.go"
  "internal/game/"
  "internal/handler/"
  "internal/middleware/"
  "internal/protocol/"
  "internal/worker/"
  "internal/domain/"
)
IMPORTANT_FRONTEND=(
  "src/game/message_codec.ts"
  "src/game/ws_handlers.ts"
  "src/game/ws_handlers_events.ts"
  "src/game/ws_handlers_snapshot.ts"
  "src/game/ws_connect.ts"
  "src/shared/network/auth.ts"
  "src/game/state_types.ts"
  "src/game/phase_sync.ts"
  "src/shared/game/protocol.ts"
)

# Exclude from per-file floor (types, entry points, test helpers — no business logic).
EXCLUDE_PATTERNS=(
  "vite-env.d.ts"
  ".d.ts"
  "/main.ts"
  "/index.ts"
  "constants.ts"
  "constants.go"
  "testutil/"
  "cmd/server/main.go"
)

usage() {
  echo "Usage: $0 [unit|integration|frontend|all] [coverage.out]"
  echo "  unit         — backend unit layer (default: ${UNIT_OUT})"
  echo "  integration  — backend integration layer (default: ${INT_OUT})"
  echo "  frontend     — Vitest lcov (${FRONTEND_LCOV})"
  echo "  all          — run all layers (generates profiles if missing)"
}

should_exclude() {
  local path="$1"
  for pat in "${EXCLUDE_PATTERNS[@]}"; do
    [[ "$path" == *"$pat"* ]] && return 0
  done
  return 1
}

resolve_cover_file() {
  local cover_file="$1"
  case "$cover_file" in
    /*)
      printf '%s' "$cover_file"
      return 0
      ;;
  esac
  if [[ -f "${ROOT}/${cover_file}" ]]; then
    printf '%s' "${ROOT}/${cover_file}"
    return 0
  fi
  if [[ -f "${BACKEND}/${cover_file}" ]]; then
    printf '%s' "${BACKEND}/${cover_file}"
    return 0
  fi
  if [[ -f "${BACKEND}/unit.out" && ( "$cover_file" == "unit.out" || "$cover_file" == "backend/unit.out" ) ]]; then
    printf '%s' "${BACKEND}/unit.out"
    return 0
  fi
  if [[ -f "${BACKEND}/int.out" && ( "$cover_file" == "int.out" || "$cover_file" == "backend/int.out" ) ]]; then
    printf '%s' "${BACKEND}/int.out"
    return 0
  fi
  printf '%s' "${ROOT}/${cover_file}"
}

check_go_total() {
  local min_pct="$1"
  local label="$2"
  local cover_lines="$3"
  local pct
  pct=$(printf '%s\n' "$cover_lines" | awk '/^total:/ {gsub(/%/,"",$3); print $3}')
  echo "${label} total coverage: ${pct}% (min ${min_pct}%)"
  if awk -v p="$pct" -v t="$min_pct" 'BEGIN { exit (p+0 >= t+0) ? 0 : 1 }'; then
    return 0
  fi
  echo "FAIL: ${label} total ${pct}% < ${min_pct}%"
  return 1
}

load_go_cover_func() {
  local cover_file="$1"
  local resolved
  resolved=$(resolve_cover_file "$cover_file")
  if [[ ! -f "$resolved" ]]; then
    echo "ERROR: coverage file not found: $cover_file" >&2
    return 1
  fi
  local rel="${resolved#"$BACKEND/"}"
  (cd "$BACKEND" && go tool cover -func="$rel")
}

check_go_per_file() {
  local cover_file="$1"
  local min_pct="$2"
  local prefix="${3:-}"
  local cover_lines="$4"
  local failed=0
  while IFS= read -r line; do
    [[ "$line" == total:* ]] && continue
    local file pct
    file=$(echo "$line" | awk '{print $1}')
    pct=$(echo "$line" | awk '{print $NF}' | tr -d '%')
    [[ -n "$prefix" && "$file" != *"$prefix"* ]] && continue
    should_exclude "$file" && continue
    if awk -v p="$pct" -v t="$min_pct" 'BEGIN { exit (p+0 >= t+0) ? 0 : 1 }'; then
      continue
    fi
    echo "FAIL file ${file}: ${pct}% < ${min_pct}%"
    failed=1
  done < <(printf '%s\n' "$cover_lines" | grep -E '\.go:' || true)
  return $failed
}

check_go_important() {
  local cover_file="$1"
  local min_pct="$2"
  shift 2
  local cover_lines="$1"
  shift
  local paths=("$@")
  local failed=0
  for imp in "${paths[@]}"; do
    local agg=0 count=0
    while IFS= read -r line; do
      local pct
      pct=$(echo "$line" | awk '{print $NF}' | tr -d '%')
      agg=$(awk -v a="$agg" -v p="$pct" 'BEGIN { print a + p }')
      count=$((count + 1))
    done < <(printf '%s\n' "$cover_lines" | grep "$imp" | grep -E '\.go:' || true)
    if [[ $count -eq 0 ]]; then
      echo "WARN important path has no coverage entries: $imp"
      continue
    fi
    local avg
    avg=$(awk -v a="$agg" -v c="$count" 'BEGIN { printf "%.1f", a/c }')
    if awk -v p="$avg" -v t="$min_pct" 'BEGIN { exit (p+0 >= t+0) ? 0 : 1 }'; then
      echo "OK  important $imp: ${avg}%"
    else
      echo "FAIL important $imp: ${avg}% < ${min_pct}%"
      failed=1
    fi
  done
  return $failed
}

run_unit_coverage() {
  cd "$BACKEND"
  local cover_file="${COVER_TMP:-/tmp/balloon-unit.out}"
  local pkgs
  pkgs=$(go list ./internal/... 2>/dev/null | grep -v /internal/testutil | grep -v /internal/testsecrets || go list ./internal/... | grep -v /internal/testutil | grep -v /internal/testsecrets)
  # shellcheck disable=SC2086
  go test $pkgs -short -coverprofile="$cover_file" -covermode=atomic -timeout 180s
  echo "$cover_file"
}

run_integration_coverage() {
  cd "$BACKEND"
  go test ./tests/integration/... -timeout 120s -coverprofile=int.out -covermode=atomic
}

run_frontend_coverage() {
  cd "$FRONTEND"
  npm run test:frontend
}

check_frontend_important() {
  local min_pct="$1"
  local failed=0
  for imp in "${IMPORTANT_FRONTEND[@]}"; do
    local total hit
    read -r total hit <<< "$(awk -v imp="$imp" '
      /^SF:/ {
        path = $0
        sub(/^SF:/, "", path)
        gsub(/\\/, "/", path)
        infile = index(path, imp) > 0
        next
      }
      infile && /^DA:/ {
        split($0, parts, /[:,]/)
        total++
        if (parts[3] + 0 > 0) hit++
      }
      /^end_of_record/ { infile = 0 }
      END { print total + 0, hit + 0 }
    ' "$FRONTEND_LCOV")"
    if [[ "${total:-0}" -eq 0 ]]; then
      echo "WARN important frontend path has no coverage entries: $imp"
      continue
    fi
    local pct
    pct=$(awk -v h="$hit" -v t="$total" 'BEGIN { printf "%.1f", (h/t)*100 }')
    if awk -v p="$pct" -v m="$min_pct" 'BEGIN { exit (p+0 >= m+0) ? 0 : 1 }'; then
      echo "OK  important frontend $imp: ${pct}%"
    else
      echo "FAIL important frontend $imp: ${pct}% < ${min_pct}%"
      failed=1
    fi
  done
  return $failed
}

check_unit() {
  local cover_file="${1:-$UNIT_OUT}"
  local failed=0
  local resolved
  resolved=$(resolve_cover_file "$cover_file")
  if [[ ! -f "$resolved" ]]; then
    echo "ERROR: Unit coverage file not found: $cover_file"
    return 1
  fi
  local cover_lines
  cover_lines=$(load_go_cover_func "$cover_file")
  check_go_total "$UNIT_MIN" "Unit" "$cover_lines" || failed=1
  check_go_per_file "$cover_file" "$FILE_MIN" "github.com/uppy-clone/backend/" "$cover_lines" || failed=1
  check_go_important "$cover_file" "$IMPORTANT_MIN" "$cover_lines" "${IMPORTANT_BACKEND[@]}" || failed=1
  return $failed
}

check_integration() {
  local cover_file="${1:-$INT_OUT}"
  local failed=0
  check_go_total "$cover_file" "$INT_MIN" "Integration" || failed=1
  return $failed
}

check_frontend_lcov_metric() {
  local metric="$1" label="$2" min_pct="$3"
  local total=0 hit=0
  case "$metric" in
    lines)
      total=$(grep -c '^DA:' "$FRONTEND_LCOV" || true)
      hit=$(grep '^DA:' "$FRONTEND_LCOV" | awk -F, '$2 > 0' | wc -l | tr -d ' ')
      ;;
    branches)
      while IFS= read -r brda; do
        taken="${brda##*,}"
        total=$((total + 1))
        if [[ "$taken" =~ ^-[0-9]+$ || "$taken" =~ ^[0-9]+$ ]] && [ "$taken" -gt 0 ]; then
          hit=$((hit + 1))
        fi
      done < <(grep '^BRDA:' "$FRONTEND_LCOV" || true)
      ;;
    functions)
      local fn_found fn_hit
      fn_found=$(grep '^FNF:' "$FRONTEND_LCOV" | awk -F: '{s+=$2} END {print s+0}' || true)
      fn_hit=$(grep '^FNH:' "$FRONTEND_LCOV" | awk -F: '{s+=$2} END {print s+0}' || true)
      total=$fn_found
      hit=$fn_hit
      ;;
  esac
  if [[ "$total" -eq 0 ]]; then
    echo "WARN frontend $label: no data (all excluded?)"
    return 0
  fi
  local pct
  pct=$(awk -v h="$hit" -v t="$total" 'BEGIN { printf "%.1f", (h/t)*100 }')
  echo "Frontend $label coverage: ${pct}% (min ${min_pct}%)"
  if awk -v p="$pct" -v t="$min_pct" 'BEGIN { exit (p+0 >= t+0) ? 0 : 1 }'; then
    return 0
  fi
  echo "FAIL: frontend $label ${pct}% < ${min_pct}%"
  return 1
}

check_frontend() {
  if [[ ! -f "$FRONTEND_LCOV" ]]; then
    echo "ERROR: frontend lcov not found: $FRONTEND_LCOV"
    return 1
  fi
  local failed=0
  check_frontend_lcov_metric "lines"     "line"     "$FRONTEND_LINES_MIN"     || failed=1
  check_frontend_lcov_metric "branches"  "branch"   "$FRONTEND_BRANCHES_MIN"  || failed=1
  check_frontend_lcov_metric "functions" "function" "$FRONTEND_FUNCTIONS_MIN" || failed=1
  check_frontend_important "$IMPORTANT_MIN" || failed=1
  return $failed
}

MODE="${1:-unit}"
ARG_FILE="${2:-}"

case "$MODE" in
  unit)
    if [[ -n "$ARG_FILE" ]]; then
      check_unit "$ARG_FILE"
    else
      cover_file=$(run_unit_coverage)
      check_unit "$cover_file"
    fi
    ;;
  integration)
    if [[ -n "$ARG_FILE" ]]; then
      check_integration "$ARG_FILE"
    else
      run_integration_coverage
      check_integration "$INT_OUT"
    fi
    ;;
  frontend)
    if [[ ! -f "$FRONTEND_LCOV" ]]; then
      run_frontend_coverage
    fi
    check_frontend
    ;;
  all)
    run_unit_coverage && check_unit "$UNIT_OUT"
    run_integration_coverage && check_integration "$INT_OUT"
    run_frontend_coverage && check_frontend
    ;;
  *)
    usage
    exit 1
    ;;
esac

echo "Coverage governance passed ($MODE)."
