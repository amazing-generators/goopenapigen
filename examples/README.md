# goopenapigen examples

The example source lives in [`source/`](./source). It demonstrates the file layout expected by the CLI.

[`variants/`](./variants) has its own `go.mod` only to keep generated example packages outside the root module's
`go test ./...` traversal. Root tests must not compile example outputs, because those outputs may intentionally contain
placeholder imports or dependencies that belong to a consuming project.

```text
source/
  openapi.yaml
  servers.yml
  tags.yml
  security.yml
  paths/
    auth_login.yaml
    health.yaml
    task_by_id.yaml
    tasks.yaml
  components/
    auth.yaml
    common.yaml
    tasks.yaml
```

`openapi.yaml` contains the stable root envelope: `openapi` and `info`. Root array fields are provided by sidecar files
with the same field name. For example, `tags.yml` contains the array value for root `tags`; it does not contain a
wrapper key.

`paths/` and `components/` are folder-merged into the final OpenAPI document. Internal `#/...` references inside these
files resolve against the assembled root document.

## Manifest

```bash
go run ../cmd/goopenapigen manifest sync -source ./source -create
```

Expected result: `source/meta.yml` with `ver` and the current source graph `hash`; it must not contain `name`.

## Public JSON

```bash
go run ../cmd/goopenapigen json generate \
  -source ./source \
  -out ./variants/json/target \
  -force
```

Expected result: one bundled `openapi.json` file without `x-func`.

## ogen

```bash
go run ../cmd/goopenapigen generate \
  -source ./source \
  -out ./variants/ogen/target/api \
  -pkg api \
  -router=false \
  -openapi-json-go=false \
  -meta-go=false \
  -force
```

Expected result: generated `ogen` package from the bundled spec.

## Router

```bash
go run ../cmd/goopenapigen generate \
  -source ./source \
  -out ./variants/router/target/api \
  -pkg api \
  -openapi-json-go=false \
  -meta-go=false \
  -force
```

Expected result: `ogen` package and router bridge generated from the actual `ogen` interfaces. The bridge exposes a
generated `FuncInterface` for application handlers. `openapi_json_gen.go` is intentionally disabled in this variant.

## Full Go Output

```bash
go run ../cmd/goopenapigen generate \
  -source ./source \
  -out ./variants/full-go/target/api \
  -pkg api \
  -force
```

Expected result: all Go outputs from one command: `ogen` files, `router_gen.go`, `openapi_json_gen.go`, and
`openapi_meta_gen.go`.

## Meta

```bash
go run ../cmd/goopenapigen meta generate \
  -source ./source \
  -out ./variants/meta/target \
  -pkg target \
  -force
```

Expected result: Go constants for OpenAPI `info.title`, effective version, hash, and generation dates.
