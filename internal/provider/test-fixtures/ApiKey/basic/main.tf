resource "supabase_project" "test" {
  organization_id   = var.organization_id
  name              = var.project_name
  database_password = "barbaz"
  region            = "us-east-1"
}

resource "supabase_apikey" "test" {
  project_ref = supabase_project.test.id
  name        = var.name
  description = var.description
}

variable "organization_id" {
  nullable    = false
  description = "The organization Id"
}

variable "project_name" {
  nullable = false
}

variable "name" {
  nullable = false
}

variable "description" {
  default = null
}
