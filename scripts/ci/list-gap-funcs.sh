#!/usr/bin/env bash
set -euo pipefail
cd "$(dirname "$0")/../../backend"
go tool cover -func=unit.out | grep -E '\.go:' | grep -v '100.0%' | sort -t: -k3 -n | head -60
