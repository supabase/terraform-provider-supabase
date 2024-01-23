resource "supabase_settings" "production" {
  project_ref = "mayuaycdtijbctgqbycg"

  api = {
    schemas           = ["public", "storage", "graphql_public"]
    extra_search_path = ["public", "extensions"]
    max_rows          = 1000
  }

  auth = {
    site_url = "https://example.com"
  }

  storage = {
    file_size_limit = "50MB"
  }

  # Webhooks, pooler, etc.
}
