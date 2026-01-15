resource "supabase_project" "test" {
  organization_id   = "continued-brown-smelt"
  name              = "foo"
  database_password = "barbaz"
  region            = "us-east-1"
  instance_size     = "micro"
}
