#!/bin/bash
# Resolve Docker image digests for SLSA L2 reproducible builds.
# Usage: ./scripts/ci/pin-digests.sh
set -euo pipefail

images=(
    "node:20.18.0-alpine3.20"
    "golang:1.25-alpine3.21"
    "gcr.io/distroless/static-debian12:nonroot"
)

echo "Resolving image digests..."
echo "---"
for image in "${images[@]}"; do
    digest=$(docker buildx imagetools inspect "$image" --format '{{json .}}' 2>/dev/null \
        | grep -oE '"digest":"sha256:[a-f0-9]{64}"' | head -1 | cut -d'"' -f4 || echo "UNKNOWN")
    if [ "$digest" != "UNKNOWN" ] && [ -n "$digest" ]; then
        echo "$image -> ${image}@${digest}"
    else
        echo "WARNING: Could not resolve digest for $image (is Docker running?)"
    fi
done
echo "---"
echo "Update Dockerfile FROM lines to: <image>@sha256:<digest>"
