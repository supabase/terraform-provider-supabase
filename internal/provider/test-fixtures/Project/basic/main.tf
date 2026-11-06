resource "supabase_project" "test" {
  organization_id   = var.organization_id
  name              = var.name
  database_password = var.database_password
  region            = "us-east-1"
  instance_size     = var.instance_size
}

variable "organization_id" {
  nullable    = false
  description = "The organization Id"
}

variable "name" {
  nullable = false
}

variable "instance_size" {
  default = "micro"
}

variable "database_password" {
  default = "barbaz"
}