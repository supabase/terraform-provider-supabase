# Releasing the Supabase Terraform provider

This guide walks maintainers through cutting a new release of the provider. A release builds and signs binaries for every supported platform and publishes them as a GitHub release, and the [Terraform Registry](https://registry.terraform.io/providers/supabase/supabase/latest) picks up the new version from there.

You need maintainer permissions on the repository, and the [GitHub CLI](https://cli.github.com/) makes the steps quicker. Nothing is published until you approve the last step, so you can't take a release live by accident.

## Cutting a release

Start by drafting the release. To create a draft with auto-generated release notes, run the following command:

```shell
gh release create <version> --draft --generate-notes
```

Replace the following:

- `<version>`: the new version, prefixed with `v` and following [semantic versioning](https://semver.org/), such as `v1.2.3`. Check the [releases page](https://github.com/supabase/terraform-provider-supabase/releases) and the existing tags first, so you don't reuse a version.

The command prints a link where you can polish the notes, and you can also edit them with `gh release edit`. If you'd rather draft the release on the [new release page](https://github.com/supabase/terraform-provider-supabase/releases/new), keep the release title identical to the tag name, because GoReleaser finds the draft by its title.

Drafting a release doesn't create the git tag, so you create and push it yourself. To push the tag, run the following commands:

```shell
git tag <version>
git push origin <version>
```

Pushing the tag starts the [release workflow](.github/workflows/release.yml). [GoReleaser](https://goreleaser.com/) builds and signs the binaries and attaches them to your draft, and it leaves your release notes as they are. If you skipped the draft, GoReleaser creates one with an auto-generated changelog.

When the workflow finishes, review the draft release and approve the workflow's `publish` job. That publishes the release, and the Terraform Registry picks up the new version automatically.

## Common pitfalls

A draft release on its own does nothing, because the workflow only starts when a tag is pushed, and GitHub doesn't create the tag until a release is published. That's why you push the tag yourself. Avoid the **Publish release** button for the same reason, since it creates the tag but takes the release live before any binaries exist.

You can tag a commit that already has a release tag, because the workflow tells GoReleaser which tag to release through `GORELEASER_CURRENT_TAG`. If uploading assets fails with `422 already_exists` errors, land a new commit, such as an empty commit through a pull request, and tag that instead.
