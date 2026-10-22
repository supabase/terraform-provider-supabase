# Examples

The Terraform snippets in this directory do two jobs. They're the example usage on every docs page, and many of them double as test configurations, so the examples people copy into their own projects are known to work.

## Where the docs generator looks

The docs generator, [tfplugindocs](https://github.com/hashicorp/terraform-plugin-docs), reads files from these locations:

- `provider/provider.tf` becomes the example on the provider's index page.
- `resources/<resource name>/resource.tf` becomes the example on that resource's page, and the `import.sh` next to it becomes the page's import section.
- `data-sources/<data source name>/data-source.tf` becomes the example on that data source's page.

The generator ignores any other `.tf` file, so you can keep extra configurations here without them showing up in the docs.

## How the tests use them

[`examples.go`](examples.go) embeds the example files as Go strings, and tests in `internal/provider/` use those strings as their Terraform configuration. When you add an example, embed it in `examples.go` too and use it in at least one test, so a broken example fails the build instead of reaching the docs.

After you change an example, run `make generate` to regenerate the docs, as described in [Updating the docs](../CONTRIBUTING.md#updating-the-docs).
