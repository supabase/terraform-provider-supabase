# AGENTS.md

Instructions for AI coding agents working in this repository. It collects what maintainers most often ask for in pull request review. If you're contributing by hand, [CONTRIBUTING.md](CONTRIBUTING.md) is the friendlier place to start.

## Repository map

- `internal/provider/`: all provider code, one file per resource or data source (`<name>_resource.go`, `<name>_data_source.go`) with tests alongside (`<name>_resource_test.go`).
- `internal/provider/provider.go`: provider config, HTTP client with GET retries, and the `Resources()` / `DataSources()` registration lists.
- `internal/provider/utils.go`: shared helpers (`NullableToString`, `waitForProjectActive`, `waitForServicesActive`, `waitForAuthServiceActive`).
- `internal/errs/`: typed errors shared across resources (`NewNotFoundErr`, `NotFound`).
- `internal/validators/`: Terraform Plugin Framework validators shared across resources, such as `URL`. Check `terraform-plugin-framework-validators` first, and add a validator here only when it has no equivalent.
- `internal/planmodifiers/`: plan modifiers shared across resources, such as `RequiresReplaceIfJSONChanged`. Check the framework's built-in modifiers (`stringplanmodifier` and others) first.
- `internal/jsonvalue/`: helpers for attributes whose values are JSON strings, such as `Equal`, which compares two documents ignoring formatting.
- `internal/provider/constants_test.go`: shared test fixtures (`testProjectRef`, `testThirdPartyAuthUUID`) and the API paths the remaining gock tests use.
- `internal/provider/provider_test.go`: provider factories for tests, `testAccPreCheck` and `offlineProviderConfig`.
- `internal/provider/sweep_test.go`: `sweeperClient()`, an API client for live tests that need to call the API directly, and the sweepers that clean up leftover test projects.
- `internal/provider/resource_test_helpers_test.go`: helpers for unit tests that call resource methods directly (`testResourceSchema`, `testObjectValue`).
- `internal/provider/test-fixtures/`: Terraform configurations for the live acceptance tests.
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
| Live acceptance tests, in parallel        | `make testacc` (needs `SUPABASE_ACCESS_TOKEN` and `ORGANIZATION_ID`)              |
| Lint                                      | `make lint`                                                                       |
| Regenerate `docs/` and `docs/schema.json` | `make generate`                                                                   |
| Find API client methods                   | `go doc github.com/supabase/cli/pkg/api ClientWithResponses \| grep -i <keyword>` |

- If you run `go test` directly, ALWAYS set `TF_ACC=1`. Without it, some of the tests are skipped and `go test` still prints `ok`.
- Live acceptance tests run against the Management API and create and delete real projects. They are skipped unless `ORGANIZATION_ID` is set, so don't set it unless you mean to run them. Unit tests and the remaining gock-based `TestAcc*` tests need no credentials or network.
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
- Put API calls in helpers (`createX`, `readX`, `deleteX`) that return an `error`. The CRUD method turns it into a diagnostic with `resp.Diagnostics.AddError("Client Error", err.Error())`.
- `readX` returns `errs.NewNotFoundErr("<kind>", id)` from `internal/errs` on 404, so callers can tell "gone" from "failed".
- `Read` handles out-of-band deletion. On 404, remove the resource from state instead of erroring or keeping old state:
  ```go
  err := readThirdPartyAuth(ctx, &data, r.client)
  if errs.NotFound(err) {
      resp.State.RemoveResource(ctx)
      return
  }
  ```
- `deleteX` treats 404 as success and returns `nil`.
- After create or update, set state from the API response or read the object back. Return an error if that fails.
- Never treat unhealthy or failed statuses as success.
- `RequiresReplace` on attributes where a change means a different object (always `project_ref`). Use `UseStateForUnknown` on computed attributes that don't change. For JSON attributes, use `planmodifiers.RequiresReplaceIfJSONChanged()` so formatting-only changes don't replace the resource.
- Use sets for unique, unordered values.
- Import IDs are `<project_ref>/<id>`. Parse with `strings.SplitN(req.ID, "/", 2)`, reject empty parts with an "Invalid Import ID" error, and document the format in `examples/resources/supabase_<name>/import.sh` matching the other resources. `ImportState` only sets the IDs; Terraform calls `Read` afterwards, which fills in the rest and reports an object that doesn't exist.
- Don't repeat checks the config validators already make in `Create` or the request builders.
- Offer `timeouts` only for operations that poll (`waitFor*` helpers), and pass the timeout into those helpers. Several waits in one operation share one deadline.

## Errors

- Every API error names the operation, and the status and body when there is a response. Error strings start lowercase (`staticcheck` ST1005):
  ```go
  if err != nil {
      return fmt.Errorf("unable to create third-party auth integration, got error: %w", err)
  }
  if httpResp.JSON201 == nil {
      return fmt.Errorf("unable to create third-party auth integration, got status %d: %s", httpResp.StatusCode(), httpResp.Body)
  }
  ```
