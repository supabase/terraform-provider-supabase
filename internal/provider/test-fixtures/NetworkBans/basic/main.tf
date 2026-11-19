data "supabase_network_bans" "test" {
  project_ref = supabase_project.test.id
}

resource "supabase_project" "test" {
  organization_id   = var.organization_id
  name              = var.project_name
  database_password = "barbaz"
  region            = "us-east-1"
}

variable "organization_id" {
  nullable    = false
  description = "The organization Id"
}

variable "project_name" {
  nullable = false
}
