# Management API client generation

This directory holds everything that produces the generated Management API
client in [`internal/gen/api`](../gen/api).

Nothing under `internal/gen` is edited by hand. Change an input here and
regenerate.

## How it works

The client is generated from the OpenAPI documents the Management API
publishes about itself, in four steps:

```
v1-openapi.yaml ─┐
                 ├─ go run ./mergespec ──► openapi.merged.yaml
v2-openapi.yaml ─┘                               │
                                                 ▼
             go tool oapi-codegen  (overlay.yaml, types.cfg.yaml, client.cfg.yaml)
                                                 │
                                                 ▼
                              ../gen/api/types.gen.go, ../gen/api/client.gen.go
```

The stack, top to bottom:

- **OpenAPI 3.0 documents** served by the API at `/api/v1-yaml` and
  `/api/v2-yaml`. They are committed in this repository as snapshots so that 
  generation needs no network and is reproducible from a checkout.
- **`mergespec`**, a small Go program in this directory built on
  `gopkg.in/yaml.v3`. oapi-codegen takes one input document, so this unions
  the two snapshots: `paths`, `components.schemas` and
  `components.securitySchemes` are merged. It fails on any entry that appears in
  both documents with different definitions, on a duplicate operation id, or on 
  an operation id that does not start with its path's leading segment (`v1-...` 
  under `/v1/...`, `v2-...` under `/v2/...`).
