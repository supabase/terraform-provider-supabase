# Plan: generate the Management API Go client in this repository

Status: proposal for review. Nothing here is merged. Every number and claim
below comes from a dry run in a scratch copy of this repo on 2026-11-06, using
the v1 and v2 documents served by `https://api.supabase.com` that day.

## 1. Why

- The provider's HTTP client today is `github.com/supabase/cli/pkg/api` v1.2.3.
  That package is itself oapi-codegen output (v2.4.1) built from a Management
  API v1 snapshot that lived in the Go CLI repo.
- The CLI has been rewritten in TypeScript. `develop` is the TS monorepo; the Go
  code sits in `apps/cli-go` as a frozen mirror. Its snapshot will not keep
  tracking the API on our behalf, and every `cli/pkg` bump we take (5 since
  Nov 2025) silently pulls whatever snapshot the CLI happened to have.
- `cli/pkg` only ever covered v1. The v2 Management API (`/api/v2-json`) is not
  reachable from the provider at all today.
- The TS CLI's `packages/api` solves this for JS: fetch both documents, merge
  them into one, patch, commit a snapshot, generate, verify in CI, and let a
  scheduled workflow open a PR when upstream drifts. We want the same loop in
  Go, with oapi-codegen, which is what the Go CLI already used, so the
  generated API surface stays familiar.
- Bonus: dropping `cli/pkg` removes a module whose own `go.mod` carries 49
  direct requirements (pgx, viper, grpc, docker-units, ...) from our graph.

## 2. Goals and non-goals

Goals

- This repo owns the client: v1 and v2 spec snapshots, overlay, generator
  config, merge tool and generated code all live here.
- One generated package covering both API versions, so a resource can call
  `client.V1GetProjectWithResponse(...)` and
  `client.V2GetProjectConfigWithResponse(...)` on the same client.
- `go generate ./...` rebuilds the client from the committed snapshots with no
  network access. The existing CI `generate` job then fails whenever generated
  code and snapshots disagree.
- The snapshots can be refreshed from any source: production, staging, a
  locally running branch of the management API, or spec files on disk.
- A scheduled workflow refreshes both snapshots from production and opens a PR
  on drift.
- No behaviour change for provider users.

Non-goals, for now

- Publishing a reusable Go SDK for the Management API. If that is wanted it
  should be its own module, not an `internal` package of this provider.
- Any change to the retry transport or request editors in `provider.go`.
- New resources on top of v2. This change makes v2 callable; it does not add
  resources that use it.

## 3. What the dry run showed

