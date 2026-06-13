#!/usr/bin/env bash
# Regenerates every example variant from the single source project in examples/source.
# These outputs are the golden reference: after a behavior-preserving change the final
# `git diff --exit-code examples/` must be empty.
#
# NOTE: ogen variants require an available `ogen` binary. The JSON, meta, and manifest commands
# use only this CLI.

set -Eeuo pipefail

run_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
root_dir="$(cd "${run_dir}/.." && pwd)"
cd "${root_dir}"

gen="go run ./cmd/goopenapigen"

echo "==> json: public OpenAPI JSON only"
${gen} json generate \
  -source ./examples/source \
  -out ./examples/variants/json/target \
  -force

echo "==> ogen: typed Go server"
${gen} generate \
  -source ./examples/source \
  -out ./examples/variants/ogen/target/api \
  -pkg api \
  -router=false \
  -openapi-json-go=false \
  -meta-go=false \
  -force

echo "==> router: bundle + ogen + router bridge"
${gen} generate \
  -source ./examples/source \
  -out ./examples/variants/router/target/api \
  -pkg api \
  -openapi-json-go=false \
  -meta-go=false \
  -force

echo "==> full-go: typed server + router bridge + OpenAPI JSON Go + metadata Go"
${gen} generate \
  -source ./examples/source \
  -out ./examples/variants/full-go/target/api \
  -pkg api \
  -force

echo "==> meta: Go metadata constants"
${gen} meta generate \
  -source ./examples/source \
  -out ./examples/variants/meta/target \
  -pkg target \
  -force

echo "==> manifest: create/sync the project manifest"
${gen} manifest sync -source ./examples/source -create

# Self-check: regeneration must be a no-op on a committed tree.
git diff --exit-code examples/ \
  && echo "regen-examples: clean (no drift)" \
  || { echo "regen-examples: drift detected (review the diff above)"; exit 1; }