- Never swallow an error. Return it, or log with `tflog` if it's genuinely non-fatal.
- In `Configure`, `ConfigValidators` and validators, collect all diagnostics before returning so users see every problem in one run.

## Code style

- Reuse and extend `utils.go` helpers instead of writing near-copies. The `dupl` linter flags duplicated blocks over 250 tokens.
- Use the API client's typed constants (`api.ApiKeyResponseTypeLegacy`, `api.V1ServiceHealthResponseNameAuth`), not string literals.
- Don't use deprecated API fields or removed enum values.
- Prefer the standard library (`slices.Contains`, `maps`, `strings`). Keep functions small and variables scoped to where they're used.
- Comments are full sentences ending with a period (`godot` linter).

## Tests

A resource has two kinds of tests. `third_party_auth_resource_test.go` shows both.

**Acceptance tests** (`resource.Test`, `resource.ParallelTest`) check the resource end to end against the live Management API:

- Name them `TestAcc<Name>Resource_<case>`, skip them unless `ORGANIZATION_ID` is set, and use `testAccProtoV6ProviderFactoriesActual`.
- Use `resource.ParallelTest` unless the resource can't be tested in parallel; say why in a comment if not.
- Keep configurations in `test-fixtures/<Name>/` and pass values with `ConfigVariables`. Each test creates the project it needs.
- Cover create, every attribute in state, import with `ImportStateVerify`, replacement on changes that require it, and deletion outside Terraform (delete the object with `sweeperClient()` in a step's `PreConfig`, then expect a plan that creates it again).
- Import steps use `ImportStateVerify`, which compares every attribute, not a subset. List an attribute in `ImportStateVerifyIgnore` only when the API can't return it, such as a write-only secret or a local file path, and say why in a comment.
- The one acceptance test that doesn't call the API plans the embedded example: `Config: offlineProviderConfig + examples.<Name>ResourceConfig` with `PlanOnly: true` and `ExpectNonEmptyPlan: true`. `offlineProviderConfig` points the provider at a closed local port so the test can never reach a real API. This only works when planning makes no API calls: not for data sources, or for resources whose `ModifyPlan` reads from the API. Skip it there.

**Unit tests** cover everything that isn't an integration test: validators, plan modifiers, import ID parsing, request builders, response handling, and `Update` when it makes no API call.

- Make them plain Go tests that call the code directly and use `t.Parallel()`, usually as table tests keyed by case name. Don't start their names with `TestAcc`: that prefix is for tests that run through `resource.Test` or `resource.ParallelTest`.
- To call resource methods such as `ImportState` or `Update`, build the request with `testResourceSchema` and `testObjectValue`. Pass a nil client when the method shouldn't call the API.
- If a unit test needs HTTP, use `httptest.NewServer` with a client pointed at `server.URL`. The handler checks the method, path and body of every request and fails the test on anything unexpected, so the test catches the provider sending the wrong thing. GETs that get a 408, 429 or 5xx are retried up to 3 times (`getOnlyRetryPolicy` in `provider.go`), so a handler that fails a GET is called four times; writes aren't retried. Don't combine a real server with `testing/synctest`: keep-alive connections block fake time and the test hangs.

**gock** is being actively phased out. Many resources still have offline `TestAcc*` tests that mock the API with gock; they're not examples to follow. Don't add gock mocks or copy those tests. When you change a resource that's tested with gock, cover your change with a unit test or a live acceptance test. Touch an existing gock test only as much as your change requires, or replace it with the live and unit tests above. If you do edit one: gock intercepts `http.DefaultClient` for the whole process, so those tests can't call `t.Parallel()`; it matches paths as regular expressions, so register mocks for sub-paths like `/health` before the project mock; and a failing GET mock needs `Times(4)` because of the retries. `testAccPreCheck` sets environment variables with `t.Setenv`, so a test that uses it can't call `t.Parallel()`.

Also:

- One test case per edge case handled in code: empty values, invalid input, missing fields, 404 on read, 404 on delete.
- Use the embedded example (`examples.<Name>ResourceConfig`) as the config of at least one test. New examples must be added to `examples/examples.go`.
- Add shared fixture IDs to `constants_test.go`.

## Adding a resource or data source

1. `internal/provider/<name>_resource.go` (or `_data_source.go`), modeled on the reference.
2. Register the constructor in `Resources()` or `DataSources()` in `provider.go`.
3. `examples/resources/supabase_<name>/resource.tf` and `import.sh` (data sources: `examples/data-sources/supabase_<name>/data-source.tf`).
4. Embed the example in `examples/examples.go`.
5. `internal/provider/<name>_resource_test.go` and `test-fixtures/<Name>/`, following the test rules above.
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