| Check | Result |
|---|---|
| oapi-codegen v2.4.1 (the CLI's version) on today's v1 spec | Fails to load the spec: `exclusiveMinimum` is now numeric. Upgrading the generator is mandatory. |
| oapi-codegen v2.8.2, v1 only, no overlay | Generates, but the package does not compile (2 errors, see 5.3). |
| v2.8.2, v1 only, with the old Go CLI overlay | Package compiles. |
| v2.8.2, v1 and v2 merged, same overlay | Package compiles. No v2-specific overlay entry needed today. |
| Shape of the merge | v1: 116 paths, 171 operations, 149 schemas. v2: 37 paths, 53 operations, 38 schemas. No path or schema name appears in both. All v2 operation ids start with `v2-`. |
| Provider after swapping the import | 62 undefined symbols, all mechanical renames (see 5.5 and Appendix A). |
| After the renames | 2 genuine spec-drift edits and one overlay entry left (see 5.6). |
| After those fixes, v1-only package | `go build`, `go vet`, golangci-lint v2.15.0 (0 issues) and the terraform-free unit tests all pass. |
| Same provider code, merged v1+v2 package | `go build`, `go vet` and the same unit tests pass with no further edits. Adding v2 changes no v1 name. |
| Final layout, `internal/openapi` inputs and `internal/gen/api` output | `go generate ./internal/openapi/...` runs merge and both codegen steps in 4 s. Output lands in `internal/gen/api` and is byte-identical to the single-directory run. Build, vet and unit tests pass with the new import path. |
| `reveal` / `enabled` pinned to boolean by the overlay (decision 11) | Two overlay actions regenerate the params as `*bool` / `bool`. Provider code exactly as on `main` compiles against it and the unit tests pass. No v2 parameter has either name, so the wildcard target is safe. |
| Binary size | 30.58 MB today. 31.82 MB with v1 only. 34.48 MB with v1 and v2. |
| Generated line count, v1+v2 | `client.gen.go` 52.7k, `types.gen.go` 25.7k. 73 `V2...WithResponse` methods. |
| Go merge tool prototype (Appendix C) | 70 lines on `gopkg.in/yaml.v3`. Deterministic output. Generated code is byte-identical to a JSON merge done the TS CLI's way. Rejects a conflicting duplicate path, accepts an identical one. |
| `go.mod` / `go.sum` | +14 / +103 lines for the generator as a Go tool; `cli/pkg` gone; `gopkg.in/yaml.v3` promoted from indirect to direct. |
| YAML vs JSON snapshot | Byte-identical generated code. |
| Overlay and output paths in the config | Resolved relative to the working directory, not the config file. |

Not verified locally: the acceptance tests (`TF_ACC=1`, gock-mocked). There is
no terraform binary on this machine. CI runs them on the PR. The path-based
lint exclusion in 5.2 was not run either; the header-based exclusion that
already exists was.

## 4. Design decisions

Each item lists the recommendation first, then the alternative considered.

1. **Generator: oapi-codegen v2.8.2, pinned with a go.mod `tool` directive.**
   The old Go CLI did exactly this (`//go:generate go tool oapi-codegen ...`).
   `go generate` then works on any machine with Go 1.24+, no separate install,
   and the version is in `go.mod`. v2.8.2 requires Go 1.25; `.go-version` is
   1.26.10. Alternative: `go run ...@v2.8.2` in the directive. Works too, but
   the pin is hidden in a comment and nothing audits it.
2. **One package from a merged v1+v2 document.** oapi-codegen takes a single
   input, so a small in-repo tool (`internal/openapi/mergespec`, Appendix C)
   unions `paths`, `components.schemas` and `components.securitySchemes` from
   the two snapshots into one document, with the TS CLI's validation rules: a
   path or schema that appears in both documents must be identical, operation
   ids must be unique, and an operation id must start with its path's version
   segment. The provider keeps a single `*api.ClientWithResponses`, so
   `configure_client.go` and every resource's `Configure` stay as they are.
   Alternative: two packages, `internal/gen/api` and `internal/gen/apiv2`,
   each with its own config and overlay, no merge tool. That needs a second
   client threaded through provider data into all 12 resources and data
   sources, two overlays to keep in step, and two sets of config, for the
   benefit of avoiding 70 lines of Go.
3. **Generated code in `internal/gen/api`, hand-maintained inputs in
   `internal/openapi`.** The `gen` tree holds nothing but generator output,
   so the path alone says "do not edit", and guards can be path-based
   (`.gitattributes`, lint exclusions) instead of relying on the
   `Code generated ... DO NOT EDIT` header. The inputs directory holds the
   snapshots, overlay, configs, the `go:generate` directives, the merge tool
   and the README. This is the split the TS CLI uses (`scripts/` in,
   `src/generated/` out) and the common `internal/gen` convention in Go
   services. The import path ends in `api`, so provider code still reads
   `api.ClientWithResponses`. Alternatives: one directory holding both inputs
   and output (my first draft; a `gen` name would then be misleading, and an
   `api` name hides that it is generated), or a root-level `api/` for the
   inputs as the Go CLI and the project-layout convention do. Root `api/`
   would also work; `internal/openapi` keeps the repo root uncluttered and
   nothing outside the provider consumes the specs.
4. **Inputs next to the directives, output one hop away.** `go generate`
   runs in the package directory of the file holding the directives, and
   oapi-codegen resolves every path relative to the working directory. With
   the directives in `internal/openapi/generate.go`, the overlay, the merged
   document and the config files are bare filenames, and the only path that
   crosses directories is `output: ../gen/api/<file>.gen.go`. `internal/openapi`
   is a Go package with no code, which is fine; `generate.go` gives it a
   package clause and a doc comment.
5. **The merged document is an ignored intermediate, not a committed file.**
   The merge runs as the first `go:generate` step and writes
   `openapi.merged.yaml` next to the snapshots; `.gitignore` excludes it. The
   snapshots are what reviewers read in a sync PR, and the generated code is
   what CI diffs. Committing the merged document as well would add a third
   large YAML diff to every sync PR with no new information. The TS CLI commits
   only the merged document; we commit only the inputs.
6. **Snapshot format: YAML from `/api/v1-yaml` and `/api/v2-yaml`.**
   Line-oriented, so the diff in a sync PR reads as the upstream API change.
   Output is byte-identical to generating from the JSON endpoints. The TS CLI
   commits JSON; the Go CLI committed YAML.
7. **Default snapshot source: production. Any source on demand.** The
   scheduled sync reads `https://api.supabase.com`, because provider users
   run against production and a release should not reference fields that only
   staging accepts. The Makefile target takes the two spec locations as
   variables, each a URL or a local file, with a base-URL shorthand on top. So
   the same command refreshes from staging (`api.supabase.green`), from a
   branch of the management API running locally (the Go CLI's docs used
   `http://127.0.0.1:8080/api/v1-yaml`), or from spec files a branch's build
   produced. The Go CLI tracked staging by default; the TS CLI tracks
   production and only offers the base-URL override. Decided. How a branch
   produces the documents, by serving them locally or by emitting the two
   files in its build, is on the API repo's side and was not checked.
8. **Overlay: port the Go CLI's `apps/cli-go/api/overlay.yaml` as is.** It is
   an OpenAPI Overlay document that oapi-codegen applies natively
   (`output-options.overlay.path`) to the merged document, so one file covers
   both versions. Two of its entries are required for the package to compile
   at all (5.3); the rest neutralise shapes the generator handles badly. The
   TS CLI's `openapi-overrides.json` is JSON Patch and mostly fixes response
   nullability for routes we do not call; nothing in it is needed by the
   provider today.
9. **Generator config: same as the Go CLI plus `always-prefix-enum-values`.**
   `models` and `client` in two files, `nullable-type: true` (the API key
   resource uses `nullable.Nullable`). The extra compatibility flag makes every
   enum constant `<Type><Value>` instead of only prefixing on collision. It
   renames 83 constants the provider does not use and nothing it does use, and
   it stops future syncs from renaming a constant just because another enum,
   now possibly in the other API version, gained the same value. Do not enable
   any other output option now; the goal of this change is a like-for-like
   swap. Decided: on.
10. **Keep the full client.** Pruning with `include-tags` or
    `include-operation-ids` would shrink the generated files but means editing
    the config every time a resource needs a new endpoint. The full v1+v2
    client costs 3.9 MB of binary over today; that is the lever to pull if
    size ever matters. Decided: keep the full client.
11. **Keep the `reveal` and `enabled` parameters boolean through the
    overlay.** Upstream changed them from `type: boolean` to `type: string`
    ("boolean string", accepting `true`, `1`, `yes`, `on` and so on). Two
    overlay actions (5.3) set them back to `type: boolean`, so the generated
    params stay `*bool` / `bool` and the seven call sites in the provider are
    untouched. A Go `true` encodes as `reveal=true`, which the API accepts, so
    nothing changes on the wire. Decided, against my recommendation to edit
    the call sites instead; the reason to prefer the overlay is that PR 1
    then changes no provider logic at all. The cost is that the overlay now
    also pins API shapes to what our code expects, so if upstream flips
    another boolean to a string the same treatment is needed, and a sync PR
    that touches a pinned parameter must be read with the overlay in mind.
12. **Two PRs.** The swap can land on its own and is verified end to end. The
    sync workflow needs GitHub App credentials this repo does not have;
    Christian adds them when PR 2 starts, and PR 1 does not wait for them
    (5.9).

## 5. Implementation

### 5.1 Layout

```
internal/openapi/           hand-maintained inputs
  README.md                 what this is, how to refresh, where the overlay applies
  generate.go               package doc + the three //go:generate lines
  v1-openapi.yaml           committed snapshot of https://api.supabase.com/api/v1-yaml
  v2-openapi.yaml           committed snapshot of https://api.supabase.com/api/v2-yaml
  openapi.merged.yaml       merge output, gitignored, rebuilt by go generate
  overlay.yaml              OpenAPI Overlay applied during codegen
  types.cfg.yaml            oapi-codegen config, models
  client.cfg.yaml           oapi-codegen config, client
  mergespec/
    main.go                 merges the snapshots into openapi.merged.yaml
    main_test.go            identical duplicate passes, conflicting duplicate fails,
                            operation id rules
internal/gen/api/           generated only, package api
  types.gen.go
  client.gen.go
.gitattributes              marks internal/gen/** and the snapshots linguist-generated
```

### 5.2 Config files

`internal/openapi/generate.go`

```go
// Package openapi holds the Management API OpenAPI snapshots (v1 and v2), the
// overlay and the oapi-codegen configuration that produce internal/gen/api.
// Run `go generate ./internal/openapi/...` after changing any of them; see
// README.md for how to refresh the snapshots.
package openapi

//go:generate go run ./mergespec openapi.merged.yaml v1-openapi.yaml v2-openapi.yaml
//go:generate go tool oapi-codegen -config types.cfg.yaml openapi.merged.yaml
//go:generate go tool oapi-codegen -config client.cfg.yaml openapi.merged.yaml
```

`go generate` runs the directives of one file in order, so the merge always
precedes codegen. The generated package needs no hand-written file: oapi-codegen
emits the package clause and doc comment itself.

`internal/openapi/types.cfg.yaml`

```yaml
package: api
generate:
  models: true
compatibility:
  always-prefix-enum-values: true
output-options:
  nullable-type: true
  overlay:
    path: overlay.yaml
output: ../gen/api/types.gen.go
```

`internal/openapi/client.cfg.yaml`

```yaml
package: api
generate:
  client: true
compatibility:
  always-prefix-enum-values: true
output-options:
  overlay:
    path: overlay.yaml
output: ../gen/api/client.gen.go
```

`.gitattributes` (new file)

```
internal/gen/** linguist-generated=true
internal/openapi/v1-openapi.yaml linguist-generated=true
internal/openapi/v2-openapi.yaml linguist-generated=true
```

`.gitignore` addition

```
internal/openapi/openapi.merged.yaml
```

`.golangci.yml` addition, under both `linters.exclusions.paths` and
`formatters.exclusions.paths`. The existing `generated: lax` setting already
skips the files by their header (verified); this makes the intent visible in
the config and survives a generator that drops the header.

```yaml
      - ^internal/gen/
```

`GNUmakefile` addition. Each spec location is a URL or a local path; curl
handles both with the `file://` scheme, so a file works the same way as a URL.
The base-URL shorthand covers the common cases.

```make
SUPABASE_API_URL ?= https://api.supabase.com
API_SPEC_V1 ?= $(SUPABASE_API_URL)/api/v1-yaml
API_SPEC_V2 ?= $(SUPABASE_API_URL)/api/v2-yaml

# Refresh the committed OpenAPI snapshots and regenerate the client.
#   make update-api-spec                                        production
#   make update-api-spec SUPABASE_API_URL=https://api.supabase.green   staging
#   make update-api-spec SUPABASE_API_URL=http://127.0.0.1:8080        a branch of the API running locally
#   make update-api-spec API_SPEC_V1=file:///path/v1.yaml API_SPEC_V2=file:///path/v2.yaml
.PHONY: update-api-spec
update-api-spec:
	curl -fsSL "$(API_SPEC_V1)" -o internal/openapi/v1-openapi.yaml
	curl -fsSL "$(API_SPEC_V2)" -o internal/openapi/v2-openapi.yaml
	go generate ./internal/openapi/...
```

### 5.3 Overlay

Copy `apps/cli-go/api/overlay.yaml` from the `supabase/cli` repo at tag
`v2.123.0-beta.17` (last Go release) into `internal/openapi/overlay.yaml`. It
applies to the merged document, so its wildcard targets cover v2 too. The
entries that matter most:

- `services` query parameter of `v1-get-services-health`: upstream models it
  as `anyOf [comma-separated string, array of enum]`. Without the overlay the
  generator emits a self-referential type (`V1GetServicesHealthParamsServices1`
  refers to itself) and the package does not compile. The overlay replaces it
  with a plain array of enums, which is also the shape the provider already
  relies on (`api.V1GetServicesHealthParamsServices`).
- Deprecated `connectionString` fields next to `connection_string`: both
  normalise to the Go field `ConnectionString`, so
  `SupavisorConfigResponse_Output` gets a duplicate field. The overlay removes
  the deprecated one.
- `password_required_characters` enum values that contain quotes and
  backslashes, SAML `attribute_mapping` unions, `name_id_format` duplicate
  enum values, null-only `release_channel` / `postgres_engine` on
  `V1CreateProjectBody`, and the JIT access response: all shapes oapi-codegen
  either rejects or turns into unusable types. None of these endpoints are
  called by the provider, but keeping the overlay complete means the whole
  package always compiles, so a future resource never has to debug this again.

Today's v2 document needs no overlay entry of its own.

Append two actions of our own, the only additions over the Go CLI file. They
implement decision 11 and were verified in the dry run: the generated params
come out as `*bool` / `bool`, and provider code as on `main` compiles and
passes its unit tests against them. The wildcard target is safe today because
no other parameter in either document is named `reveal` or `enabled`; the
merge tool's conflict check does not cover this, so a sync PR that adds a
parameter with one of these names needs the target narrowed by path.

```yaml
- target: $.paths.*.*.parameters[?(@.name=='reveal')].schema
  description: Upstream documents reveal as a "boolean string"; keep the Go parameter a bool, which encodes as the same true/false on the wire
  update:
    type: boolean
- target: $.paths.*.*.parameters[?(@.name=='enabled')].schema
  description: Same for the legacy API keys enabled flag
  update:
    type: boolean
```

Add a comment at the top of the file saying where it came from, which actions
are ours, and that each action must keep its `description`.

### 5.4 Tool pinning

```sh
go get -tool github.com/oapi-codegen/oapi-codegen/v2/cmd/oapi-codegen@v2.8.2
go mod tidy
```

This adds `tool github.com/oapi-codegen/oapi-codegen/v2/cmd/oapi-codegen`, a
`// indirect` requirement on `oapi-codegen/v2 v2.8.2`, and 13 indirect
requirements for its parser stack (kin-openapi, speakeasy openapi, jsonpath,
yaml). They are not linked into the provider binary. govulncheck's `./...`
scan does not reach them either, since no package imports them.

`mergespec` imports `gopkg.in/yaml.v3`, which is already an indirect
requirement; `go mod tidy` promotes it to direct.

Bumping the generator later is a deliberate step, not a Dependabot one:
`go get -tool ...@vX.Y.Z`, `go generate ./internal/openapi/...`, review the
diff, build, test. I have not confirmed whether Dependabot opens PRs for a
`// indirect` tool requirement; if it does, the `generate` CI job will fail on
that PR until someone regenerates, which is the right outcome.

### 5.5 Mechanical renames in `internal/provider`

Change the import in all 27 files:

```
github.com/supabase/cli/pkg/api  ->  github.com/supabase/terraform-provider-supabase/internal/gen/api
```

Then rename 62 symbols. They fall into two groups, both independent of the
generator version and unaffected by adding v2:

- **The spec added an `_Output` suffix to response schemas.** `SecretResponse`
  is now `SecretResponse_Output`, which oapi-codegen renders as
  `SecretResponseOutput`. Every derived name moves with it:
  `V1ProjectWithDatabaseResponseStatusACTIVEHEALTHY` becomes
  `V1ProjectWithDatabaseResponseOutputStatusACTIVEHEALTHY`. 57 of the 62.
- **Bare enum constants are now type-prefixed.** `ACTIVEHEALTHY`, `COMINGUP`,
  `UNHEALTHY` (service health status) and `Specific` (region selection type)
  were emitted without a prefix by v2.4.1. They are now
  `V1ServiceHealthResponseOutputStatusACTIVEHEALTHY` and friends. 4 of the 62.
- One union accessor method:
  `AsListProjectAddonsResponseSelectedAddonsVariantId0` becomes
  `AsListProjectAddonsResponseOutputSelectedAddonsVariantId0`.

Appendix A has the full table. Apply it with a perl script (BSD sed lacks
`\b`), then `gofmt -w internal/provider/*.go`, because longer identifiers
break the alignment of several composite literals in the tests.

What does not change: `api.NewClientWithResponses`, `api.WithHTTPClient`,
`api.WithRequestEditorFn`, `*api.ClientWithResponses`, and all 43
`...WithResponse` methods the provider calls. Operation ids are unchanged.
`provider.go` and `configure_client.go` only need the new import path.

### 5.6 Genuine spec drift (two edits and one overlay entry)

1. **`reveal` and `enabled` query parameters are now strings upstream.**
   Handled by the two overlay actions in 5.3, per decision 11. No provider
   code changes; the seven call sites in `apikey_resource.go`,
   `apikeys_data_source.go` and `project_resource.go` keep passing booleans.
2. **`StorageConfigResponse_Output.features` gained `purgeCache`.** The
   anonymous struct literal in `TestParseConfigNestedOmitempty`
   (`settings_resource_test.go`, around line 1134) needs the new field in the
   right position. Consider replacing that literal with a JSON fixture
   unmarshalled into the generated type, so the next field addition does not
   touch the test.
3. The union accessor rename from 5.5.

### 5.7 Dependency cleanup

`go mod tidy` drops `github.com/supabase/cli/pkg`. `oapi-codegen/runtime` and
`oapi-codegen/nullable` are already direct requirements and stay.

### 5.8 CI

Nothing to add for verification. The existing `generate` job in
`.github/workflows/test.yml` runs `go generate ./...` and fails on any diff,
which now covers `internal/gen/api/*.gen.go` against the committed snapshots.
It is hermetic: the merge is local and nothing looks at the live API. The
`build` job's golangci-lint run skips the generated files through the existing
`exclusions.generated: lax` setting (confirmed: 0 issues on the migrated tree)
plus the path exclusion from 5.2, and lints `mergespec` like any other
package. `go build ./...` and `go test ./...` cover `mergespec` too.

### 5.9 Sync workflow (second PR)

`.github/workflows/api-sync.yml`, modelled on the Go CLI's
`cli-go-api-sync.yml`:

- Triggers: daily cron (decided; the TS CLI runs hourly, the Go CLI ran
  daily), `workflow_dispatch`, and `repository_dispatch` type `api-sync`.
  Christian will ask the API team to dispatch to this repo, the way they did
  for `supabase/cli`, when it becomes useful; the trigger ships with the
  workflow so nothing changes on our side when they add us.
- Steps: checkout with `persist-credentials: false`, setup-go from
  `.go-version`, `make update-api-spec`, then check for changes:
  `git diff --exit-code --quiet internal/openapi/v1-openapi.yaml
  internal/openapi/v2-openapi.yaml` byte-for-byte (so a trailing-whitespace
  change upstream cannot be refetched and discarded forever) and
  `git diff --ignore-space-at-eol --exit-code --quiet internal/gen/api/`.
- On change: mint a GitHub App token with `actions/create-github-app-token`
  and open a PR with `peter-evans/create-pull-request` on branch
  `sync/api-spec` against `main`, title
  `chore(api): sync Management API OpenAPI spec`.
- Do not gate PR creation on `go build`. A breaking upstream change should
  still produce the PR; the PR's own CI shows the breakage, and that is the
  signal we want. A merge conflict between v1 and v2 (same schema name,
  different definition) fails `go generate` inside the workflow instead; that
  is a loud failure on the scheduled run, which is also what we want.
- Never auto-merge these PRs. The `_Output` rename above is exactly the kind
  of change that arrives through this path.

Prerequisite, owned by Christian and scheduled for the start of PR 2: PRs
opened with `GITHUB_TOKEN` do not trigger workflows, so the job needs an App
or PAT. `supabase/cli` uses a GitHub App via the `GH_APP_CLIENT_ID` variable
and `GH_APP_PRIVATE_KEY` secret. This repo has neither (variables list is
empty; secrets are `GPG_PRIVATE_KEY`, `PASSPHRASE`, `SUPABASE_ACCESS_TOKEN`,
`SUPABASE_API_ENDPOINT`). Install the same App on this repo and add both
values, or fall back to a fine-grained PAT with contents and pull-requests
write. PR 1 does not depend on any of this.

### 5.10 Docs

- `internal/openapi/README.md`: what the directory is, that `internal/gen/api`
  is output only, that `go generate` is hermetic, how to refresh from
  production, staging, a local branch of the API or files on disk (the
  Makefile examples in 5.2), how the two snapshots are merged and what the
  merge rejects, when to touch the overlay, and how to bump the generator.
- `CONTRIBUTING.md`: a short "Updating the Management API client" section
  pointing at that README. Note that `go generate` now also regenerates the
  client, not only the docs.

## 6. Order of work

PR 1, `chore: generate the Management API client in-repo`

1. Add `internal/openapi/` with both snapshots, the overlay including the two
   `reveal` / `enabled` actions from 5.3, configs, `generate.go`, `mergespec`
   and its tests, and the `.gitignore` entry.
2. `go get -tool ...@v2.8.2`, `go generate ./internal/openapi/...`, which
   creates `internal/gen/api/`.
3. Swap the import, apply Appendix A, `gofmt -w`.
4. The two edits from 5.6.
5. `go mod tidy`; confirm `cli/pkg` is gone.
6. `.gitattributes`, `.golangci.yml` exclusion, Makefile target, README,
   CONTRIBUTING.
7. `go build ./... && go vet ./... && go test ./... && go generate ./... && git diff --exit-code`.
8. Open the PR; CI runs the acceptance tests.

About half a day; the mechanical part is scripted and was exercised in the
dry run, and the merge tool is prototyped (Appendix C).

PR 2, `ci: open a PR when the Management API spec drifts`

1. Christian installs the GitHub App on the repo and adds `GH_APP_CLIENT_ID`
   and `GH_APP_PRIVATE_KEY` (5.9).
2. Add `api-sync.yml`.
3. Trigger it manually once to see a no-op run, then once against a stale
   snapshot to see a PR.
4. When useful, Christian asks the API team to add this repo to their
   `api-sync` dispatch list. Nothing in the workflow changes for that.

One to two hours.

## 7. Risks

- **Sync PRs can be red.** Upstream renames propagate into Go type names, as
  the `_Output` suffix just did. This is the intended behaviour: today the
  same change arrives silently inside a `cli/pkg` bump. Budget a little
  maintainer time per sync PR that touches something we use.
- **A v1/v2 name clash fails generation.** If upstream ever publishes the same
  schema name with different definitions in the two documents, `mergespec`
  refuses to merge and the sync run fails. The fix is an overlay-style rename
  in `mergespec` (prefix the v2 schema), which is a deliberate decision, not
  something to automate away. There is no clash today.
- **The overlay now pins two parameter types.** Decision 11 makes the overlay
  carry an opinion about the API, not only about what the generator can
  parse. If upstream turns another boolean parameter into a "boolean string",
  the provider's call sites break at compile time and the fix is one more
  overlay action. If upstream ever stops accepting `true`/`false` for these
  parameters, the overlay would hide it from the generator; the acceptance
  tests mock the query string, so that case would show up in a live run, not
  in CI.
- **Snapshot diffs are large.** The two YAML files total about 1.2 MB.
  `linguist-generated` collapses them in the PR view, but the reviewer of a
  sync PR should still open them: the snapshot diff is the upstream API
  change, and the generated diff is just its echo.
- **Generator bumps change output.** Every oapi-codegen upgrade must be
  followed by regeneration and a build. The `generate` CI job enforces this.
- **go.mod grows by the generator's parser stack.** 14 indirect requirements,
  none linked into the binary. Dependabot behaviour on them is unconfirmed.
- **Binary grows 3.9 MB.** 1.2 MB from the v2.8 client shape (getters and
  more helpers than v2.4), 2.7 MB from v2. The framework's reflection keeps
  every client method reachable, so the linker cannot drop the unused ones.
  `include-tags` pruning is the lever if this ever matters.

## 8. Reviewer decisions

All seven questions were answered by Christian on 2026-11-07 and folded into
the body. The answers stay here as the record; the "Folded into" lines say
where each one landed.

1. Production as the snapshot source, with staging as an opt-in override?
   Answer: Yes, but we want to make the source-of-truth even more configurable, 
   e.g. point to a branch of the management API. Not just live services
   Response: Folded into decision 7 and the Makefile target in 5.2: each spec 
   location is a variable that accepts a URL or a `file://` path, with the 
   base-URL shorthand for live services.
2. Happy to enable `always-prefix-enum-values`? It renames 83 constants the
   provider does not use and none that it does.
   Answer: Enable it.
   Folded into decision 9 and both config files in 5.2.
3. Daily sync cadence (the TS CLI runs hourly, the Go CLI ran daily)?
   Answer: Run daily
   Folded into the triggers in 5.9.
4. Who can install the GitHub App on this repo and add the two values?
   Answer: I can, but it should wait until step 2 where we hook up the 
   lifecycle. First step is just making the generator work.
   Folded into decision 12, the prerequisite in 5.9 and the PR 2 steps in
   section 6. PR 1 has no dependency on the credentials.
5. Should we ask the API team to add this repo to their `api-sync`
   `repository_dispatch` list?
   Answer: Yes, but I will do that manually when needed.
   Folded into the triggers in 5.9 and PR 2 step 4 in section 6. The
   `repository_dispatch` trigger ships with the workflow regardless.
6. Keep `reveal` / `enabled` as strings in provider code, or overlay them
   back to booleans? I recommend the code change.
   Answer: Convert them to booleans
   Read as: pin the generated parameters to boolean through the overlay so
   provider code is untouched. Confirmed by Christian. Folded into decision
   11, the two overlay actions in 5.3, item 1 of 5.6, the dry-run table in
   section 3 and a new entry in the risks.
7. Accept the 3.9 MB binary growth for the full v1+v2 client, or prune to the
   tags the provider uses? I recommend accepting it.
   Answer: Accept it.
   Folded into decision 10.

## Appendix A: symbol renames (old to new)

```
ACTIVEHEALTHY                                       V1ServiceHealthResponseOutputStatusACTIVEHEALTHY
COMINGUP                                            V1ServiceHealthResponseOutputStatusCOMINGUP
UNHEALTHY                                           V1ServiceHealthResponseOutputStatusUNHEALTHY
Specific                                            V1CreateProjectBodyRegionSelection0TypeSpecific
ApiKeyResponse                                      ApiKeyResponseOutput
ApiKeyResponseType                                  ApiKeyResponseOutputType
ApiKeyResponseTypeLegacy                            ApiKeyResponseOutputTypeLegacy
ApiKeyResponseTypePublishable                       ApiKeyResponseOutputTypePublishable
ApiKeyResponseTypeSecret                            ApiKeyResponseOutputTypeSecret
AuthConfigResponse                                  AuthConfigResponseOutput
BranchDetailResponse                                BranchDetailResponseOutput
BranchDetailResponseStatus                          BranchDetailResponseOutputStatus
BranchDetailResponseStatusACTIVEHEALTHY             BranchDetailResponseOutputStatusACTIVEHEALTHY
BranchDetailResponseStatusCOMINGUP                  BranchDetailResponseOutputStatusCOMINGUP
BranchDetailResponseStatusINITFAILED                BranchDetailResponseOutputStatusINITFAILED
BranchResponse                                      BranchResponseOutput
DeployFunctionResponse                              DeployFunctionResponseOutput
DeployFunctionResponseStatusACTIVE                  DeployFunctionResponseOutputStatusACTIVE
FunctionSlugResponse                                FunctionSlugResponseOutput
FunctionSlugResponseStatusACTIVE                    FunctionSlugResponseOutputStatusACTIVE
ListProjectAddonsResponseAvailableAddonsVariantsId0Ci16xlarge   ListProjectAddonsResponseOutputAvailableAddonsVariantsId0Ci16xlarge
ListProjectAddonsResponseAvailableAddonsVariantsId0CiMicro      ListProjectAddonsResponseOutputAvailableAddonsVariantsId0CiMicro
ListProjectAddonsResponseSelectedAddonsTypeComputeInstance      ListProjectAddonsResponseOutputSelectedAddonsTypeComputeInstance
NetworkBanResponse                                  NetworkBanResponseOutput
NetworkRestrictionsResponse                         NetworkRestrictionsResponseOutput
PostgresConfigResponse                              PostgresConfigResponseOutput
SecretResponse                                      SecretResponseOutput
SslEnforcementResponse                              SslEnforcementResponseOutput
StorageConfigResponse                               StorageConfigResponseOutput
SupavisorConfigResponse                             SupavisorConfigResponseOutput
SupavisorConfigResponseDatabaseTypePRIMARY          SupavisorConfigResponseOutputDatabaseTypePRIMARY
SupavisorConfigResponsePoolModeTransaction          SupavisorConfigResponseOutputPoolModeTransaction
ThirdPartyAuth                                      ThirdPartyAuthOutput
V1PostgrestConfigResponse                           V1PostgrestConfigResponseOutput
V1ProjectResponse                                   V1ProjectResponseOutput
V1ProjectWithDatabaseResponse                       V1ProjectWithDatabaseResponseOutput
V1ProjectWithDatabaseResponseStatus                 V1ProjectWithDatabaseResponseOutputStatus
V1ProjectWithDatabaseResponseStatusACTIVEHEALTHY    V1ProjectWithDatabaseResponseOutputStatusACTIVEHEALTHY
V1ProjectWithDatabaseResponseStatusACTIVEUNHEALTHY  V1ProjectWithDatabaseResponseOutputStatusACTIVEUNHEALTHY
V1ProjectWithDatabaseResponseStatusCOMINGUP         V1ProjectWithDatabaseResponseOutputStatusCOMINGUP
V1ProjectWithDatabaseResponseStatusGOINGDOWN        V1ProjectWithDatabaseResponseOutputStatusGOINGDOWN
V1ProjectWithDatabaseResponseStatusINACTIVE         V1ProjectWithDatabaseResponseOutputStatusINACTIVE
V1ProjectWithDatabaseResponseStatusINITFAILED       V1ProjectWithDatabaseResponseOutputStatusINITFAILED
V1ProjectWithDatabaseResponseStatusPAUSEFAILED      V1ProjectWithDatabaseResponseOutputStatusPAUSEFAILED
V1ProjectWithDatabaseResponseStatusPAUSING          V1ProjectWithDatabaseResponseOutputStatusPAUSING
V1ProjectWithDatabaseResponseStatusREMOVED          V1ProjectWithDatabaseResponseOutputStatusREMOVED
V1ProjectWithDatabaseResponseStatusRESIZING         V1ProjectWithDatabaseResponseOutputStatusRESIZING
V1ProjectWithDatabaseResponseStatusRESTARTING       V1ProjectWithDatabaseResponseOutputStatusRESTARTING
V1ProjectWithDatabaseResponseStatusRESTOREFAILED    V1ProjectWithDatabaseResponseOutputStatusRESTOREFAILED
V1ProjectWithDatabaseResponseStatusRESTORING        V1ProjectWithDatabaseResponseOutputStatusRESTORING
V1ProjectWithDatabaseResponseStatusUNKNOWN          V1ProjectWithDatabaseResponseOutputStatusUNKNOWN
V1ProjectWithDatabaseResponseStatusUPGRADING        V1ProjectWithDatabaseResponseOutputStatusUPGRADING
V1ServiceHealthResponse                             V1ServiceHealthResponseOutput
V1ServiceHealthResponseName                         V1ServiceHealthResponseOutputName
V1ServiceHealthResponseNameAuth                     V1ServiceHealthResponseOutputNameAuth
V1ServiceHealthResponseNameDb                       V1ServiceHealthResponseOutputNameDb
V1ServiceHealthResponseNameDbPostgresUser           V1ServiceHealthResponseOutputNameDbPostgresUser
V1ServiceHealthResponseNamePgBouncer                V1ServiceHealthResponseOutputNamePgBouncer
V1ServiceHealthResponseNamePooler                   V1ServiceHealthResponseOutputNamePooler
V1ServiceHealthResponseNameRealtime                 V1ServiceHealthResponseOutputNameRealtime
V1ServiceHealthResponseNameRest                     V1ServiceHealthResponseOutputNameRest
V1ServiceHealthResponseNameStorage                  V1ServiceHealthResponseOutputNameStorage

method: AsListProjectAddonsResponseSelectedAddonsVariantId0  ->  AsListProjectAddonsResponseOutputSelectedAddonsVariantId0
```

Apply with one `perl -pi` per line, longest names first, matching `\bapi\.OLD\b`.

## Appendix B: reproducing the dry run

```sh
# 1. Inputs, from the repo root
mkdir -p internal/openapi/mergespec internal/gen/api
curl -fsSL https://api.supabase.com/api/v1-yaml -o internal/openapi/v1-openapi.yaml
curl -fsSL https://api.supabase.com/api/v2-yaml -o internal/openapi/v2-openapi.yaml
git -C ../cli show v2.123.0-beta.17:apps/cli-go/api/overlay.yaml > internal/openapi/overlay.yaml
# ... generate.go, the two configs from 5.2, and Appendix C as mergespec/main.go ...

# 2. Pin the generator and generate
go get -tool github.com/oapi-codegen/oapi-codegen/v2/cmd/oapi-codegen@v2.8.2
go generate ./internal/openapi/...

# 3. Swap, rename, fix, verify
grep -rl 'supabase/cli/pkg/api' --include='*.go' . | xargs sed -i '' 's#github.com/supabase/cli/pkg/api#github.com/supabase/terraform-provider-supabase/internal/gen/api#g'
# ... Appendix A renames, the two edits from 5.6 ...
gofmt -w internal/provider/*.go
go mod tidy && go build ./... && go vet ./...
go test -count=1 -run '^Test(Build|Capped|Decode|GetOnly|IsData|Nullable|Parse|Resolve|Retry|Secret|SetThird|WaitFor)' ./internal/provider/
go run github.com/golangci/golangci-lint/v2/cmd/golangci-lint@latest run ./...
```

## Appendix C: merge tool prototype

This is the program exercised in the dry run, to be committed as
`internal/openapi/mergespec/main.go` with a test file. It mirrors the TS
CLI's `download-openapi.ts` rules: union `paths`, `components.schemas` and
`components.securitySchemes`; an entry present in both documents must be
identical; operation ids must be unique and start with the path's leading
segment. `info.title` is normalised to `Supabase API`; `servers` and `tags`
are dropped because the generator does not use them. `yaml.v3` marshals maps
with sorted keys, which is what makes the output deterministic.

```go
// mergespec merges the Management API v1 and v2 OpenAPI documents into one,
// so oapi-codegen can generate a single package from them.
package main

import (
	"fmt"
	"os"
	"reflect"
	"strings"

	"gopkg.in/yaml.v3"
)

func main() {
	if len(os.Args) < 4 {
		fmt.Fprintln(os.Stderr, "usage: mergespec OUT.yaml IN.yaml [IN.yaml...]")
		os.Exit(2)
	}
	out := os.Args[1]
	paths := map[string]any{}
	schemas := map[string]any{}
	security := map[string]any{}
	merged := map[string]any{
		"paths":      paths,
		"components": map[string]any{"schemas": schemas, "securitySchemes": security},
	}
	for i, in := range os.Args[2:] {
		raw, err := os.ReadFile(in)
		check(err)
		var doc map[string]any
		check(yaml.Unmarshal(raw, &doc))
		if i == 0 {
			merged["openapi"] = doc["openapi"]
			merged["info"] = map[string]any{"title": "Supabase API", "version": asMap(doc["info"])["version"]}
		}
		mergeInto(paths, asMap(doc["paths"]), in, "paths")
		comp := asMap(doc["components"])
		mergeInto(schemas, asMap(comp["schemas"]), in, "components.schemas")
		mergeInto(security, asMap(comp["securitySchemes"]), in, "components.securitySchemes")
	}
	seen := map[string]string{}
	for p, item := range paths {
		for method, op := range asMap(item) {
			opm, ok := op.(map[string]any)
			if !ok {
				continue
			}
			id, _ := opm["operationId"].(string)
			if id == "" {
				fail("%s %s has no operationId", method, p)
			}
			if prev, dup := seen[id]; dup {
				fail("duplicate operationId %q: %s and %s %s", id, prev, method, p)
			}
			seen[id] = method + " " + p
			seg := strings.SplitN(strings.TrimPrefix(p, "/"), "/", 2)[0]
			if !strings.HasPrefix(id, seg+"-") {
				fail("operationId %q does not start with the path's version segment %q (%s)", id, seg, p)
			}
		}
	}
	b, err := yaml.Marshal(merged)
	check(err)
	check(os.WriteFile(out, b, 0o644))
}

func mergeInto(dst, src map[string]any, file, what string) {
	for k, v := range src {
		if existing, ok := dst[k]; ok && !reflect.DeepEqual(existing, v) {
			fail("%s: %s/%s conflicts with an earlier document", file, what, k)
		}
		dst[k] = v
	}
}

func asMap(v any) map[string]any {
	m, _ := v.(map[string]any)
	return m
}

func check(err error) {
	if err != nil {
		fail("%v", err)
	}
}

func fail(format string, args ...any) {
	fmt.Fprintf(os.Stderr, "mergespec: "+format+"\n", args...)
	os.Exit(1)
}
```

References

- TS CLI package this mirrors: `supabase/cli` `packages/api/README.md`,
  `packages/api/scripts/download-openapi.ts` and
  `.github/workflows/api-package-sync.yml` on `develop`.
- Go CLI predecessor: `supabase/cli` tag `v2.123.0-beta.17`,
  `apps/cli-go/api/`, `apps/cli-go/pkg/api/*.cfg.yaml`,
  `.github/workflows/cli-go-api-sync.yml`, and the `codegen` job in
  `.github/workflows/cli-go-ci.yml`.
- oapi-codegen configuration schema:
  https://github.com/oapi-codegen/oapi-codegen/blob/v2.8.2/configuration-schema.json
