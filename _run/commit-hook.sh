#!/usr/bin/env bash

set -Eeuo pipefail

echo "[HOOK]" "Commit"

run_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
root_path="$(cd "${run_dir}/.." && pwd)"

version="$(
  go run github.com/amazing-generators/gometagen/cmd/gometagen@latest \
    version print -source "${run_dir}/values.yml"
)"
branch="$(
  go run github.com/amazing-generators/gometagen/cmd/gometagen@latest \
    git branch -source "${root_path}"
)"

printf "%s [%s]\n\n%s" "${branch}" "${version}" "$(cat "$1")" > "$1"

cd "${root_path}"
go test -v ./...
