#!/bin/bash
# WS protocol sync gate: ensure backend protocol constants are documented
# and match frontend protocol.ts / constants.ts.
# Usage: bash scripts/ci/check-ws-protocol-sync.sh
# CI: docs-governance.yml ws-protocol-sync job.
# Runs from repo root. Exits non-zero on any mismatch.

set -euo pipefail

# 1. Ensure every Msg* constant in backend is documented in ws-protocol.md and asyncapi.yaml.
consts=$(grep -oE 'Msg[A-Za-z]+' backend/internal/protocol/constants.go | sort -u)
missing=0
for c in $consts; do
  if ! grep -q "$c" docs/api/ws-protocol.md; then
    echo "::error::Message constant $c missing from docs/api/ws-protocol.md"
    missing=1
  fi
  if ! grep -q "$c" docs/api/asyncapi.yaml; then
    echo "::error::Message constant $c missing from docs/api/asyncapi.yaml"
    missing=1
  fi
done
if [ "$missing" -ne 0 ]; then exit 1; fi

# 2. Ensure frontend protocol.ts (hex constants) matches backend constants.go.
check_hex() {
  local name="$1" go_val="$2" ts_file="$3"
  local ts_val
  ts_val=$(grep -E "${name}:" "$ts_file" | head -1 | grep -oE '0x[0-9a-fA-F]+')
  if [ "$go_val" != "$ts_val" ]; then
    echo "::error::$name mismatch: constants.go=$go_val protocol.ts=$ts_val"
    exit 1
  fi
}
ts="frontend/src/shared/game/protocol.ts"
check_hex SNAPSHOT 0x01 "$ts"
check_hex PLAYER_JOIN 0x02 "$ts"
check_hex PLAYER_LEAVE 0x03 "$ts"
check_hex TAP_ACCEPTED 0x04 "$ts"
check_hex TAP_REJECTED 0x05 "$ts"
check_hex GAME_STATE_CHANGE 0x06 "$ts"
check_hex RESTART_STATUS 0x07 "$ts"
check_hex PONG 0x21 "$ts"
# Client message types (CLIENT_MSG)
check_hex TAP 0x10 "$ts"
check_hex SET_NICKNAME 0x11 "$ts"
check_hex RESTART_VOTE 0x12 "$ts"
check_hex PING 0x20 "$ts"

# 3. Ensure PHASE_CODE and END_REASON (decimal) constants match backend.
# check_dec verifies a decimal constant in a TS object literal.
# Usage: check_dec NAME GO_VALUE TS_FILE
check_dec() {
  local name="$1" go_val="$2" ts_file="$3"
  local ts_val
  ts_val=$(grep -E "^\s*${name}:" "$ts_file" | head -1 | sed -E 's/.*:\s*([0-9]+).*/\1/')
  if [ "$go_val" != "$ts_val" ]; then
    echo "::error::$name mismatch: backend=$go_val $ts_file=$ts_val"
    exit 1
  fi
}
# PHASE_CODE: backend protocol/constants.go vs frontend protocol.ts
phase_ts="frontend/src/shared/game/protocol.ts"
check_dec WAITING 0 "$phase_ts"
check_dec PLAYING 1 "$phase_ts"
check_dec ENDED 2 "$phase_ts"
check_dec COUNTDOWN 3 "$phase_ts"
# END_REASON: backend protocol/constants.go vs frontend constants.ts (generated)
end_ts="frontend/src/shared/game/constants.ts"
check_dec NONE 0 "$end_ts"
check_dec GROUND 1 "$end_ts"
check_dec BIRD 2 "$end_ts"
check_dec GHOST 3 "$end_ts"

# 4. Ensure generated constants.ts PALETTE_COLORS hex values match backend RGB triples.
palette_line=$(grep -E "^export const PALETTE_COLORS" "$end_ts" || true)
if [ -z "$palette_line" ]; then
  echo "::error::PALETTE_COLORS missing from $end_ts"
  exit 1
fi
# Extract RGB triples from backend, convert to hex, sort for comparison
backend_hex=$(grep -oE '\{[0-9]+, *[0-9]+, *[0-9]+\}' backend/internal/protocol/constants.go | \
  sed -E 's/\{([0-9]+), *([0-9]+), *([0-9]+)\}/\1 \2 \3/' | \
  while read -r r g b; do printf "#%02x%02x%02x\n" "$r" "$g" "$b"; done | sort)
# Extract hex values from frontend, sort for comparison
frontend_hex=$(echo "$palette_line" | grep -oE "#[0-9a-fA-F]{6}" | sort)
if [ "$backend_hex" != "$frontend_hex" ]; then
  echo "::error::PALETTE_COLORS mismatch:"
  echo "  backend (RGB→hex): $(echo "$backend_hex" | tr '\n' ' ')"
  echo "  frontend (hex):    $(echo "$frontend_hex" | tr '\n' ' ')"
  exit 1
fi

echo "WS protocol sync: all checks passed"
