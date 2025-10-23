# Supabase Edge Function Resource

This resource manages Supabase Edge Functions.

## Example Usage

### Basic Edge Function

```terraform
resource "supabase_edge_function" "hello_world" {
  project_ref = "your-project-ref"
  slug        = "hello-world"
  name        = "Hello World Function"

  body = base64encode(<<-EOT
    export default async (req: Request) => {
      return new Response(
        JSON.stringify({ message: "Hello World!" }),
        { headers: { "Content-Type": "application/json" } }
      )
    }
  EOT
  )

  verify_jwt = true
  import_map = false
}
```

### Loading Function Code from File

```terraform
resource "supabase_edge_function" "from_file" {
  project_ref = "your-project-ref"
  slug        = "my-function"
  name        = "My Function"

  body = base64encode(file("${path.module}/functions/my-function/index.ts"))

  verify_jwt = false
  import_map = true
}
```

## Argument Reference

* `project_ref` - (Required) The project reference identifier.
* `slug` - (Required) The function slug (unique identifier). Cannot be changed after creation.
* `body` - (Required) The function code, base64 encoded.
* `name` - (Optional) Display name for the function. Defaults to the slug.
* `verify_jwt` - (Optional) Require JWT verification for function invocations. Defaults to `true`.
* `import_map` - (Optional) Enable import map support. Defaults to `false`.

## Attribute Reference

In addition to all arguments above, the following attributes are exported:

* `id` - The function identifier.
* `version` - The current version of the function.
* `status` - The function status (`ACTIVE`, `DEPLOYING`, `ERROR`, `REMOVED`).

## Import

Edge Functions can be imported using the project reference and function slug, e.g.:

```shell
terraform import supabase_edge_function.example project_ref:function_slug
```
