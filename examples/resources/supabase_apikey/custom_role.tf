# A secret key bound to a custom Postgres role, for example one created in a
# migration with only the grants a backend service needs. Requests made with
# the key run as this role instead of service_role.
resource "supabase_apikey" "reader" {
  project_ref = "mayuaycdtijbctgqbycg"
  name        = "reader"
  description = "Read-only key for the reporting service"

  secret_jwt_template = {
    role = "app_reader"
  }
}
