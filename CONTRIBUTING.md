# Terraform Provider Scaffolding (Terraform Plugin Framework)

_This template repository is built on the [Terraform Plugin Framework](https://github.com/hashicorp/terraform-plugin-framework). The template repository built on the [Terraform Plugin SDK](https://github.com/hashicorp/terraform-plugin-sdk) can be found at [terraform-provider-scaffolding](https://github.com/hashicorp/terraform-provider-scaffolding). See [Which SDK Should I Use?](https://developer.hashicorp.com/terraform/plugin/framework-benefits) in the Terraform documentation for additional information._

This repository is a _template_ for a [Terraform](https://www.terraform.io) provider. It is intended as a starting point for creating Terraform providers, containing:

- A resource and a data source (`internal/provider/`),
- Examples (`examples/`) and generated documentation (`docs/`),
- Miscellaneous meta files.

These files contain boilerplate code that you will need to edit to create your own Terraform provider. Tutorials for creating Terraform providers can be found on the [HashiCorp Developer](https://developer.hashicorp.com/terraform/tutorials/providers-plugin-framework) platform. _Terraform Plugin Framework specific guides are titled accordingly._

Please see the [GitHub template repository documentation](https://help.github.com/en/github/creating-cloning-and-archiving-repositories/creating-a-repository-from-a-template) for how to create a new repository from this template on GitHub.

Once you've written your provider, you'll want to [publish it on the Terraform Registry](https://developer.hashicorp.com/terraform/registry/providers/publishing) so that others can use it.

## Requirements

- [Terraform](https://developer.hashicorp.com/terraform/downloads) >= 1.0
- [Go](https://golang.org/doc/install) >= 1.20

## Building The Provider

1. Clone the repository
1. Enter the repository directory
1. Build the provider using the Go `install` command:

```shell
go install
```

## Adding Dependencies

This provider uses [Go modules](https://github.com/golang/go/wiki/Modules).
Please see the Go documentation for the most up to date information about using Go modules.

To add a new dependency `github.com/author/dependency` to your Terraform provider:

```shell
go get github.com/author/dependency
go mod tidy
```

Then commit the changes to `go.mod` and `go.sum`.

## Developing the Provider

If you wish to work on the provider, you'll first need [Go](http://www.golang.org) installed on your machine (see [Requirements](#requirements) above).

To compile the provider, run `go install`. This will build the provider and put the provider binary in the `$GOPATH/bin` directory.

To generate or update documentation, run `go generate`.

In order to run the full suite of Acceptance tests, run `make testacc`.

_Note:_ Acceptance tests create real resources, and often cost money to run.

```shell
make testacc
```

## Releasing the Provider

Releases are driven by git tags and require maintainer permissions. To cut a release, tag the tip of `main` with a new version following [semantic versioning](https://semver.org/) and push it:

```shell
git tag v1.10.0
git push origin v1.10.0
```

Pushing the tag triggers the [release workflow](.github/workflows/release.yml), which:

1. Builds and signs binaries for all supported platforms via [GoReleaser](https://goreleaser.com/), then creates a draft GitHub release with the binaries attached and a changelog generated from merged PR titles.
1. Waits for manual approval from a maintainer (under the workflow run's `publish` job).
1. Once approved, publishes the release. The [Terraform Registry](https://registry.terraform.io/providers/supabase/supabase/latest) picks up the new version automatically.

### Common pitfalls

- **Prefer pushing a tag over creating a release in the GitHub UI.** The workflow is only triggered by a tag being pushed. Saving a draft release in the UI doesn't create the tag, so nothing happens; publishing one does create the tag, but the release then goes live before any binaries exist. Pushing the tag directly lets the workflow create the release, and it stays a draft until the binaries are built and approved.
- **Each release needs its own fresh commit.** GoReleaser fails with a `422 already_exists` error when uploading assets if the tagged commit already carries a previously released tag (e.g., tagging `v1.10.0` on the same commit as an already-released `v1.10.0-alpha`). If the tip of `main` has already been released under another tag, land a new commit first (an empty commit via a PR works) and tag that.
