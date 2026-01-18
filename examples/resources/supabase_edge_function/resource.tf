resource "supabase_edge_function" "foo" {
  project_ref = "mayuaycdtijbctgqbycg"
  slug        = "foo"
  entrypoint  = "supabase/functions/foo/index.ts"
}
