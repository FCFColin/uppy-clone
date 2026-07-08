#!/bin/bash
# Enterprise rationale: Enforce Docker image digest pinning for supply-chain security.
# SLSA Level 2 requires reproducible builds — tags are mutable, digests are immutable.
# This script verifies all FROM lines in Dockerfile use @sha256: format.
# Usage: ./scripts/ci/check-docker-digests.sh
# CI: Add as a gate job before docker build.

set -euo pipefail

DOCKERFILE="${1:-Dockerfile}"

if [ ! -f "$DOCKERFILE" ]; then
    echo "ERROR: Dockerfile not found at $DOCKERFILE"
    exit 1
fi

echo "Checking Dockerfile for digest-pinned FROM lines..."

# Extract all FROM lines
from_lines=$(grep -nE '^\s*FROM\s+' "$DOCKERFILE" || true)

if [ -z "$from_lines" ]; then
    echo "ERROR: No FROM lines found in $DOCKERFILE"
    exit 1
fi

errors=0
while IFS= read -r line; do
    lineno=$(echo "$line" | cut -d: -f1)
    content=$(echo "$line" | cut -d: -f2-)

    # Skip multi-stage build references (FROM ... AS ...)
    # Check if the image reference contains @sha256:
    if echo "$content" | grep -qE 'FROM\s+\S+@sha256:[a-f0-9]{64}'; then
        image=$(echo "$content" | sed -E 's/.*FROM\s+(\S+).*/\1/')
        echo "  OK (line $lineno): $image"
    else
        # Extract the image name for the error message
        image=$(echo "$content" | sed -E 's/.*FROM\s+(\S+).*/\1/')
        echo "  FAIL (line $lineno): $image — not pinned with @sha256:<digest>"
        errors=$((errors + 1))
    fi
done <<< "$from_lines"

if [ "$errors" -gt 0 ]; then
    echo ""
    echo "ERROR: $errors FROM line(s) not digest-pinned."
    echo "Run: 'docker pull <image>' then replace tag with @sha256:<digest>."
    exit 1
fi

echo ""
echo "All FROM lines are digest-pinned."
