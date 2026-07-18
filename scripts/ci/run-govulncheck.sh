#!/bin/bash
# Run govulncheck with a pinned version for reproducible supply-chain builds.
# v2-R-15: pin v1.1.4 (consistent across go-ci.yml and security-scan.yml).
# Usage: bash scripts/ci/run-govulncheck.sh
# Must be run from the backend/ directory (or adapt the working dir).

set -euo pipefail

go install golang.org/x/vuln/cmd/govulncheck@v1.1.4
govulncheck -test=false ./cmd/... ./internal/...
