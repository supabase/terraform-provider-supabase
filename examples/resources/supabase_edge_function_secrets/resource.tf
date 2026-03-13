resource "supabase_edge_function_secrets" "example" {
  project_ref = "mayuaycdtijbctgqbycg"

  secrets = [
    {
      name  = "API_KEY"
      value = "your-api-key-here"
    },
    {
      name  = "DATABASE_URL"
      value = "postgresql://user:pass@localhost:5432/db"
    },
    {
      name  = "STRIPE_SECRET_KEY"
      value = "sk_test_..."
    }
  ]
}
