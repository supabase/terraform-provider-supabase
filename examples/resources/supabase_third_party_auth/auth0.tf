# Your Auth0 tenant's domain, including its region if it has one.
resource "supabase_third_party_auth" "auth0" {
  project_ref     = "mayuaycdtijbctgqbycg"
  oidc_issuer_url = "https://my-tenant.us.auth0.com/"
}
