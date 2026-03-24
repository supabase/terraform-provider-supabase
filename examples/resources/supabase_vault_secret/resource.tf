resource "supabase_vault_secret" "example" {
  project_ref = "mayuaycdtijbctgqbycg"
  value       = "my-secret-api-key-12345"
  name        = "api_key"
  description = "Third-party API key"
}

# Example without description (optional field)
resource "supabase_vault_secret" "simple" {
  project_ref = "mayuaycdtijbctgqbycg"
  value       = "another-secret-value"
  name        = "database_password"
}
