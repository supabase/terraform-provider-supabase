# Contributing to the Supabase Terraform provider

Thanks for your interest in contributing! This guide takes you from your first clone to a merged pull request, whether you're fixing a typo, chasing down a bug or adding support for a new resource.

If anything here is unclear or out of date, please let the maintainers know. A fix to this guide is just as welcome as a fix to the code.

## Table of contents

- [Your first contribution](#your-first-contribution)
- [Setting up](#setting-up)
- [Working on the provider](#working-on-the-provider)
- [Adding a resource or data source](#adding-a-resource-or-data-source)
- [Updating the docs](#updating-the-docs)
- [Review](#review)
- [Commit guidelines](#commit-guidelines)
- [Where to ask](#where-to-ask)
- [License](#license)

## Your first contribution

Most contributions start with an issue. Search the [existing issues](https://github.com/supabase/terraform-provider-supabase/issues) first, since someone may already be working on the same thing, and if nothing matches, open a new one that describes the bug or the feature. Mention that you'd like to work on it, so a maintainer can help you agree on an approach before you put time in. Small fixes like typos don't need an issue, and you can go straight to a pull request.

Once you know what you're building, fork the repository and clone your fork:

```shell
git clone https://github.com/<your-github-username>/terraform-provider-supabase.git
cd terraform-provider-supabase
```

Create a branch, make your change, and add or update tests that show it works. Before you push, run the same checks that CI runs:

```shell
make check
```

This lints the code, runs every test and regenerates the docs. If it changes anything in `docs/`, commit those changes along with your own.

Then push your branch and open a pull request. Give it a title in the [commit format](#commit-guidelines), link the issue in the description, and explain what changed and why. From there, a maintainer reviews it, and [Review](#review) explains what to expect.

Everyone taking part in this project follows the [Supabase code of conduct](https://github.com/supabase/.github/blob/main/CODE_OF_CONDUCT.md).

## Setting up

To work on the provider, you need [Go](https://go.dev/doc/install) 1.21 or later. You don't have to match the exact version in [`go.mod`](go.mod), because Go downloads it for you the first time you build.

You also need a few other tools:

- [make](https://www.gnu.org/software/make/) runs the common tasks. It comes with the Xcode command line tools on macOS, and with `build-essential` or your distribution's equivalent on Linux.
- [Terraform](https://developer.hashicorp.com/terraform/install) generates the docs and lets you try your build by hand.
- [golangci-lint](https://golangci-lint.run/welcome/install/) v2 lints the code.
- `jq` and `envsubst` help generate `docs/schema.json`. If your package manager doesn't list `envsubst` by name, install `gettext`, which includes it.

Use the latest releases of Terraform and golangci-lint, because CI uses them too, and older versions can produce slightly different results.

To see every task the Makefile offers, run `make` without any arguments.

## Working on the provider

The provider is built on the [Terraform Plugin Framework](https://developer.hashicorp.com/terraform/plugin/framework). Each resource calls the [Supabase Management API](https://supabase.com/docs/reference/api/introduction) through the generated Go client in [`github.com/supabase/cli/pkg/api`](https://pkg.go.dev/github.com/supabase/cli/pkg/api), so a resource can only use the endpoints that client exposes. If you're planning a feature, check that the endpoint you need is there before you start, and mention it in your issue if it isn't.

Most of your work happens in `internal/provider/`, where each resource lives in its own file, such as `apikey_resource.go`, with its tests next to it in `apikey_resource_test.go`. The Terraform snippets in `examples/` appear in the docs, and many tests use them as their configuration too. The [examples README](examples/README.md) explains how that works.

### Building and testing

Three commands cover most of what a change needs. `make build` builds the provider, `make test` runs the full test suite, and `make lint` checks the code against the rules in [`.golangci.yml`](.golangci.yml):

```shell
make build
make test
make lint
```

When the tests pass, the output of `make test` ends with a line similar to the following:

```
ok  	github.com/supabase/terraform-provider-supabase/internal/provider	12.561s
```

Every test mocks the Management API with [gock](https://github.com/h2non/gock), so you don't need a Supabase account or an access token, and the tests never create or change anything real.

Most tests are Terraform _acceptance tests_, which the Terraform testing framework skips unless the `TF_ACC` environment variable is set. `make test` sets it for you. If you run `go test` yourself, set `TF_ACC=1` too, because otherwise `go test` still reports success while running fewer than half the tests.

While you work, you can run only the tests you care about by passing extra flags through `TESTARGS`. For example, to run the API key resource tests with verbose output, run the following command:

```shell
make test TESTARGS="-run TestAccApiKeyResource -v"
```

### Trying your build with Terraform

Tests cover most changes, but sometimes you want to see your build handle a real configuration. Terraform's _development overrides_ point Terraform at the binary you built, instead of the release in the Terraform Registry.

To install your build into your Go bin directory, run the following command:

```shell
make install
```

Go puts the binary in `$(go env GOPATH)/bin`, or in `$(go env GOBIN)` if you've set it.

Next, create a file named `dev.tfrc` outside the repository with the following content:

```hcl
provider_installation {
  dev_overrides {
    "supabase/supabase" = "<go-bin-directory>"
  }
  direct {}
}
```

Replace the following:

- `<go-bin-directory>`: the absolute path of your Go bin directory, such as `/home/you/go/bin`.

To run Terraform with your build, run the following command from a directory with your configuration:

```shell
TF_CLI_CONFIG_FILE=/path/to/dev.tfrc terraform plan
```

Setting `TF_CLI_CONFIG_FILE` on the command itself, rather than exporting it, means the override applies only to that run, and your other Terraform work keeps using the published provider.

You can skip `terraform init` while the override is active. Terraform prints a warning that provider development overrides are in effect, which tells you it's using your build. After each code change, run `make install` again.

Your build talks to the real Management API with your access token, so `terraform apply` changes real projects. A dedicated test project is the safest place to experiment.

## Adding a resource or data source

A new resource touches a handful of places, and the easiest way to start is to copy an existing one. [`third_party_auth_resource.go`](internal/provider/third_party_auth_resource.go) is a good model, because it follows the patterns reviewers look for.

Start with the resource itself in `internal/provider/<name>_resource.go`, or `<name>_data_source.go` for a data source. It holds the schema, the CRUD methods and, for resources, `ImportState`. Give the resource and each attribute a `MarkdownDescription`, because that's the text users read in the docs.

Then register the constructor in `Resources()` or `DataSources()` in [`provider.go`](internal/provider/provider.go), so Terraform knows the resource exists.

Add an example in `examples/resources/supabase_<name>/resource.tf`, with an `import.sh` next to it that shows the import ID in the same style as the other resources. A data source gets `examples/data-sources/supabase_<name>/data-source.tf` instead. Embed the example in [`examples/examples.go`](examples/examples.go) and use it as the configuration in at least one test, so a broken example fails the tests before it reaches the docs.

Write the tests in `<name>_resource_test.go`, and mock every API call with gock. Shared API paths live in [`constants_test.go`](internal/provider/constants_test.go). Finally, regenerate the docs as described in the next section.

## Updating the docs

The pages in `docs/resources/` and `docs/data-sources/` are generated by [tfplugindocs](https://github.com/hashicorp/terraform-plugin-docs) from the `MarkdownDescription` fields in the code and the files in `examples/`. To change what a page says, change the description or the example, then regenerate the docs. To regenerate them, run the following command:

```shell
make generate
```

This rewrites the docs pages and `docs/schema.json`, and you commit the results with your change. CI checks that the committed docs match the code, so an edit made by hand to a generated page will fail a check.

When a page needs a section the generator can't produce, such as notes on how a resource behaves, add a template to `templates/`, the way [`edge_function_secrets.md.tmpl`](templates/resources/edge_function_secrets.md.tmpl) does. Use a template only when the description, the example and `import.sh` can't express what you need.

The Supabase docs site builds the [Terraform provider reference](https://supabase.com/docs/guides/deployment/terraform/reference) from `docs/schema.json`, and it republishes `README.md` and `docs/tutorial.md`, so write those three for Terraform users rather than contributors.

## Review

Every pull request is reviewed by the project maintainers or the wider Supabase SDK group, and needs an approval from one of them before it merges. You don't need to tag anyone, because GitHub requests reviewers automatically from [`CODEOWNERS`](.github/CODEOWNERS).

GitHub Copilot also reviews each pull request automatically. Its findings are often useful but not always right, so reply to each one, either with a fix or with a short note on why it doesn't apply. Maintainers read those threads during review.

If this is your first contribution to the repository, GitHub may wait for a maintainer to approve your CI run before it starts. That's a standard GitHub safeguard for new contributors.

If you'd like feedback on an approach before you finish, open a draft pull request and say what you're unsure about.

Most review comments come back to the same few patterns, such as how a resource handles a 404 or formats its import ID. [AGENTS.md](AGENTS.md) collects them for coding agents, and its [resource patterns](AGENTS.md#resource-patterns), [errors](AGENTS.md#errors) and [tests](AGENTS.md#tests) sections are just as useful to skim before you ask for review.

If you spot something else worth improving along the way, it's very welcome as its own pull request. Smaller pull requests are quicker to review.

### Working through review

Nobody expects a pull request to be perfect on the first pass. Most review comments are questions or suggestions, and reviewers mark the optional ones as a nit, so you're free to take those or leave them.

If you disagree with a comment or aren't sure what it means, reply and say so. Maintainers are happy to talk it through, and sometimes they're the ones who missed something.

When you push changes, add them as new commits rather than force-pushing, so reviewers can see what changed. Pull requests are squash merged, so the extra commits don't end up on `main`.

Please leave **Allow edits by maintainers** checked. It lets a maintainer tidy up the last few details when a release is close, and the work stays credited to you.

If you haven't heard back for a while, a comment on the pull request is a welcome nudge. A quiet review usually means maintainers are busy, never that your contribution isn't wanted.

## Commit guidelines

This repository uses [Conventional Commits](https://www.conventionalcommits.org/en/v1.0.0/), in the format `type(scope): description`. The scope is usually the resource or area you changed, and a `!` after the scope marks a breaking change.

Pull requests are squash merged, so the pull request title becomes the commit on `main`. For a pull request with a single commit, GitHub uses that commit's message instead, so it's easiest to make both follow the format.

| Good                                                       | Avoid                  |
| ---------------------------------------------------------- | ---------------------- |
| `fix(apikey): include description when creating keys`      | `Fixed the apikey bug` |
| `feat(branch): support declaration of persistent branches` | `updates`              |
| `feat(project)!: replace instance_size values`             | `fix stuff`            |

## Where to ask

If you're stuck or unsure about an approach, ask in your issue or pull request. Maintainers are happy to help at any stage.

If you've found a bug in Supabase itself rather than in the provider, please report it in the [Supabase repository](https://github.com/supabase/supabase/issues). For a security issue, follow the [security policy](https://github.com/supabase/terraform-provider-supabase/security/policy) instead of opening a public issue.

## License

See [LICENSE](LICENSE).

If you're ready to start, browse the [open issues](https://github.com/supabase/terraform-provider-supabase/issues) or open one for the change you have in mind. Maintainers cutting a release can follow [RELEASING.md](RELEASING.md).
