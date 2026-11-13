resource "supabase_branch" "test" {
  parent_project_ref = supabase_project.test.id
  git_branch         = var.git_branch
  persistent         = var.persistent
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

variable "git_branch" {
  nullable = false
}

variable "persistent" {
  default = false
}
