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

Releasing requires maintainer permissions, and nothing goes live until a manual approval at the very end.

1. Draft a new release on the [GitHub releases page](https://github.com/supabase/terraform-provider-supabase/releases/new): choose a new tag following [semantic versioning](https://semver.org/) (e.g., `v1.10.0`), click **Generate release notes**, and edit the notes as needed. Keep the release title identical to the tag name (GoReleaser locates the draft by its title), then click **Save draft**.
1. Saving a draft does _not_ create the git tag, so create and push it yourself:

   ```shell
   git tag v1.10.0
   git push origin v1.10.0
   ```

1. Pushing the tag triggers the [release workflow](.github/workflows/release.yml): [GoReleaser](https://goreleaser.com/) builds and signs binaries for all supported platforms and attaches them to your draft, leaving your release notes untouched. (Step 1 is optional — if no draft exists, GoReleaser creates one with an auto-generated changelog.)
1. Review the draft release, then approve the workflow's `publish` job. This publishes the release, and the [Terraform Registry](https://registry.terraform.io/providers/supabase/supabase/latest) picks up the new version automatically.

### Common pitfalls

- **A draft release alone does nothing.** The workflow only triggers on a tag push, and GitHub doesn't create the tag until a release is published — hence step 2. Conversely, avoid the **Publish release** button: it also creates the tag, but takes the release live before any binaries exist.
- **Tagging a commit that already carries a released tag.** The workflow pins GoReleaser to the pushed tag (`GORELEASER_CURRENT_TAG`), so this should work, but if the release still fails with `422 already_exists` errors while uploading assets, land a new commit (an empty commit via a PR works) and tag that instead.
