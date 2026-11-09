# Contributing to the Supabase Provider

This file holds the provider-specific guidance for working in this repository.
General, organization-wide contribution policy lives in our [shared `.github` repository](https://github.com/supabase/.github)'s CONTRIBUTING.md file.

## Requirements

- [Terraform](https://developer.hashicorp.com/terraform/downloads) >= 1.0
- [Go](https://go.dev/doc/install), at the version pinned in `.go-version` (see [Go version policy](#go-version-policy))

### Go version policy

This provider is built with the Go version pinned in `.go-version`, which trails the latest Go release by one major version (the `y` in Go's `x.y.z`).
When a new Go major release ships, we move to the previous one; for example, when 1.28 is released we move to 1.27.
Within the pinned series we adopt patch releases as they are released.

Currently, this means that Go **1.26** is the supported version for developing and building this project.

The `go` directive in `go.mod` is only the minimum language version and may be lower than the pinned version.

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

If you wish to work on the provider, you'll first need [Go](https://go.dev/doc/install) installed on your machine (see [Requirements](#requirements) above).

To compile the provider, run `go install`. This will build the provider and put the provider binary in the `$GOPATH/bin` directory.

To generate or update documentation, run `go generate`.

In order to run the full suite of Acceptance tests, run `make testacc`.

_Note:_ Acceptance tests create real resources, and often cost money to run.

```shell
make testacc
```

## Releasing the Provider

Releasing requires maintainer permissions, and nothing goes live until a manual approval at the very end.

1. Create a draft release with a new tag following [semantic versioning](https://semver.org/) (e.g., `v1.10.0`) and auto-generated release notes:

   ```shell
   gh release create v1.10.0 --draft --generate-notes
   ```

   Edit the notes as needed via the link the command prints (or `gh release edit`). You can also draft the release from the [GitHub releases page](https://github.com/supabase/terraform-provider-supabase/releases/new) instead — if you do, keep the release title identical to the tag name, since GoReleaser locates the draft by its title.
1. Drafting a release does _not_ create the git tag, so create and push it yourself:

   ```shell
   git tag v1.10.0
   git push origin v1.10.0
   ```

1. Pushing the tag triggers the [release workflow](.github/workflows/release.yml): [GoReleaser](https://goreleaser.com/) builds and signs binaries for all supported platforms and attaches them to your draft, leaving your release notes untouched. (Step 1 is optional — if no draft exists, GoReleaser creates one with an auto-generated changelog.)
1. Review the draft release, then approve the workflow's `publish` job. This publishes the release, and the [Terraform Registry](https://registry.terraform.io/providers/supabase/supabase/latest) picks up the new version automatically.

### Common pitfalls

- **A draft release alone does nothing.** The workflow only triggers on a tag push, and GitHub doesn't create the tag until a release is published — hence step 2. Conversely, avoid the **Publish release** button: it also creates the tag, but takes the release live before any binaries exist.
- **Tagging a commit that already carries a released tag.** The workflow pins GoReleaser to the pushed tag (`GORELEASER_CURRENT_TAG`), so this should work, but if the release still fails with `422 already_exists` errors while uploading assets, land a new commit (an empty commit via a PR works) and tag that instead.
