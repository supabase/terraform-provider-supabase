resource "supabase_settings" "production" {
  project_ref = "mayuaycdtijbctgqbycg"

  database = jsonencode({
    statement_timeout = "10s"
  })

  pooler = jsonencode({
    default_pool_size = 15
    max_client_conn   = 200
  })

  network = jsonencode({
    restrictions = [
      "0.0.0.0/0",
      "::/0"
    ]
  })

  api = jsonencode({
    db_schema            = "public,storage,graphql_public"
    db_extra_search_path = "public,extensions"
    max_rows             = 1000
  })

  auth = jsonencode({
    site_url = "http://localhost:3000"
  })
}
