# Any provider that supports OpenID Connect (OIDC) discovery.
resource "supabase_third_party_auth" "custom_oidc" {
  project_ref     = "mayuaycdtijbctgqbycg"
  oidc_issuer_url = "https://login.example.com"
}
