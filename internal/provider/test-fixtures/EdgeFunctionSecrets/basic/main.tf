resource "supabase_edge_function_secrets" "test" {
  project_ref = supabase_project.test.id
  secrets     = var.secrets
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

variable "secrets" {
  type = list(object({
    name  = string
    value = string
  }))
  default = []
}
