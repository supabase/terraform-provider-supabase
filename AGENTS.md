# AGENTS.md

Instructions for AI coding agents working in this repository. It collects what maintainers most often ask for in pull request review. If you're contributing by hand, [CONTRIBUTING.md](CONTRIBUTING.md) is the friendlier place to start.

## Repository map

- `internal/provider/`: all provider code, one file per resource or data source (`<name>_resource.go`, `<name>_data_source.go`) with tests alongside (`<name>_resource_test.go`).
- `internal/provider/provider.go`: provider config, HTTP client with GET retries, and the `Resources()` / `DataSources()` registration lists.
- `internal/provider/utils.go`: shared helpers (`NullableToString`, `waitForProjectActive`, `waitForServicesActive`, `waitForAuthServiceActive`).
- `internal/provider/constants_test.go`: shared test fixtures (`testProjectRef`, API paths like `thirdPartyAuthApiPath`).
- `examples/`: Terraform snippets used in the docs, embedded by `examples/examples.go` for tests.
- `templates/`: optional tfplugindocs templates.
- `docs/`: GENERATED. Never edit by hand.

Reference implementation: `internal/provider/third_party_auth_resource.go` and its test. Read both before writing a new resource or changing CRUD logic.

## Commands

| Task                                      | Command                                                                           |
| ----------------------------------------- | --------------------------------------------------------------------------------- |
| Everything required before finishing      | `make check`                                                                      |
| All tests                                 | `make test`                                                                       |
| Some tests                                | `make test TESTARGS="-run TestAccThirdPartyAuthResource -v"`                      |
| Lint                                      | `make lint`                                                                       |
| Regenerate `docs/` and `docs/schema.json` | `make generate`                                                                   |
| Find API client methods                   | `go doc github.com/supabase/cli/pkg/api ClientWithResponses \| grep -i <keyword>` |

- If you run `go test` directly, ALWAYS set `TF_ACC=1`. Without it, some of the tests are skipped and `go test` still prints `ok`.
- Tests need no credentials or network. Every API call is mocked with gock.
- `make check` may modify files under `docs/`. Commit those changes. CI fails when committed docs differ from generated ones.

## Workflow

1. Read the relevant resource file, its test, and the reference implementation.
2. Confirm every endpoint you need exists in `github.com/supabase/cli/pkg/api` (command above). If one is missing, STOP and report it. Do not hand-write HTTP calls and do not bump the client version.
3. Write or update a test that fails without your change. For bug fixes, reproduce the bug in a test first.
4. Make the change.
5. Run `make check` and fix every failure. Commit generated `docs/` changes.

## Scope

- Change only what the task asks for. Unrelated refactors, renames, formatting sweeps and drive-by fixes get rejected in review. Mention them in your summary instead.
- Do not touch `go.mod`/`go.sum` versions, `.github/`, `.goreleaser.yml`, `GNUmakefile` or release files unless the task is about them.
- Never edit `docs/resources/` or `docs/data-sources/` directly. Change the `MarkdownDescription` in Go, the example in `examples/`, or a template in `templates/`, then run `make generate`. Add a template only when a description, example and `import.sh` cannot express what's needed.

## Resource patterns

Follow `third_party_auth_resource.go`:

- Interface assertions at the top of the file, one per interface implemented:
  ```go
  var (
      _ resource.Resource                = &ThirdPartyAuthResource{}
      _ resource.ResourceWithImportState = &ThirdPartyAuthResource{}
  )
  ```
- `Configure` uses `extractClient(req.ProviderData, &resp.Diagnostics)`.
- Put API calls in helpers (`createX`, `readX`, `deleteX`) that return `diag.Diagnostics`. `readX` returns `(found bool, diags)`.
- `Read` handles out-of-band deletion. On 404, remove the resource from state instead of erroring or keeping old state:
  ```go
  if !found {
      resp.State.RemoveResource(ctx)
      return
  }
  ```
- `Delete` treats 404 as success.
- After create or update, set state from the API response or read the object back. Return an error if that fails.
- Never treat unhealthy or failed statuses as success.
- `RequiresReplace` on attributes where a change means a different object (always `project_ref`). Use `UseStateForUnknown` on computed attributes that don't change.
- Use sets for unique, unordered values.
- Import IDs are `<project_ref>/<id>`. Parse with `strings.SplitN(req.ID, "/", 2)`, reject empty parts, and document the format in `examples/resources/supabase_<name>/import.sh` matching the other resources.
- Offer `timeouts` only for operations that poll (`waitFor*` helpers), and pass the timeout into those helpers.

