resource "supabase_vault_secret" "example" {
  project_ref = "mayuaycdtijbctgqbycg"
  name        = "stripe_secret_key"
  value       = "sk_test_vault_value"
  description = "Stripe API secret"
}
