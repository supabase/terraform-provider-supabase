resource "supabase_project" "new" {
  organization_id = "continued-brown-smelt"
  name = "foo"
  database_password = "bar"
  region = "us-east-1"
  plan = "free"
}