## Errors

- Every API error diagnostic names the operation, and the status and body when there is a response:
  ```go
  if err != nil {
      msg := fmt.Sprintf("Unable to read third-party auth integration, got error: %s", err)
      return false, diag.Diagnostics{diag.NewErrorDiagnostic("Client Error", msg)}
  }
  if httpResp.JSON200 == nil {
      msg := fmt.Sprintf("Unable to read third-party auth integration, got status %d: %s", httpResp.StatusCode(), httpResp.Body)
      return false, diag.Diagnostics{diag.NewErrorDiagnostic("Client Error", msg)}
  }
  ```
- Never swallow an error. Return a diagnostic, or log with `tflog` if it's genuinely non-fatal.
- In `Configure`, `ConfigValidators` and validators, collect all diagnostics before returning so users see every problem in one run.

## Code style

- Reuse and extend `utils.go` helpers instead of writing near-copies. The `dupl` linter flags duplicated blocks over 250 tokens.
- Use the API client's typed constants (`api.ApiKeyResponseTypeLegacy`, `api.V1ServiceHealthResponseNameAuth`), not string literals.
- Don't use deprecated API fields or removed enum values.
- Prefer the standard library (`slices.Contains`, `maps`, `strings`). Keep functions small and variables scoped to where they're used.
- Comments are full sentences ending with a period (`godot` linter).

## Tests

Test names start with `TestAcc` for anything using `resource.Test`. Shape:

```go
func TestAccThingResource_Case(t *testing.T) {
    defer gock.OffAll()

    gock.New(defaultApiEndpoint).
        Post(thingApiPath).
        AddMatcher(matchJSONBody(t, map[string]any{"name": "x"})).
        Reply(http.StatusCreated).
        JSON(response)
    // ...one mock per expected request

    resource.Test(t, resource.TestCase{
        PreCheck:                 func() { testAccPreCheck(t) },
        ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
        Steps: []resource.TestStep{ /* ... */ },
    })

    if !gock.IsDone() {
        t.Fatalf("unconsumed mocks: %v", gock.Pending())
    }
}
```

- Always use `testAccPreCheck` and `testAccProtoV6ProviderFactories`. They set a fake token and endpoint and route requests through `http.DefaultClient`, which gock intercepts.
- Mocks must fail if the provider sends the wrong thing. Assert request bodies on writes with `matchJSONBody` (in `third_party_auth_resource_test.go`). Use `Times(n)` with the exact count instead of `Persist()` where practical.
- GETs answering 408, 429 or 5xx are retried up to 3 times (see `getOnlyRetryPolicy` in `provider.go`). A failing GET mock needs `Times(4)`. Writes are never retried.
- gock is global. Never call `t.Parallel()` in a test that uses gock.
- One test case per edge case handled in code: empty values, invalid input, missing fields, 404 on read, 404 on delete.
- Import tests check every attribute in `ImportStateCheck`, not a subset.
- Use the embedded example (`examples.ThingResourceConfig`) as the config of at least one test. New examples must be added to `examples/examples.go`.
- Add new API paths and fixture IDs to `constants_test.go`.

## Adding a resource or data source

1. `internal/provider/<name>_resource.go` (or `_data_source.go`), modeled on the reference.
2. Register the constructor in `Resources()` or `DataSources()` in `provider.go`.
3. `examples/resources/supabase_<name>/resource.tf` and `import.sh` (data sources: `examples/data-sources/supabase_<name>/data-source.tf`).
4. Embed the example in `examples/examples.go`.
5. `internal/provider/<name>_resource_test.go`, following the test rules above.
6. `make check`, then commit the generated docs.

## Docs

- `MarkdownDescription` is user-facing docs. Write for a Terraform user who doesn't know the Management API: say where values come from in the Supabase dashboard, and give units for sizes and durations.
- `README.md`, `docs/tutorial.md` and `docs/schema.json` are republished on supabase.com. Write them for Terraform users, not contributors.

## Commits and pull requests

- Conventional Commits: `type(scope): description`, scope is the resource or area, such as `fix(settings): keep pooler settings on import`. `!` after the scope marks a breaking change. PRs are squash merged, so the PR title becomes the commit on `main`.
- PR description: link the issue, explain what changed and why, and note anything the reviewer should check by hand.
- Copilot reviews every PR automatically. Address each finding with a fix or a reply explaining why it doesn't apply.

## Security

Never put tokens or real project refs in code, tests or examples. Report security problems through the [security policy](https://github.com/supabase/terraform-provider-supabase/security/policy), not in public issues or PRs.
