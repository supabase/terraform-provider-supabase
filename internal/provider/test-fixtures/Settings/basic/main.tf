resource "supabase_settings" "test" {
  project_ref = supabase_project.test.id

  api      = var.api == null ? null : jsonencode(var.api)
  auth     = var.auth == null ? null : jsonencode(var.auth)
  database = var.database == null ? null : jsonencode(var.database)
  storage  = var.storage == null ? null : jsonencode(var.storage)

  ssl_enforcement = var.ssl_enforcement
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

variable "api" {
  type    = any
  default = null
}

variable "auth" {
  type    = any
  default = null
}

variable "database" {
  type    = any
  default = null
}

variable "storage" {
  type    = any
  default = null
}

variable "ssl_enforcement" {
  type    = bool
  default = null
}
