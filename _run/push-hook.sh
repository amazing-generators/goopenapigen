#!/usr/bin/env bash

set -Eeuo pipefail

echo "[HOOK]" "Push"

run_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
root_path="$(cd "${run_dir}/.." && pwd)"

cd "${root_path}"
export CGO_ENABLED=1

if [[ -f go.work ]]; then
  go work sync
fi

go mod tidy

echo "==> Running tests with race detector..."
go test -race -v ./...

echo ""
echo "==> Running benchmarks..."
go test -bench=. -run=NONE -benchmem -v ./...

echo ""
echo "[HOOK] All tests and benchmarks passed"
