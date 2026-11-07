# Management API OpenAPI inputs

This directory holds everything that produces the generated Management API
client in [`internal/gen/api`](../gen/api). Nothing under `internal/gen` is
edited by hand.

| File | Role |
|---|---|
| `v1-openapi.yaml`, `v2-openapi.yaml` | Committed snapshots of `https://api.supabase.com/api/v1-yaml` and `/api/v2-yaml`. |
| `mergespec/` | Merges the two snapshots into `openapi.merged.yaml` (gitignored). An entry present in both must be identical; operation ids must be unique and start with the path's leading segment. |
| `overlay.yaml` | [OpenAPI Overlay](https://github.com/OAI/Overlay-Specification) applied by oapi-codegen to the merged document. Fixes shapes the generator cannot consume, plus two actions that keep the `reveal` and `enabled` query parameters boolean. |
| `types.cfg.yaml`, `client.cfg.yaml` | oapi-codegen configuration for the models and the client. |
| `generate.go` | The `go:generate` directives, run in this directory. |

## Regenerating

```sh
go generate ./internal/openapi/...
```

This is hermetic: it reads only the committed snapshots. The generator is
pinned in `go.mod` as a Go tool (`go tool oapi-codegen`), so nothing needs to
be installed. CI's `generate` job runs `go generate ./...` and fails if the
result differs from what is committed.

## Refreshing the snapshots

```sh
make update-api-spec                                              # production
make update-api-spec SUPABASE_API_URL=https://api.supabase.green  # staging
make update-api-spec SUPABASE_API_URL=http://127.0.0.1:8080       # a branch of the API running locally
make update-api-spec API_SPEC_V1=file:///path/v1.yaml API_SPEC_V2=file:///path/v2.yaml
```

The target downloads both documents and regenerates. Commit the snapshots and
the generated code together. Read the snapshot diff: it is the upstream API
change, and the generated diff is its echo. Renamed schemas or changed
parameter types surface as compile errors in `internal/provider`.

## When to touch the overlay

Only when the generator cannot consume an upstream shape (the package does not
compile, or a type is unusable), or to pin a parameter type the provider
depends on. Every action carries a `description` saying why it exists; drop an
action once upstream no longer needs it. The wildcard targets apply to both
API versions, so check the v2 document too before adding one.

## Bumping the generator

```sh
go get -tool github.com/oapi-codegen/oapi-codegen/v2/cmd/oapi-codegen@vX.Y.Z
go generate ./internal/openapi/...
go build ./... && go test ./...
```

A generator upgrade can rename generated identifiers; review the diff before
committing.
