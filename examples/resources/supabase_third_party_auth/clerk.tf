# Your Clerk instance's domain.
resource "supabase_third_party_auth" "clerk" {
  project_ref     = "mayuaycdtijbctgqbycg"
  oidc_issuer_url = "https://example.clerk.accounts.dev"
}