- **An [OpenAPI Overlay](https://github.com/OAI/Overlay-Specification)**,
  `overlay.yaml`, patches the merged document before generation. It fixes
  shapes the generator cannot turn into usable Go and pins two query
  parameter types the provider relies on: `reveal` on the project API key
  endpoints and `enabled` on the legacy API keys endpoint. Upstream now
  documents both as "boolean strings" (`true`, `1`, `yes`, ...), which would
  generate as `string`; the overlay keeps them `type: boolean` so the provider
  passes Go booleans, which encode as the same `true`/`false` on the wire.
  oapi-codegen applies the overlay through Speakeasy's overlay library in
  strict mode.
- **[oapi-codegen](https://github.com/oapi-codegen/oapi-codegen) v2**
  generates `types.gen.go` (models) from `types.cfg.yaml` and
  `client.gen.go` (client) from `client.cfg.yaml`. It is pinned in `go.mod`
  with a `tool` directive and runs as `go tool oapi-codegen`, so nothing is
  installed by hand; this needs Go 1.24 or later. The generated code depends
  on `github.com/oapi-codegen/runtime` for request building and parameter
  encoding, and on `github.com/oapi-codegen/nullable` for nullable fields.
- **`go generate`** runs the three steps in order from the directives in
  `generate.go`. It runs with this directory as the working directory, which
  is why every path in the config files is a bare filename and the output
  path is `../gen/api/...`.

| File | Role |
|---|---|
| `v1-openapi.yaml`, `v2-openapi.yaml` | Committed snapshots of `https://api.supabase.com/api/v1-yaml` and `https://api.supabase.com/api/v2-yaml`. |
| `mergespec/` | Merges the two snapshots into `openapi.merged.yaml` (gitignored). `main_test.go` covers the merge rules. |
| `overlay.yaml` | OpenAPI Overlay applied to the merged document. Every action carries a `description` saying why it exists. |
| `types.cfg.yaml`, `client.cfg.yaml` | oapi-codegen configuration for the models and the client. |
| `generate.go` | The `go:generate` directives. |

## Using the generated client

For contributors working in `internal/provider`:

- Import `github.com/supabase/terraform-provider-supabase/internal/gen/api`;
  the package name is `api`. One client covers both API versions.
- `provider.go` builds the single `*api.ClientWithResponses` with the
  retrying HTTP client and the request editors that add the bearer token and
  user agent. Resources and data sources receive it through `extractClient`.
- Method names come from operation ids: `v1-list-all-projects` becomes
  `V1ListAllProjectsWithResponse`, `v2-get-project-config` becomes
  `V2GetProjectConfigWithResponse`. Operations with non-JSON bodies (eszip,
  multipart) also get a `...WithBodyWithResponse` variant that takes a
  content type and an `io.Reader`.
- Each operation has companion types: `api.<Op>Params` for path and query
  parameters, `api.<Op>JSONRequestBody` or the named body schema (for
  example `api.CreateBranchBody`) for the body, and `api.<Op>Response` for the
  result, with `StatusCode()`, the raw `Body`, and typed `JSON200`,
  `JSON4xx` pointers.
- Schema names map to Go types with underscores removed. Upstream names
  response schemas with an `_Output` suffix, so `SecretResponse_Output` is
  `api.SecretResponseOutput`.
- Enum constants are always `<Type><Value>`, for example
  `api.V1ProjectWithDatabaseResponseOutputStatusACTIVEHEALTHY`. This is the
  `always-prefix-enum-values` setting: a constant's name depends only on its
  own type and value, never on what other enums contain.
- Nullable fields are `nullable.Nullable[T]`; see `NullableToString` in
  `internal/provider/utils.go` for the Terraform mapping.
- To find a type or method, grep `internal/gen/api/types.gen.go` for the JSON
  field name or `client.gen.go` for the operation id.

## Regenerating

```sh
go generate ./internal/openapi/...
```

This is hermetic: it reads only the committed snapshots. CI's `generate` job
runs `go generate ./...` and fails if the result differs from what is
committed, which also catches a generator bump that was not followed by
regeneration.

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

## What CI checks

- The `generate` job in `.github/workflows/test.yml` regenerates from the
  committed snapshots and fails on any diff. Nothing in CI reads the live
  API.
- The `build` job runs golangci-lint. `internal/gen` is excluded by path in
  `.golangci.yml`, and also by the `Code generated ... DO NOT EDIT` header.
  `mergespec` is linted and tested like any other package.

## Changing the overlay

Touch the overlay only when the generator cannot consume an upstream shape
(the package does not compile, or a type is unusable), or to pin a parameter
type the provider depends on. Prefer adapting provider code when the upstream
change is a real API change.

An action is a `target` (a JSONPath into the merged document) with either
`update` (deep-merged into every match) or `remove: true`, plus a
`description` saying why it exists. Wildcard targets apply to both API
versions, so check the v2 document before adding one.

Strict mode means an action whose target matches nothing fails generation
with `selector "..." did not match any targets`. So every action in the
committed file is known to match, and an action that upstream no longer needs
shows up as a failure the next time the snapshots are refreshed; delete it
then. To see what a target selects, inspect `openapi.merged.yaml`, which the
first generate step leaves on disk.

The filter expressions (`[?(@.name=='reveal')]`) use the legacy JSONPath
dialect. oapi-codegen's validator mentions `x-speakeasy-jsonpath: rfc9535` for
the RFC 9535 dialect; switching would need every target re-verified and has
not been done.

## Changing mergespec

`merge` is a pure function over parsed documents; `run` is the thin file
layer around it. Add a test for every new rule in `main_test.go` and run:

```sh
go test ./internal/openapi/...
```

If v1 and v2 ever publish the same schema name with different definitions,
the right fix is a deliberate rename in the merge (for example prefixing the
v2 schema), not a relaxed check.

## Bumping the generator

```sh
go get -tool github.com/oapi-codegen/oapi-codegen/v2/cmd/oapi-codegen@vX.Y.Z
go generate ./internal/openapi/...
go build ./... && go test ./...
```

A generator upgrade can rename generated identifiers; review the diff before
committing. Dependabot does not bump the tool, so this is always a deliberate
change.

## Troubleshooting

| Symptom | Cause and fix |
|---|---|
| `mergespec: v2-openapi.yaml: components.schemas/X conflicts with an earlier document` | v1 and v2 define `X` differently. Decide on a rename in `mergespec`. |
| `selector "..." did not match any targets` | An overlay action is stale. Delete it, or fix its target. |
| `X redeclared` in `types.gen.go` | Two JSON names normalise to one Go field, such as `connectionString` and `connection_string`. Remove one with an overlay action. |
| `invalid recursive type ...Params...` | A query parameter is an `anyOf` union. Replace it with a concrete schema in the overlay, as done for `services`. |
| `undefined: api.Something` in `internal/provider` after a refresh | Upstream renamed a schema. Rename the uses in provider code. |
| `go: no such tool "oapi-codegen"` or `unknown directive: tool` | Go older than 1.24, or the `tool` line is missing from `go.mod`. `.go-version` has the version CI uses. |
