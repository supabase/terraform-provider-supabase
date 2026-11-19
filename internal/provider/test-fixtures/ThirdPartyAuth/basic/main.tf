resource "supabase_third_party_auth" "test" {
  project_ref     = supabase_project.test.id
  oidc_issuer_url = var.oidc_issuer_url
  jwks_url        = var.jwks_url
  custom_jwks     = var.custom_jwks
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

variable "oidc_issuer_url" {
  default = null
}

variable "jwks_url" {
  default = null
}

variable "custom_jwks" {
  default = null
}
