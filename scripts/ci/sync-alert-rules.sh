#!/usr/bin/env bash
# Generate deploy/alertmanager/rules-configmap.yaml from rules.yml (single source of truth).
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
SRC="${ROOT}/deploy/alertmanager/rules.yml"
OUT="${ROOT}/deploy/alertmanager/rules-configmap.yaml"

if [[ ! -f "$SRC" ]]; then
  echo "missing source: $SRC" >&2
  exit 1
fi

{
  cat <<'HEADER'
# Generated from deploy/alertmanager/rules.yml — run: make sync-alert-rules
apiVersion: v1
kind: ConfigMap
metadata:
  name: alertmanager-rules
  labels:
    app: prometheus
data:
  rules.yml: |
HEADER
  sed 's/^/    /' "$SRC"
} > "$OUT"

echo "wrote $OUT"
