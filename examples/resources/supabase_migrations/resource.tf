resource "supabase_migrations" "example" {
  project_ref = "mayuaycdtijbctgqbycg"

  migrations = [
    {
      file_path = "./migrations/001_initial_schema.sql"
    },
    {
      file_path = "./migrations/002_add_users_table.sql"
    },
    {
      file_path = "./migrations/003_add_posts_table.sql"
    }
  ]
}
