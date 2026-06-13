#!/usr/bin/env bash

set -Eeuo pipefail

run_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
root_dir="$(cd "${run_dir}/.." && pwd)"

cd "${root_dir}"

go install github.com/amazing-generators/gometagen/cmd/gometagen@latest

go run github.com/amazing-generators/gometagen/cmd/gometagen@latest git add-commit-hook -source .
go run github.com/amazing-generators/gometagen/cmd/gometagen@latest git add-push-hook -source .

go mod tidy
