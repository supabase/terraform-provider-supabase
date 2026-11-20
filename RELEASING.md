# Releasing the Supabase Terraform provider

This guide walks maintainers through cutting a new release of the provider. A release builds and signs binaries for every supported platform and publishes them as a GitHub release, and the [Terraform Registry](https://registry.terraform.io/providers/supabase/supabase/latest) picks up the new version from there.

You need maintainer permissions on the repository, and the [GitHub CLI](https://cli.github.com/) makes the steps quicker. Nothing is published until you approve the last step, so you can't take a release live by accident.

## Cutting a release

First, update the changelog. Pull requests add their entries under `.changes/unreleased/` (see [Changelog](CONTRIBUTING.md#changelog)), and a release folds them into [CHANGELOG.md](CHANGELOG.md). To do that, run the following command:

```shell
make changelog-release VERSION=<changelog-version>
```

Replace the following:

- `<changelog-version>`: the new version without the `v` prefix, such as `1.2.3`. You can also pass `major`, `minor` or `patch` and changie works out the next version from the latest one.

The target runs `changie batch`, which collects the unreleased entries into `.changes/<changelog-version>.md` and deletes the entry files, and then `changie merge`, which regenerates `CHANGELOG.md`. Check the result, and edit `.changes/<changelog-version>.md` and run `go tool -modfile=tools/go.mod changie merge` again if an entry needs rewording. Then open a pull request with these changes and merge it, so the tag in the next steps points at a commit that includes the new changelog.

Next, create the git tag from the updated `main` branch and push it. Make it an annotated tag, which records who created it and when. To create the tag and push it, run the following commands:

```shell
git tag -a <version> -m "<version>"
git push origin <version>
```

Replace the following:

- `<version>`: the new version, prefixed with `v` and following [semantic versioning](https://semver.org/), such as `v1.2.3`. It must match `<changelog-version>`. Check the [releases page](https://github.com/supabase/terraform-provider-supabase/releases) and the existing tags first, so you don't reuse a version.

Pushing the tag starts the [release workflow](.github/workflows/release.yml). The workflow takes the release notes from `.changes/<changelog-version>.md` and fails if that file is missing, which means the changelog step was skipped. [GoReleaser](https://goreleaser.com/) then builds and signs the binaries and creates a draft release with those notes and the binaries attached.

If you want to edit the notes before publishing, change them in the draft release with `gh release edit`. Keep `CHANGELOG.md` in step by making the same change in a follow-up pull request.

You can also create the draft yourself before pushing the tag, for example to write custom notes. Run `gh release create <version> --draft --notes-file <file>`, or use the [new release page](https://github.com/supabase/terraform-provider-supabase/releases/new) and keep the release title identical to the tag name, because GoReleaser finds the draft by its title. GoReleaser attaches the binaries to your draft and leaves its notes as they are.

When the workflow finishes, review the draft release and approve the workflow's `publish` job. That publishes the release, and the Terraform Registry picks up the new version automatically.

## Common pitfalls

A draft release on its own does nothing, because the workflow only starts when a tag is pushed, and GitHub doesn't create the tag until a release is published. That's why you push the tag yourself. Avoid the **Publish release** button for the same reason, since it creates the tag but takes the release live before any binaries exist.

You can tag a commit that already has a release tag, because the workflow tells GoReleaser which tag to release through `GORELEASER_CURRENT_TAG`. If uploading assets fails with `422 already_exists` errors, land a new commit, such as an empty commit through a pull request, and tag that instead.
