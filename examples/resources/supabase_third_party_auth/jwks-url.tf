# The URL must return a JSON Web Key Set with the provider's public keys.
resource "supabase_third_party_auth" "jwks_url" {
  project_ref = "mayuaycdtijbctgqbycg"
  jwks_url    = "https://login.example.com/.well-known/jwks.json"
}
