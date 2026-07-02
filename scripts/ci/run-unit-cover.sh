#!/usr/bin/env bash
set -euo pipefail
cd "$(dirname "$0")/../../backend"
pkgs=$(go list ./internal/... | grep -v /internal/testutil | grep -v /internal/testsecrets)
# shellcheck disable=SC2086
go test $pkgs -short -p 1 -coverprofile=unit.out -covermode=atomic -timeout 180s
go tool cover -func=unit.out | tail -1
