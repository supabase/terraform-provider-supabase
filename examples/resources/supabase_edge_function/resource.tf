# Example Edge Function with inline code
resource "supabase_edge_function" "hello_world" {
  project_ref = "your-project-ref"
  slug        = "hello-world"
  name        = "Hello World Function"

  # Function code must be base64 encoded
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

# Example Edge Function loading code from file
resource "supabase_edge_function" "from_file" {
  project_ref = "your-project-ref"
  slug        = "my-function"
  name        = "My Function"

  # Load function code from a file and encode it
  body = base64encode(file("${path.module}/functions/my-function/index.ts"))

  verify_jwt = false
  import_map = true
}
