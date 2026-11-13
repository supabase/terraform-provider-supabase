resource "supabase_edge_function" "test" {
  project_ref = supabase_project.test.id
  slug        = var.slug
  name        = var.name
  entrypoint  = var.entrypoint
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

variable "slug" {
  nullable = false
}

variable "name" {
  default = null
}

variable "entrypoint" {
  nullable    = false
  description = "Absolute path to the function entrypoint"
}
