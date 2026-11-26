# Replace client_01EXAMPLE with your WorkOS client ID.
resource "supabase_third_party_auth" "workos" {
  project_ref     = "mayuaycdtijbctgqbycg"
  oidc_issuer_url = "https://api.workos.com/user_management/client_01EXAMPLE"
}
