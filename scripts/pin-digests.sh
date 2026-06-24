#!/bin/bash
# Enterprise rationale: Pin Docker image digests for build reproducibility.
# This script resolves tag-based image references to their SHA256 digests.
# Tags are mutable — the same tag can point to different images over time.
# SLSA Level 2 requires reproducible builds.
# Usage: ./scripts/pin-digests.sh
# After running, update Dockerfile FROM lines to use image@sha256:<digest>.

set -euo pipefail

images=(
    "node:20.18.0-alpine3.20"
    "golang:1.26.0-alpine3.20"
    "alpine:3.19.4"
)

echo "Resolving image digests..."
echo "---"
for image in "${images[@]}"; do
    digest=$(docker buildx imagetools inspect "$image" --format '{{.Digest}}' 2>/dev/null || echo "UNKNOWN")
    if [ "$digest" != "UNKNOWN" ]; then
        echo "$image -> ${image}@${digest}"
    else
        echo "WARNING: Could not resolve digest for $image (is Docker running?)"
    fi
done
echo "---"
echo "Update Dockerfile FROM lines to: <image>@sha256:<digest>"
