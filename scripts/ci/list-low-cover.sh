#!/usr/bin/env bash
# List Go files below threshold from unit.out (run from repo root).
set -euo pipefail
ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
BACKEND="${ROOT}/backend"
THRESH="${1:-100}"
(cd "$BACKEND" && go tool cover -func=unit.out) | awk -v t="$THRESH" '
  /^total:/ { next }
  /\.go:/ {
    split($1, a, ":")
    file=a[1]
    pct=$NF
    gsub(/%/, "", pct)
    sum[file]+=pct
    cnt[file]++
  }
  END {
    for (f in sum) {
      avg=sum[f]/cnt[f]
      if (avg < t) printf "%.1f%% %s\n", avg, f
    }
  }
' | sort -t% -k1 -n
