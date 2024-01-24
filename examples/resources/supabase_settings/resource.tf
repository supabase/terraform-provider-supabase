resource "supabase_settings" "production" {
  project_ref = "mayuaycdtijbctgqbycg"

  api = jsonencode({
    db_schema            = "public,storage,graphql_public"
    db_extra_search_path = "public,extensions"
    max_rows             = 1000
  })

  # auth = jsonencode({
  #   site_url = "https://example.com"
  # })

  # storage = jsonencode({
  #   file_size_limit = "50MB"
  # })

  # pooler = jsonencode({
  #   default_pool_size         = 15
  #   ignore_startup_parameters = ""
  #   max_client_conn           = 200
  #   pool_mode                 = "transaction"
  # })
}
